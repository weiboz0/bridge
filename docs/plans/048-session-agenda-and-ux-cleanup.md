# Plan 048 — Session Agenda Single-Source + UX Cleanup

## Status

- **Date:** 2026-04-30
- **Branch:** `feat/048-session-agenda-and-ux-cleanup`
- **Sources:** `docs/reviews/007-comprehensive-browser-review-2026-04-29.md`, plus deferrals from review 006 (P2) and plan 047's out-of-scope list
- **Goal:** Make the live-session agenda one source of truth (so teacher and student see the same focus areas), rename Topic → "Focus Area" in user-visible copy, fix the editor's hang-on-error, stop linking ended sessions from dashboards, give My Work a navigation target, and silence the dev-overlay theme-bootstrap warning. P0 dev-bypass leak is environment config, not in scope.

## Sources + scope

Review 007 was the first browser-driven review since plan 047 unblocked `/login`. It confirmed plan 047's fixes hold and surfaced a new substantive bug plus several deferred P2s:

- **P1 — Session agenda divergence.** Teacher live page reads `courseTopics` from `class.course.topics` (every focus area in the course); student live page reads `/api/sessions/{id}/topics` which queries the `session_topics` join. New sessions don't auto-populate `session_topics` from the course's focus areas, so teacher sees an agenda with `unitId: null` while students see an empty material panel. Both surfaces must read from the same place.
- **P2** — Focus-area (topic) editor hangs forever on invalid/inaccessible routes (already filed in review 006 P2 #5; not yet shipped).
- **P2** — Dashboard + class detail still link ended sessions into a "Session ended" placeholder. The dedicated `/teacher/sessions` list got the non-link treatment in plan 043; these two surfaces were missed.
- **P2** — Theme bootstrap still triggers the Next dev-overlay warning ("Encountered a script tag while rendering React component"). Plan 040 phase 8 moved to `next/script` with `beforeInteractive`, but Next 16 still warns.
- **P2** — My Work cards are static; no link back to the class/problem/session/attempt context (deferred from review 006 P2 #6).
- **Cosmetic** — Topic → Focus Area rename in user-visible copy. Db column / route names stay; this is a copy-only pass that aligns the product taxonomy with what plans 044-046 already shipped.

### Non-Goals

- **`/register-org` form** (org self-onboarding for teachers). Promised in plan 047's deferral list but distinct UX domain — its own plan (049 or later).
- **Parent-child linking** (`parent_links` schema, re-enable parent reports). Plan 049.
- **DEV_SKIP_AUTH bypass on the tunneled review server.** Environment config, not a code fix. Action: TODO.md entry + `/admin/dashboard` banner when `DEV_SKIP_AUTH=true` is detected at runtime — out of scope for this plan, separate ticket.
- **Topic table / column / route URL rename.** A schema rename is a high-blast-radius change with migration coordination and not a UX fix. Only the user-visible copy ("Topic" → "Focus Area") changes here. The DB column stays `topics`/`topic_id`, the route stays `/teacher/courses/{id}/topics/{topicId}`, the Drizzle export stays `topics`, the Go store stays `TopicStore`. Future plan can rename the DB if/when product wants to commit fully.
- **Ended-session full review surface** (attendance, focus areas summary, links to student work). Plan 050 territory if product wants it; for now the dashboard / class-detail simply stop linking.
- **My Work full schema for problem-attempt context.** This plan extends the `/api/documents` response to include `classId` / `sessionId` / `topicId` (already on the document row) so the UI can construct a target route. Re-mapping My Work to "problem attempts" (a different data model entirely) is out of scope.

## Phases

### Phase 1: Session agenda single source (P1)

**Files:**

- Modify: `platform/internal/handlers/sessions.go::CreateSession` — after the unlinked-topic guard runs (the same one plan 047 added) AND after `Sessions.CreateSession` succeeds, snapshot every topic in the class's course into `session_topics` via a single bulk insert.
- Modify: `platform/internal/store/sessions.go` — add `BulkLinkSessionTopics(ctx, sessionID, topicIDs []string) error`. Implementation: `INSERT INTO session_topics (session_id, topic_id) SELECT $1, unnest($2::uuid[]) ON CONFLICT DO NOTHING`. The `ON CONFLICT DO NOTHING` makes the call idempotent if a teacher hits Start twice (or if a future plan re-snapshots).
- Modify: `platform/internal/handlers/sessions.go::GetTeacherPage` — replace the existing `h.Topics.ListTopicsByCourse(class.CourseID)` block with a call to `h.Sessions.GetSessionTopics(sessionID)` (the same helper student-side already uses). The struct shape changes from "all course topics" to "this session's agenda" — match the existing `teacherPageTopicRef` shape so the frontend doesn't need changes.
- Verify: `src/components/session/student/student-session.tsx` already reads from `/api/sessions/{id}/topics` (which is `GetSessionTopics`). No change needed; verify in the browser smoke that students see the same focus areas the teacher sees.

**Edge cases:**

- **Empty course.** Guard skips, snapshot is a no-op (empty array). Both teacher and student see no agenda.
- **Override path (`confirmUnlinkedTopics: true`).** Snapshot still runs — every focus area lands in `session_topics`, even unlinked ones, because the teacher consciously chose to start with a partial syllabus.
- **Mid-session topic add.** Existing `POST /api/sessions/{id}/topics` (LinkSessionTopic) still works; teachers can attach focus areas after start. Both teacher and student auto-pick up the new row on next refresh.
- **Mid-session topic remove.** Existing `DELETE /api/sessions/{id}/topics` (UnlinkSessionTopic) — same.

**Tests (Go):**

- `platform/internal/handlers/sessions_create_guard_test.go` — extend with:
  - `TestCreateSession_SnapshotsSessionTopics` — class with three linked focus areas, create session, assert `session_topics` has three rows for the new session id.
  - `TestCreateSession_OverrideAlsoSnapshots` — all-unlinked + `confirmUnlinkedTopics: true` → session created AND session_topics populated with all three.
  - `TestCreateSession_EmptyCourse_NoSnapshot` — empty course → session created, zero session_topics rows.
- `platform/internal/handlers/sessions_page_integration_test.go` (or a new file) — `TestGetTeacherPage_ReadsFromSessionTopics`:
  - Course has Topic A + Topic B. Create session, only LinkSessionTopic for Topic A.
  - Wait — actually the snapshot plan auto-links both. Test the post-create state: GetTeacherPage returns A and B.
  - Then UnlinkSessionTopic for B. GetTeacherPage now returns only A.
  - Confirms teacher reads from session_topics, not class.course.topics.

**Tests (Vitest):** none — student page already reads from session_topics; the change is server-side.

**Verification:**

- `cd platform && DATABASE_URL=... go test ./internal/handlers/ -run "TestCreateSession_Snapshots|TestCreateSession_OverrideAlso|TestGetTeacherPage_ReadsFromSessionTopics" -count=1`.
- Browser: create a class with two focus areas (one with linked Unit, one without), Start Session, navigate to teacher view, observe both in the agenda. Open the student view in another tab/role, confirm same two focus areas with the same Unit links.

### Phase 2: Focus-area editor error states + UUID validation

**Files:**

- Modify: `src/app/(portal)/teacher/courses/[id]/topics/[topicId]/page.tsx`:
  - Validate both UUIDs (`params.id`, `params.topicId`) at top of the component using the existing `isValidUUID` helper. On invalid → render an error state ("Invalid focus-area URL").
  - Track fetch state explicitly: `loading | ready | not-found | forbidden | error`. When `loadTopic` returns non-200, set the corresponding state instead of leaving `topic === null` forever.
  - Render explicit cards for each error state with a "Back to Course" link. Avoid the silent `Loading...` hang.
  - Update copy: heading "Edit Topic" → "Edit Focus Area"; "Topic Details" card → "Focus Area Details".
- Test: `tests/integration/topic-editor-error-states.test.tsx` (new) — render the page with mocked fetch returning 404 / 403 / 500 and an invalid UUID; assert the right error card renders for each, and that "Back to Course" links to `/teacher/courses/{id}`.

**Verification:**

- Vitest: 4 cases (404, 403, 500, invalid UUID) → no infinite loading.
- Browser: visit `/teacher/courses/not-a-uuid/topics/also-not-a-uuid`, observe explicit error.

### Phase 3: Topic → Focus Area rename (user-visible copy only)

**Files (UI strings only — code identifiers, db, routes UNCHANGED):**

- Modify: `src/app/(portal)/teacher/courses/[id]/topics/[topicId]/page.tsx` — heading + card titles ("Edit Topic" → "Edit Focus Area"; "Topic Details" → "Focus Area Details"). Already covered by Phase 2.
- Modify: `src/app/(portal)/teacher/courses/[id]/page.tsx` — list header "Topics" → "Focus Areas"; empty state copy.
- Modify: `src/app/(portal)/student/classes/[id]/page.tsx` — section header `<h2>Topics</h2>` → `<h2>Focus Areas</h2>`; "No topics yet" → "No focus areas yet".
- Modify: `src/components/teacher/add-topic-form.tsx` — button label / placeholder.
- Modify: `src/components/session/teacher/teacher-dashboard.tsx` — any "Topic" labels in presentation mode.
- Modify: `src/components/session/student/student-session.tsx` — empty-state "No material yet for this topic" → "No material yet for this focus area".

**No changes to:**

- DB column `topics` / `topic_id`.
- Drizzle export name `topics`.
- API routes `/api/courses/{id}/topics/{topicId}`, `/api/sessions/{id}/topics`.
- Variable names `topic`, `topics`, `topicId` in TS / Go code.
- Test names referring to `topic`.

**Test:** `tests/unit/focus-area-rename.test.ts` (new) — source-text regression: read each modified file and assert the user-visible string changed to "Focus Area" (or the appropriate variant), AND no remaining user-visible "Topic" / "Topics" strings on those surfaces. Catches re-introduction during merge conflicts. Pattern:

```ts
const source = readFileSync("src/app/(portal)/teacher/courses/[id]/topics/[topicId]/page.tsx", "utf8");
expect(source).toMatch(/Edit Focus Area/);
expect(source).not.toMatch(/Edit Topic/);
```

### Phase 4: Ended sessions stop linking from dashboard + class detail

**Files:**

- Modify: `src/app/(portal)/teacher/page.tsx` (around line 145) — wrap the row in `<Link>` only when `session.status === "live"`. For ended sessions, render the same row as a non-clickable `<div>` with the "Ended" badge and ended-at timestamp.
- Modify: `src/app/(portal)/teacher/classes/[id]/page.tsx` (around line 154 — the `pastSessions` map) — same treatment. The "View →" affordance gets removed for ended sessions; only show for live (matches plan 043's `/teacher/sessions` list).

**Test:** `tests/unit/ended-sessions-non-link.test.ts` (new, source-text) — assert each file has a `status === "live"` conditional gating the `<Link>` render. Pattern:

```ts
const source = readFileSync("src/app/(portal)/teacher/page.tsx", "utf8");
expect(source).toMatch(/status\s*===\s*["']live["']/);
expect(source).toMatch(/<Link[^>]*href=\{?\s*[`"]\/teacher\/sessions/);
// Live and ended branches both present
```

### Phase 5: My Work navigability

**Files:**

- Modify: `platform/internal/handlers/documents.go::ListDocuments` — extend the response shape to include `classId` (resolved via `session.classId` when `session_id` is set, otherwise the deprecated `classroom_id`), `sessionId`, `topicId`. The columns already exist on the documents table; just add them to the JSON.
- Modify: `src/app/(portal)/student/code/page.tsx`:
  - Read the new fields. Compute a target href:
    - If `sessionId` and `classId`: link to `/student/sessions/{sessionId}` (live or ended; ended page already exists).
    - Else if `classId`: link to `/student/classes/{classId}`.
    - Else: render the card non-clickable (orphaned doc — shouldn't happen, but defensive).
  - Wrap each card in `<Link>` when a target is computable; show small context line ("From: {className} · {sessionStatus}") under the language badge if metadata is rich enough.
- Rename the file/page heading: "My Code" → "My Work" matches the nav already (verify nav-config.ts) — fix mismatch if any.

**Tests:**

- `platform/internal/handlers/documents_test.go` — extend with `TestListDocuments_IncludesNavMetadata`: insert a document with sessionId set, fetch as the owner, assert `classId` resolved + `sessionId` + `topicId` in response.
- `tests/integration/my-work-navigation.test.tsx` (new, Vitest+RTL): mock `/api/documents` returning docs with various combinations of sessionId/classId/topicId; assert the right href on each card.

### Phase 6: Theme bootstrap dev-overlay fix

**File:**

- Modify: `src/app/layout.tsx`:
  - Remove the `<Script id="bridge-theme-bootstrap" strategy="beforeInteractive">` element from `<body>`. Move the bootstrap to a literal `<script>` element inside the `<head>` of the `<html>` tag, using `dangerouslySetInnerHTML`. This is the supported App Router pattern for sync inline scripts that need to run before hydration. The `<head>` location avoids the React-rendered-script-in-body warning that fires in Next 16 dev.
  - Update the comment to explain why `<head>` + `dangerouslySetInnerHTML` is the right pattern (and link to the next/script issue).

```tsx
<html lang="en" className={...} suppressHydrationWarning>
  <head>
    <script dangerouslySetInnerHTML={{ __html: themeScript }} />
  </head>
  <body className={...}>
    <SessionProvider>...</SessionProvider>
  </body>
</html>
```

**Test:** none. The fix is a configuration change verified in the browser by the absence of the dev-overlay error on every route.

**Verification:**

- Browser: load `/login`, open dev tools console, observe no "Encountered a script tag while rendering React component" warning. Toggle dark mode, observe FOUC-free behavior on subsequent navigation.

## Implementation Order

Strictly:

1. **Phase 1 first.** The substantive P1; everything else is UX. Phase 1 has the most surface (Go + tests) so we want it landed and verified before we start touching frontend copy.
2. **Phase 2** (editor error states + UUID validation). Bundles the rename heading change for that one file.
3. **Phase 3** (rename across the rest). Source-text regression tests catch backslides.
4. **Phase 4** (ended-session non-link).
5. **Phase 5** (My Work navigability).
6. **Phase 6** (theme bootstrap).

Each phase commits separately. The first commit on `feat/048-session-agenda-and-ux-cleanup` is this plan file with the agreed-on Codex review summary already embedded (per CLAUDE.md plan review gate).

## Risk Surface

- **Phase 1 — snapshot atomicity.** `Sessions.CreateSession` and `BulkLinkSessionTopics` are two SQL operations. If snapshot fails after session insert, the session exists but has no agenda. Recommend wrapping both in the existing transaction in `CreateSession` (it already begins a tx for the "end any other live session" cleanup). Phase 1 implementation must extend that tx, not start a new one.
- **Phase 1 — backwards compatibility for in-flight sessions.** Existing sessions in dev/seed data have empty `session_topics`. After the cutover, teacher view of those sessions would show empty agenda even though the course has topics. Two options:
  1. Backfill: a one-time migration script that inserts session_topics for every live session whose course has topics.
  2. Accept: a "we're pre-production, the seed re-runs anyway" stance. No backfill needed.
  
  Plan 048 picks option 2 — Bridge is pre-production, the demo seed will re-run, and any real session that loses its agenda is a debugging clue, not a data loss event. **Documented here so the trade-off is auditable.**
- **Phase 3 — copy drift.** A future PR could re-introduce "Topic" copy on a new surface. Source-text tests in Phase 3 cover the existing surfaces but not new ones. We mitigate by adding a single combined regression test that grep-asserts against a known set of files; new surfaces with "Topic" copy land in PR review.
- **Phase 5 — `/api/documents` response shape change.** Adding fields is forward-compatible (existing TS reads ignore them). Removing or renaming fields would break clients. We only add.
- **Phase 6 — `<head>` script rendering in App Router.** Putting `<script>` in `<head>` via React is supported in Next 16 App Router but is a relatively narrow pattern. Verify the bootstrap actually runs before hydration via the browser smoke (no dark-mode FOUC on first paint).

## Out of scope (explicit deferrals)

- **Plan 049 — `/register-org` form + parent_links + re-enabled parent reports.** Both are substantive UI/auth product features; bundled together because plan 048 already touches the auth flow gently in onboarding copy and we want the next plan to be an auth-forward push.
- **Plan 050 — Ended-session full review surface.** Once teachers have feedback that "non-link ended sessions" is the wrong UX, ship the read-only review (attendance, focus areas, student work).
- **Topic table / column / route URL schema rename.** Out of scope — costly, low UX win, separate plan if/when product commits to "Focus Area" everywhere.

## Codex Review of This Plan

(Pending — dispatch `codex:codex-rescue` per CLAUDE.md plan review gate before any implementation begins.)

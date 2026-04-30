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
- **My Work full schema for problem-attempt context.** This plan extends the `/api/documents` response to include `classId` (resolved via `LEFT JOIN sessions`) / `sessionId` / `topicId` so the UI can construct a target route. Re-mapping My Work to "problem attempts" (a different data model entirely) is out of scope.

## Phases

### Phase 1: Session agenda single source (P1) — atomicity-correct

**Atomicity (Codex CRITICAL fix):** The original draft was going to call a new `BulkLinkSessionTopics` after `Sessions.CreateSession` returned — but `CreateSession` already commits its transaction at `platform/internal/store/sessions.go:172` before returning. A separate post-commit insert would run on a fresh connection, breaking atomicity (a session row could exist with an empty agenda if the snapshot insert fails). The corrected design folds the snapshot inside the existing transaction.

**Files:**

- Modify: `platform/internal/store/sessions.go`:
  - Extend `CreateSessionInput` with `TopicIDs []string` (optional; nil/empty = no snapshot, preserves ad-hoc behavior).
  - Inside `CreateSession`'s transaction, AFTER the session INSERT and BEFORE `tx.Commit`, if `len(input.TopicIDs) > 0`, run a single bulk insert: `INSERT INTO session_topics (session_id, topic_id) SELECT $1, unnest($2::uuid[]) ON CONFLICT DO NOTHING`. The `ON CONFLICT` makes it safe against duplicate calls (and against a future plan that re-snapshots).
  - Existing `LinkSessionTopic` / `UnlinkSessionTopic` stay unchanged — they're for mid-session add/remove and run on `s.db` (their own connection) which is fine because they're single-row operations and not part of session creation.
- Modify: `platform/internal/handlers/sessions.go::CreateSession`:
  - When `body.ClassID != nil` and the unlinked-topic guard passes (or override flag set), gather the topic IDs via `h.Topics.ListTopicsByCourse(class.CourseID)` (same call the guard already makes — verify whether we can plumb the result through to avoid a second fetch).
  - Pass `TopicIDs: <ids>` into `Sessions.CreateSession`. The store now snapshots inside the tx.
  - When `body.ClassID == nil` (ad-hoc session), `TopicIDs` stays empty; no snapshot. Preserves existing ad-hoc behavior.
- Modify: `platform/internal/handlers/sessions.go::GetTeacherPage`:
  - Replace the `h.Topics.ListTopicsByCourse(class.CourseID)` block (around line 1296-1342) with a call to `h.Sessions.GetSessionTopics(sessionID)`.
  - The shape change: `GetSessionTopics` returns `[]SessionTopicWithDetails` which already has `UnitID`, `UnitTitle`, `UnitMaterialType`. Map directly into the existing `teacherPageTopicRef` struct — no payload schema change for the frontend.
  - The fetch dropping the `class.CourseID → ListTopicsByCourse → ListUnitsByTopicIDs → map` chain in favor of `GetSessionTopics(sessionID)` is also a small efficiency win — one query instead of three.

**Optimization (Codex MINOR):** plumbing the guard's already-fetched topics through to avoid a second `ListTopicsByCourse` call. The guard runs first, has the list, can pass it forward via a refactored helper. We make this a cleanup-not-blocker note: the second fetch is on an indexed column and the impact is sub-millisecond at seed scale. Phase 1 ships without the plumbing; if we see the call show up in slow logs, plan 050 cleans up.

**Edge cases:**

- **Empty course.** `body.TopicIDs` ends up empty → no snapshot insert. Both teacher and student see no agenda (correct — there's nothing to show).
- **Override path (`confirmUnlinkedTopics: true`).** Snapshot still runs — every focus area lands in `session_topics`, even unlinked ones, because the teacher consciously chose to start with a partial syllabus.
- **Mid-session topic add.** Existing `POST /api/sessions/{id}/topics` (LinkSessionTopic) still works; teachers can attach focus areas after start. Both teacher and student auto-pick up the new row on next refresh.
- **Mid-session topic remove.** Existing `DELETE /api/sessions/{id}/topics` (UnlinkSessionTopic) — same.
- **Ad-hoc session (no classId).** GetTeacherPage's `if class != nil` branch is preserved; ad-hoc sessions skip the topics block entirely. No regression. Phase 1 verification asserts this.
- **In-flight sessions whose `session_topics` are empty post-deploy.** Bridge is pre-production; the demo seed re-runs and any real session that loses its agenda is a debugging clue, not data loss. **Documented here so the trade-off is auditable.** No backfill migration ships.

**Tests (Go):**

- `platform/internal/handlers/sessions_create_guard_test.go` — extend with:
  - `TestCreateSession_SnapshotsSessionTopics` — class with two linked focus areas, create session, assert `session_topics` has two rows for the new session id (matched by `INNER JOIN topics`).
  - `TestCreateSession_OverrideAlsoSnapshots` — all-unlinked + `confirmUnlinkedTopics: true` → 201 AND `session_topics` populated.
  - `TestCreateSession_EmptyCourse_NoSnapshot` — empty course → 201, zero `session_topics` rows.
  - `TestCreateSession_NoClassID_NoSnapshot` — ad-hoc session → 201, zero `session_topics` rows.
  - `TestCreateSession_TopicSnapshot_TransactionRollback` — simulate a snapshot failure and assert no session row is created (proves atomicity). Practical implementation: pre-insert a `session_topics` row with a topic_id that doesn't exist in `topics` to violate the FK, OR seed a topic_id that violates a different constraint. If both prove fragile, document the atomicity invariant and lean on the bulk-insert single-statement nature (PG INSERT is atomic per-statement).
- `platform/internal/handlers/sessions_page_integration_test.go` (or new file) — `TestGetTeacherPage_ReadsFromSessionTopics`:
  - Course has Topic A + Topic B. Both linked Units. Create session — both auto-snapshotted.
  - GetTeacherPage returns A and B with their Unit refs.
  - UnlinkSessionTopic for B. GetTeacherPage now returns only A.
  - Confirms teacher reads from session_topics, not `class.course.topics`.

**Tests (Vitest):** none — student page already reads from session_topics; the change is server-side.

**Verification:**

- `cd platform && DATABASE_URL=... go test ./internal/handlers/ -run "TestCreateSession_Snapshot|TestCreateSession_OverrideAlso|TestCreateSession_EmptyCourse_NoSnapshot|TestCreateSession_NoClassID_NoSnapshot|TestGetTeacherPage_ReadsFromSessionTopics" -count=1`.
- Browser: create a class with two focus areas (one with linked Unit, one without), Start Session, navigate to teacher view, observe both in the agenda. Open the student view in another tab/role, confirm same two focus areas with the same Unit links.

### Phase 2: Theme bootstrap dev-overlay fix (moved up per Codex MINOR — quieter console for subsequent browser smokes)

**Files:**

- Modify: `src/app/layout.tsx`:
  - **Try first:** the simplest App-Router-supported pattern — render the script in `<head>` via React's `dangerouslySetInnerHTML`:

    ```tsx
    <html lang="en" className={...} suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body>...</body>
    </html>
    ```

  - **Verify in browser** (this phase IS a verification gate): load `/login`, open dev tools console, observe no "Encountered a script tag while rendering React component" warning.
  - **Fallback if the warning persists:** serve the bootstrap as a static `.js` file (e.g., `public/theme-bootstrap.js`) and reference via `<Script src="/theme-bootstrap.js" strategy="beforeInteractive" />`. Per Codex IMPORTANT: the existing `next/script` with `beforeInteractive` *should* be the supported pattern, so if the head-inline doesn't fix the warning, the bug is something subtler (Next 16 dev mode oddity), and a static-file `<Script src>` is the documented escape hatch.
  - **If both still warn:** revert and treat the warning as a Next-16 dev-only nuisance until upstream fixes it. Document in TODO.md.

The phase ships ONE of: head-inline, static-file fallback, or revert + TODO. The browser-verification step picks which.

**Test:** none — runtime browser observation.

**Verification (mandatory, gates the phase):**

- Browser: load multiple portal routes (`/login`, `/teacher`, `/student`, `/teacher/sessions/{id}` if accessible). For each, open dev console. The "Encountered a script tag" warning must be ABSENT. Theme dark mode toggle still works without FOUC.

### Phase 3: Session agenda — frontend verification (no code changes expected)

After Phase 1 lands the server-side change, this phase confirms the frontend agrees:

- Verify: `src/components/session/student/student-session.tsx` — already reads from `/api/sessions/{id}/topics` (which is `GetSessionTopics`). No code change.
- Verify: `src/components/session/teacher/teacher-dashboard.tsx` — receives `courseTopics` from the server payload. The shape is unchanged from before Phase 1 (still `teacherPageTopicRef[]`). No code change.

If verification surfaces a frontend gap, this phase ships a fix; otherwise it's a verification-only commit referenced from the post-execution report. We keep the phase as a reminder so the verification is auditable.

### Phase 4: Focus-area editor error states + UUID validation

**Files:**

- Modify: `src/app/(portal)/teacher/courses/[id]/topics/[topicId]/page.tsx`:
  - Validate both UUIDs (`params.id`, `params.topicId`) at top of the component using the existing `isValidUUID` helper. On invalid → render an error state ("Invalid focus-area URL").
  - Track fetch state explicitly: `loading | ready | not-found | forbidden | error`. When `loadTopic` returns non-200, set the corresponding state instead of leaving `topic === null` forever.
  - Render explicit cards for each error state with a "Back to Course" link. Avoid the silent `Loading...` hang.
  - Update copy: heading "Edit Topic" → "Edit Focus Area"; "Topic Details" card → "Focus Area Details".
- Test: `tests/integration/topic-editor-error-states.test.tsx` (new) — render the page with mocked fetch returning 404 / 403 / 500 and an invalid UUID; assert the right error card renders for each, and that "Back to Course" links to `/teacher/courses/{id}`.

**Verification:**

- Vitest: 4 cases (404, 403, 500, invalid UUID) → no infinite loading, correct copy.
- Browser: visit `/teacher/courses/not-a-uuid/topics/also-not-a-uuid`, observe explicit error.

### Phase 5: Topic → Focus Area rename (user-visible copy only)

**Files (UI strings only — code identifiers, db, routes UNCHANGED):**

- Modify: `src/app/(portal)/teacher/courses/[id]/topics/[topicId]/page.tsx` — heading + card titles ("Edit Topic" → "Edit Focus Area"; "Topic Details" → "Focus Area Details"). Already covered by Phase 4.
- Modify: `src/app/(portal)/teacher/courses/[id]/page.tsx` — list header "Topics" → "Focus Areas"; empty state copy.
- Modify: `src/app/(portal)/student/classes/[id]/page.tsx` — section header `<h2>Topics</h2>` → `<h2>Focus Areas</h2>`; "No topics yet" → "No focus areas yet"; **"No material yet for this topic" → "No material yet for this focus area"** (Codex IMPORTANT #3 — was missed in original plan).
- Modify: `src/components/teacher/add-topic-form.tsx` — button label / placeholder.
- Modify: `src/components/session/teacher/teacher-dashboard.tsx` — any "Topic" labels in presentation mode.
- Modify: `src/components/session/student/student-session.tsx` — empty-state "No material yet for this topic" → "No material yet for this focus area".
- Modify: `src/components/parent/live-session-viewer.tsx` — same empty-state copy (Codex IMPORTANT #3).
- Modify: `src/components/teacher/unit-picker-dialog.tsx` — tooltip "Linked to another topic" → "Linked to another focus area" (Codex IMPORTANT #3).

**No changes to:**

- DB column `topics` / `topic_id`.
- Drizzle export name `topics`.
- API routes `/api/courses/{id}/topics/{topicId}`, `/api/sessions/{id}/topics`.
- Variable names `topic`, `topics`, `topicId` in TS / Go code.
- Test names referring to `topic`.
- Component names like `TopicEditorPage`.

**Test (revised per Codex IMPORTANT #4):** `tests/unit/focus-area-rename.test.ts` (new) — source-text regression that asserts the SPECIFIC rendered copy strings changed, NOT a blanket `Topic` regex. Pattern:

```ts
const source = readFileSync("src/app/(portal)/student/classes/[id]/page.tsx", "utf8");
// User-visible heading
expect(source).toMatch(/<h2[^>]*>Focus Areas<\/h2>/);
expect(source).not.toMatch(/<h2[^>]*>Topics<\/h2>/);
// Empty-state copy
expect(source).toMatch(/No focus areas yet/);
expect(source).not.toMatch(/No topics yet/);
// "No material yet" rename
expect(source).toMatch(/No material yet for this focus area/);
expect(source).not.toMatch(/No material yet for this topic/);
```

The regex targets specific JSX text nodes / quoted strings, never identifiers or routes. Each modified file gets its own assertion block.

### Phase 6: Ended sessions stop linking from dashboard + class detail

**Reference pattern (Codex MINOR #1):** the model is the `SessionRow` component / pattern in `src/app/(portal)/teacher/sessions/page.tsx` (lines 17-72), specifically the conditional `<Link>` vs `<div>` branching on `s.status === "live"` at lines 55-72.

**Files:**

- Modify: `src/app/(portal)/teacher/page.tsx` (around line 145) — wrap the row in `<Link>` only when `session.status === "live"`. For ended sessions, render the same row as a non-clickable `<div>` with the "Ended" badge and ended-at timestamp. Mirror the SessionRow two-branch structure.
- Modify: `src/app/(portal)/teacher/classes/[id]/page.tsx` (around line 154 — the `pastSessions` map) — same treatment.

**Test:** `tests/unit/ended-sessions-non-link.test.ts` (new, source-text) — assert each file has a `status === "live"` conditional gating the `<Link>` render (specific rendered structure, not blanket regex). Pattern:

```ts
const source = readFileSync("src/app/(portal)/teacher/page.tsx", "utf8");
// Live branch wraps in Link to the session route
expect(source).toMatch(/status\s*===\s*["']live["']/);
expect(source).toMatch(/<Link[^>]*href=\{?\s*[`"]\/teacher\/sessions/);
```

### Phase 7: My Work navigability

**Files:**

- Modify: `platform/internal/handlers/documents.go::ListDocuments` — extend the response shape to include `classId`, `sessionId`, `topicId`. **Resolution (Codex IMPORTANT #1):**
  - `sessionId`: from the document's existing `session_id` column.
  - `topicId`: from the document's existing `topic_id` column.
  - `classId`: resolved via `LEFT JOIN sessions s ON s.id = documents.session_id` returning `s.class_id`. **NOT** via the dropped `documents.classroom_id` column (column was removed in migration `0012_drop_legacy_classrooms.sql:47-49`). Single JOIN, no N+1.
- Modify: `src/app/(portal)/student/code/page.tsx`:
  - Read the new fields. Compute a target href:
    - **If session is live** (`sessionId` set, session.status === "live"): link to `/student/sessions/{sessionId}`.
    - **If session is ended OR sessionId set but status unknown** (Codex IMPORTANT #2 — `/student/sessions/{id}` returns 404 for ended sessions per `platform/internal/handlers/sessions.go:1365-1386`, same placeholder problem Phase 6 is fixing on the teacher side): link to `/student/classes/{classId}` instead.
    - **If only classId** (no session): link to `/student/classes/{classId}`.
    - **If no metadata**: render the card non-clickable.
  - To distinguish live vs ended without re-fetching every session, ListDocuments returns the session status alongside session_id (added column to the JSON: `sessionStatus`). Single query already; just project one more field.
  - Show small context line ("Class: {className} · {sessionTitle}") under the language badge if metadata is rich enough.
- Verify: nav config — the page heading ("My Code") should match the nav label ("My Work") if the nav uses "My Work". Either rename the heading or leave alone if they already match.

**Tests:**

- `platform/internal/handlers/documents_test.go` — extend with `TestListDocuments_IncludesNavMetadata`:
  - Insert a document with `session_id` set + a session with a class. Fetch as the owner. Assert `classId` resolved + `sessionId` + `topicId` + `sessionStatus` in response.
  - Insert a document with no session — assert `classId`, `sessionId`, `topicId`, `sessionStatus` are null.
- `tests/integration/my-work-navigation.test.tsx` (new) — Vitest + RTL: mock `/api/documents` with various combinations:
  - sessionId + status="live" → href `/student/sessions/{sessionId}`
  - sessionId + status="ended" → href `/student/classes/{classId}` (not the broken sessions route)
  - classId only → href `/student/classes/{classId}`
  - no metadata → card not wrapped in `<Link>`

## Implementation Order (revised per Codex MINOR #2)

Strictly:

1. **Phase 1** — substantive P1, most surface, server-side. Lands first.
2. **Phase 2** — theme-bootstrap fix. Moved earlier (was Phase 6) so subsequent browser smokes for Phases 3-7 have a quieter dev console — easier to spot any new issues.
3. **Phase 3** — frontend verification post-Phase-1. Likely no code change but commits the verification.
4. **Phase 4** — editor error states + UUID validation.
5. **Phase 5** — Topic → Focus Area rename across surfaces (the wider copy pass).
6. **Phase 6** — ended-session non-link.
7. **Phase 7** — My Work navigability.

Each phase commits separately. The first commit on `feat/048-session-agenda-and-ux-cleanup` is this plan file with the agreed-on Codex review summary already embedded (per CLAUDE.md plan review gate).

## Risk Surface

- **Phase 1 — atomicity** (Codex CRITICAL #1, fixed). Snapshot now runs INSIDE the existing `CreateSession` transaction via `TopicIDs []string` on `CreateSessionInput`. Test `TestCreateSession_TopicSnapshot_TransactionRollback` exercises the failure path.
- **Phase 1 — back-compat for in-flight sessions.** Pre-production stance: no backfill, accept empty agenda on existing in-flight sessions, demo seed re-runs anyway. **Documented in non-goals.**
- **Phase 2 — `<head>` script in App Router** (Codex IMPORTANT #5). Treated as an experiment. Browser verification gates the phase: head-inline → fallback to static `<Script src>` → fallback to TODO if both warn. The phase doesn't ship blind.
- **Phase 5 — copy drift.** Source-text tests are surface-specific (regex on the JSX string, not the file in general) so they don't false-positive on identifiers / routes / type names.
- **Phase 7 — `/api/documents` response shape change.** Forward-compatible (adding fields, not renaming). Existing TS clients ignore unknown fields. The classId resolution uses a JOIN, not the dropped `classroom_id` column.
- **Phase 7 — ended-session link target** (Codex IMPORTANT #2, fixed). My Work links ended sessions to `/student/classes/{classId}` instead of `/student/sessions/{id}` (which 404s).

## Out of scope (explicit deferrals)

- **Plan 049 — `/register-org` form + parent_links + re-enabled parent reports.** Both are substantive UI/auth product features; bundled together because plan 048 already touches the auth flow gently in onboarding copy and we want the next plan to be an auth-forward push.
- **Plan 050 — Ended-session full review surface.** Once teachers have feedback that "non-link ended sessions" is the wrong UX, ship the read-only review (attendance, focus areas, student work).
- **Topic table / column / route URL schema rename.** Out of scope — costly, low UX win, separate plan if/when product commits to "Focus Area" everywhere.

## Codex Review of This Plan

- **Date:** 2026-04-30
- **Verdict:** Plan needed substantive revision (Phase 1 atomicity gap was a real CRITICAL; several IMPORTANT issues missed surfaces and broken patterns). This document IS the post-correction plan.

### Corrections applied

1. `[CRITICAL]` **Phase 1 atomicity gap.** Original draft called a new `BulkLinkSessionTopics` post-`CreateSession`, but `CreateSession`'s transaction is already committed by then — separate connection, separate atomicity context, possible session-without-agenda partial state. → Snapshot now happens INSIDE `CreateSession`'s existing tx via `TopicIDs []string` on `CreateSessionInput`. Single transaction, single commit, atomic. New test `TestCreateSession_TopicSnapshot_TransactionRollback` exercises the failure path.
2. `[IMPORTANT]` **Phase 7 dropped `classroom_id` fallback.** Column was removed in `drizzle/0012_drop_legacy_classrooms.sql`. → ClassID now resolves via `LEFT JOIN sessions s ON s.id = documents.session_id` returning `s.class_id`. Single JOIN, no N+1.
3. `[IMPORTANT]` **Phase 7 ended-session link target was broken.** `/student/sessions/{id}` returns 404 for ended sessions (`platform/internal/handlers/sessions.go:1365-1386`). → My Work now links ended sessions to `/student/classes/{classId}` instead. Phase 6's "stop linking ended sessions to placeholder" applies symmetrically on the student side.
4. `[IMPORTANT]` **Phase 5 missed surfaces.** Original plan list of 6 files missed three: student class detail "No material yet for this topic", parent live-session viewer (same), unit-picker-dialog tooltip "Linked to another topic". → Added all three to Phase 5 file list.
5. `[IMPORTANT]` **Phase 5 source-text regex too broad.** Blanket `not.toMatch(/Topic/)` would false-positive on identifiers, routes, type names. → Tests now match SPECIFIC rendered JSX strings (e.g., `<h2[^>]*>Topics<\/h2>`, `No material yet for this topic`), never code identifiers.
6. `[IMPORTANT]` **Phase 2 head-inline pattern unproven.** The current `next/script` with `beforeInteractive` IS the official pattern; the warning persisting suggests a Next 16 dev oddity. → Phase 2 is now explicitly an experiment with a fallback ladder: head-inline → `<Script src=>` from a static file → TODO + revert if both still warn. Browser verification gates the phase.
7. `[MINOR]` **Phase 6 reference component name missing.** → Plan now references `SessionRow` in `src/app/(portal)/teacher/sessions/page.tsx:17-72` as the model.
8. `[MINOR]` **Phase 6 ordering.** Theme bootstrap was Phase 6 originally — but the dev-overlay warning masks real QA issues during browser smokes for the other phases. → Promoted to Phase 2 so subsequent browser smokes have a clean console.

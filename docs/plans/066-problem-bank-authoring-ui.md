# Plan 066 — Problem-bank authoring UI (Next-side surface for the Go CRUD)

## Status

- **Date:** 2026-05-03
- **Origin:** Live UI testing of the teacher portal — `/teacher/problems`
  lists problems but rows aren't clickable, there's no "Add problem"
  button, and there's no way to view/edit a problem's body or test
  cases from the UI. Teachers currently have to author via raw SQL
  (or the `content/python-101/` YAML importer for bulk loads).
- **Scope:** Next.js portal pages only. Backend is fully shipped —
  Go API has Create / Get / Update / Delete / Publish / Archive /
  Fork plus test-case CRUD plus attempt CRUD already wired
  (`platform/internal/handlers/problems.go:50-83`).
- **Predecessor context:** plan 042 (problem editor responsive
  layout) and `docs/design-gaps-problem-workflow.md` §5 ("No
  problem-authoring UI") — same gap, same root cause, finally
  addressed.

## Problem

The teacher problem-bank surface today:

| Feature | Backend | UI |
|---|---|---|
| List problems w/ filters | ✅ `GET /api/problems` | ✅ `/teacher/problems` |
| View a problem | ✅ `GET /api/problems/{id}` | ❌ no detail page |
| Create a problem | ✅ `POST /api/problems` | ❌ no form |
| Edit a problem | ✅ `PATCH /api/problems/{id}` | ❌ no form |
| Delete a problem | ✅ `DELETE /api/problems/{id}` | ❌ no action |
| Publish/Archive | ✅ `POST /api/problems/{id}/{publish,archive,unarchive}` | ❌ no action |
| Fork to personal | ✅ `POST /api/problems/{id}/fork` | ❌ no action |
| List test cases | ✅ `GET /api/problems/{id}/test-cases` | ❌ no UI |
| Manage test cases | ✅ `POST/PATCH/DELETE /api/test-cases/...` | ❌ no UI |

The list page (`src/app/(portal)/teacher/problems/page.tsx`) renders
each problem as a plain `<td>` cell with no `Link` wrapper and no
edit affordance. The page header has no "Add" button.

Teachers can browse their problem catalog but can't act on it from
the UI. To author a single problem they must:

1. Drop into `psql` (or the YAML importer for bulk).
2. Refresh the list page to confirm.
3. Manually log in as a student to verify rendering.

Plan 042 polished the *attempting* experience for students. This
plan closes the symmetric gap on the *authoring* side.

## Out of scope

- The Pyodide ↔ Piston output drift fix from
  `docs/design-gaps-problem-workflow.md` §6 — separate concern.
- The harness/test-comparator improvements from §1, §2, §7 — they
  affect the `problems` schema and the runner, not the UI we're
  adding here. The UI will surface whatever fields the schema has
  today (description, starter_code, difficulty, tags, etc.).
- A new authoring wizard. Single form, single page, no multi-step
  flow.
- Markdown-preview rendering inside the editor. Defer to a small
  follow-up (the existing `react-markdown` renderer can be reused
  once the editor lands).
- Bulk import via UI. The YAML path is the canonical bulk import.
- Problem-bank admin features (e.g., promote a personal problem to
  org-scope) — separate authorization plan.

## Approach

Five small Next pages + one shared form component. All call existing
Go endpoints via the `api()` helper. No backend changes.

1. **Make rows linkable** in `/teacher/problems`. Wrap each row's
   title cell in `<Link href="/teacher/problems/{id}">…</Link>`,
   add a row hover state. Add an "Add problem" button at the
   top-right that links to `/teacher/problems/new`.

2. **Detail page** at `/teacher/problems/[problemId]/page.tsx`. Server component that fetches the problem + its test cases + its focus-area attachments (NOT attempts — see Phase 2 §"NOT /attempts" note for why). Layout:
   - Problem metadata header (title, slug, scope, difficulty, tags, status, gradeLevel) with **Edit / Publish / Archive / Fork / Delete** action buttons (per current claims/scope).
   - Description rendered with `react-markdown`.
   - Starter code in a read-only Monaco panel.
   - Test cases as a card list — example cases shown inline, hidden cases collapsed (count visible, body hidden).
   - Focus-area attachments panel via `<focus-area-attach>` (see §7).
   - **No "View as student" link in v1.** The design route at `/design/problem-student` is hardcoded placeholder content with no `searchParams` support; the link would not resolve to the real problem (Codex pass-1). Defer until that route is wired to a real fetch.

3. **Create page** at `/teacher/problems/new/page.tsx`. Renders the
   shared `<ProblemForm>` in "create" mode. POSTs to
   `/api/problems`, redirects to the new detail page on success.
   Required fields: title, description, difficulty, gradeLevel,
   scope (defaults to `personal`), starter_code. Optional: slug,
   tags.

4. **Edit page** at `/teacher/problems/[problemId]/edit/page.tsx`.
   Same shared `<ProblemForm>` in "edit" mode pre-populated from
   `GET /api/problems/{id}`. PATCHes the same id, redirects back
   to detail.

5. **Test-case editor** as a sub-component on the detail page.
   Click "Edit cases" → expands an inline editor: list of
   existing cases (each with stdin + expected_stdout textareas,
   `is_example` checkbox, `name` input, sort_order via drag or
   numeric input), "+ Add case" button. Save submits one
   POST/PATCH/DELETE per dirty case; surfaces errors per row.

6. **Action buttons** on detail (Publish, Archive, Unarchive,
   Fork, Delete) — each is a thin client component that POSTs
   to its endpoint and refreshes. Confirm dialog on Delete; no
   confirm needed on Publish/Archive (reversible). Fork redirects
   to the forked copy's detail page.

7. **Attach-to-Focus-Area action** (review 010 §P2). Detail page exposes "Attach to focus area" → opens a modal listing the teacher's courses (one fetch per the user's `org_memberships` orgs — `GET /api/courses?orgId={orgId}`) and their focus areas (per-course `GET /api/courses/{courseId}/topics`); selecting one POSTs `/api/topics/{topicId}/problems` with `{ problemId, sortOrder }` (`platform/internal/handlers/topic_problems.go:40`). **The handler does NOT auto-infer sort_order** — Codex pass-2 caught the original assumption: omitted `sortOrder` defaults to `0`, which drops the new attachment to the top of the list and shoves any existing 0-sorted entry. The modal's submit therefore reads the focus-area's current `topic_problems` list first and submits `sortOrder = (max existing sort_order) + 1` so the new attachment lands at the bottom by default. Users can drag-reorder later (separate UI; not in this plan). Same surface lists EXISTING attachments at the top with detach buttons. Errors:
   - 409 (already attached) → modal closes with "Already in this focus area."
   - 404 on detach (already removed) → silently treat as success + refresh the list.
   - 403 (caller lost teacher/org_admin in the focus-area's org) → inline banner.

   **No 10-second undo affordance** — Codex pass-2 noted this contradicts the plan's "no optimistic UI" rule (Decisions §3) and adds state-tracking complexity. Detach is a one-click action with a confirmation dialog instead.

8. **Attach-to-Unit action deferred** (review 010 §P2). Per the recent taxonomy clarification: Units are teaching MATERIAL, Focus Areas are SYLLABUS framing. A problem can be referenced from a Unit's content (e.g., embedded in a markdown lesson) but the canonical "this problem is part of this curriculum slot" relation is the focus-area attachment from §7. Phase 2 of this plan ships only the focus-area attach; Unit-attach is deferred until product confirms the desired UX (markdown shortcode? an ordered list on the Unit edit page? both?). The detail page's "Attached to" panel shows both kinds when the latter ships.

9. **Terminology rename** (review 010 §P2). User-facing strings on `/teacher/problems` and the new detail/create/edit pages use "focus areas" instead of "topics". Schema, URL paths, and field names (`topicId`, `topic_problems`, `/api/topics/...`) stay unchanged — only labels visible to teachers change. The list-page header "Problems you can attach to topics" (`src/app/(portal)/teacher/problems/page.tsx:94`) is the most prominent offender; sweep all other strings on the same surface during Phase 1.

## Decisions to lock in

1. **Server components by default.** Detail and list pages are
   server components calling `api()`. Forms are client components
   (need state for typing). Keeps SSR fast and bundle small.
2. **Form library: `useState` + manual onSubmit.** No react-hook-form,
   no formik. The form is small (8 fields); a custom hook would
   be over-engineering. Existing patterns in `src/app/(portal)/teacher/units/new/page.tsx`
   set the precedent.
3. **No optimistic UI.** Mutations show a spinner, await the
   response, then refresh server state. Simpler; latency is fine
   on a localhost stack.
4. **Permissions by reuse.** Each Next page just calls the Go
   endpoint and surfaces the 403 it returns (the same way
   `/teacher/problems` does today). No client-side permission
   pre-check.
5. **Slug: optional + auto-generate from title.** When the user
   types a title, suggest a slug (lowercased, hyphenated). User
   can override. Backend already enforces uniqueness within scope.
6. **Test-case editor is per-problem only.** No global "test case
   library" — cases live with their problem.
7. **Markdown preview deferred.** Description textarea renders raw
   markdown; preview is a follow-up. The detail page DOES render
   the description as markdown, so authors can save + click
   "preview" via reload as a workaround.
8. **`scopeId` is REQUIRED on every create** (Codex pass-1).
   `personal` scope requires `scopeId === claims.UserID`
   (`platform/internal/handlers/problem_access.go:44`); `org`
   requires `scopeId === orgId` of an org where caller is
   teacher/org_admin; `platform` requires
   `IsPlatformAdmin && scopeId === platformId`. The form fills
   `scopeId` automatically based on the selected scope:
   - `personal` → `identity.userId`.
   - `org` → the org dropdown lets the user pick from their
     teacher/org_admin memberships; preselect the only one if
     they have just one.
   - `platform` → only shown to platform admins; `scopeId` =
     the canonical platform-org id (read from `/api/me/identity`
     or `/api/me/portal-access`).
9. **`tags` is `[]string` with a 64-char per-tag limit** (Codex
   pass-1). Backend does no normalization — UI should trim and
   reject empty entries client-side; let the user choose
   case freely.
10. **Test cases default to `isCanonical: true`** in the editor
    (Codex pass-1). Non-canonical cases create user-owned
    private cases instead of problem-owned canonical cases —
    that's the student "My cases" surface, not authoring. The
    field stays hidden in the test-case editor (always true);
    expose it only if/when an "import canonical → personal"
    flow is needed.

## Files

### Phase 1 — list-page enhancements

**Modify:**
- `src/app/(portal)/teacher/problems/page.tsx`:
  - Wrap title `<td>` content in `<Link href={`/teacher/problems/${p.id}`}>`.
  - Add row hover style (`hover:bg-muted/30 cursor-pointer`).
  - Add "+ Add problem" button at top-right of header,
    `<Link href="/teacher/problems/new">` styled as a primary
    `Button`.
  - Add a "Quick actions" trailing column with a 3-dot menu
    that exposes Publish/Archive/Delete inline (defer this to
    Phase 4 if it complicates the row layout).
  - **Terminology sweep**: replace "topics" with "focus areas" in user-facing copy on this page. Specifically the header line at `src/app/(portal)/teacher/problems/page.tsx:94` ("Problems you can attach to topics" → "Problems you can attach to focus areas"). Audit the rest of the page for any other "topic" mentions in copy (not in URLs / field names).

### Phase 2 — detail page

**Add:**
- `src/app/(portal)/teacher/problems/[problemId]/page.tsx` —
  server component. Fetches:
  - `GET /api/problems/{id}` (problem body + metadata)
  - `GET /api/problems/{id}/test-cases` (case list)
  - **NOT `/attempts`** (Codex pass-1): the existing endpoint
    returns the *caller's own* attempts only, not a cross-user
    activity feed. A teacher viewing a problem they authored
    would see "you have 0 attempts" which is misleading.
    Defer cross-user activity to a follow-up that adds a new
    teacher-scoped endpoint (`/api/problems/{id}/usage` or
    similar).
- `src/components/problem/problem-detail.tsx` — pure presentation: metadata header, markdown description, starter-code Monaco (read-only), test-cases card list, action buttons, **focus-area attachments panel** (see below).
- `src/components/problem/test-case-list.tsx` — read-only display with example/hidden split.
- `src/components/problem/problem-actions.tsx` — client component with Publish/Archive/Unarchive/Fork/Delete buttons. Each posts and triggers `router.refresh()`.
- `src/components/problem/focus-area-attach.tsx` — client component for the attach-to-focus-area flow. Renders the existing attachments list (one row per `topic_problems` entry, with a Detach button) and an "Attach to focus area" button. Clicking opens a modal listing the teacher's courses (each expandable to its focus areas); selecting one POSTs `/api/topics/{topicId}/problems` with `{ problemId }` (the existing endpoint at `platform/internal/handlers/topic_problems.go:40` infers sort_order). Detach POSTs `DELETE /api/topics/{topicId}/problems/{problemId}` (`platform/internal/handlers/topic_problems.go:44`). Surfaces 403/409 inline.

Required new fetch:
- `GET /api/problems/{id}/topics` — list focus-area attachments for this problem. **Need to verify this endpoint exists** (Phase 0 question for Codex). If it doesn't, list is computed client-side by walking the teacher's courses + filtering attachments — slower, but no backend change needed for v1.

### Phase 3 — create + edit pages

**Add:**
- `src/app/(portal)/teacher/problems/new/page.tsx` — server
  component that renders `<ProblemForm mode="create" />`.
- `src/app/(portal)/teacher/problems/[problemId]/edit/page.tsx`
  — server component. Fetches the problem; renders
  `<ProblemForm mode="edit" initial={problem} />`.
- `src/components/problem/problem-form.tsx` — client component.
  Fields: title, description, slug (optional, auto-suggested),
  difficulty (select), gradeLevel (select), scope (select —
  options gated by user's roles, defaults to `personal`),
  starter_code (Monaco), tags (chip input). Submit:
  - Create: `POST /api/problems` → redirect to detail.
  - Edit: `PATCH /api/problems/{id}` → redirect to detail.
  Surfaces 400 (validation) inline; 403 as a banner.

### Phase 4 — test-case editor

**Add:**
- `src/components/problem/test-case-editor.tsx` — client
  component, swappable on the detail page (toggled by an "Edit
  cases" button). Renders rows with inline form fields. On save,
  diffs against original list and submits per-row mutations.

**Modify:**
- `src/components/problem/problem-detail.tsx` — pass an
  `editing` prop down to swap read-only list for editor.

### Phase 5 — minor polish (deferrable, not blocking)

- Markdown preview side-by-side in the description field
  (`react-markdown` already in deps).
- Improve the existing `/teacher/problems` empty state to link
  directly to `/teacher/problems/new` instead of just mentioning
  the YAML importer.
- "View as student" button — **deferred** until the design
  route accepts a real `problemId` (Codex pass-1: today the
  page renders hardcoded placeholder content, no
  `searchParams` wiring). Adding `searchParams` support to
  `src/app/design/problem-student/page.tsx` is one part; the
  other is fetching the real problem + test cases and feeding
  them through the existing student shell (currently
  hardcoded). Track as plan-066 follow-up; not blocking the
  authoring UI itself.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Test-case editor rows get out of sync after partial-failure mutations | medium | After save, refetch the full case list rather than trusting local state. One extra GET per save. |
| Monaco bundle size on the form page | low | Already loaded for the student/teacher problem pages; same bundle. |
| `scope=platform` problems can be edited by non-platform-admins via the form | low | Backend rejects with 403 (`UpdateProblem` checks ownership). Form should disable scope selector when editing if user can't change scope, or surface 403 cleanly. |
| Slug uniqueness collisions surface mid-form | low | Backend returns 409 on conflict; form maps to inline error on the slug field. |
| Tags are a free-text array; users invent new tags constantly | low | For v1 just allow free input. Tag-suggestion / autocomplete is a follow-up. |
| Delete cascades may surprise teachers (kills test-cases + attempts) | medium | Backend actually GUARDS rather than cascades (Codex pass-1): `DeleteProblem` returns 409 if the problem is attached to topics OR has any attempts (`platform/internal/handlers/problems.go:414, :443, :452`). Confirm dialog should warn the user; if 409 returned, surface **"remove from focus areas + clear attempts before deleting"** (terminology rename per §9 — backend's "topics" message gets re-mapped to "focus areas" in the user-facing error string). No fallback delete. |

## Phases

### Phase 0: Pre-impl Codex review

Per CLAUDE.md plan-review gate. Dispatch `codex:codex-rescue` to
review against:
- `platform/internal/handlers/problems.go` (the routes the UI
  consumes)
- `src/app/(portal)/teacher/problems/page.tsx` (the existing
  list)
- `src/app/(portal)/teacher/units/new/page.tsx` (existing
  create-form pattern to mirror)
- `src/app/design/problem-student/page.tsx` (the design route
  used for the "view as student" link)

Specific questions:
1. Does the Go API enforce all the auth gates the UI assumes?
   Specifically: can a teacher create a `personal`-scope problem
   without belonging to any org? Can an org_admin create
   `org`-scope problems? What about platform admins creating
   `platform`-scope?
2. Are there any hidden constraints on test-case creation (e.g.,
   must be created in a specific order, or a problem must have at
   least one example case before it can be published)?
3. Is there an existing component pattern for "Monaco in a form"
   I should reuse rather than re-implement?
4. Is the request shape for `POST /api/problems` documented
   anywhere outside the Go handler? Anything I'd miss reading
   only `CreateProblem`?
5. Anything surprising in the `tags` / `metadata` field
   handling — e.g., is `tags` literally `[]string` or is there
   a normalization step?
6. The "view as student" link (`/design/problem-student?problemId={id}`)
   — does the design route actually accept a `problemId` query
   param today? If not, that's a one-line addition for it to
   load real data instead of the placeholder.

### Phase 1: List enhancements (PR 1)

Tiny. Confirm the visual change feels right before scaling out
the larger pages.

- Wrap title in Link.
- Add hover row state.
- Add "+ Add problem" button → `/teacher/problems/new`.
- Smoke test in dev.
- Self-review.
- PR + merge.

### Phase 2: Detail page (PR 2)

- Implement server component + sub-components.
- Action buttons (Publish/Archive/Unarchive/Fork/Delete) with
  confirm-on-delete.
- Smoke test against the Python 101 problems.
- Codex post-impl review.
- PR + merge.

### Phase 3: Create + edit forms (PR 3)

- Implement `<ProblemForm>`.
- New page + edit page.
- Smoke test create flow end-to-end (form → POST → detail page).
- Smoke test edit flow.
- Codex post-impl review.
- PR + merge.

### Phase 4: Test-case editor (PR 4)

- Inline editor as toggleable sub-component on detail page.
- Save logic with per-row mutation diffing.
- Smoke test add / edit / delete cases.
- Codex post-impl review.
- PR + merge.

### Phase 5: Polish (PR 5, optional)

- "View as student" button.
- Empty-state link to `/teacher/problems/new`.
- Optional: markdown preview in description field.

## Codex Review of This Plan

### Pass 2 — 2026-05-03: CONCUR-WITH-CHANGES → 4 items folded in

Codex pass-2 confirmed pass-1 fixes are clean (scopeId required, isCanonical default, attempts removed) but caught four follow-on issues from the attach-action expansion:

1. **Stale Approach text** — §Approach §2 still mentioned fetching "recent attempts" + "View as student" link, contradicting Phase 2 + Phase 5 deferral. Rewritten to match.
2. **sort_order is NOT inferred** — Codex confirmed `topic_problems.go:137-141` defaults to `0`, which would shove any existing 0-sorted entry. Decisions §7 now reads max existing `sortOrder` + 1 client-side and passes it explicitly.
3. **10-second undo affordance contradicted "no optimistic UI"** — replaced with a one-click detach + confirmation dialog (matches Decisions §3).
4. **Backend's "topics" error message** at delete-conflict path was leaking through the terminology rename. Risks-table mitigation now explicitly remaps "topics" → "focus areas" in the user-facing error.
5. **"List teacher's courses" was ambiguous** — courses require `orgId`. §Approach §7 now spells out the per-org-membership fetch shape.

### Pass 1 — 2026-05-03: CONCUR-WITH-CHANGES → 4 items folded in

Codex returned CONCUR with 3 BLOCKERS for Phase 3 (create form)
plus 1 IMPORTANT-non-blocking. All folded:

1. **scopeId is required** — `personal` requires `scopeId === claims.UserID`,
   `org` requires scopeId equal to an org where the caller is
   teacher/org_admin, `platform` requires the canonical platform
   scopeId. Decisions §8 added with the form's auto-fill rules.

2. **Recent-attempts panel mis-scoped** — `GET /api/problems/{id}/attempts`
   returns the caller's own attempts, not cross-user. Removed
   from Phase 2 detail page; deferred to a follow-up plan that
   adds a teacher-scoped usage endpoint.

3. **Test-case editor must default `isCanonical: true`** — non-canonical
   cases are user-owned private cases, not authored content.
   Decisions §10 added; the field stays hidden in the editor.

4. **`tags` has no backend normalization** — UI must trim and
   reject empty entries client-side; 64-char per-tag limit.
   Decisions §9 added.

5. **"View as student" button deferred** — `/design/problem-student`
   is hardcoded placeholder content with no `searchParams`
   support. Moved out of Phase 5 polish into a follow-up.

Confirmed by Codex (no resolution needed):
- All Go routes the plan consumes exist (`platform/internal/handlers/problems.go:50-83`).
- `/teacher/problems` list page handles 401/403 correctly.
- Existing units/new form is the right pattern to mirror.
- No "must have example case" constraint blocks Publish.

Verdict: **CONCUR-WITH-CHANGES** → all changes folded → ready
for Phase 1.

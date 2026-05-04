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

2. **Detail page** at `/teacher/problems/[problemId]/page.tsx`. Server
   component that fetches the problem + its test cases + recent
   attempts (the latter for "is this problem getting used"). Layout:
   - Problem metadata header (title, slug, scope, difficulty, tags,
     status, gradeLevel) with **Edit / Publish / Archive / Fork /
     Delete** action buttons (per current claims/scope).
   - Description rendered with `react-markdown`.
   - Starter code in a read-only Monaco panel.
   - Test cases as a card list — example cases shown inline,
     hidden cases collapsed (count visible, body hidden).
   - "View as student" link to `/design/problem-student?problemId={id}`
     (uses the existing design-route shell — closes
     `docs/design-gaps-problem-workflow.md` §8).

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

### Phase 2 — detail page

**Add:**
- `src/app/(portal)/teacher/problems/[problemId]/page.tsx` —
  server component. Fetches:
  - `GET /api/problems/{id}` (problem body + metadata)
  - `GET /api/problems/{id}/test-cases` (case list)
  - `GET /api/problems/{id}/attempts?limit=10` (recent activity,
    nice-to-have; if endpoint requires class scoping, drop)
- `src/components/problem/problem-detail.tsx` — pure presentation:
  metadata header, markdown description, starter-code Monaco
  (read-only), test-cases card list, action buttons.
- `src/components/problem/test-case-list.tsx` — read-only display
  with example/hidden split.
- `src/components/problem/problem-actions.tsx` — client component
  with Publish/Archive/Unarchive/Fork/Delete buttons. Each
  posts and triggers `router.refresh()`.

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

- "View as student" button on detail page that opens
  `/design/problem-student?problemId={id}` in a new tab.
- Markdown preview side-by-side in the description field
  (`react-markdown` already in deps).
- Improve the existing `/teacher/problems` empty state to link
  directly to `/teacher/problems/new` instead of just mentioning
  the YAML importer.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Test-case editor rows get out of sync after partial-failure mutations | medium | After save, refetch the full case list rather than trusting local state. One extra GET per save. |
| Monaco bundle size on the form page | low | Already loaded for the student/teacher problem pages; same bundle. |
| `scope=platform` problems can be edited by non-platform-admins via the form | low | Backend rejects with 403 (`UpdateProblem` checks ownership). Form should disable scope selector when editing if user can't change scope, or surface 403 cleanly. |
| Slug uniqueness collisions surface mid-form | low | Backend returns 409 on conflict; form maps to inline error on the slug field. |
| Tags are a free-text array; users invent new tags constantly | low | For v1 just allow free input. Tag-suggestion / autocomplete is a follow-up. |
| Delete cascades may surprise teachers (kills test-cases + attempts) | medium | Confirm dialog with explicit "this will delete N test cases and M attempts" preview. |

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

_(To be populated by Codex pass — see Phase 0.)_

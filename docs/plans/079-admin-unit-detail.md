# Plan 079 — Platform-admin read-only unit detail page

## Problem (browser review 011-2026-05-09 §P1 #3)

`/admin/units` (`src/app/(portal)/admin/units/page.tsx:157`) renders each row's title as `<Link href={"/teacher/units/${u.id}/edit"}>`. A platform admin who is not also a teacher gets bounced back to `/admin` because the teacher portal's layout (`src/app/(portal)/teacher/layout.tsx:4` → `<PortalShell portalRole="teacher">`) requires the caller to have role `teacher` (`src/components/portal/portal-shell.tsx:43-46`). The admin lists units they cannot inspect.

Two real consequences:

- Platform admins can browse the platform unit library but can't see any unit beyond the row data — no slug confirmation, no description, no content preview, no audit fields (createdAt, ownerScope, lineage), no status history.
- The link target advertises an EDIT action to platform admins, suggesting authoritative platform-library curation, while the click silently fails. Worse-than-useless UX.

Review recommendation, verbatim:

> Add `/admin/units/{id}` and optionally `/admin/units/{id}/edit`, with platform-admin authorization and a clear read/edit distinction. Until that exists, render admin unit titles as plain text or link to a read-only admin detail route only.

## Approach

Add a read-only `/admin/units/[id]/page.tsx` that platform admins reach from the existing list page. Update the list-page link to point there. Defer the optional `/admin/units/{id}/edit` to a separate plan — admin authoritatively editing a unit owned by a teacher could conflict with realtime collaboration semantics and deserves its own design pass. Read-only solves the browser bug today and unblocks the audit use case.

### Backend — no changes

`GET /api/units/{id}` already exists at `platform/internal/handlers/teaching_units.go:614`. Authorization via `canViewUnit` (`teaching_units.go:137`) bypasses for platform admin at every scope/status. The Next page server-component fetches that endpoint with the existing `api()` client.

### Frontend — single new page

`src/app/(portal)/admin/units/[id]/page.tsx`:

- Server component, async, `params: { id: string }`.
- Validates `id` looks like a UUID; renders 404 panel if not.
- `await api<UnitDetail>("/api/units/" + id)` — returns the full unit row.
- Renders structured panel: title, slug, scope/scopeId, status, materialType, gradeLevel, summary, createdAt, ownerScope label.
- Optional: short content preview (first ~500 chars of `unit.body` if present), with a "Open in Yjs collaborative editor" affordance — but ONLY if the admin is also a teacher (and so won't bounce). Skip the affordance entirely if the admin lacks `teacher` role.
- Back link to `/admin/units`.
- ApiError 404 → render "Unit not found" panel; 403 → render "Not authorized" panel; 5xx → render error panel with retry hint. Distinct messages, not silent redirects.

### List-page link update

`src/app/(portal)/admin/units/page.tsx:157`:

```tsx
<Link href={`/teacher/units/${u.id}/edit`} ...>
```

becomes:

```tsx
<Link href={`/admin/units/${u.id}`} ...>
```

One-line change. The `/admin/units/[id]` page handles the rest.

## Decisions to lock in

1. **Read-only only.** No edit/publish/archive controls in plan 079. The reviewer suggested edit as optional; deferring keeps scope tight and avoids the realtime-collaboration conflict question.
2. **No backend changes.** `GET /api/units/{id}` is already platform-admin-accessible. Adding `/api/admin/units/{id}` aliasing would be duplication.
3. **No "open in editor" link by default.** The teacher unit editor is at `/teacher/units/{id}/edit`; an admin without `teacher` role would still bounce. The detail page can show that link IF the caller has `teacher` role too (a small `data.roles.includes("teacher")` check), but otherwise omit it. Simpler choice for v1: omit entirely. Admins who want to edit can switch to teacher view via the role switcher in the sidebar (if they have teacher role) and navigate from there. Documented in §Decisions for future revisit.
4. **No new tests for the read-only page.** It's a thin server-component wrapper around an existing endpoint. The Go-side handler tests cover the data path; the Next page is presentation only. If display logic grows non-trivial, add a Vitest snapshot test in a follow-up.

## Files

**Modify (1 file):**

- `src/app/(portal)/admin/units/page.tsx` — line 157, change `<Link href={`/teacher/units/${u.id}/edit`}>` → `<Link href={`/admin/units/${u.id}`}>`. One line.

**Create (1 file):**

- `src/app/(portal)/admin/units/[id]/page.tsx` — server component, ~80 lines. Fetches `/api/units/{id}`; renders structured panel; handles 404/403/5xx as error states; back-link to `/admin/units`.

**No changes to:**

- `platform/` — `GET /api/units/{id}` is already platform-admin-accessible.
- `src/lib/portal/nav-config.ts` — `/admin/units` already in admin nav.
- `src/components/portal/portal-shell.tsx` — admin role-gate already correct.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| `GET /api/units/{id}` returns less data than the list endpoint expects (e.g., no `summary` or `gradeLevel` on the detail shape) | low | The list page already calls `/api/units/search` which returns rich data; the detail endpoint returns a fuller TeachingUnit. The fields needed for read-only display are a subset. Verify shape during impl. |
| Admin opens a unit they don't have access to (theoretical — `canViewUnit` says platform admin bypasses everywhere) | low | If the bypass ever changes, the page renders the 403 state cleanly. No silent "redirect to /". |
| Stale link — if any other Next page also links to `/teacher/units/...` for an admin context, this plan misses it | low | Pre-impl grep `grep -rn "/teacher/units/" src/app/\(portal\)/admin/` before commit. Expect 0 hits outside the list page. |
| The "open in editor" affordance is missing, leaving admins without an obvious next-step CTA | very low | Documented in §Decisions; admins navigate via role-switcher when they want edit. v1 trade-off. |
| The detail page's status badge / scope label rendering drifts from the list page's | low | Reuse the same `SCOPE_LABELS` / `STATUS_LABELS` / `statusBadge` helpers from the list page. Either inline them in both pages (current pattern) or extract to `src/lib/portal/unit-display.ts`. Pick simpler: inline for now. |

## Phases

### Phase 1 — Detail page + list-link update (commit 1)

- Pre-impl: `grep -rn "/teacher/units/" src/app/\(portal\)/admin/` to confirm only the list page is affected.
- Create `src/app/(portal)/admin/units/[id]/page.tsx` with the structured display.
- Update `src/app/(portal)/admin/units/page.tsx:157` link.
- Run `bunx tsc --noEmit` — confirm tsc baseline (10 errors) preserved; no new errors.
- Run `bun run test` — confirm existing tests pass.
- Commit: `plan 079: read-only /admin/units/[id] detail + retarget list link`.

### Phase 2 — Verify + post-execution report (commit 2)

- Manual browser smoke: log in as platform admin → `/admin/units` → click a unit row → confirm `/admin/units/{id}` renders structured detail without bouncing.
- Update post-execution report.
- Commit: `docs: plan 079 post-execution report`.

After Phase 2, run the 5-way code review against the consolidated branch diff (single-PR-per-plan policy), fold findings, open the PR via Step 6.

## Plan Review

(pending — 5-way before implementation)

## Code Review

(pending — 5-way at branch-diff time)

## Post-execution report

(pending)

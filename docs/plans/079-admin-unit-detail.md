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
- `await api<UnitDetail>("/api/units/" + id)` — returns the full unit row. **Explicit response shape** (Kimi K2.6 round-1 NIT 2 — original draft referenced an undefined `UnitDetail`):
  ```ts
  interface UnitDetail {
    id: string;
    scope: string;          // "platform" | "org" | "personal"
    scopeId: string | null;
    title: string;
    slug: string | null;
    summary: string;
    gradeLevel: string | null;
    status: string;
    materialType: string;
    createdAt: string;
    createdBy: string;      // user UUID — no display name in v1
    // body / content NOT present on this endpoint per Kimi K2.6 round-1
    // BLOCKER 4(a). Document preview lives at /api/units/{id}/document.
  }
  ```
- Renders structured panel: title, slug, scope/scopeId, status, materialType, gradeLevel, summary, createdAt. (Codex round-1 NIT 2: NO ownerScope display label in v1 — `GET /api/units/{id}` returns `store.TeachingUnit` which has raw `createdBy` UUID but no human-readable owner. A separate lookup would expand scope; defer to a future plan that wants owner attribution. The raw `scopeId` IS displayed and tells admins which org/user owns the unit.)
- **No content preview in v1** (GLM 5.1 round-1 NIT). `unit.body` (or whatever the field is named) holds a Yjs document state — likely encoded JSON or base64 — NOT human-readable text. Naively slicing "first 500 chars" would surface raw binary/JSON, not a useful preview. Skip the preview entirely in plan 079; metadata fields alone resolve the immediate broken-UX. Defer the content-preview workflow (which needs a Yjs → plaintext extractor or a markdown projector) to a separate plan.
- Back link to `/admin/units`.
- ApiError 404 → "Unit not found" panel; 5xx → error panel with retry hint. **No 403 panel** (Kimi K2.6 round-1 BLOCKER 4b): the Go handler returns 404 for both missing-row AND unauthorized-row cases (`teaching_units.go:620-633`), so 403 is dead code in practice. Platform admins bypass `canViewUnit` regardless. Two error states, not three. Distinct messages, not silent redirects.

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
4. **One small Vitest** for the error-rendering branches (Kimi K2.6 round-1 NIT 5). The 404 + 5xx panels have non-trivial copy + structure that should be guarded against accidental regression; Vitest covers them via `vi.mock("@/lib/api-client")` returning ApiError. Happy-path stays manual smoke (it's a thin server-component wrapper around an existing endpoint). Specifically:
   - `tests/unit/admin-unit-detail.test.tsx` — render the page server-component with mocked api → ApiError 404 → assert "Unit not found" copy. Same for ApiError 500 → assert error panel + retry hint.

## Files

**Modify (1 file):**

- `src/app/(portal)/admin/units/page.tsx` — line 157, change `<Link href={`/teacher/units/${u.id}/edit`}>` → `<Link href={`/admin/units/${u.id}`}>`. One line.

**Create (2 files):**

- `src/app/(portal)/admin/units/[id]/page.tsx` — server component, ~80 lines. Fetches `/api/units/{id}`; renders structured panel; handles 404/5xx as error states (no 403 — Kimi BLOCKER 4b); back-link to `/admin/units`.
- `tests/unit/admin-unit-detail.test.tsx` — Vitest for the 404 + 5xx error branches (Kimi NIT 5). `vi.mock("@/lib/api-client")` to inject `ApiError`. Happy path stays manual smoke.

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
| Parallel bug at `src/app/(portal)/org/units/page.tsx:172` — same `/teacher/units/{id}/edit` link, same bounce class for org admins without teacher role (GLM 5.1 round-1 flag, out of scope) | medium | Out of scope for plan 079 — needs an `/org/units/[id]` detail page following the same pattern. Filed as TODO.md follow-up; can be a thin parallel plan or fold into the same PR if the design transfers cleanly. (Codex round-1 NIT 5 disagreed, said "org context, not admin" — but the link target IS `/teacher/units/...` which bounces an org-admin without teacher role. GLM is correct that it's the same bug class.) |
| Impersonation clears `IsPlatformAdmin` (Codex round-1 NIT 4): a platform admin impersonating a non-teacher non-owner user would lose the `canViewUnit` admin bypass (`access.go:279`) and get 404 from the Go handler. The detail page renders the 404 panel correctly, so observably this works, but the failure mode is "admin in impersonation mode can't browse units they could browse without impersonation" | low (correct behavior) | Document. The intent of impersonation is "see what THIS user sees", so losing the admin bypass is intentional. The 404 panel is the right UX. Worth stating explicitly so future readers understand. |
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

### Round 1 (2026-05-09)

#### Self-review (Opus 4.7) — clean

No concerns surfaced before externals returned.

#### Codex — CONCUR (4 NITs, 2 FIXED + 1 acknowledged + 1 disagreement-with-GLM kept)

1. `[ACKNOWLEDGED]` NIT 1: read-only is defensible — edit auth is narrower than view auth (canViewUnit has blanket admin bypass; edit gates per-scope). Confirmed direction.
2. `[FIXED]` NIT 2: `GET /api/units/{id}` returns `store.TeachingUnit` directly with raw `createdBy` UUID, no human-readable owner display. → **Response**: dropped "ownerScope display label" from §Approach. Raw `scopeId` IS displayed and tells admins which org/user owns the unit.
3. `[ACKNOWLEDGED]` NIT 3: roles are objects with `.role` field, not strings — if a future plan adds the editor-CTA, use `.some(r => r.role === "teacher")`. v1 has no CTA so moot today.
4. `[FIXED]` NIT 4: impersonation clears `IsPlatformAdmin` so admin-impersonating-non-teacher would lose the bypass. → **Response**: added §Risks row; correct behavior, just worth stating.
5. `[KEPT-AS-FOLLOWUP]` NIT 5: Codex disagreed with GLM that `/org/units/page.tsx:172` is a parallel bug (Codex said "org context, not admin"). → **Response**: GLM is correct — the link TARGET is `/teacher/units/...` which bounces org-admins without teacher role. Annotation in §Risks acknowledges Codex's view but keeps the TODO.

#### DeepSeek V4 Pro — pending

#### GLM 5.1 — CONCUR (1 NIT, FIXED + 1 out-of-scope flag noted)

1. `[FIXED]` NIT (Q3 risk gap): `unit.body` is a Yjs document snapshot, not human-readable text. Naively slicing "first 500 chars" would surface raw binary/JSON. → **Response**: dropped the content-preview affordance entirely in v1. §Approach now says "No content preview in v1"; metadata fields alone resolve the broken-UX. Yjs → plaintext / markdown projector deferred to a separate plan.
2. `[NOTED, OUT-OF-SCOPE]` Parallel bug at `src/app/(portal)/org/units/page.tsx:172` — org admins without teacher role hit the same bounce. Added a §Risks row + TODO.md entry for a follow-up plan to add `/org/units/[id]` detail.

GLM round-1 also confirmed direction: "Read-only deferral is the right call. The immediate bug is 'admin can't inspect a unit at all' — read-only solves that. Edit involves realtime-collab conflict design and deserves its own plan."

#### Kimi K2.6 — CONCUR (1 BLOCKER + 4 NITs, all FIXED + 1 deferred)

1. `[FIXED]` NIT 2: undefined `UnitDetail` reference. → **Response**: §Approach now declares the explicit interface upfront with all 11 fields.
2. `[ACKNOWLEDGED]` NIT 3: inline duplication of `SCOPE_LABELS`/`STATUS_LABELS` matches existing 4-page pattern; defer extraction to a future refactor.
3. `[FIXED]` BLOCKER 4(a): `unit.body` does NOT exist on `GET /api/units/{id}` — content preview would need `/api/units/{id}/document`. → **Response**: already dropped via GLM's NIT (no content preview in v1). Kimi's finding reinforces.
4. `[FIXED]` BLOCKER 4(b): the Go handler returns 404 for both missing AND unauthorized rows, so the 403 panel was dead code. → **Response**: §Approach now declares only 404 + 5xx error states.
5. `[FIXED]` NIT 5: add a trivial Vitest for the 404/5xx error branches. → **Response**: added `tests/unit/admin-unit-detail.test.tsx` to §Files Create list. Happy path stays manual smoke.

Kimi round-1 also confirmed direction: scope split is clean.

## Code Review

(pending — 5-way at branch-diff time)

## Post-execution report

(pending)

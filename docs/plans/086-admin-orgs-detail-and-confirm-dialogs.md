# Plan 086 — Org detail page + edit + uniform confirm-dialog pattern

## Problem

Two related gaps surfaced after plan 085:

1. **No org detail page.** Just like `/admin/users` had no per-user route before plan 085, `/admin/orgs` has no per-org route. Clicking an org name goes nowhere. Operations are constrained to the list-page dropdown.
2. **No org edit ops.** When an admin needs to fix a typo in an org name or correct a contact email after onboarding (most common admin-support task per discussion), they have to drop to the DB. No UI path.
3. **`window.confirm()` for auth-changing ops** (toggle platform-admin, reactivate user, reactivate org, approve org) is a UX downgrade vs the custom popup we already use for suspensions. The native browser dialog has inconsistent styling across browsers and feels uncalibrated to the app's design language. Per user direction: replace with a custom `ConfirmDialog` for ALL auth-changing operations. Suspensions keep their existing type-to-confirm dialogs (different pattern — heavier friction for higher blast radius).

## Approach

Three thematic changes, three phases by domain (matching `docs/coding-agent.md` dispatch policy):

- **Phase 1 — Backend (Codex)** — new admin org endpoints: `GET /api/admin/orgs/{orgID}` returning enriched org (with membership counts), `PATCH /api/admin/orgs/{orgID}/details` (name + contact email). Tests.
- **Phase 2 — Frontend (Sonnet)** — new generic `<ConfirmDialog>` component; org detail placeholder page; org edit form; `UserActions` + `OrgActions` rewrite to use `ConfirmDialog` instead of `window.confirm`. Tests.
- **Phase 3 — Verify + docs.**

### Phase 1 — Backend (Codex)

#### 1a. Extend `Org` struct with membership counts (new `AdminOrg` shape)

Pattern mirrors `User` (lean) / `AdminUser` (enriched) from plan 085. Keep base `Org` untouched (used by every caller); introduce a separate `AdminOrg` struct for the enriched admin view:

```go
type AdminOrg struct {
    Org
    TeacherCount int `json:"teacherCount"`
    StudentCount int `json:"studentCount"`
    ParentCount  int `json:"parentCount"`
    AdminCount   int `json:"adminCount"`  // org_admin role
    TotalActive  int `json:"totalActive"` // sum across roles, status='active'
}
```

#### 1b. New store method — `GetAdminOrgByID`

```go
func (s *OrgStore) GetAdminOrgByID(ctx context.Context, orgID string) (*AdminOrg, error)
```

SQL: a single query joining `organizations` + a subquery on `org_memberships` grouped by role with `status = 'active'`:

```sql
SELECT o.id, o.name, o.slug, o.type, o.status, o.contact_email, o.contact_name,
       o.domain, o.settings, o.verified_at, o.created_at, o.updated_at,
       COALESCE(m.teacher_count, 0) AS teacher_count,
       COALESCE(m.student_count, 0) AS student_count,
       COALESCE(m.parent_count, 0)  AS parent_count,
       COALESCE(m.admin_count, 0)   AS admin_count,
       COALESCE(m.total_active, 0)  AS total_active
FROM organizations o
LEFT JOIN LATERAL (
  SELECT
    COUNT(*) FILTER (WHERE role = 'teacher')   AS teacher_count,
    COUNT(*) FILTER (WHERE role = 'student')   AS student_count,
    COUNT(*) FILTER (WHERE role = 'parent')    AS parent_count,
    COUNT(*) FILTER (WHERE role = 'org_admin') AS admin_count,
    COUNT(*)                                   AS total_active
  FROM org_memberships
  WHERE org_id = o.id AND status = 'active'
) m ON TRUE
WHERE o.id = $1
```

The composite index `org_memberships_user_status_created_idx (user_id, status, created_at)` added in plan 085 doesn't serve this query (it's keyed on user_id, not org_id). An index on `(org_id, status)` likely exists already (verify pre-impl); if not, add `CREATE INDEX org_memberships_org_status_idx ON org_memberships (org_id, status)` in a new tiny migration.

#### 1c. New store method — `UpdateOrgDetails`

```go
func (s *OrgStore) UpdateOrgDetails(ctx context.Context, orgID string, name, contactEmail string) (*Org, error)
```

SQL: `UPDATE organizations SET name = $1, contact_email = $2, updated_at = NOW() WHERE id = $3 RETURNING ...`. Both fields are required (no partial updates v1 — admin enters the full pair in the UI). Validate both are non-empty before calling.

#### 1d. New handler — `GetAdminOrg` (`GET /api/admin/orgs/{orgID}`)

- Validate orgID as UUID (existing `ValidateUUIDParam` middleware).
- Call `GetAdminOrgByID`. 404 if not found. 200 + `*AdminOrg` body otherwise.
- Mounted under admin-only chain (existing).

#### 1e. New handler — `UpdateAdminOrgDetails` (`PATCH /api/admin/orgs/{orgID}/details`)

```go
type updateOrgDetailsRequest struct {
    Name         string `json:"name"`
    ContactEmail string `json:"contactEmail"`
}
```

- Validate orgID as UUID.
- Decode body; validate `Name` and `ContactEmail` are non-empty trimmed strings (400 otherwise).
- Validate `ContactEmail` is a valid email format (use existing helper or `net/mail.ParseAddress`).
- Call `UpdateOrgDetails`. 200 + updated `*Org` body (or `*AdminOrg` if a follow-up GET is needed — either is fine; plan picks lean `*Org` since the membership counts haven't changed).

#### 1f. Route registration

Wire both new endpoints under the admin route group in `cmd/api/main.go` (or wherever admin routes live — see plan 085 step 1f for the convention).

#### 1g. Tests

- `platform/internal/store/orgs_test.go` (EXTEND if exists, else CREATE) — `GetAdminOrgByID` with zero memberships, mixed memberships, suspended memberships excluded; `UpdateOrgDetails` happy path + persistence; nil-on-not-found.
- `platform/internal/handlers/admin_test.go` (EXTEND) — `GetAdminOrg` 200 / 404 / 400 (bad UUID) / 401 / 403; `UpdateAdminOrgDetails` 200 / 400 (empty name) / 400 (empty email) / 400 (malformed email) / 400 (bad UUID) / 404 / 401 / 403.

### Phase 2 — Frontend (Sonnet)

#### 2a. New generic component — `src/components/ui/confirm-dialog.tsx`

A reusable confirm dialog for any cancel/confirm flow. Adapts the suspend-org-dialog pattern minus the type-to-confirm input:

```tsx
"use client";

interface ConfirmDialogProps {
  open: boolean;
  onClose: () => void;
  onConfirm: () => Promise<void> | void;
  title: string;
  body: React.ReactNode;
  cancelLabel?: string;       // default "Cancel"
  confirmLabel?: string;      // default "Confirm"
  confirmingLabel?: string;   // default "Confirming…"
  destructive?: boolean;      // default false → primary button; true → variant="destructive"
}
```

Behavior:
- `role="dialog"`, `aria-modal="true"`, `aria-labelledby` referencing the title id.
- Escape + backdrop click close (suppressed while submitting).
- `onCloseRef` stability so the Escape listener doesn't rebind on every parent render (same pattern as `suspend-user-dialog.tsx`).
- Error from `onConfirm` (rejected promise) surfaces inline with `role="alert"` and does NOT close the dialog.
- `succeeded` local + post-finally invocation to avoid dead state updates after unmount.
- Confirm button auto-focused on open.
- Reset error state each time the dialog re-opens.

Lives in `src/components/ui/` since it's app-wide reusable, not just admin.

#### 2b. New page — `src/app/(portal)/admin/orgs/[id]/page.tsx`

Mirror `src/app/(portal)/admin/users/[id]/page.tsx` structure (plan 085):

- Server component. Validate `id` UUID; 400 card on malformed.
- Fetch `GET /api/admin/orgs/{id}`. Handle 401 → redirect to /login; 403 → platform-admin-required card; 404 → not-found card.
- Happy path renders two Cards:
  1. **Org metadata Card** with title `{org.name}`: status badge (active/pending/suspended with appropriate color), type, contact email, contact name, slug, joined date, last updated date, plus a Members row: "5 teachers · 32 students · 3 parents · 2 org admins · 42 active total". Each role formatted from the counts.
  2. **Activity placeholder Card** titled "Activity" with copy: "Session volume, recent admin actions, and per-org metrics will appear here."
- "← Back to organizations" link top-left.
- An **Edit organization** button top-right that opens the new `OrgEditDialog` (§2c).

#### 2c. New component — `src/components/admin/org-edit-dialog.tsx`

A modal form with two text inputs (Name, Contact Email) and Cancel/Save buttons. Same modal patterns as `SuspendOrgDialog` (`role="dialog"`, escape/backdrop close, onCloseRef stability, network-error catch with `role="alert"`).

Props:
```tsx
interface Props {
  org: { id: string; name: string; contactEmail: string };
  open: boolean;
  onClose: () => void;
  onSaved: () => void;  // caller calls router.refresh()
}
```

Submit: PATCH `/api/admin/orgs/{org.id}/details` with `{name, contactEmail}`. Disable Save when both fields are unchanged OR either is empty (trimmed). Surface 400 / 404 / network errors inline.

#### 2d. Make org name in list clickable + update OrgActions

`src/app/(portal)/admin/orgs/page.tsx`:
- Wrap each org name in a `<Link href={`/admin/orgs/${org.id}`}>` with the same hover treatment as the user list.

`src/components/admin/org-actions.tsx` (REWRITE the menu structure to match user-actions pattern):
- Add a **View details** item at the top of every menu (links to `/admin/orgs/{id}`).
- Pending org menu: View details · **Approve organization**.
- Active org menu: View details · **Edit organization…** · **Suspend organization…**.
- Suspended org menu: View details · **Reactivate organization**.
- Replace inline approve `<Button>` with a dropdown menu item — consistent with the rest of the table.

#### 2e. Replace `window.confirm` with `ConfirmDialog` everywhere

Affected files:

**`src/components/admin/user-actions.tsx`** — two replacements:
- Reactivate: was `window.confirm("Reactivate {name}? They will be able to sign in again.")` → opens a `<ConfirmDialog>` with title "Reactivate account", body "{name} will be able to sign in again.", confirm "Reactivate", `destructive=false`.
- Toggle platform-admin: was `window.confirm("Make {name} a platform admin?" | "Remove {name}'s platform-admin role?")` → opens `<ConfirmDialog>` with title "Grant platform-admin role" or "Remove platform-admin role", appropriate body copy, confirm "Grant" or "Remove", `destructive=true` for the remove case.

**`src/components/admin/org-actions.tsx`** — three new dialogs (approve, reactivate, edit):
- Approve: confirm "Approve organization", body "Activate {name}? Members will gain access.", `destructive=false`.
- Reactivate: same pattern as user reactivate.
- Edit: opens the `OrgEditDialog` from §2c (a form, not a confirm-only dialog).

After any successful confirm: `router.refresh()`.

#### 2f. Tests

- `tests/unit/confirm-dialog.test.tsx` (NEW) — generic dialog cases: closed-returns-null, body/title render, confirm callback fires, error surface, reset-on-reopen, Escape close suppressed while submitting, destructive=true → destructive button class.
- `tests/unit/admin-org-detail-page.test.tsx` (NEW) — page renders metadata + counts; 400/404/403 panels; "Edit organization" button present.
- `tests/unit/org-edit-dialog.test.tsx` (NEW) — Save disabled when unchanged / empty; submit triggers PATCH; error inline; reset-on-reopen.
- `tests/unit/org-actions.test.tsx` (NEW) — menu items by status; each action opens the right dialog; Approve clicks confirm + fires PATCH; navigation to detail page.
- `tests/unit/user-actions.test.tsx` (EXTEND existing) — reactivate now opens ConfirmDialog (not window.confirm); toggle-admin same.

### Phase 3 — Verify + docs

- Run full test suite (Vitest + Go).
- Smoke-test in dev: open `/admin/orgs/{id}`, exercise Edit + Approve + Reactivate + Suspend flows; same on `/admin/users` for the new confirm dialog UX.
- Update `docs/api.md` with the new endpoints.
- Self-review the combined branch diff.

## Files

**Modify (5):**
- `platform/internal/store/orgs.go` — add `AdminOrg` struct + `GetAdminOrgByID` + `UpdateOrgDetails`.
- `platform/internal/handlers/admin.go` — add `GetAdminOrg` + `UpdateAdminOrgDetails` handlers; register routes.
- `cmd/api/main.go` (or routes file) — register the 2 new endpoints.
- `src/app/(portal)/admin/orgs/page.tsx` — wrap name in Link to detail page.
- `src/components/admin/org-actions.tsx` — rewrite menu (View details + Approve/Edit/Suspend/Reactivate items); use ConfirmDialog for Approve + Reactivate.
- `src/components/admin/user-actions.tsx` — replace `window.confirm` with ConfirmDialog for reactivate + toggle-admin.

**Create (5 + tests):**
- (Maybe) `drizzle/00XX_org_memberships_org_status_idx.sql` — only IF pre-impl verifies the index doesn't already exist.
- `src/components/ui/confirm-dialog.tsx` — generic Cancel/Confirm modal.
- `src/app/(portal)/admin/orgs/[id]/page.tsx` — org detail page.
- `src/components/admin/org-edit-dialog.tsx` — Name + Contact Email edit form modal.
- `tests/unit/confirm-dialog.test.tsx`
- `tests/unit/admin-org-detail-page.test.tsx`
- `tests/unit/org-edit-dialog.test.tsx`
- `tests/unit/org-actions.test.tsx`

**Extend (2 + tests):**
- `platform/internal/store/orgs_test.go` (or create if missing) — `GetAdminOrgByID` + `UpdateOrgDetails` cases.
- `platform/internal/handlers/admin_test.go` — 2 new endpoint test suites.
- `tests/unit/user-actions.test.tsx` — update reactivate + toggle-admin tests for ConfirmDialog.

## Decisions to lock in

1. **`AdminOrg` struct mirrors plan 085's `AdminUser` split.** Base `Org` stays lean (auth path unaffected); enriched view for the admin endpoint. Same precedent.
2. **Membership counts via FILTER aggregates in one LATERAL subquery.** Single round-trip; avoids N+1. Acceptable at current org sizes; if a single org grows past ~50k members the counts can be cached later.
3. **Edit endpoint is `PATCH /api/admin/orgs/{orgID}/details`, distinct from `PATCH /api/admin/orgs/{orgID}` (status).** Keeps the status-change path narrowly scoped; avoids "what changed?" ambiguity in handler logic. Two endpoints, two concerns.
4. **Edit accepts name + contact email only.** Slug change is destructive (breaks URLs / SEO); type and domain are larger decisions. Defer those to follow-up plans.
5. **No partial-update support v1.** Admin submits both fields; both are required. Eliminates merge-semantics bugs.
6. **`ConfirmDialog` lives in `src/components/ui/`** (not `admin/`) since it's app-wide reusable.
7. **`ConfirmDialog` is for reversible auth-changing ops + low-risk admin operations.** Suspend-org and suspend-user keep their type-to-confirm dialogs — heavier friction for higher blast radius. Two patterns, deliberate by risk level.
8. **`destructive: true` toggles the confirm button to `variant="destructive"`.** Used for "Remove platform-admin role" but NOT for "Reactivate" (constructive).
9. **`window.confirm` is removed from `user-actions.tsx` and `org-actions.tsx` entirely.** No fallback path. The custom dialog is the only UX.
10. **OrgActions adopts the menu pattern from UserActions** (everything behind `...` even for active orgs). Approve becomes a menu item, not an inline button. Consistent table density.
11. **View details on every status.** Same as UserActions Decision #18 — read-only access shouldn't be gated.
12. **Pending orgs do NOT show Edit.** Edit is for active orgs; pending orgs typically need Approve first. (If you want to fix a typo before approving, Approve the org with the typo and then Edit. Cheap; avoids state-explosion in the UI.)

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| `GetAdminOrgByID` LATERAL subquery performance at scale | low | FILTER aggregates over a single org's memberships are bounded by the active-member count (typically <100s, max thousands). Existing `org_memberships_org_idx` index covers `org_id` lookups; LATERAL inherits. Pre-impl: confirm the index exists. |
| `ConfirmDialog` is reused across user-actions + org-actions + future plans — interface drift over time | low | Keep the prop interface minimal (title, body, labels, destructive flag, callbacks). Don't add domain-specific knobs. New use cases either fit the existing shape or fork into a domain-specific dialog. |
| `org_memberships(org_id, status)` index may be missing | medium | Pre-impl: `psql -d bridge_test -c "\d org_memberships"` to confirm. If missing, the plan's migration adds it. If present, drop §1's migration mention. |
| Replacing `window.confirm` breaks any existing reactivate/toggle test that asserts `window.confirm` was called | low | Plan 085 used `vi.spyOn(window, 'confirm').mockReturnValue(true)` in tests. Those tests need updating to mock the ConfirmDialog open/close cycle instead. Listed under §2f. |
| OrgActions test file doesn't exist today; plan 085 only had UserActions test | low | The plan creates `tests/unit/org-actions.test.tsx` from scratch. Pattern is established. |
| Edit endpoint allows changing contact_email to anything — no domain check / disposable-email blocker | low (v1) | Accept any valid RFC 5322 email format. Validation tighter than that (DNS check, disposable-email blocklists) is out of scope. |
| Approve flow currently uses `<Button size="sm">` inline; converting to menu item is a regression for one-click approve UX | low | Two reasonable views: (a) one-click Approve is faster, (b) menu-item Approve is consistent with other ops. The plan picks (b) for consistency. A "primary action" button could be added later if approving becomes a high-frequency operation. Acceptable v1. |
| ConfirmDialog used for `Toggle platform-admin` doesn't gate via type-to-confirm — single-click can promote someone | medium | This is by design per the graded-pattern decision (Decision #7 / #8). Promotion is a sensitive op but reversible (Demote uses same dialog, `destructive=true`). If audit log is added later (deferred from plan 085), the cost of a misclick is recoverable. Acceptable v1 trade-off. |
| Org detail "Activity" placeholder card may give the false impression that something will appear soon | low | Same risk as the user detail Activity card from plan 085. Acceptable — the placeholder signals intent without committing to a timeline. |
| Slug-rename "broken URLs / SEO" risk from Decision #4 is currently theoretical — slugs aren't exposed in public URLs | low | Verify slug isn't in any user-facing URL. If it isn't, the deferral rationale weakens but the consistency-of-scope argument still holds. |

## Phases

### Phase 1 — Backend (Codex dispatch)

1. **Pre-impl audit**: confirm `organizations` table name (we know from plan 085 it's `organizations`, not `orgs`). Check whether `org_memberships(org_id, status)` index exists. Inventory existing callers of `GetOrg` / `ListOrgs` to ensure they don't need changes.
2. **Store**: add `AdminOrg` struct, `GetAdminOrgByID` (LATERAL FILTER counts), `UpdateOrgDetails`.
3. **Handlers**: `GetAdminOrg`, `UpdateAdminOrgDetails`. Wire routes.
4. **(Conditional) Migration**: `drizzle/00XX_org_memberships_org_status_idx.sql` IF the index doesn't exist.
5. **Tests**: store + handler. All happy + error paths.
6. **Run** `cd platform && TEST_DATABASE_URL=... go test ./... -count=1 -timeout 180s`. Green.
7. **Self-review** the Go diff on Opus.
8. **Commit** as `plan 086 phase 1 (backend)`. Push.

### Phase 2 — Frontend (Sonnet dispatch)

1. **Pre-impl grep**: existing usages of `window.confirm`; the org-actions menu state; any test mocking confirm.
2. **Create `ConfirmDialog`** in `src/components/ui/confirm-dialog.tsx`. Adapt patterns from suspend-org-dialog.
3. **Create org detail page** + Edit dialog.
4. **Rewrite `OrgActions`** menu structure (View details + status-conditional items; use ConfirmDialog for Approve + Reactivate; opens OrgEditDialog for Edit; opens SuspendOrgDialog for Suspend).
5. **Wrap org name in Link** on `/admin/orgs` list page.
6. **Replace `window.confirm` in `UserActions`** with ConfirmDialog for reactivate + toggle-admin.
7. **Tests**: ConfirmDialog, OrgEditDialog, OrgActions, admin-org-detail-page; extend user-actions test for ConfirmDialog migration.
8. **Run** `bun run test`, `bun run lint`, `bunx tsc --noEmit`. No regressions vs baseline.
9. **Self-review** TS diff on Opus.
10. **Commit** as `plan 086 phase 2 (frontend)`. Push.

### Phase 3 — Verify + docs

1. **Full test suite** — Vitest + Go.
2. **Smoke-test in dev**: org list → click name → detail page renders; Edit dialog saves; Approve/Suspend/Reactivate all use the new dialogs; user toggle-admin + reactivate use ConfirmDialog (no native confirm).
3. **Update `docs/api.md`** with the 2 new endpoints (GET admin org by id, PATCH org details).
4. **Self-review** the combined branch diff.
5. **Commit** as `plan 086 phase 3 (verify + docs)`. Push.
6. **Trigger 4-way code review** against the branch diff.

## Testing plan

| Layer | Test file | Cases |
|-------|-----------|-------|
| Go store | `platform/internal/store/orgs_test.go` (EXTEND) | `GetAdminOrgByID` with zero memberships, mixed roles, suspended excluded, not-found nil; `UpdateOrgDetails` happy path + persistence |
| Go handler | `platform/internal/handlers/admin_test.go` (EXTEND) | `GetAdminOrg` 200/404/400/401/403; `UpdateAdminOrgDetails` 200, 400 (empty name / empty email / malformed email / bad UUID), 404, 401, 403 |
| TS dialog | `tests/unit/confirm-dialog.test.tsx` (NEW) | closed-returns-null, body/title render, confirm fires async callback, error inline on rejected promise, reset on reopen, destructive prop styles confirm button, Escape suppressed during submit |
| TS detail page | `tests/unit/admin-org-detail-page.test.tsx` (NEW) | renders metadata + member counts; 400/404/403 panels; Edit button visible |
| TS edit dialog | `tests/unit/org-edit-dialog.test.tsx` (NEW) | Save disabled when both fields unchanged / either empty; PATCH on submit; inline error on 4xx; reset on reopen |
| TS org actions | `tests/unit/org-actions.test.tsx` (NEW) | menu items by status; Approve opens ConfirmDialog → fires PATCH on confirm; Reactivate opens ConfirmDialog; View details navigates |
| TS user actions | `tests/unit/user-actions.test.tsx` (EXTEND) | reactivate + toggle-admin no longer call `window.confirm`; instead open ConfirmDialog; clicking Confirm fires PATCH |

## Verification steps

After each phase: lint + type-check + relevant tests pass.

Before opening the PR: full test suite (Go + Vitest), manual smoke.

Lint baseline: 101 errors / 45 warnings (from plan 085's branch tip). Must not regress.
TSC baseline: 7 errors. Must not regress.
Vitest baseline: 709 pass + 3 pre-existing failures. Must not regress.

## Plan Review

(Placeholder — to be filled by 4-way plan review before implementation.)

## Code Review

(Placeholder — to be filled after Phase 3.)

## Post-Execution Report

(Placeholder — to be filled before opening the PR.)

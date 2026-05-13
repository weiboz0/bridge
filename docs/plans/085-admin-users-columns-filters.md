# Plan 085 — Admin users page: role + org columns, filters, and per-row operations

## Problem

The platform-admin user list at `/admin/users` (`src/app/(portal)/admin/users/page.tsx`) currently shows: Name, Email, Admin (Yes/blank), Joined, Actions. The Actions menu (`src/components/admin/user-actions.tsx`) has exactly one item: "Login as {firstName}" (impersonate). Three operational gaps the user flagged:

1. **No role visibility** — when scanning users, the admin can't see whether each row is a teacher, student, parent, or org-admin without clicking through to inspect their org membership. The most useful at-a-glance signal is missing.
2. **No org visibility** — same problem for org affiliation. Multi-org future aside, every non-platform-admin user today belongs to exactly one org, and that name is the primary disambiguator.
3. **No filters** — the page renders every user as one flat list. With even modest growth this becomes unscannable. The adjacent `/admin/orgs` page already has filter chips for status (`src/app/(portal)/admin/orgs/page.tsx:67-71`); applying the same pattern here is the natural fix.
4. **No per-row operations beyond Impersonate.** When an admin needs to support a user (forgot password, account compromised, role change), there's no UI path. The Go backend doesn't currently support disabling a user at all (no `users.status` column — verified by grepping `drizzle/*.sql` and `src/lib/db/schema.ts`).
5. **No user detail page.** Clicking on a user row goes nowhere. Even a thin placeholder route (rendering the user's metadata + "more details coming soon") would preserve the entry point so future detail features have a home — without it, the feature gets forgotten when v1 ships. (Five gaps in total — the §Problem heading "5 operational gaps" should be read literally; the earlier "4" was a stale count.)

The Go API at `/api/admin/users` (`platform/internal/handlers/admin.go:67-75`) returns the `User` struct from `platform/internal/store/users.go:13-22`, which only surfaces id/name/email/avatar/isPlatformAdmin/timestamps. Role data lives in `org_memberships` (per `src/lib/db/schema.ts:99-104` — enum `org_admin`/`teacher`/`student`/`parent`). The intent column `users.intended_role` (signup_intent enum: `teacher`/`student`) is what the user said at signup but isn't authoritative — a "student" intent could later be upgraded to a teacher org membership. The org-membership role wins for display.

Auth middleware lives in `platform/internal/auth/` (per `grep RequireAuth` — `middleware_phase3_test.go` references `mw.RequireAuth`). Adding `users.status` means RequireAuth needs to reject `status='suspended'` users so disabling actually blocks sign-in.

## Approach

Three thematic changes, organized into phases by domain (matching the new domain-based dispatch from `docs/coding-agent.md`):

- **Phase 1 — Backend (Codex)** — all Go work: migration, struct/SQL extensions for columns + filters, three new admin endpoints (suspend/reactivate/toggle-admin/password-reset), RequireAuth update, tests.
- **Phase 2 — Frontend (Sonnet)** — all TS work: page columns, filter chips, org dropdown, new action dropdown items + suspend dialog, tests.
- **Phase 3 — Verify + docs** — full suite, smoke-test, docs.

### Phase 1 — Backend (Codex)

#### 1a. Migration: add `users.status`

```sql
-- drizzle/00XX_users_status.sql
CREATE TYPE "public"."user_status" AS ENUM ('active', 'suspended');
ALTER TABLE "users" ADD COLUMN "status" "user_status" DEFAULT 'active' NOT NULL;
CREATE INDEX "users_status_idx" ON "users" USING btree ("status");

-- Composite index serving the LATERAL `WHERE user_id = $1 AND status = 'active'
-- ORDER BY created_at ASC LIMIT 1` lookup in ListUsers/GetAdminUserByID.
-- Kimi K2.6 round-1: only `org_memberships_user_idx` (user_id alone) exists today.
CREATE INDEX "org_memberships_user_status_created_idx"
  ON "org_memberships" USING btree ("user_id", "status", "created_at");
```

Update `src/lib/db/schema.ts` to add the enum + column. Existing rows backfill to `'active'` via DEFAULT. Drizzle `db:generate` should produce a clean migration matching the hand-written SQL.

#### 1b. Extend `User` struct minimally; introduce `AdminUser` for the enriched view

Per Kimi K2.6 round-1 BLOCKER: putting the new fields on the shared `User` struct breaks the auth path. `GetUserByEmail` (used during login) would either need updating to populate them OR would return partially-valid structs that the new auth-status check would misread as `status=""`.

Resolution: keep `User` lean (add only `Status` since it's needed everywhere — login auth needs it too), introduce a separate `AdminUser` struct for the enriched admin view.

```go
// Base User struct — gets ONE new field (status). Every callsite already
// touches password_hash via password verification, so HasPassword is NOT
// added here.
type User struct {
    ID              string    `json:"id"`
    Name            string    `json:"name"`
    Email           string    `json:"email"`
    AvatarURL       *string   `json:"avatarUrl"`
    IsPlatformAdmin bool      `json:"isPlatformAdmin"`
    Status          string    `json:"status"`   // NEW — 'active' | 'suspended'
    CreatedAt       time.Time `json:"createdAt"`
    UpdatedAt       time.Time `json:"updatedAt"`
}

// AdminUser — enriched view for /admin/users list + detail endpoints.
// Embeds User and adds the membership + has-password fields.
type AdminUser struct {
    User
    OrgRole     *string `json:"orgRole"`     // primary org_memberships.role
    OrgID       *string `json:"orgId"`       // primary org_memberships.org_id
    OrgName     *string `json:"orgName"`     // joined organizations.name
    HasPassword bool    `json:"hasPassword"` // password_hash IS NOT NULL
}
```

**Why include `HasPassword`**: the frontend uses it to grey out the "Reset password" menu item for OAuth-only users. Click-then-error is a worse UX than visibly-unavailable.

**Migration safety**: every existing store method that returns `User` (or `*User`) must select the new `status` column. That includes `GetUserByID`, `GetUserByEmail`, the auth-path lookup, and any test fixture builders. Pre-impl step: grep `SELECT.*FROM users` to inventory; update each.

`ListUsers` and `GetUserByID` change return types — `ListUsers(...) ([]AdminUser, error)` and add `GetAdminUserByID(...) (*AdminUser, error)`. The existing `GetUserByID` stays on the lean `User` shape so the auth path isn't affected.

#### 1c. Rewrite `ListUsers` with filters

Change signature: `ListUsers(ctx, ListUsersFilter)` where:

```go
type ListUsersFilter struct {
    Role  *string // org_admin | teacher | student | parent | platform_admin | unassigned
    OrgID *string // UUID
}
```

SQL (LATERAL pulls the primary membership — earliest active by `created_at` for determinism, since single-org-per-user is the de facto invariant today):

```sql
SELECT u.id, u.name, u.email, u.avatar_url, u.is_platform_admin, u.status,
       (u.password_hash IS NOT NULL) AS has_password,
       m.role, m.org_id, o.name AS org_name,
       u.created_at, u.updated_at
FROM users u
LEFT JOIN LATERAL (
  SELECT role, org_id, created_at
  FROM org_memberships
  WHERE user_id = u.id AND status = 'active'
  ORDER BY created_at ASC
  LIMIT 1
) m ON TRUE
LEFT JOIN organizations o ON o.id = m.org_id
WHERE (...filter clauses...)
ORDER BY u.created_at DESC
```

Filter clauses:

| Filter value | Clause |
|--------------|--------|
| `role=org_admin\|teacher\|student\|parent` | `AND m.role = $param` |
| `role=platform_admin` | `AND u.is_platform_admin = TRUE` |
| `role=unassigned` | `AND m.role IS NULL AND u.is_platform_admin = FALSE` |
| `orgId=<uuid>` | `AND m.org_id = $param` |
| both | both clauses (AND) |
| neither | no extra clause |

#### 1d. Update `ListAllUsers` handler

- Accept `?role=` and `?orgId=` from the URL.
- Validate `role` ∈ {`org_admin`, `teacher`, `student`, `parent`, `platform_admin`, `unassigned`} — reject anything else with 400.
- Validate `orgId` as a UUID — reject malformed with 400.
- Empty params → `nil` filter fields (no clause).

#### 1e. New endpoint — PATCH `/api/admin/users/{userID}/status`

Toggles `users.status` between `active` and `suspended`.

```go
type updateUserStatusRequest struct {
    Status string `json:"status"`  // "active" | "suspended"
}
```

Handler:
- Validate `userID` as UUID (existing `ValidateUUIDParam` middleware).
- Decode body, validate `Status` is one of the two values.
- **Self-target guard**: reject if `userID == requestor.ID` with 400 ("Cannot change own status"). Same pattern admin/orgs uses to prevent self-lockout.
- Update `users.status` via parameterized UPDATE.
- Return 200 with the updated user row.

#### 1f. New endpoint — PATCH `/api/admin/users/{userID}/platform-admin`

Toggles `users.is_platform_admin`.

```go
type updatePlatformAdminRequest struct {
    IsPlatformAdmin bool `json:"isPlatformAdmin"`
}
```

- Validate userID as UUID.
- Decode body.
- **Self-target guard**: reject if `userID == requestor.ID && body.IsPlatformAdmin == false` with 400 ("Cannot demote self from platform admin"). Promoting self isn't possible anyway (you can't reach this endpoint without already being admin).
- UPDATE users.is_platform_admin.
- Return 200 with the updated user row.

#### 1g. New endpoint — POST `/api/admin/users/{userID}/password-reset`

Sends a password-reset email.

- Validate userID as UUID.
- Look up user; check `password_hash IS NOT NULL`. If null (OAuth-only), return 400 with `{error: "User has no password to reset (OAuth-only account)"}`.
- Use the existing password-reset email pathway (grep `passwordReset` / `ResetToken` to find it; if no admin-triggered path exists, add a helper that reuses the user-triggered flow).
- Return 200 with `{ok: true}`. Don't include the token in the response.

**Pre-verified** by GLM round-1: grep of `platform/` for `passwordReset|ResetToken|recover_token|forgot` returned zero results. There is no existing password-reset infrastructure. Decision: **defer §1g (password-reset endpoint) to a follow-up plan**. The "Reset password" menu item is also dropped from §2b (UserActions). Document the deferral in the post-execution report. The other 3 new endpoints (get-user, suspend/reactivate, toggle-admin) ship in this plan.

#### 1h. New endpoint — GET `/api/admin/users/{userID}`

Returns a single user enriched with org membership + status (same shape as the list endpoint per user).

- Validate `userID` as UUID.
- Reuse the LATERAL SQL from `ListUsers` filtered to a single user (or extend `GetUserByID` to do the LEFT JOIN itself — preferred, since `GetUserByID` is the canonical single-user fetch).
- Return 404 if not found.
- Return 200 with the enriched user.

#### 1i. Update `RequireAuth` middleware — share the admin-check cache for status

Per GLM 5.1 + Kimi K2.6 round-1 BLOCKERS: `RequireAuth` (`platform/internal/auth/middleware.go:147`) currently validates the JWT/bridge.session, calls `injectLiveAdmin` (line 289), and never loads the user row from the DB. `injectLiveAdmin` uses `CachedAdminChecker` with a **60-second TTL** (`admin_check.go:31`). A naive added-per-request DB lookup would regress auth performance by ~1 query/request on every authenticated path.

Resolution (GLM option A, Kimi option (i)): **extend the cached lookup to return both `is_admin` AND `status`**. Concretely:

1. Rename `LookupIsAdmin` → `LookupAdminAndStatus` in `platform/internal/auth/admin_check.go`:
   ```go
   type AdminLookup interface {
       LookupAdminAndStatus(ctx context.Context, userID string) (isAdmin bool, status string, err error)
   }
   ```
2. Cache value becomes `struct { IsAdmin bool; Status string }` keyed by userID. TTL unchanged at 60s.
3. `injectLiveAdmin` becomes `injectLiveStatus` — populates `claims.IsPlatformAdmin` AND `claims.Status` from the cache.
4. `RequireAuth` checks `claims.Status == "suspended"` after `injectLiveStatus` and returns 401 with `{error: "Account suspended. Contact your administrator."}`.
5. `Claims` struct (`platform/internal/auth/jwt.go` or equivalent) gets `Status string` field. JWT payload does NOT carry status — it's set from the live cache after verification.
6. **Cache eviction on suspend/reactivate**: `UpdateUserStatus` handler calls `AdminChecker.Purge(userID)` after the UPDATE succeeds. `Purge` already exists on the checker (verified at `admin_check_test.go:134-150`). This makes suspend take effect IMMEDIATELY for new requests — not "on next request after 60s cache expiry".
7. **Cache eviction on toggle-admin**: `UpdateUserPlatformAdmin` also calls `Purge(userID)` (same rationale — instant effect for security-sensitive ops).

**Suspend-dialog UX copy update**: "Existing sessions are invalidated immediately — the user will be signed out on their next request."

**Existing test impact**: tests that stub `LookupIsAdmin` need updating to the new signature (returning `(bool, string, error)`). Grep `LookupIsAdmin` to inventory. The 5 existing tests in `admin_check_test.go` all use a `stubLookup` — updating its return shape is mechanical.

**New tests**:
- `admin_check_test.go`: `TestCachedAdminChecker_StatusFieldCached` (verify status survives the cache hit path).
- `middleware_test.go`: `TestRequireAuth_RejectsSuspended` — 401 returned when user.status='suspended' (Status field on claims).
- `middleware_test.go`: `TestRequireAuth_ActiveStatusPasses` — control case.

#### 1j. Tests

- `platform/internal/store/users_test.go` (EXTEND — already exists, contains `TestUserStore_GetUserByID_NotFound` etc.) — happy-path coverage for `ListUsers` with each filter shape (8+ cases including `role=platform_admin + orgId` combined SQL path) + new status field assertions; new `UpdateStatus`, `UpdatePlatformAdmin` cases; `GetAdminUserByID` returns the enriched shape (org membership + has_password + status); existing `GetUserByID` test fixtures updated for the new `status` column.
- `platform/internal/handlers/admin_test.go` (extend) — 200/400/401/403 paths for all 4 new endpoints (suspend/reactivate, toggle-admin, password-reset, get-user-by-id). Self-target guards covered. Cross-user isolation.
- `platform/internal/auth/middleware_test.go` (extend) — `TestRequireAuth_RejectsSuspended`.

### Phase 2 — Frontend (Sonnet)

#### 2a. Page (`src/app/(portal)/admin/users/page.tsx`)

Extend interface, render new columns, add filter UI:

```tsx
interface UserItem {
  id: string;
  name: string;
  email: string;
  isPlatformAdmin: boolean;
  status: "active" | "suspended";   // NEW
  orgRole: string | null;            // NEW
  orgId: string | null;              // NEW
  orgName: string | null;            // NEW
  hasPassword: boolean;              // NEW — greys "Reset password" for OAuth-only users
  createdAt: string;
}

interface AdminOrg { id: string; name: string; }
```

Server component reads `searchParams.role` and `searchParams.orgId`, builds the query string, fetches `/api/admin/users?...` + `/api/admin/orgs` (for the org filter dropdown) + `/api/me/identity` in one `Promise.all`.

**Columns** — insert Role + Org between Email and Admin:

```
Name | Email | Role | Org | Admin | Status | Joined | Actions
```

(Add Status column too, so suspended users are visibly flagged — text "Suspended" in red OR a small badge.)

Display formatting: `orgRole` → "Org admin" / "Teacher" / "Student" / "Parent" / "—"; `orgName` → name or "—"; `status` → "—" for active, "Suspended" badge for suspended.

**Filter chips (role)** — same visual pattern as `admin/orgs/page.tsx:66-71`. Inline `FilterChip` component builds hrefs that preserve the other filter:

```tsx
<FilterChip current={role} value={undefined} preserve={{ orgId }}>All</FilterChip>
<FilterChip current={role} value="org_admin" preserve={{ orgId }}>Org admin</FilterChip>
<FilterChip current={role} value="teacher" preserve={{ orgId }}>Teacher</FilterChip>
<FilterChip current={role} value="student" preserve={{ orgId }}>Student</FilterChip>
<FilterChip current={role} value="parent" preserve={{ orgId }}>Parent</FilterChip>
<FilterChip current={role} value="platform_admin" preserve={{ orgId }}>Platform admin</FilterChip>
<FilterChip current={role} value="unassigned" preserve={{ orgId }}>Unassigned</FilterChip>
```

**Org filter** — `<select>` (chips don't scale past ~5 options). Fetch `/api/admin/orgs?status=active` so only active orgs appear in the dropdown (filtering by a suspended org is rarely useful and may surprise an admin). Implemented as a small `"use client"` component `src/components/admin/org-filter-select.tsx` that submits its form on change:

```tsx
"use client";
export function OrgFilterSelect({ orgs, current, role }: ...) {
  return (
    <form method="get" action="/admin/users">
      {role && <input type="hidden" name="role" value={role} />}
      <select name="orgId" defaultValue={current ?? ""} onChange={(e) => e.currentTarget.form?.submit()}>
        <option value="">All orgs</option>
        {orgs.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
      </select>
    </form>
  );
}
```

#### 2b. UserActions dropdown (`src/components/admin/user-actions.tsx`)

Extend props with `userStatus`, `isPlatformAdmin`, `isSelf` (already implicit via the page's existing self-row hiding, but pass explicit for clarity), `orgId` (for future use). Menu items by row state:

| Status | Items |
|--------|-------|
| `active`, not self | View details · Login as {firstName} · Toggle platform-admin · Suspend account… |
| `suspended`, not self | View details · Reactivate account (recommended first) · Toggle platform-admin (always enabled — independent of status) |
| self | View details only (no destructive/impersonate ops on self) — render `<UserActions isSelf={true} />` with narrowed item set rather than hiding the component entirely as today |

Behaviors:
- **View details** — navigates to `/admin/users/{userID}` (the new detail page below). Available on every row including self.
- **Login as** — existing impersonate path, unchanged.
- **Reset password** — **DEFERRED to follow-up plan** (no password-reset infra in Bridge today; verified by GLM round-1 grep). Not in this plan's UI. `hasPassword` field still ships on the API response so the future plan can use it.
- **Toggle platform-admin** — `window.confirm("Make {name} a platform admin?" | "Remove {name}'s platform-admin role?")` → PATCH `/api/admin/users/{userID}/platform-admin`.
- **Suspend account** — Open `SuspendUserDialog` (NEW, mirroring `SuspendOrgDialog` pattern from plan 084-equivalent / PR #148): type-to-confirm with the user's NAME.
- **Reactivate account** — `window.confirm("Reactivate {name}? They will be able to sign in again.")` → PATCH `/api/admin/users/{userID}/status` with `{status: "active"}`.

After any operation: `router.refresh()`.

#### 2c. New page — `src/app/(portal)/admin/users/[id]/page.tsx` (placeholder)

Server component, matches the structure of `src/app/(portal)/admin/units/[id]/page.tsx` (plan 079):

- Pre-validate `[id]` param as UUID; render 400 card on malformed.
- Fetch `GET /api/admin/users/{id}` (new endpoint from §1h). 404 + 401 + 403 handled the same way the index page does.
- Render a Card with the user's metadata:
  - Name (heading), Email
  - Status badge ("Active" / "Suspended")
  - Role (formatted org role + "Platform admin" badge if applicable)
  - Org (name as plain text — the current `/admin/orgs` page filters by status, not orgId, so a deeplink would no-op. A future plan can add orgId filtering on admin/orgs then upgrade this to a link.)
  - Joined date, Last updated date
- Below the metadata, render a `<Card>` titled "Activity" with placeholder copy: "Session history, audit log, and per-user metrics will appear here." This makes the future-feature intent explicit so it isn't forgotten.
- Top-right: a "Back to users" link → `/admin/users`.

NOT in this plan (call out explicitly in §Decisions): no sessions list, no audit log, no per-user activity charts, **no actions on the detail page** (Suspend/Reactivate/Toggle-admin live only on the list-page row dropdown). The detail page is read-only metadata in v1.

#### 2d. New component — `src/components/admin/suspend-user-dialog.tsx`

Direct adaptation of `src/components/admin/suspend-org-dialog.tsx` (PR #148). Same props shape (`userId`, `userName`, `open`, `onClose`, `onSuspended`), same patterns: `role="dialog"`, type-to-confirm, network-error catch, `onClose` ref stability, `role="alert"`, symmetric `.trim()`, callbacks-after-finally. Copy-adapt the file rather than introducing a generic abstraction — the modal is small (~130 lines) and two copies are clearer than one parametrized component.

#### 2e. Tests

- `tests/unit/admin-users-page.test.tsx` (NEW) — render with mocked api: (a) all-users render with role+org+status columns; (b) filter chips render with active state matching the URL param; (c) chip hrefs preserve `orgId`; (d) org `<select>` renders all orgs with the right default selected; (e) suspended user shows the badge; (f) self-row hides destructive Actions but exposes View details.
- `tests/unit/admin-user-detail-page.test.tsx` (NEW) — render with mocked api: (a) renders user metadata; (b) renders "Activity" placeholder card; (c) renders 400 card on malformed UUID; (d) renders 404 card when API returns 404; (e) renders 403 panel on platform-admin-required.
- `tests/unit/suspend-user-dialog.test.tsx` (NEW) — direct adaptation of `tests/unit/suspend-org-dialog.test.tsx` (PR #148). 8 cases: closed-returns-null, disabled-on-mismatch, enabled-on-match, success-callbacks, HTTP-error inline, network-error inline + role="alert", reset-on-reopen, symmetric-trim.
- `tests/unit/user-actions.test.tsx` (NEW) — verify each menu item appears / hides for each status; verify each action triggers the right confirm + fetch; verify View details appears for self. (Codex round-1 BLOCKER about testing greyed-out Reset Password is MOOT — Reset Password is deferred to a follow-up plan; the related test ships with that plan when it lands.)

### Phase 3 — Verify + docs

- Run full test suite (Vitest + Go).
- Smoke-test in dev: open `/admin/users`, exercise filters, perform each operation against a test user.
- Update `docs/api.md` with the new endpoints + query params.
- Self-review the full branch diff for cross-phase consistency (TS field names match Go JSON tags, schema migration matches Drizzle schema).

## Files

**Modify (7):**
- `src/lib/db/schema.ts` — add `userStatusEnum`, `users.status` column.
- `platform/internal/store/users.go` — `User` struct + `ListUsers` filter signature + new SQL; `GetUserByID` returns enriched shape; add `UpdateStatus`, `UpdatePlatformAdmin` methods.
- `platform/internal/handlers/admin.go` — `ListAllUsers` reads query params; add `GetAdminUser`, `UpdateUserStatus`, `UpdateUserPlatformAdmin` handlers. (Password-reset deferred.)
- `platform/internal/auth/middleware.go` (or wherever `RequireAuth` lives) — reject suspended users.
- `cmd/api/main.go` (or wherever admin routes mount) — register the 3 new endpoints (get-user, suspend/reactivate, toggle-admin).
- `src/app/(portal)/admin/users/page.tsx` — fields + columns + filters + searchParams.
- `src/components/admin/user-actions.tsx` — extend with the new menu items (View details + ops) + dialog wiring.

**Create (8):**
- `drizzle/00XX_users_status.sql` — migration.
- `platform/internal/store/users_test.go` — store integration tests for `ListUsers` filters + new methods + enriched `GetUserByID`.
- `src/app/(portal)/admin/users/[id]/page.tsx` — placeholder user detail page.
- `src/components/admin/suspend-user-dialog.tsx` — type-to-confirm dialog, adapted from `suspend-org-dialog.tsx`.
- `src/components/admin/org-filter-select.tsx` — `"use client"` org dropdown.
- `tests/unit/admin-users-page.test.tsx` — page-level rendering + filter tests.
- `tests/unit/admin-user-detail-page.test.tsx` — detail page tests (metadata + placeholder card + error states).
- `tests/unit/suspend-user-dialog.test.tsx` — dialog tests, adapted from `suspend-org-dialog.test.tsx`.
- `tests/unit/user-actions.test.tsx` — action-menu state tests.

**Extend (2):**
- `platform/internal/handlers/admin_test.go` — coverage for the 4 new endpoints (get-user-by-id, suspend/reactivate, toggle-admin, password-reset).
- `tests/unit/schema.test.ts` — update the users-table assertion to include the new `status` column (Kimi K2.6 round-1 nit).
- `platform/internal/auth/middleware_test.go` (existing — confirm path) — `TestRequireAuth_RejectsSuspended`.

**Touch (1):**
- `docs/api.md` — document the new endpoints + query params.

## Decisions to lock in

1. **`org_memberships.role` wins over `users.intended_role`.** Intent is signup-time; org role is current truth.
2. **Primary-membership = earliest active membership.** Single-org-per-user is de facto today. LATERAL `LIMIT 1` deterministic. Future multi-org would need a different display strategy.
3. **Filter values map 1:1 to enum names + `platform_admin` + `unassigned`.** Stable URL contract.
4. **`role=platform_admin` is OR with org role, not AND.** A platform admin with a teacher org membership shows under both filters. Max flexibility.
5. **`role=unassigned` excludes platform admins.** They have no org but they ARE assigned. Defined explicitly in SQL.
6. **Org filter is a `<select>`, not chips.** Doesn't scale past ~5 options.
7. **`form.submit()` on `<select> onChange`** — server-component-friendly. Whole page re-renders against new URL.
8. **Filter combinations are AND.** No combinators.
9. **Validation rejects unknown values with 400.** Don't silently ignore.
10. **No pagination in this plan.** Out of scope; defer to a future plan when user counts warrant it.
11. **Self-target guards in 3 places:** suspend, demote-from-admin, AND the page-level "hide Actions on self" check (existing). Defense in depth.
12. **Suspend uses type-to-confirm; reactivate + admin-toggle use `window.confirm()`; reset-password uses `window.confirm()`.** Friction matches reversibility: suspend is hard to recover from a misclick + has user-facing impact; admin-toggle and reactivate are easily reversed; reset-password is read-only-side-effect from Bridge's perspective.
13. **Suspended users blocked at auth middleware, NOT at session creation.** Existing sessions go invalid on next request. Simpler than session revocation. Slight delay window (until next request) is acceptable for admin support flows.
14. **OAuth-only users can't have password reset.** Return 400 from the endpoint; UI surfaces it inline. Don't try to be clever (no "send them an account-recovery email" alternative in v1).
15. **`SuspendUserDialog` is a copy of `SuspendOrgDialog`, not a shared abstraction.** Two ~130-line files are clearer than one parametrized component with title/copy props. If a third "suspend X" dialog appears later, refactor then.
16. **`status` column on `users` is an enum, not a boolean.** Mirrors `orgs.status` precedent. Leaves room for future states (`pending_email_verification`, `pending_admin_approval`, etc.) without another migration.
17. **Detail page is a placeholder route.** v1 renders only the user's metadata + a "Session history, audit log, and per-user metrics will appear here" placeholder card. No sessions/audit/charts in this plan — those need new infra (Bridge doesn't track last-seen yet). The placeholder exists so the entry point doesn't get forgotten and future plans have a home.
18. **View details is the one non-destructive action available on the self row.** Other actions (impersonate, reset password, suspend, toggle admin) remain hidden on self per existing pattern. Implementation: pass `isSelf={true}` to `<UserActions />` and let it render the narrowed set, rather than hiding the component entirely (the current row-level `user.id !== identity.userId` check at `page.tsx:84` becomes a prop-level branch).
19. **`HasPassword` field surfaces in the API response** so the frontend can grey out "Reset password" for OAuth-only users — click-then-error is a worse UX than visibly-unavailable. The endpoint still returns 400 on no-password as defense in depth.
20. **No audit log for admin operations in this plan.** Bridge has no audit-log infra today (verify in Phase 1 step 1). Suspend/reactivate/toggle-admin/password-reset are not recorded beyond `updated_at`. Acceptable v1 — a future plan can add an `admin_actions` table and retrofit. Flagged in §Risks.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| LATERAL JOIN performance on `org_memberships` for large user counts | low | Composite index `(user_id, status, created_at)` does NOT exist today (verified by Kimi: only `org_memberships_user_idx` on `userId` alone). The §1a migration MUST add this composite index. Without it, the LATERAL `ORDER BY created_at ASC LIMIT 1` does a per-user sort over all the user's membership rows. |
| User has multiple active memberships in different orgs — primary pick is ambiguous | medium | LATERAL `ORDER BY created_at ASC LIMIT 1` is deterministic. Code comment in SQL: "earliest active membership wins." Future multi-org expansion needs a different display strategy. |
| Existing `ListUsers` callers outside the admin handler | low | Pre-impl grep `grep -rn "\.ListUsers(" platform/`. If others exist, give them `ListUsersFilter{}` (no-op) or split into `ListUsers()` (compat) + `ListUsersFiltered(filter)`. |
| Existing tests that mock the old `/api/admin/users` response shape | low | Pre-impl grep for `UserItem` and `admin/users` mocks. Update to include new fields. |
| `<select>` `onChange={form.submit()}` flagged as client-side in server component | medium (handled) | Decision #7 — wrap in `"use client"` component `OrgFilterSelect`. |
| Migration safety: existing `users` rows backfilled to `'active'` may surprise | low | Default `'active'` is the most permissive choice; no admin needed to "approve" existing users. Document in migration comment. |
| `users.status` rollout: existing OAuth + email users sign in normally | low | RequireAuth check is post-load; if `status='active'` (default), pass. No regression for existing users. |
| RequireAuth change rejects users mid-session if admin suspends them | medium (intended) | This IS the feature — admin wants to lock out a compromised account immediately. Acceptable; surface to admin in the suspend-dialog copy: "Existing sessions will be invalidated on their next request." |
| Self-suspend / self-demote: admin locks themselves out | high | Decision #11 — self-target guards both at handler (400) and at row level (Actions hidden for self). |
| Password-reset infrastructure doesn't exist yet | medium | Pre-impl: grep `passwordReset`, `ResetToken`, `verifyToken`, `RecoverToken`. If absent, defer 1g to a follow-up plan and update §Decisions. The rest of the plan ships without it; the "Reset password" menu item is dropped from §2b. Document the deferral in the post-execution report. |
| No audit log for admin operations | low (v1) | Decision #20. Future plan adds an `admin_actions` table and retrofits the 4 new endpoints. Until then, `updated_at` is the only trail. |
| Password-reset endpoint sends to OAuth-only user accidentally | low | Decision #14 — endpoint returns 400 if `password_hash IS NULL`. Frontend surfaces. |
| Password-reset endpoint can be used as an email-flooding vector | low | Same rate-limiting concerns as the user-initiated path. Reuse the same throttle if one exists; otherwise add a per-target rate limit. |
| 3-action menu becomes cluttered on the row | low | Dropdown menu, not inline buttons — clutter is hidden behind the `...` trigger. |
| Toggle-admin without a strong confirmation | medium | Decision #12 — `window.confirm()`. Two-direction (promote / demote) so a misclick is easily reversed. Acceptable v1; can add type-to-confirm later if abuse surfaces. |
| Suspend dialog "Existing sessions invalidated" copy may be wrong if RequireAuth caches user | low | Pre-impl: verify RequireAuth fetches user on every request (no cache). If it caches, suspend won't take effect until cache TTL. Likely fine (Bridge currently does per-request DB lookup for user). |
| Reactivate via dropdown — no friction means a misclick on a recently-suspended account re-enables it | low | Reactivate is reversible (re-suspend). `window.confirm()` is sufficient. |
| Schema migration mismatch: hand-written SQL vs. Drizzle `db:generate` output | low | Generate with `bun run db:generate` after editing schema.ts; if the output differs from the hand-written SQL, prefer the generated version and update the plan. |
| Auth middleware test ordering: existing tests assume no `status` check | low | Add the `status='active'` field to test fixtures. RequireAuth test for suspended is additive. |

## Phases

### Phase 1 — Backend (Codex dispatch)

1. **Pre-impl audit**: grep callers of `ListUsers`, hunt for existing password-reset infra (`passwordReset`/`ResetToken`/`recover_token`), confirm `users` table has no existing `status` column, locate RequireAuth implementation.
2. **Migration**: add `drizzle/00XX_users_status.sql` + matching `schema.ts` change. Generate via `bun run db:generate` to confirm parity, apply to `bridge_test`.
3. **Struct + ListUsers SQL**: extend `User` struct, replace `ListUsers` with filtered version, write SQL with LATERAL + LEFT JOIN orgs + filter clauses.
4. **Handler param parsing**: `ListAllUsers` reads + validates `role` and `orgId`.
5. **GET single user**: extend `GetUserByID` to return enriched fields (or add `GetUserByIDWithMembership`). Add `GetAdminUser` handler + route.
6. **New PATCH endpoints**: `UpdateUserStatus`, `UpdateUserPlatformAdmin`. Wire routes in `cmd/api/main.go`.
7. **Password reset endpoint** (deferred to a follow-up plan if infra missing — verify in step 1).
8. **RequireAuth update**: reject `status='suspended'`.
9. **Tests**: store tests for all filter shapes + new methods + enriched `GetUserByID`; handler tests for 4 endpoints; auth middleware test for suspended-reject.
10. **Run** `cd platform && go test ./... -count=1 -timeout 120s`. All green.
11. **Self-review** the Go diff on Opus.
12. **Commit** as `plan 085 phase 1 (backend)`. Push.

### Phase 2 — Frontend (Sonnet dispatch)

1. **Pre-impl grep**: tests that mock `/api/admin/users` or `UserItem` shape.
2. **Update list page** (`page.tsx`): extend `UserItem`, add columns, add filter UI, thread searchParams.
3. **Create `OrgFilterSelect`** — `"use client"` component.
4. **Create detail page** (`[id]/page.tsx`): metadata Card + placeholder Activity card + error states.
5. **Update `UserActions`**: expand with new menu items (View details + ops), wire to new endpoints.
6. **Create `SuspendUserDialog`** — copy + adapt `SuspendOrgDialog`.
7. **Tests**: list page, detail page, dialog, actions menu.
8. **Run** `bun run test`, `bun run lint`, `bunx tsc --noEmit`. Baselines unchanged.
9. **Self-review** the TS diff on Opus.
10. **Commit** as `plan 085 phase 2 (frontend)`. Push.

### Phase 3 — Verify + docs

1. **Full test suite** — Vitest + Go.
2. **Smoke-test in dev**: open `/admin/users`, try each filter, perform each operation against a non-admin test user. Verify suspended user is blocked from sign-in.
3. **Update `docs/api.md`** with the new endpoints + query params.
4. **Self-review** the combined branch diff. Cross-phase consistency: TS field names ↔ Go JSON tags; schema migration ↔ Drizzle schema.
5. **Commit** any docs/cleanup as `plan 085 phase 3 (verify + docs)`. Push.
6. **Trigger 5-way code review** against the branch diff.

## Testing plan

| Layer | Test file | Cases |
|-------|-----------|-------|
| Go store | `platform/internal/store/users_test.go` (NEW) | `ListUsers` all-users; role=teacher; role=student; role=org_admin; role=parent; role=platform_admin; role=unassigned; orgId=X; role+orgId combined; `UpdateStatus` active→suspended; `UpdateStatus` suspended→active; `UpdatePlatformAdmin` true; `UpdatePlatformAdmin` false |
| Go handler | `platform/internal/handlers/admin_test.go` (extend) | `ListAllUsers` 200 with filters (including `role=platform_admin + orgId` combined SQL path); 400 on bad role; 400 on bad orgId; `GetAdminUser` 200; 404 missing; 400 bad UUID; suspend/reactivate 200; 400 on self-suspend; 400 on bad UUID; 401 unauth; 403 non-admin; toggle-admin 200; 400 on self-demote; 404 on missing user; cross-user isolation; admin-cache `Purge` is called on suspend + toggle-admin |
| Go auth | `platform/internal/auth/middleware_test.go` (extend) | `TestRequireAuth_RejectsSuspended` — 401 returned when user.status='suspended' |
| TS list page | `tests/unit/admin-users-page.test.tsx` (NEW) | columns render with API data; filter chips active state ↔ searchParam; chip hrefs preserve orgId; org `<select>` defaults + options; suspended badge renders; self-row hides destructive Actions; View details visible on self |
| TS detail page | `tests/unit/admin-user-detail-page.test.tsx` (NEW) | metadata Card renders all fields; Activity placeholder Card renders; 400 on malformed UUID; 404 card on missing user; 403 panel on non-admin |
| TS dialog | `tests/unit/suspend-user-dialog.test.tsx` (NEW) | closed-returns-null; disabled-on-mismatch; enabled-on-match; success-callbacks; HTTP-error inline; network-error inline + role="alert"; reset-on-reopen; symmetric-trim |
| TS actions | `tests/unit/user-actions.test.tsx` (NEW) | items visible by status (active vs suspended); each action triggers right confirm + fetch; current user row hidden |

## Verification steps

After each phase: lint + type-check + relevant tests pass.

Before opening the PR: full test suite (Go + Vitest), manual smoke in dev.

Lint baseline: 102 errors / 45 warnings on `main`. Must not regress.
TSC baseline: 7 errors on `main` (`tests/unit/identity-assert.test.ts`). Must not regress.
Vitest baseline: 3 pre-existing failures in `tests/integration/auth-jwt-refresh.test.ts`. Must not regress.

## Plan Review

### Self-review (Opus 4.7) — 2026-05-12

**Verdict: CONCUR with self-applied refinements.**

Self-review concerns identified and folded into the plan before external dispatch:

1. **`HasPassword` field decision was inconclusive** → locked in as Decision #19 (include the field, grey out menu item for OAuth-only).
2. **"View details on self" implementation was ambiguous** → locked in as Decision #18 (pass `isSelf` prop, render narrowed set, vs. current hide-component pattern).
3. **Detail page org link** → locked in as plain text rather than a deeplink (`/admin/orgs` doesn't filter by orgId today; a future plan can upgrade).
4. **Password-reset infra unverified** → §Risks updated with explicit fallback (defer 1g to follow-up plan if grep finds nothing; rest of the plan ships unaffected).
5. **No audit logging** → captured as Decision #20 + Risk row (acceptable v1; future plan retrofits).

Remaining open concerns flagged for external reviewers to weigh in on:

- **`<form method="get">` on `<select>` inside a server component** — pure HTML form should work but App Router behavior worth a second opinion. Mitigation: separate `"use client"` component (already planned), but the form itself is plain HTML.
- **Self-suspend / self-demote** guards exist at handler level (400) but the UI's `isSelf` prop is a separate codepath — could drift. The plan calls out defense in depth; a reviewer may want explicit tests that the UI guard is consistent with the API guard.
- **Migration ordering** — adding `users.status` with DEFAULT 'active' is safe for existing rows, but if Bridge has any unmigrated dev DBs the field will appear NULL when read by old code paths. Bridge tests run on `bridge_test` which is reset between runs, so dev DB hygiene is the user's concern. Worth a reviewer's eye.

### Round 1 verdicts — 2026-05-12

| Reviewer | Verdict | Blockers (resolved status) |
|----------|---------|----------------------------|
| Self (Opus 4.7) | CONCUR | (none) |
| Codex | **BLOCKER** | (a) Table is `organizations`, not `orgs`. RESOLVED — all SQL refs updated. (b) `hasPassword` field on Go side but missing from TS `UserItem`. RESOLVED — added to interface in §2a. (c) No test for greyed-out reset for OAuth-only users. MOOT — Reset password is now deferred to a follow-up plan (zero infra). |
| DeepSeek V4 Pro | pending | — |
| GLM 5.1 | **BLOCKER** | (a) `RequireAuth` doesn't load user row; `injectLiveAdmin` is cached 60s. Suspend won't take effect for up to 60s. RESOLVED — §1i now extends `CachedAdminChecker` to carry status alongside admin; `Purge(userID)` on suspend/toggle-admin invalidates the entry. Plus 4 nits all resolved (HasPassword SQL, composite index, password-reset deferral wording, test matrix for combined filter). |
| Kimi K2.6 | **BLOCKER** | (a) Same as GLM (a): RequireAuth DB-lookup mechanism unspecified. RESOLVED via the same fix in §1i. (b) Shared `User` struct scope: adding fields breaks the auth path. RESOLVED — §1b now splits into a lean `User` (status added) + a new enriched `AdminUser` for the admin view; `GetUserByEmail` and other lean callsites are unaffected by the membership/has-password additions. Plus 7 nits resolved (users_test.go is extend not create, composite index "add if missing", suspended-row menu clarified, gap count corrected, OrgFilterSelect filters to active, schema.test.ts noted, detail page explicitly read-only). |

**Plan revised in commit ⟨pending⟩**. Re-dispatching Codex, GLM, Kimi to confirm resolution. DeepSeek V4 Pro's verdict still pending; will fold any new findings in round 2.

## Code Review

(Placeholder — to be filled after Phase 3.)

## Post-Execution Report

(Placeholder — to be filled before opening the PR.)

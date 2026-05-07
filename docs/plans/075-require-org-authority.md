# Plan 075 — `RequireOrgAuthority` helper + per-handler migration

## Problem (review 011 §1.3)

Class-scoped access has a clean shared helper at
`platform/internal/handlers/access.go:67` (`RequireClassAuthority`). Org-scoped
access does NOT — every org-gated handler reimplements role-scan boilerplate
inline. The current state:

- `orgs.go` has SIX inline `GetUserRolesInOrg`+role-loop blocks at lines
  161, 187, 253, 283, 361, 442 — across `GetOrg`, `UpdateOrg`, `ListMembers`,
  `AddMember`, `UpdateMember`, `RemoveMember`.
- `org_parent_links.go` introduced a private `requireOrgAdmin` helper at
  line 79 — better locally, but scoped to only that file. Used at lines
  65, 116, 148, 221.
- `classes.go:74, 122` and `courses.go:72, 122` repeat the same pattern with a
  "teacher or org_admin" twist for create / list-by-org.

Three concrete consequences:

1. **Behavior drift**: each call site decides for itself whether platform
   admin bypasses, whether impersonator-of-admin (plan 039) bypasses, what
   error message to return, and whether to 403/404. The class-side helper has
   correct uniform answers; the org-side rolls each one independently. None of
   the current org-side checks honor the impersonator-of-admin carve-out.
2. **Maintenance debt**: when the rules change (e.g., adding a new role like
   `office_admin` or a new bypass pathway), every call site needs editing.
3. **Test surface**: the role-scan logic is unit-tested only at the store
   level (`GetUserRolesInOrg` in `store/orgs_test.go`); the per-handler
   wrappers have no consolidated unit-test coverage of their decision rule.

Review 011 §1.3 recommendation, verbatim:

> Add `RequireOrgAuthority(ctx, orgs, claims, orgID, level)` with read/admin
> levels and platform-admin override semantics. Migrate org dashboard,
> membership, courses/classes, parent-link handlers in the next
> auth-consolidation plan.

## Current state — three call patterns observed

After reading all 14 call sites in the four named handler files, the inline checks fall into three buckets:

### Pattern A — "any active member" (read access)
- `orgs.go:161` (`GetOrg`)
- `orgs.go:253` (`ListMembers`)
- `classes.go:122` (`ListClassesByOrg`)
- `courses.go:122` (`ListCoursesByOrg`)

Decision: `len(roles) > 0` → grant; else 403.

### Pattern B — "org_admin only" (admin access)
- `orgs.go:187` (`UpdateOrg`)
- `orgs.go:283` (`AddMember`)
- `orgs.go:361` (`UpdateMember`)
- `orgs.go:442` (`RemoveMember`)
- `org_parent_links.go:88` (inside the local `requireOrgAdmin` helper, used 4 times)

Decision: any role with `Role == "org_admin"` → grant; else 403.

### Pattern C — "teacher or org_admin" (teach access)
- `classes.go:74` (`CreateClass`)
- `courses.go:72` (`CreateCourse`)

Decision: any role with `Role == "teacher"` OR `Role == "org_admin"` → grant; else 403.

### Out of scope — intentionally NOT migrated in plan 075

A grep of the broader handler tree turns up 7+ additional sites that call
`GetUserRolesInOrg`:

- `teacher.go:55, 108` (teacher dashboard org filter)
- `unit_ai.go:592` (unit AI auth)
- `schedule.go:73` (schedule create/edit)
- `sessions.go:343, 1033, 1082, 1128, 1392` (session-access fallback paths)

Each of these is a CLASS-or-ORG fallback pattern — the handler checks
class-membership FIRST, then falls back to org_admin as a secondary pathway.
Migrating them to `RequireOrgAuthority` would be wrong: the helper grants on
EVERY org_admin path, but in these contexts the inline check is a deliberate
secondary fallback inside a larger composite auth decision. They need their
own consolidation (potentially a `RequireClassOrOrgAccess` composed helper),
which is a separate plan. The review 011 §1.3 recommendation explicitly named
the handlers in scope ("org dashboard, membership, courses/classes,
parent-link handlers") — this plan honors that scope.

`access.go:103, 190` also call `GetUserRolesInOrg`. Those are inside
`RequireClassAuthority` and `CanViewUnit` respectively — internal to other
auth helpers, not handler-level auth gates. Out of scope.

## Approach

Introduce `RequireOrgAuthority` in `access.go` mirroring the
`RequireClassAuthority` shape. Three levels matching the three call patterns.
Migrate all 14 call sites in the four named handler files. Delete the local
`requireOrgAdmin` helper in `org_parent_links.go`.

### API shape

```go
type OrgAccessLevel string

const (
    OrgRead  OrgAccessLevel = "read"   // any active member
    OrgTeach OrgAccessLevel = "teach"  // teacher or org_admin (course/class create)
    OrgAdmin OrgAccessLevel = "admin"  // org_admin only
)

func RequireOrgAuthority(
    ctx context.Context,
    orgs *store.OrgStore,
    claims *auth.Claims,
    orgID string,
    level OrgAccessLevel,
) (bool, error)
```

Returns `(true, nil)` on grant; `(false, nil)` on deny (caller writes 403);
`(false, err)` on DB error (caller writes 500). Returns
`ErrAccessHelperMisconfigured` if `orgs` is nil — same pattern as
`RequireClassAuthority` so callers see 500 on misconfig, not silent denial.

### Bypass semantics (matches `RequireClassAuthority`)

- `claims == nil` → deny (caller writes 401 separately).
- `claims.IsPlatformAdmin` → grant (every level).
- `claims.ImpersonatedBy != ""` → grant (per plan 039 — admin inspecting an
  org while impersonating a student retains their access). **This IS a
  behavior change for the org subsystem**: pre-plan-075, none of the inline
  org checks honored the impersonator carve-out. Post-plan-075, they do —
  matching the class-side behavior. Documented in §Decisions.

### Status filter

`RequireClassAuthority` checks `m.Status == "active"` only on the
class-membership rule, not on org_admin (`access.go:108-112`). For the new
helper, mirror that: org_admin must be `Status == "active"` to bypass at any
level (matches the existing org-side pattern of suspended-admins-don't-pass);
teacher must be `Status == "active"` for `OrgTeach` to grant; member must
be `Status == "active"` for `OrgRead` to grant.

This is a slight tightening relative to the existing inline checks: most
inline sites do `len(roles) > 0` without checking status, which would let a
suspended member pass `OrgRead`. The new helper REQUIRES active status. The
review 011 §1.3 recommendation didn't specify status semantics, but matching
class-side behavior is more correct and is consistent with how the
self-action guard (plan 069 phase 4) already filters by `status == "active"`.
Documented in §Decisions.

## Decisions to lock in

1. **Three levels (Read/Teach/Admin), not two.** Review 011 §1.3 said
   "read/admin"; current call patterns reveal a third pattern at
   `classes.go:74` + `courses.go:72` (teacher-or-org_admin). Adding `OrgTeach`
   keeps the helper exhaustive and matches actual usage rather than forcing
   class/course creation handlers back to inline boilerplate.
2. **Add impersonator-of-admin bypass to org checks.** Behavior change:
   admin-impersonating-student now passes org-gated requests too, matching
   plan 039's carve-out and `RequireClassAuthority`'s behavior. Justification:
   the underlying admin retained those privileges before impersonation;
   asymmetric behavior between class and org subsystems is a bug, not a
   feature.
3. **Require `Status == "active"` at every level.** Slight tightening:
   suspended members no longer pass `OrgRead`; suspended teachers no longer
   pass `OrgTeach`; suspended org_admins no longer bypass. Matches
   `RequireClassAuthority` precedent + plan 069 phase 4's self-action guard.
4. **Delete `requireOrgAdmin` in `org_parent_links.go`.** All 4 call sites
   migrate to `RequireOrgAuthority(... OrgAdmin)`. The local helper exists
   only because the shared one didn't; removing it once the shared one lands
   is the whole point of this plan.
5. **No store-level changes.** `GetUserRolesInOrg` keeps its current
   signature; the helper just orchestrates the existing query + role-scan.
6. **No client / Next-side changes.** All migration is in the Go API.

## Files

**Modify (5 files):**

- `platform/internal/handlers/access.go` — add `OrgAccessLevel` type, three
  level constants, and `RequireOrgAuthority` function. Mirrors
  `RequireClassAuthority` style (header doc comment + impl).
- `platform/internal/handlers/orgs.go` — replace 6 inline blocks with calls
  to `RequireOrgAuthority`. Lines 159-170 (`GetOrg`, `OrgRead`), 184-203
  (`UpdateOrg`, `OrgAdmin`), 251-262 (`ListMembers`, `OrgRead`), 281-296
  (`AddMember`, `OrgAdmin`), 359-374 (`UpdateMember`, `OrgAdmin`), 440-455
  (`RemoveMember`, `OrgAdmin`).
- `platform/internal/handlers/org_parent_links.go` — delete `requireOrgAdmin`
  helper (lines 76-103); replace 4 call sites (lines 65, 116, 148, 221) with
  inline calls to `RequireOrgAuthority(... OrgAdmin)`.
- `platform/internal/handlers/classes.go` — replace inline blocks at
  lines 70-89 (`CreateClass`, `OrgTeach`) and 121-131 (`ListClassesByOrg`,
  `OrgRead`).
- `platform/internal/handlers/courses.go` — replace inline blocks at
  lines 68-87 (`CreateCourse`, `OrgTeach`) and 117-127 (`ListCoursesByOrg`,
  `OrgRead`).

**Create (1 file):**

- `platform/internal/handlers/access_org_test.go` — unit tests for
  `RequireOrgAuthority`. Same test style as `org_self_action_guard_test.go`.
  Coverage:
  - `OrgRead`: active member grants; non-member denies; suspended member
    denies; platform admin bypasses; impersonator-of-admin bypasses;
    org_admin (active) bypasses; nil claims denies; nil orgs returns
    `ErrAccessHelperMisconfigured`.
  - `OrgTeach`: active teacher grants; active org_admin grants; student
    denies; parent denies; suspended teacher denies.
  - `OrgAdmin`: active org_admin grants; active teacher denies; active
    student denies; suspended org_admin denies.

**No changes to:**

- `platform/internal/store/orgs.go` — `GetUserRolesInOrg` signature unchanged.
- Existing handler tests that exercise the old inline behavior — the
  decision contract is unchanged for the happy path; only the
  status-filter tightening + impersonator-bypass extension are new
  behaviors. Existing tests for non-admin-denied / org-admin-allowed pass
  unchanged. Add new tests in `access_org_test.go` for the new behaviors.
- Any test that asserts a SUSPENDED member can pass `OrgRead` — none exists
  (verified by grep `Status.*suspended` in handler tests; no such
  assertion).

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Status-filter tightening breaks an existing test that relied on suspended-member-passes-`OrgRead` | low | Pre-impl grep planned: `grep -rn "suspended" platform/internal/handlers/*_test.go` to enumerate. Expected zero hits where a suspended member is asserted to PASS a read check. If any exists, evaluate before merging. |
| Impersonator-bypass extension changes behavior in production for impersonating admins | medium | The class-side helper already does this (plan 039); adding it to org-side is correctness, not regression. Document in §Decisions; mention in the PR body. No production user has reported a bug from the asymmetric behavior, so latent. |
| Migrating 14 call sites in one PR risks a typo (wrong level chosen) silently widening or narrowing access | medium | Each call site replacement is a SMALL diff (3-4 lines per site). The new `access_org_test.go` covers level semantics; per-handler integration tests still pass with the new helper because the contract is identical for the happy path. Cross-phase consistency check (CLAUDE.md Step 4) catches level mismatches by re-reading the diff. |
| `requireOrgAdmin` deletion exposes a subtle behavior difference in `org_parent_links.go` (e.g., the local helper logged differently) | low | Read the local helper's body in detail before deletion; confirm the replacement has identical observable behavior (same status code, same error message format). Plan 075 phase 2 starts with this confirmation step. |
| The `RequireClassAuthority` precedent uses a `Status == "active"` filter only inside the class-membership branch (`access.go:108`), not on the org_admin branch. Mirroring "everywhere" is a slight deviation from precedent | low | Both behaviors are correct in intent; the more uniform "active everywhere" matches user expectations (suspended ≠ allowed). Document the deviation explicitly in `RequireOrgAuthority`'s doc comment so future readers don't think it's a copy-paste bug. |
| Tests dispatched to a Sonnet subagent might subtly miss the impersonator-bypass case since plan 039 is recent and the pattern isn't in muscle memory | low | Plan 075's own test list (in §Files) explicitly enumerates `impersonator-of-admin bypasses` for `OrgRead`. Dispatch brief includes the §Files test list verbatim. |
| The "any active member" check at `orgs.go:161` (`GetOrg`) currently grants access to anyone with at least one role row, regardless of whether ALL their roles are suspended (e.g., a former teacher whose teacher row is suspended but whose org_admin row is also suspended — both count). Tightening to active-only flips this | low | This is a real behavior change but represents a security improvement: a fully-suspended user should NOT pass `OrgRead`. Document in §Decisions; verify no existing test relies on the old behavior. |

## Phases

### Phase 1 — Add `RequireOrgAuthority` + unit tests (commit 1)

- Add `OrgAccessLevel` enum + 3 constants to `access.go`.
- Add `RequireOrgAuthority` function with the bypass semantics + status-filter
  + level-specific decision rules from §Approach.
- Doc-comment header explaining: behavior, levels, bypass order
  (claims→IsPlatformAdmin→ImpersonatedBy→active-org_admin→level-rule),
  uniform `active`-status filter.
- Create `access_org_test.go` with the test coverage matrix in §Files.
- Run `cd platform && go test ./internal/handlers/... -run RequireOrgAuthority -count=1 -v` —
  all new tests pass.
- Run `cd platform && go test ./... -count=1 -timeout 120s` — full suite
  green (no regressions yet; nothing migrated).
- Commit: `plan 075 phase 1: add RequireOrgAuthority helper + unit tests`.

### Phase 2 — Migrate `orgs.go` (commit 2)

- Replace 6 inline blocks in `orgs.go` with `RequireOrgAuthority` calls. For
  each: error → 500, deny → 403, grant → continue.
- Run `cd platform && go test ./internal/handlers/... -count=1 -v` — including
  `org_self_action_guard_test.go` and `org_list_integration_test.go` — all
  pass.
- Commit: `plan 075 phase 2: migrate orgs.go (6 sites) to RequireOrgAuthority`.

### Phase 3 — Migrate `org_parent_links.go` + delete local helper (commit 3)

- First confirm the local `requireOrgAdmin` body matches the helper's
  behavior (status code, error message). Adjust if needed; the new helper's
  error message is generic ("Access denied") so callers may prefer to write
  their own 403 with a more specific message — that's fine.
- Replace 4 call sites with `RequireOrgAuthority(... OrgAdmin)`.
- Delete `requireOrgAdmin` (lines 76-103).
- Run org-parent-links integration tests.
- Commit: `plan 075 phase 3: migrate org_parent_links.go + remove local requireOrgAdmin`.

### Phase 4 — Migrate `classes.go` + `courses.go` (commit 4)

- Replace 2 sites in each (`CreateClass`/`ListClassesByOrg`,
  `CreateCourse`/`ListCoursesByOrg`).
- Run handler tests for both.
- Commit: `plan 075 phase 4: migrate classes/courses.go to RequireOrgAuthority`.

### Phase 5 — Verify + post-execution report (commit 5)

- Run full Go test suite + Vitest suite.
- Pre-impl grep `grep -rn "GetUserRolesInOrg" platform/internal/handlers/`
  should return EXACTLY: 2 hits in `access.go` (lines 103 + 190 — internal
  to `RequireClassAuthority`/`CanViewUnit`, out of scope), 1 hit in
  `access.go` for the new `RequireOrgAuthority` body, plus 7 hits across
  the documented out-of-scope files (`teacher.go`, `unit_ai.go`,
  `schedule.go`, `sessions.go`). The 14 in-scope handler call sites in
  `orgs.go`, `org_parent_links.go`, `classes.go`, `courses.go` are gone.
- Update plan file's post-execution report.
- Commit: `docs: plan 075 post-execution report`.

After Phase 5, run the 5-way code review against the consolidated branch
diff (single-PR-per-plan policy), fold findings, open the PR via Step 6.

## Plan Review

(pending — 5-way before implementation)

## Code Review

(pending — 5-way at branch-diff time)

## Post-execution report

(pending)

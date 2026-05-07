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

After reading all 16 call sites in the SIX handler files now in scope (Codex round-1 BLOCKER: original draft missed `org_dashboard.go` and `topic_problems.go`), the inline checks fall into three buckets:

### Pattern A — "any active member" (read access)
- `orgs.go:161` (`GetOrg`)
- `orgs.go:253` (`ListMembers`)
- `classes.go:122` (`ListClassesByOrg`)
- `courses.go:122` (`ListCoursesByOrg`)

Decision: `len(roles) > 0` → grant; else 403.

(`org_dashboard.go:69` is `OrgAdmin`, not Read — the dashboard auth helper requires org_admin specifically.)

### Pattern B — "org_admin only" (admin access)
- `orgs.go:187` (`UpdateOrg`)
- `orgs.go:283` (`AddMember`)
- `orgs.go:361` (`UpdateMember`)
- `orgs.go:442` (`RemoveMember`)
- `org_parent_links.go:88` (inside the local `requireOrgAdmin` helper, used 4 times)
- `org_dashboard.go:69` (inside the `authorizeOrgAdmin` helper used by the dashboard handlers — already correctly bypasses on `IsPlatformAdmin || ImpersonatedBy != ""`, so impersonator semantics ARE consistent here; just missing the helper consolidation)

Decision: any role with `Role == "org_admin"` → grant; else 403.

### Pattern C — "teacher or org_admin" (teach access)
- `classes.go:74` (`CreateClass`)
- `courses.go:72` (`CreateCourse`)
- `topic_problems.go:69` (problem-access auth helper — checks `m.Status == "active" && (m.Role == "org_admin" || m.Role == "teacher")`)

Decision: any role with `Role == "teacher"` OR `Role == "org_admin"` → grant; else 403.

### Out of scope — intentionally NOT migrated in plan 075

A full grep `grep -rn "GetUserRolesInOrg" platform/internal/handlers/` (excluding `_test.go`) returns 30 hits. After in-scope cleanup the remaining ~14 hits are all out of scope, in three buckets:

**Bucket 1: class-or-org fallback patterns** — handler checks class-membership FIRST, then falls back to `org_admin` as a secondary pathway. Migrating to `RequireOrgAuthority` would be wrong: the helper grants on every `org_admin` path, but here the inline check is a deliberate secondary fallback inside a composite auth decision. Needs a separate `RequireClassOrOrgAccess` plan.
- `schedule.go:73` (schedule create/edit)
- `sessions.go:343, 1033, 1082, 1128, 1392` (session-access composite paths)

**Bucket 2: scope-discriminating helpers** (Codex round-1 audit caught these). Each is inside a function like `canViewCollection`, `canEditUnit`, `problemAccessForRole`, etc. that handles multiple scopes (`platform`/`org`/`personal`) via a `switch scope` — only the `case "org":` branch reads org-roles. Migrating these to `RequireOrgAuthority(... OrgTeach)` is conceptually correct, but the helper signature is `(bool, error)` while these helpers return different shapes (e.g., `bool`, `(bool, int)`, `(string, *authDecision)`); migration touches helper signatures, not just one inline block. Reserve for a separate per-helper migration plan.
- `unit_collections.go:61, 87` (`canViewCollection`, `canEditCollection`)
- `unit_ai.go:592` (`canEditScopedUnit`)
- `realtime_token.go:491` (token-mint scope check)
- `topics.go:366` (`canEditTopic` org branch)
- `problem_access.go:34, 144` (`canViewProblemAtScope`, `canEditProblemAtScope`)
- `teaching_units.go:164, 871` (`canEditUnit`, `canViewUnit` org branches)

**Bucket 3: internal to other auth helpers** — not handler-level gates.
- `access.go:103, 190` (inside `RequireClassAuthority`/`CanViewUnit` — these consume `GetUserRolesInOrg` as part of their own decision logic).

The review 011 §1.3 recommendation explicitly named the handlers in scope ("org dashboard, membership, courses/classes, parent-link handlers") — plan 075 honors that scope and adds the two pure-handler-level sites Codex caught (`org_dashboard.go`, `topic_problems.go`).

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

**Modify (6 files in scope + 8 files for TODO-comment-only insertion):**

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
  - **Special migration note for `CreateLink` (line 148)** (Kimi K2.6 round-1 NIT): the local helper returned `(*auth.Claims, bool)` and `CreateLink` consumes the claims at line 197 (`claims.UserID` as the `created_by` value when inserting the parent link). Post-migration, `RequireOrgAuthority` returns `(bool, error)` only — the caller must fetch claims separately via `claims := auth.GetClaims(r.Context())` BEFORE the auth check, then nil-guard. Verify the `claims.UserID` reference at the original line 197 still resolves correctly after the helper is replaced.
- `platform/internal/handlers/classes.go` — replace inline blocks at
  lines 70-89 (`CreateClass`, `OrgTeach`) and 121-131 (`ListClassesByOrg`,
  `OrgRead`).
- `platform/internal/handlers/courses.go` — replace inline blocks at
  lines 68-87 (`CreateCourse`, `OrgTeach`) and 117-127 (`ListCoursesByOrg`,
  `OrgRead`).
- `platform/internal/handlers/org_dashboard.go` — replace inline block at
  line 69 (inside the `authorizeOrgAdmin` helper, `OrgAdmin`). This file's
  helper ALREADY has `IsPlatformAdmin || ImpersonatedBy != ""` bypass, so
  no semantic change there; the migration just consolidates with the new
  shared helper.
- `platform/internal/handlers/topic_problems.go` — replace inline block at
  line 69 (`OrgTeach`).

**TODO-comment-only insertions (8 files for Phase 5)** to prevent future devs from "cleaning up" out-of-scope inline checks (Kimi K2.6 round-1 NIT):
- `sessions.go`, `schedule.go` (Bucket 1, class-or-org fallback) — `// TODO(plan-NNN): class-or-org fallback, migrate to RequireClassOrOrgAccess`
- `unit_collections.go`, `unit_ai.go`, `realtime_token.go`, `topics.go`, `problem_access.go`, `teaching_units.go` (Bucket 2, scope-discriminating helpers) — `// TODO(plan-NNN): scope-discriminating helper, separate per-helper migration plan`

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
| Status-tightening is UNEVEN across current sites (GLM 5.1 round-1 NIT). `UpdateOrg`/`AddMember`/`UpdateMember`/`RemoveMember` currently check `Role == "org_admin"` WITHOUT a status filter; a suspended org_admin could still mutate the org. The new helper's `OrgAdmin` level requires `Status == "active"` AND `Role == "org_admin"`. Tightens these 4 sites at the same time as the GetOrg/ListMembers Read tightening | low (security improvement) | A suspended org_admin should NOT be able to mutate the org — the existing inline checks were incorrect. The migration unifies this with `org_parent_links.go`'s existing helper which already filtered on active status. Pre-impl grep `Status.*suspended` in handler tests confirms no existing test relies on suspended-org_admin-passes-mutation. Document in the commit message. |
| Inter-phase drift: a NEW handler with inline `GetUserRolesInOrg` lands between Phase 1 (helper exists) and Phase 5 (verification grep). The Phase 5 grep expectation would silently pass at the moment Phase 4 ships but be inaccurate by Phase 5 | low | (Kimi K2.6 round-1 NIT.) Run the Phase 5 grep IMMEDIATELY before opening the PR — not just after Phase 4 — so any new inline check added during PR review fires a fresh delta. The plan's single-PR-per-plan policy keeps the window narrow; this plan is also small enough that no concurrent feature work is expected to land. |
| No persistent enforcement once plan 075 ships — a future PR adds a new handler with inline `GetUserRolesInOrg` and the lint silently lets it through | low (deferred) | (Kimi K2.6 round-1 NIT.) Out of scope for plan 075. A follow-up plan could add a Vitest/Go-vet style guard that asserts no NEW handler-level `GetUserRolesInOrg` calls outside of `access.go`. Mirror plan 074's `shadow-routes.test.ts` pattern. Add to TODO.md tech-debt section as part of Phase 5. |

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

**Signature contract differences (GLM 5.1 round-1 NIT)**: the local
`requireOrgAdmin` helper writes HTTP responses directly and returns
`(*auth.Claims, bool)`. The new `RequireOrgAuthority` returns `(bool, error)`
and leaves response writing to the caller. Two concrete migration deltas at
each of the 4 call sites:

1. **401 handling**: local helper writes 401 on nil claims; new helper
   returns `(false, nil)` — each caller must already have a nil-claims guard
   above the call (verify all 4 call sites have one before deletion).
2. **403 message**: local helper writes "Only org admins can manage parent
   links"; new helper writes nothing — caller writes their own 403. Use the
   same wording at each call site to preserve external-facing error text;
   this is documented in the commit message.

Steps:
- Read each of the 4 call sites (`org_parent_links.go:65, 116, 148, 221`)
  and confirm a nil-claims guard exists above (the local helper's nil-claims
  401 path is otherwise relied on).
- For each call site: replace `h.requireOrgAdmin(...)` with the new helper +
  inline 403 with the preserved error message.
- Delete `requireOrgAdmin` (lines 76-103).
- Run `cd platform && go test ./internal/handlers/... -run "ParentLink" -count=1 -v`.
- Commit: `plan 075 phase 3: migrate org_parent_links.go + remove local requireOrgAdmin`. Commit body notes the signature contract changes for future grep-ability.

### Phase 4 — Migrate `classes.go` + `courses.go` + `org_dashboard.go` + `topic_problems.go` (commit 4)

(Kimi K2.6 round-1 NIT: phases 3 and 4 were originally split per-file but all are small, mechanical, single-file replacements; merging them keeps bisectability and reduces ceremony.)

- Replace 2 sites in `classes.go` (`CreateClass` `OrgTeach`, `ListClassesByOrg` `OrgRead`).
- Replace 2 sites in `courses.go` (`CreateCourse` `OrgTeach`, `ListCoursesByOrg` `OrgRead`).
- Replace `org_dashboard.go:69` (the `authorizeOrgAdmin` helper body — `OrgAdmin`). Note: this helper already had `IsPlatformAdmin || ImpersonatedBy != ""` bypass, so impersonator semantics are NOT changing here, only the helper consolidation.
- Replace `topic_problems.go:69` (`OrgTeach`).
- Run handler tests for all four files.
- Commit: `plan 075 phase 4: migrate classes/courses/org_dashboard/topic_problems to RequireOrgAuthority`.

### Phase 5 — Verify + out-of-scope TODO comments + post-execution report (commit 5)

- Run full Go test suite + Vitest suite.
- Run `grep -rn "GetUserRolesInOrg" platform/internal/handlers/ --include="*.go" | grep -v "_test.go"` (Codex round-2 NIT: explicitly exclude test files; the broader `grep -rn` would also count test-suite usage in `platform/internal/store/orgs_test.go` and `platform/internal/handlers/org_parent_links_test.go` which are not part of the migration). Assert the EXACT enumerated set of NON-test hits remains (Codex round-1 BLOCKER + DeepSeek round-1 NIT — earlier draft missed sites):
  - `access.go`: 3 hits (lines 103 + 190 internal to existing helpers; 1 new hit inside `RequireOrgAuthority` body).
  - **Class-or-org fallback** (Bucket 1, deferred): `schedule.go` (1), `sessions.go` (5).
  - **Scope-discriminating helpers** (Bucket 2, deferred): `unit_collections.go` (2), `unit_ai.go` (1), `realtime_token.go` (1), `topics.go` (1), `problem_access.go` (2), `teaching_units.go` (2).
  - Total expected hits = 18 (3 access + 6 bucket-1 + 9 bucket-2). The 16 in-scope handler call sites in `orgs.go`, `org_parent_links.go`, `classes.go`, `courses.go`, `org_dashboard.go`, `topic_problems.go` are GONE.
- Insert a `// TODO(plan-NNN): migrate to RequireClassOrOrgAccess` comment at each of the 6 Bucket-1 sites (sessions/schedule), and a `// TODO(plan-NNN): scope-discriminating helper, separate per-helper migration plan` at each of the 9 Bucket-2 sites — Kimi K2.6 round-1 NIT, prevents a future dev from "cleaning up" the inline code thinking it was missed.
- Update plan file's post-execution report.
- Commit: `docs: plan 075 post-execution report`.

After Phase 5, run the 5-way code review against the consolidated branch
diff (single-PR-per-plan policy), fold findings, open the PR via Step 6.

## Plan Review

### Round 1 (2026-05-06)

#### Self-review (Opus 4.7) — clarifications

Folded two clarifications at `7a538de`: count fix (13 → 14 call sites verified); explicit §Out of scope analysis enumerating 7 sites in `sessions.go`/`schedule.go`/`teacher.go`/`unit_ai.go` that are class-or-org fallback patterns (need a separate `RequireClassOrOrgAccess` plan).

#### Codex — BLOCKER (FIXED)

`[FIXED]` BLOCKER: Phase 5 verification grep claimed an exact remaining count, but Codex's audit found ~10 additional `GetUserRolesInOrg` sites the plan didn't enumerate (`unit_collections.go:61,87`, `topics.go:366`, `problem_access.go:34,144`, `org_dashboard.go:69`, `realtime_token.go:491`, `teaching_units.go:164,871`, `topic_problems.go:69`). → **Response**: full audit + classification:
  - **Promoted IN-scope**: `org_dashboard.go:69` (pure handler-level org_admin gate, mentioned by review §1.3) and `topic_problems.go:69` (pure handler-level OrgTeach gate). Plan now migrates 16 call sites across 6 files.
  - **Out of scope, Bucket 1** (class-or-org fallback): `sessions.go` (5), `schedule.go` (1) — composite class-or-org auth decisions; need a separate `RequireClassOrOrgAccess` plan.
  - **Out of scope, Bucket 2** (scope-discriminating helpers): `unit_collections.go` (2), `unit_ai.go` (1), `realtime_token.go` (1), `topics.go` (1), `problem_access.go` (2), `teaching_units.go` (2) — inline checks live inside helpers like `canViewUnit` that handle multiple scopes; migrating them touches helper signatures, not just inline blocks.
  - Phase 5 grep expectation rewritten with explicit per-file remaining counts (18 total expected hits).

#### DeepSeek V4 Pro — CONCUR (2 NITs, both FIXED, partial overlap with Codex)

1. `[FIXED]` `org_dashboard.go` should be migrated, not deferred. Independently surfaced the same scope gap as Codex's BLOCKER. → **Response**: promoted IN-scope (now part of Phase 4).
2. `[FIXED]` Phase 5 grep count inaccurate. Same finding as Codex's BLOCKER. → **Response**: rewrote with explicit per-file enumeration.

DeepSeek round-1 also CONCURRED on direction: "The three buckets (Read/Teach/Admin) faithfully categorize all 14 call sites" + "Both behavior changes are documented, well-reasoned, and match class-side precedent. No separate RFC needed."

#### GLM 5.1 — CONCUR (2 NITs, both FIXED)

1. `[FIXED]` Phase 3 needs explicit signature-contract delta for the `requireOrgAdmin` deletion. Local helper returns `(*auth.Claims, bool)` and writes HTTP responses; new helper returns `(bool, error)` and leaves response writing to the caller. → **Response**: expanded Phase 3 with two-bullet migration delta (401 nil-claims handling, 403 error-message preservation) and a verify-step for the nil-claims guard at each call site.
2. `[FIXED]` §Risks should also flag that `UpdateOrg`/`AddMember`/`UpdateMember`/`RemoveMember` currently check `Role == "org_admin"` WITHOUT a status filter. The new helper tightens this — suspended org_admins won't pass mutation. → **Response**: added new §Risks row covering uneven-tightening; classified as low-severity security improvement. `org_parent_links.go`'s local helper already filters on active status, so this just unifies behavior across the org-admin sites.

GLM round-1 also confirmed direction: "Three levels match the three patterns. Phase ordering is clean rollback granularity. No regressions found in the line-numbered references."

#### Kimi K2.6 — CONCUR (4 NITs, 4 FIXED + 1 deferred)

1. `[FIXED]` Inter-phase drift risk: a new handler could land between Phase 1 and Phase 5. → **Response**: added §Risks row; Phase 5 grep runs immediately before PR open, not just after Phase 4.
2. `[DEFERRED]` Persistent enforcement (CI lint to prevent future inline `GetUserRolesInOrg` calls). → **Response**: added §Risks row noting follow-up plan possibility (mirror plan 074's `shadow-routes.test.ts` pattern). Out of scope for plan 075.
3. `[FIXED]` Add inline `// TODO(plan-NNN)` comments at the 15 out-of-scope sites. → **Response**: added Phase 5 step + §Files §"TODO-comment-only insertions" listing all 7 files with comment templates per bucket.
4. `[FIXED]` Merge phases 3 and 4 — both small mechanical replacements. → **Response**: merged. Phase 4 now covers `classes.go` + `courses.go` + `org_dashboard.go` + `topic_problems.go` in one commit.
5. `[FIXED]` `CreateLink` at `org_parent_links.go:148` consumes `claims` returned by the local helper (used at line 197 for `created_by`). After migration, caller must fetch claims separately. → **Response**: added explicit migration note in §Files Phase 3 entry detailing the claims-consumption pattern.

Kimi round-1 also CONCURRED on direction including the status-filter tightening verification: "Grepped Go handler tests and Vitest/e2e: no test creates a suspended member and then calls `GetOrg`/`ListMembers`/`ListClassesByOrg`/`ListCoursesByOrg` as that suspended user expecting success."

### Codex round-2 — 2 NITs (both FIXED)

1. `[FIXED]` File count typo: "7 files for TODO-comment-only insertion" but the buckets enumerate 8 (2 in Bucket 1 + 6 in Bucket 2). → **Response**: corrected to "8 files".
2. `[FIXED]` Phase 5 grep doesn't exclude test files; the broader grep would count test-suite usage. → **Response**: explicitly added `| grep -v "_test.go"` to the grep command and called out that test-file usage in `platform/internal/store/orgs_test.go` and `platform/internal/handlers/org_parent_links_test.go` is intentionally not part of the migration.

### Convergence

All 5 reviewers concur after Codex round-1 BLOCKER (audit gap, 14 → 16 in-scope) and Codex round-2 NITs (file count + test-file exclusion). Plan 075 ready for implementation.

## Code Review

5-way code review against branch `feat/075-require-org-authority` HEAD `5cb409f` (consolidated branch diff vs main; 5 phases).

### Self (Opus 4.7) — clean

`go build ./...` clean. Full Go test suite (15 packages) PASS. New `access_org_test.go` 19 tests PASS. Bypass-order regression caught + fixed mid-Phase-2 with 2 new dedicated tests so future refactors can't drift the order.

### Codex — CONCUR (3 cosmetic NITs, all FIXED)

All 6 review questions confirmed clean. Three cosmetic NITs:

1. `[FIXED]` `access.go:189` doc-comment said `(false, err)` returns `ErrAccessHelperMisconfigured` when `orgs == nil`, but the bypass at lines 213 returns first. Reconciled the godoc to clarify that nil-orgs misconfig only fires when the caller isn't bypassing via IsPlatformAdmin/ImpersonatedBy. Fixed at `6012fb9`.
2. `[FIXED]` `schedule.go:67` TODO labeled "class-or-org fallback" but the code is actually "org-authority-via-class" (fetches class for orgID, then checks org). Relabeled + cited call-site line. Fixed at `6012fb9`.
3. `[FIXED]` `problem_access.go:11` TODO named speculative function names `canViewProblemAtScope`/`canEditProblemAtScope` but the actual functions are `authorizedForScope` (line 34) / `canViewProblemRow` (line 144). Corrected. Fixed at `6012fb9`.

Codex round-1 also confirmed: bypass ordering correct (admin/impersonator before nil-orgs); orgs.go 403 messages verbatim; CreateLink claims threading correct; org_dashboard.go and topic_problems.go return shapes preserved; all 8 out-of-scope files carry TODO annotations.

### DeepSeek V4 Flash — CONCUR (1 edge-note, no action)

Confirmed bypass order (claims-nil → IsPlatformAdmin/Impersonator → nil-orgs misconfig → role scan with active filter), OrgTeach + OrgAdmin level rules with active guard, all 6 Phase-2 levels + messages preserved, Phase 3 CreateLink claims still in scope, Phase 4 return shapes preserved, Phase 5 TODO comments at all 8 out-of-scope files.

Edge note (no action): `isTopicEditor` previously did an early-return on `IsPlatformAdmin` BEFORE the topic/course DB lookups; post-migration the platform admin goes through the lookups before the helper bypass fires. → DeepSeek's own analysis: "behavior unchanged for valid routes; arguably more correct for invalid ones" — a platform admin trying to access a non-existent topic now correctly gets 404 instead of bypassing to grant. Acceptable.

### GLM 5.1 — CONCUR (0 BLOCKERS, 0 NITS)

Confirmed Phase 3 CreateLink claims-consumption (line 144 fetch, line 203 use), uniform Status==active filter at access.go:227-229, all 16 in-scope sites migrated (zero `GetUserRolesInOrg` hits in the 6 in-scope files), `(bool, error)` return shape consistent with class-side precedent, `requireOrgAdmin` fully deleted with no dead imports. `org_dashboard.go`'s `authorizeOrgAdmin` retains its orgID-resolution before delegating; `topic_problems.go`'s `isTopicEditor` cleanly maps `(bool, error)` → `(bool, int)`.

### Kimi K2.6 — CONCUR (1 NIT, FIXED — overlap with Codex)

Confirmed CreateLink claims-consumption (claims.UserID at line 203 unmodified); zero new inline GetUserRolesInOrg in migrated files; all 8 out-of-scope files annotated; bypass-order regression documented in helper godoc + Phase-2 regression tests at `access_org_test.go` lines 247-273; `TestRequireOrgAuthority_Admin_SuspendedOrgAdminDenies` confirms the suspended-org_admin case at OrgAdmin level; full test matrix complete (3 levels × roles + 4 bypass paths + nil-orgs + misconfig = 19 tests).

`[FIXED]` NIT (overlap with Codex round-1 NIT 2): `schedule.go` TODO didn't cite the exact call-site line. Now cites line ~73 with the relabeled "org-authority-via-class pattern" wording. Fixed at `6012fb9`.

### Convergence

All 5 reviewers concur. 3 plan-review BLOCKER fixes (Codex audit gap + count typos). 4 code-review NITs all cosmetic and folded. Multi-reviewer ensemble caught real issues: Codex caught the audit gap (org_dashboard + topic_problems should be in scope) AND the 3 cosmetic doc-comment / TODO inaccuracies; Kimi independently caught the schedule.go line-number gap AND the inter-phase drift risk; DeepSeek caught the next.config.ts importability blocker (in plan 074, validating in plan 075 the precedent test pattern works); GLM independently surfaced the same Phase-3 signature contract delta.

## Post-execution report

5-phase implementation shipped on a single branch. Final state at HEAD:

### Phase 1 — `7a607d5` + `ec052db`

Added `OrgAccessLevel` type + 3 constants + `RequireOrgAuthority` function in `platform/internal/handlers/access.go` (95 new lines). Added `platform/internal/handlers/access_org_test.go` (17 integration tests + 2 bypass-order regression tests added in Phase 2). All tests pass against `bridge_test`.

One bypass-order tweak needed during Phase 2: original implementation checked `orgs == nil` BEFORE the `IsPlatformAdmin || ImpersonatedBy` bypass, breaking `TestAddMember_MissingEmail`/`TestAddMember_InvalidRole` which intentionally pass `nil` Orgs with admin claims. Moved the bypass to fire first (commit `ec052db`); added 2 regression tests so the order can't drift.

### Phase 2 — `b99be14`

Migrated 6 inline blocks in `orgs.go` (`GetOrg`, `UpdateOrg`, `ListMembers`, `AddMember`, `UpdateMember`, `RemoveMember`). 140 → 58 lines (net -82). Original 403 messages preserved verbatim. Plan 069 phase 4 self-action guards on `UpdateMember`/`RemoveMember` left intact.

### Phase 3 — `3f5d676`

Migrated 4 call sites in `org_parent_links.go` (`ListEligibleChildren`, `ListByOrg`, `CreateLink`, `RevokeLink`) and deleted the local `requireOrgAdmin` helper (25 lines removed). `CreateLink`'s `claims.UserID` consumption (Kimi K2.6 NIT) preserved by fetching claims via `auth.GetClaims` at the top of each handler. 401/403 response writing now caller-side; original `"Only org admins can manage parent links"` message preserved verbatim.

### Phase 4 — `3508ecd`

Migrated 6 sites across 4 files (Sonnet subagent):
- `classes.go:74` (`CreateClass`, `OrgTeach`) and `classes.go:122` (`ListClassesByOrg`, `OrgRead`)
- `courses.go:72` (`CreateCourse`, `OrgTeach`) and `courses.go:122` (`ListCoursesByOrg`, `OrgRead`)
- `org_dashboard.go:69` (`authorizeOrgAdmin` helper body, `OrgAdmin`) — redundant `IsPlatformAdmin || ImpersonatedBy` short-circuit removed since helper handles it
- `topic_problems.go:69` (`isTopicEditor`, `OrgTeach`) — `(bool, int)` return shape preserved

### Phase 5 — TODO comments + post-execution report

Inserted `TODO(plan-075-followup)` comments at the 8 out-of-scope files (Bucket 1: `schedule.go` + `sessions.go`; Bucket 2: `unit_collections.go`, `unit_ai.go`, `realtime_token.go`, `topics.go`, `problem_access.go`, `teaching_units.go`). For files with multiple call sites (`sessions.go` 5 sites, `unit_collections.go` 2, `problem_access.go` 2, `teaching_units.go` 2), used a single header comment naming all sites rather than spamming inline TODOs.

### Final verification

- `go build ./...` clean.
- Full Go test suite: all 15 packages PASS (handlers 174s, store 30s, others sub-second).
- Vitest: not affected (no JS/TS changes).
- Final grep `grep -rn "GetUserRolesInOrg" platform/internal/handlers/ --include="*.go" | grep -v "_test.go"`:
  - 3 hits in `access.go` (lines 103/221/295 — internal to `RequireClassAuthority`/`RequireOrgAuthority`/`CanViewUnit`).
  - 1 hit in `schedule.go` + 5 in `sessions.go` (Bucket 1, all marked with TODO comments).
  - 2 in `unit_collections.go` + 1 in `unit_ai.go` + 1 in `realtime_token.go` + 1 in `topics.go` + 2 in `problem_access.go` + 2 in `teaching_units.go` (Bucket 2, all marked with TODO comments).
  - The 16 in-scope handler call sites are gone.

### Behavior changes shipped (deliberate, documented)

1. **Impersonator-of-admin bypass** extended to org-side checks — was class-side only. Plan 039's carve-out: admins inspecting an org while impersonating a student retain their access. Asymmetric class vs org behavior was a bug; this plan fixes it.
2. **Status == "active"** filter applied uniformly. Suspended members no longer pass `OrgRead`; suspended teachers no longer pass `OrgTeach`; suspended org_admins no longer mutate via `UpdateOrg`/`AddMember`/`UpdateMember`/`RemoveMember`. The 4 `orgs.go` admin sites previously checked role-only without status; this is a security improvement.

### No deviations from final plan

All 16 in-scope sites migrated. All 8 out-of-scope files received TODO comments. Phase 1 had one bypass-order fix mid-Phase-2 (folded with 2 new regression tests). Phase 3+4 merged per Kimi NIT. The pre-existing `unit-org-A` slug-collision flake in `teaching_units_integration_test.go` was unrelated to this plan and required a one-time DB cleanup of leftover rows before tests could pass; the underlying bug (non-uniqueness-safe slug suffix) is on main and out of scope here.

### Follow-ups

- **Plan 076 (or beyond): `RequireClassOrOrgAccess`** — covers Bucket 1 (`schedule.go`, `sessions.go` × 5). The composite "class-membership-then-org_admin-fallback" pattern can't be expressed with the current `RequireOrgAuthority` shape.
- **Plan 077 (or beyond): scope-discriminating helper migration** — covers Bucket 2 (`unit_collections.go`, `unit_ai.go`, `realtime_token.go`, `topics.go`, `problem_access.go`, `teaching_units.go`). Each helper has a `switch scope` over platform/org/personal; migrating the org branch to `RequireOrgAuthority` is conceptually correct but touches helper signatures (which are currently `bool`, `(bool, error)`, `(bool, int)`, `(string, *authDecision)`, etc.). Per-helper migration plan needed.
- **Persistent enforcement** (Kimi NIT, deferred): a future plan could add a Vitest/Go-vet style guard that asserts no NEW handler-level `GetUserRolesInOrg` calls outside `access.go`. Mirror plan 074's `shadow-routes.test.ts` pattern. TODO entry added to `TODO.md` as part of this Phase 5.

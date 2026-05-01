# Plan 060 — Remove `UpdateOrgStatus` auto-verify on school activation (P1)

## Status

- **Date:** 2026-05-01
- **Severity:** P1 (silently bypasses verification semantics)
- **Origin:** `docs/reviews/009-deep-codebase-review-2026-04-30.md` §P1-4.

## Problem

`platform/internal/store/orgs.go:250-254`:

```go
if status == "active" && orgType == "school" {
    set verified_at = now()
}
```

When an org is updated to `status='active'` AND its `type` is `school`, the store unconditionally sets `verified_at = now()` — regardless of whether any actual verification occurred. The `verified_at` timestamp is meant to signal "an admin reviewed the school's signup paperwork and approved them" but the code stamps it on any active-school transition.

Concrete failure modes:
- A platform admin who flips a school from `pending` → `active` → `pending` → `active` on accident gets a fresh `verified_at` each time.
- A bulk migration script that touches every school's `status` re-stamps `verified_at` for every school, erasing the original verification dates.
- The `verified_at` field can't be used for "schools that have actually been verified" filtering because every active school has it.

The plan-049 Bridge HQ org seed (`scripts/seed_bridge_hq.sql`) explicitly sets `verified_at = now()` on insert. That's intentional — the system org is its own publisher. The bug is in the UPDATE path, not the INSERT path.

## Out of scope

- Designing a new verification workflow with a dedicated UI/API for "admin reviews paperwork." That's product work.
- Auditing whether existing `verified_at` values across the live DB are accurate — too speculative without a clear remediation.
- Touching the `INSERT` path on `organizations` — only the UPDATE path has the bug.

## Approach

Remove the auto-verify branch entirely. The store's `UpdateOrgStatus` method should ONLY change `status`, never `verified_at`. If/when a real verification flow exists, it gets its own store method (`MarkOrgVerified(ctx, orgID, byUserID)`) that sets `verified_at = now()` AND records who verified it (a separate `verified_by` column would be a follow-up).

For the existing dev DB: the legacy auto-stamped `verified_at` values stay in place. They might be wrong, but they're not actively dangerous — `verified_at` isn't gating any behavior today.

## Files

- Modify: `platform/internal/store/orgs.go:250-254` — delete the auto-stamp branch.
- Modify: `platform/internal/store/orgs_test.go` — add a test asserting that toggling `status` does NOT change `verified_at` on subsequent updates. Existing happy-path tests that rely on the auto-stamp behavior need updating.
- Verify: `tests/integration/admin-orgs-api.test.ts` still passes — the page that calls this endpoint should be unaffected (the UI renders `verified_at` if present, doesn't depend on auto-stamping).

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Existing test relies on the auto-stamp | medium | Update the test. The expected behavior changes; the test's expectation should change with it. |
| Some flow somewhere queries `WHERE verified_at IS NOT NULL AND type = 'school'` and now misses freshly-activated schools | low | Search: `grep -rn "verified_at" platform/ src/`. None expected; this is a verification-flow follow-up. |
| The "real" verification flow never gets built | low | Document the gap in `TODO.md` so it's tracked. |

## Phases

### Phase 0: pre-impl Codex review

Dispatch on this plan + the orgs.go file. Capture verdict.

### Phase 1: implement + test update

Single commit.

### Phase 2: post-impl Codex review

## Codex Review of This Plan

(Filled in after Phase 0.)

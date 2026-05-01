# Plan 055 — Delete shadow Next API routes for class members (P1)

## Status

- **Date:** 2026-05-01
- **Severity:** P1 (deferred from plan 021; same auth gap as plan 052)
- **Origin:** Review `008-...:33-39`.

## Problem

Plan 021's stated goal was to remove all non-auth Next API routes once Go owned the equivalent endpoints. Plan 021's post-execution report deferred backend deletion of several shadow routes. Two of those — both for class member management — still exist:

- `src/app/api/classes/[id]/members/route.ts` (POST/GET) — checks only `await auth()` for a valid session.
- `src/app/api/classes/[id]/members/[memberId]/route.ts` (PATCH/DELETE) — same.

These have the same auth hole as the Go handlers plan 052 fixes. They're reachable via Next's rewrites OR via direct fetch; tests can import them. They're a maintenance hazard.

## Out of scope

- Other shadow routes (audit separately; this plan is just class members).
- The Go-side fix — plan 052 covers that.

## Approach

Delete both files. Verify nothing in the codebase imports them. Verify the Next rewrites in `next.config.ts` proxy `/api/classes/*/members` to Go.

## Files

- Delete: `src/app/api/classes/[id]/members/route.ts`
- Delete: `src/app/api/classes/[id]/members/[memberId]/route.ts`
- Verify: `next.config.ts` rewrites cover the deleted paths so `/api/classes/<id>/members` continues to work in the browser via the Go proxy.
- Verify: no test imports these routes directly. `grep -rn "from.*api/classes.*members" tests/`.
- Update: any test that mocked the deleted Next handlers (rewire to the Go-proxied endpoint via `tests/api-helpers.ts`).

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| A test still mocks the deleted route | low | Grep + update. The Go integration tests cover the real handlers. |
| The Go proxy doesn't actually cover the path | medium | Manually verify by hitting `/api/classes/<id>/members` in dev and watching `platform/`'s logs for the request. |
| Browser flow breaks for a teacher's roster page | medium | Browser smoke before merge. Plan 052 ships the Go-side authority gates first; this plan lands AFTER plan 052 so the Go endpoints are correct when shadow routes go away. |

## Ordering

**Land plan 052 first.** This plan deletes the Next routes; if the Go routes still have the auth gap (plan 052 unfixed), deleting the shadow routes doesn't help — both paths leak. After 052, this plan is purely cleanup.

## Phases

### Phase 0: pre-impl Codex review

### Phase 1: delete + verify proxy + tests

Single-commit change.

## Codex Review of This Plan

(Filled in after Phase 0.)

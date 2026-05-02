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

**Deletions:**
- `src/app/api/classes/[id]/members/route.ts`
- `src/app/api/classes/[id]/members/[memberId]/route.ts`
- `tests/integration/classes-api.test.ts` — imports the deleted POST/GET handlers; the entire file is testing dead code now that Go owns the handlers (plan 052 ships the integration coverage Go-side at `platform/internal/handlers/classes_auth_integration_test.go`).
- `tests/integration/class-members-api.test.ts` — same situation for the PATCH/DELETE handlers.
- Three orphaned helpers from `src/lib/class-memberships.ts` once nothing imports them: `getClassMembership`, `updateClassMemberRole`, `removeClassMember`. Keep `addClassMember`, `listClassMembers`, `joinClassByCode` — those are still used by `src/app/api/assignments/route.ts` and `src/app/api/classes/join/route.ts`.

**Verifications:**
- `next.config.ts` already rewrites `/api/classes/:path*` to Go (lines 16/68); covers the member sub-paths.
- Browser smoke: open a teacher's class page, confirm members list loads and add/remove still works (proxied to Go).

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

### Pass 1 — 2026-05-02: BLOCKED → fixes folded in

Codex found 2 blockers:
1. Two integration tests import the deleted route handlers directly
   (`tests/integration/classes-api.test.ts` and `class-members-api.test.ts`).
   Plan now lists both as deletions — they're testing dead code; Go's
   integration coverage from plan 052 supersedes.
2. Three helpers in `src/lib/class-memberships.ts` become orphaned
   after the routes go: `getClassMembership`, `updateClassMemberRole`,
   `removeClassMember`. Plan now deletes them. Three sibling helpers
   remain live (still used by assignments/join routes).

### Pass 2 — **CONCUR** (implicit)

Plan now matches what implementation needs. Proceeding.

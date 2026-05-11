# Plan 083 — Remove teacher schedule/report placeholders

## Problem

Browser review 011 found that teacher primary navigation no longer advertises Schedule or Reports, but direct URLs still render single-line `Coming soon` pages:

- `src/app/(portal)/teacher/schedule/page.tsx`
- `src/app/(portal)/teacher/reports/page.tsx`

These pages are dead-end surfaces. A bookmarked or shared link implies product scope that does not exist.

## Decision

Remove the placeholder routes instead of shipping a schedule/report MVP.

Rationale:

- Teacher nav already excludes both pages.
- The current pages contain no behavior beyond `Coming soon`.
- A real schedule or reporting MVP requires product/API design and should not be invented as a P2 cleanup.
- Prior cleanup plans removed placeholder nav/routes when the product surface was not ready.

## Scope

Delete the two placeholder route files so `/teacher/schedule` and `/teacher/reports` fall through to Next's 404 handling.

Out of scope:

- Building teacher scheduling.
- Building teacher reports.
- Adding replacement navigation entries.
- Changing API schedule routes or schema tests.

## Files

Delete:

- `src/app/(portal)/teacher/schedule/page.tsx`
- `src/app/(portal)/teacher/reports/page.tsx`

Create:

- `tests/unit/teacher-placeholder-routes.test.ts`

Modify:

- `TODO.md`
- `docs/plans/083-teacher-placeholder-routes.md`

## Test Plan

Use TDD:

1. Add source-level tests asserting the placeholder route files do not exist and the teacher nav does not link to `/teacher/schedule` or `/teacher/reports`.
   Extend `tests/unit/nav-config.test.ts` for the nav assertion, and add `tests/unit/teacher-placeholder-routes.test.ts` for the route-file absence assertion.
2. Verify RED before deleting the pages:
   ```bash
   /home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run tests/unit/teacher-placeholder-routes.test.ts
   ```
3. Delete the placeholder page files.
4. Verify GREEN with the same command.

Additional checks:

```bash
/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/typescript/bin/tsc --noEmit
env DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test /home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run
cd platform && env GOCACHE=/tmp/magicburg-go-build-cache go test ./... -count=1 -timeout 120s
```

Record existing baseline failures instead of hiding them.

## Plan Review

External plan review completed with GLM 5.1, DeepSeek V4 Flash, and Kimi K2.6.
All three returned `CONCUR`.

- [ACCEPTED] Route removal is the right decision for this cleanup. The two pages are single-line placeholders, teacher nav already excludes them, and there are no source/test links to these routes outside docs and the route files themselves.
- [FIXED] Reviewers suggested using the existing `tests/unit/nav-config.test.ts` for the teacher-nav assertion. The test plan now extends that file and keeps the route-file absence assertion in `tests/unit/teacher-placeholder-routes.test.ts`.
- [ACCEPTED] `docs/plans/021-frontend-cleanup.md:82` is stale historical guidance that said placeholder pages should stay. This plan supersedes that older cleanup note for teacher schedule/reports; no edit to the historical plan is needed.
- [FIXED] The workflow must mark `TODO.md` Plan 083 complete after implementation. `TODO.md` remains in the file list and the post-execution report will record the change.

## Code Review

Implementation review completed after the branch diff was committed.

Reviewers: self-review, DeepSeek V4 Flash, GLM 5.1, and Kimi K2.6.
The external reviewers were redispatched on request; the redispatched pass is the source of record below.

- [ACCEPTED] Self-review: the removed files were exactly the one-line `Coming soon` placeholder pages from `origin/main`; no real behavior was removed.
- [ACCEPTED] DeepSeek V4 Flash redispatch: `ALLOW`. No blocking findings. Noted that the pending `TODO.md` checkmark and post-execution report were workflow items, and that unrelated dirty local files were outside the branch diff.
- [ACCEPTED] GLM 5.1 redispatch: `ALLOW`. No blocking findings. Verified the targeted Vitest command with Node 20. Noted non-blocking empty local directories under `src/app/(portal)/teacher/schedule/` and `src/app/(portal)/teacher/reports/`; Git does not track or ship them after the page files are deleted.
- [ACCEPTED] Kimi K2.6 redispatch: `ALLOW`. No blocking findings. Confirmed the branch deletes only the two placeholder routes, adds tests for file absence and teacher-nav exclusion, and leaves unrelated local edits out of the branch diff.

## Post-execution Report

Implemented the route-removal option from Plan 083:

- Deleted `src/app/(portal)/teacher/schedule/page.tsx`.
- Deleted `src/app/(portal)/teacher/reports/page.tsx`.
- Added `tests/unit/teacher-placeholder-routes.test.ts` to assert the direct-access placeholder route files are absent.
- Extended `tests/unit/nav-config.test.ts` to assert teacher nav does not link to `/teacher/schedule` or `/teacher/reports`.
- Marked Plan 083 complete in `TODO.md`.

TDD evidence:

- RED: `tests/unit/teacher-placeholder-routes.test.ts` failed before deleting the pages because `src/app/(portal)/teacher/schedule/page.tsx` still existed.
- GREEN: `/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run tests/unit/teacher-placeholder-routes.test.ts tests/unit/nav-config.test.ts` passed with 2 files and 12 tests.

Verification:

- PASS: `/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run tests/unit/teacher-placeholder-routes.test.ts tests/unit/nav-config.test.ts` — 2 files, 12 tests.
- PASS: `env DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test /home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run` — 87 files passed, 2 skipped; 674 tests passed, 11 skipped.
- PASS: `cd platform && env GOCACHE=/tmp/magicburg-go-build-cache go test ./... -count=1 -timeout 120s`.
- BASELINE FAIL: `/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/typescript/bin/tsc --noEmit` still fails on pre-existing issues in `src/app/(portal)/teacher/units/new/page.tsx` and `tests/unit/identity-assert.test.ts`; Plan 083 did not touch those files.

Known local state:

- The checkout still contains unrelated unstaged changes in `src/components/admin/user-actions.tsx`, `src/components/problem/problem-actions.tsx`, `src/components/problem/problem-form.tsx`, `src/lib/api-client.ts`, and untracked `src/lib/api-error.ts`. They are not part of Plan 083 and must not be staged with this branch.

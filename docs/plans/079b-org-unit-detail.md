# Plan 079b — Org-admin read-only unit detail page

## Problem

Plan 079 fixed the platform-admin version of the broken unit link by adding `/admin/units/[id]`.
The same bug class remains in the org-admin portal: `src/app/(portal)/org/units/page.tsx` links org-scope unit titles to `/teacher/units/{id}/edit`.
An org admin who does not also have the teacher role is rejected by the teacher portal layout, so the org unit table advertises an action that can bounce the user away from the org portal.

## Scope

Add a read-only `/org/units/[id]` detail page for org admins and retarget org unit links to that route.
The page mirrors Plan 079's metadata-only admin detail page and uses the existing `GET /api/units/{id}` endpoint.

Out of scope:

- Editing, publishing, archiving, or forking units.
- Adding a content preview. The unit document is Yjs state, not human-readable text.
- Adding a new Go endpoint. `GET /api/units/{id}` already applies `canViewUnit`.
- Changing teacher unit edit authorization.

## Design

### Backend

No backend changes.
`GET /api/units/{id}` returns a `store.TeachingUnit` shape and already authorizes visible units through `canViewUnit`.
For org admins, that means org-scope units in their org plus published platform-scope units that are already visible in `/org/units`.
Missing or unauthorized units both return 404.

### Frontend

Create `src/app/(portal)/org/units/[id]/page.tsx`.

The page:

- validates the route `id` with the same UUID-shaped regex used by Plan 079
- fetches `api<UnitDetail>(/api/units/{id})`
- renders title, slug, summary, status, scope, scope ID, material type, grade level, creator UUID, and creation time
- has a back link to `/org/units`
- renders "Unit not found" for invalid UUIDs and API 404
- renders a retryable "Couldn't load unit" panel for 5xx and unexpected errors
- does not render a 403-specific branch because the Go handler intentionally returns 404 for unauthorized unit rows

Update `src/app/(portal)/org/units/page.tsx` so unit titles link to `/org/units/{id}` for both org-scope and platform-scope units.
The existing platform-scope plain-text behavior was meant to avoid linking to the teacher editor; a read-only org detail route makes both scopes inspectable without changing edit permissions.

## Files

Create:

- `src/app/(portal)/org/units/[id]/page.tsx`
- `tests/unit/org-unit-detail.test.tsx`

Modify:

- `src/app/(portal)/org/units/page.tsx`
- `TODO.md`
- `docs/plans/079b-org-unit-detail.md`

## Test Plan

Use TDD:

1. Add `tests/unit/org-unit-detail.test.tsx` before production code.
2. Verify RED with:
   ```bash
   /home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run tests/unit/org-unit-detail.test.tsx
   ```
3. Implement the page and list-link update.
4. Verify GREEN with the same command.

Additional checks:

```bash
/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run tests/unit/org-unit-detail.test.tsx tests/unit/admin-unit-detail.test.tsx
/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/typescript/bin/tsc --noEmit
cd platform && env GOCACHE=/tmp/magicburg-go-build-cache go test ./... -count=1 -timeout 120s
```

Record existing baseline failures instead of hiding them.

## Code Review

External review completed with GLM 5.1, DeepSeek V4 Flash, and Kimi K2.6 against branch `feature/plan-079b-org-unit-detail`.
All three returned `ALLOW`.

Process note: this follow-up used the already-reviewed Plan 079 implementation pattern as the design template and implemented directly after committing this plan file.
The current review covered both the short 079b plan and the code diff.

- [FIXED] `TODO.md` was listed in the plan but not updated. The browser-review queue now marks Plan 079, Plan 079b, and Plan 080 complete; 079 and 080 were already merged before this branch.
- [FIXED] `## Code Review` and `## Post-execution Report` were pending. This section records the review disposition and execution evidence.
- [FIXED] The org detail test originally missed the unexpected non-`ApiError` branch. Added a test that simulates a low-level network error and asserts the page renders `request failed` without leaking `localhost:8002`.
- [FIXED] The org detail page originally rendered unexpected `Error.message` values. The page now renders a generic `request failed` for non-`ApiError` failures.
- [FIXED] Minor sibling UI drift from Plan 079. The org detail page now uses the same em dash fallback style and a left-arrow back link.
- [FIXED] Scoped eslint found a pre-existing `<a href="/">` in the touched org units page. Converted it to the already-imported Next `Link`.
- [ACCEPTED] The branch adds no new backend endpoint. `GET /api/units/{id}` is intentionally reused because it already applies `canViewUnit` and returns 404 for missing or unauthorized rows.
- [ACCEPTED] No content preview is rendered. Unit document contents are Yjs state and need a separate projection/preview design.

## Post-execution Report

Implemented:

- Added `src/app/(portal)/org/units/[id]/page.tsx`, a read-only metadata detail page for org admins.
- Retargeted `/org/units` unit title links from `/teacher/units/{id}/edit` to `/org/units/{id}` for both org-scope and platform-scope rows.
- Added `tests/unit/org-unit-detail.test.tsx` with coverage for invalid UUID, 404, 5xx, unexpected fetch failure sanitization, happy-path metadata rendering, and the list-link retarget.
- Marked Plan 079b complete in `TODO.md`; also marked stale Plan 079 and Plan 080 TODO rows complete because both are already merged on `main`.

Verification:

- RED: `/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run tests/unit/org-unit-detail.test.tsx` failed before implementation because `@/app/(portal)/org/units/[id]/page` did not exist.
- PASS: `/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run tests/unit/org-unit-detail.test.tsx` — 5 tests before review fixes, then 6 after adding unexpected-error sanitization coverage.
- PASS: `/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run tests/unit/org-unit-detail.test.tsx tests/unit/admin-unit-detail.test.tsx` — 11 tests passed after review fixes.
- PASS: `npx eslint --max-warnings 0 'src/app/(portal)/org/units/[id]/page.tsx' 'src/app/(portal)/org/units/page.tsx' 'tests/unit/org-unit-detail.test.tsx'`.
- PASS: `cd platform && env GOCACHE=/tmp/magicburg-go-build-cache go test ./... -count=1 -timeout 120s`.
- PASS: `env DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test /home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run` with escalated local database access — 86 files and 672 tests passed; 2 files and 11 tests skipped.
- BASELINE FAIL: `/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/typescript/bin/tsc --noEmit` still fails on existing unrelated errors in `src/app/(portal)/teacher/units/new/page.tsx`, `src/components/admin/user-actions.tsx`, and `tests/unit/identity-assert.test.ts`.

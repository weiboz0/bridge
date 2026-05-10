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

Pending implementation.

## Post-execution Report

Pending implementation.

# Plan 059 — User store correctness: `intended_role` + transactional `RegisterUser` (P0)

## Status

- **Date:** 2026-05-01
- **Severity:** P0 (one bug strands data; the other corrupts state on partial failure)
- **Origin:** `docs/reviews/009-deep-codebase-review-2026-04-30.md` §P0-3 + §P0-4.

## Problem

Two unrelated bugs in `platform/internal/store/users.go` that share a single small file:

### 1. Go `User` struct does not select or expose `intended_role` (§P0-3)

Migration `drizzle/0021_user_intended_role.sql:12-13` adds the `intended_role` column, and the Next.js onboarding page (`src/app/onboarding/page.tsx:24-27`) reads `intendedRole` directly from the DB to route the user. But the Go `User` struct at `users.go:13-21` has no `IntendedRole` field, and all three SELECT queries (`GetUserByID`, `GetUserByEmail`, `ListUsers`) at lines 37-38, 52-56 omit the column from their column lists.

**Failure mode:** any Go-side identity endpoint that returns user data is silently dropping the `intended_role` value. If a flow ever needs to act on it server-side (admin moderation, signup-intent reporting, role auto-assignment), it can't.

### 2. `RegisterUser` is not transactional → partial failure orphans the user (§P0-4)

`users.go:111-143` runs two INSERTs in separate implicit transactions:
- Line 121: `INSERT INTO users (...) RETURNING id`
- Line 133: `INSERT INTO auth_providers (...)`

If the first succeeds but the second fails (unique violation, constraint error, DB hiccup), a `users` row is committed with no login method. The user can never log in via OAuth — the auth_provider row that would have linked their Google ID to their user row never exists. They also cannot register again because the email is taken. The orphan row sits in the DB until manual cleanup.

## Out of scope

- Soft-delete or merge-orphans tooling — separate concern.
- Redesigning the auth-provider model (e.g., supporting multiple providers per user) — large; not this plan.
- The `signup_intent` ENUM itself — declared in `drizzle/0021` and the matching pgEnum exists in `src/lib/db/schema.ts` already. We just need Go-side parity.

## Approach

### Fix 1: surface `intended_role`

- Add `IntendedRole *string \`json:"intendedRole"\`` to the `User` struct.
- Update the three SELECT column lists (search for `userColumns` constant or expand inline lists) to include `intended_role`.
- Update each `Scan` target to receive `&user.IntendedRole`.
- Field is nullable: many existing rows pre-date the column.

### Fix 2: transactional `RegisterUser`

- Wrap both INSERTs in `db.BeginTx(ctx, nil)`.
- On any error: `tx.Rollback()` (defer + nilable error pattern from existing stores).
- On success: `tx.Commit()`.
- The store method's signature stays the same; only the body changes.

## Files

- Modify: `platform/internal/store/users.go` — both fixes inline.
- Modify: `platform/internal/store/users_test.go` — add tests:
  - `intended_role` round-trips through `GetUserByID` / `GetUserByEmail` / `ListUsers`.
  - `RegisterUser` fails atomically: simulate `auth_providers` INSERT failure (e.g., pre-insert a colliding row) → assert NO user row was committed.
- Verify: existing onboarding flow tests still pass.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Adding a column to SELECT changes serialization JSON shape | low | The new field is `intendedRole` (camelCase per project convention); consumers that don't use it ignore it. |
| Wrapping in a tx changes timing characteristics | low | The existing INSERTs are sub-millisecond; wrapping in a tx adds one BEGIN/COMMIT roundtrip. Negligible. |
| Existing test fixtures expect orphan-user state | very low | grep for any test that pre-creates a `users` row WITHOUT a corresponding `auth_providers` row. None expected; verify. |

## Phases

### Phase 0: pre-impl Codex review

Dispatch on this plan + the file `users.go` to confirm the column lists and field positions. Capture verdict.

### Phase 1: implement + tests

Two commits, one per fix; both on the same branch.

### Phase 2: post-impl Codex review

Dispatch on diff. Resolve findings.

## Codex Review of This Plan

(Filled in after Phase 0.)

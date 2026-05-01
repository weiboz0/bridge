# Plan 050 — Refresh JWT claims on every NextAuth `jwt()` call

## Status

- **Date:** 2026-05-01
- **Origin:** Codex post-impl review of PR #79 (`fix/browser-theme-and-admin-403`) — `[IMPORTANT]` follow-up.
- **Scope:** Backend auth only. No UI work.

## Problem

`src/lib/auth.ts:181-193`'s NextAuth `jwt({ token, user })` callback only refreshes `token.isPlatformAdmin` from the DB **on first sign-in** (when `user` is truthy). On every subsequent request, NextAuth re-uses the cached token without consulting the DB.

Two consequences:

1. **Stale grants.** A user who was promoted to platform admin in the DB after sign-in has `token.isPlatformAdmin === false` until they sign out and back in. They pass `PortalShell` (which reads from the same JWT) but might still 403 on individual admin endpoints, OR worse, the inverse — an admin who was demoted in the DB still has `token.isPlatformAdmin === true` until token expiry. Both ship.
2. **Defense-in-depth gap.** PR #79 added graceful 403 cards on `/admin/orgs` and `/admin/users` to mask the symptom of the stale grant. The cards remain useful, but the root cause is the JWT staleness — not the page-level error handling.

The Go side reads the same JWT field at `platform/internal/auth/jwt.go:156-158` and `platform/internal/auth/middleware.go:191-205`. Whatever the TS jwt callback writes, the Go middleware trusts.

## Out of scope

- The `id` field on the token. It's set once on first login and never legitimately changes; refreshing it would be redundant.
- Email/name updates. Those flow through profile-edit handlers; not part of the JWT claims surface for authorization.
- Any change to the Go-side claim parsing or middleware. That layer is correct as-is — it just trusts what the TS side writes.
- Soft-deleted users. Bridge has no soft-delete today; if/when it does, this plan revisits.

## Approach

Always do the DB lookup on every `jwt()` call. The query is keyed on `email` (unique index `users_email_idx`); cost is sub-millisecond. NextAuth invokes `jwt()` once per authenticated request, so this adds one DB roundtrip per request that the page renderer would do anyway.

Three specific mutations to the callback:

1. Always read `dbUser` from the DB via `email` (or `id` if the token already has one).
2. Apply `token.isPlatformAdmin = dbUser.isPlatformAdmin`.
3. If `dbUser` is missing (deleted account, email change races), null the elevated claims and return the token with `id` cleared. NextAuth will treat the next request as unauthenticated.

Pseudocode:

```ts
async jwt({ token, user }) {
  // First-login or every-call: always reconcile from DB.
  const lookupEmail = token.email;
  if (!lookupEmail) return token;
  const [dbUser] = await db.select({
    id: users.id,
    isPlatformAdmin: users.isPlatformAdmin,
  }).from(users).where(eq(users.email, lookupEmail));
  if (!dbUser) {
    // User deleted between sign-in and this request. Invalidate.
    return { ...token, id: undefined, isPlatformAdmin: false };
  }
  token.id = dbUser.id;
  token.isPlatformAdmin = dbUser.isPlatformAdmin;
  return token;
}
```

The `if (user)` branch (first-login signup-link flow at `src/lib/auth.ts:182-191`) collapses into the unconditional path.

## Files

- Modify: `src/lib/auth.ts` — JWT callback rewritten as above.
- Modify: `tests/integration/auth-jwt-refresh.test.ts` (new) — integration test covering:
  - Promotion: user signs in → not admin → DB sets `is_platform_admin=true` → next protected request shows admin role.
  - Demotion: user signs in as admin → DB sets `is_platform_admin=false` → next protected request rejected.
  - Account deletion: user's `users` row deleted → next request behaves as unauthenticated.
- Verify: existing `tests/integration/admin-orgs-api.test.ts` and `admin-users-api.test.ts` still pass — the page-level 403 card from PR #79 remains correct as defense in depth.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| DB load: every authenticated request now hits `users` | low | indexed lookup; sub-millisecond. If load grows, add a 60-second in-process cache keyed on `email`. |
| Token rewrite races | low | NextAuth's default JWT strategy is stateless; we write to the token in-place. Concurrent renders see consistent claims because each one re-reads. |
| `email` is mutable | medium | Bridge has no email-change flow today. If/when added, key the lookup on `id` (and stash a stable `id` once on first sign-in). Documented in the code as a Future Work note. |
| Tests assume cached claims | low | The 526-test baseline doesn't exercise admin-grant-after-login. New tests in this plan cover the new behavior; existing tests don't depend on caching. |

## Phases

### Phase 0: pre-impl Codex review

Per CLAUDE.md plan review gate: dispatch `codex:codex-rescue` to review THIS plan against `src/lib/auth.ts`, the existing admin tests, and the `users` schema. Resolve any blockers and re-dispatch until concur. Capture the verdict in this file's `## Codex Review of This Plan` section. Only then move to Phase 1.

### Phase 1: rewrite the callback + tests

- Implement the callback change.
- Add `tests/integration/auth-jwt-refresh.test.ts`. Cover all three scenarios above.
- Run `bun run test` end-to-end. Run `cd platform && go test ./...` (no Go-side changes; smoke).
- Self-review the diff.
- Commit.

### Phase 2: verify in dev DB + post-impl review

- Manually exercise: log in as a non-admin, promote in the DB via `UPDATE users SET is_platform_admin = true WHERE email = '<...>'`, navigate to `/admin/orgs` without re-authenticating. Confirm access works.
- Reverse: demote, navigate again, confirm 403.
- Update `TODO.md` with anything that surfaced.
- Open PR.

## Codex Review of This Plan

(Filled in after the Phase 0 review pass.)

# Plan 050 — Auth correctness: JWT refresh + DEV_SKIP_AUTH production guard

## Status

- **Date:** 2026-05-01 (re-scoped to absorb DEV_SKIP_AUTH guard)
- **Origin:** Codex post-impl review of PR #79 + `docs/reviews/009-deep-codebase-review-2026-04-30.md` §P1-2.
- **Scope:** Backend auth only. No UI work.

This plan combines two narrow auth-correctness fixes that share the same surface (NextAuth JWT + Go middleware) but are independent: refresh the JWT's `isPlatformAdmin` claim from the DB on every `jwt()` call, AND prevent `DEV_SKIP_AUTH` from accidentally activating in production.

## Problem

`src/lib/auth.ts:181-193`'s NextAuth `jwt({ token, user })` callback only refreshes `token.isPlatformAdmin` from the DB **on first sign-in** (when `user` is truthy). On every subsequent request, NextAuth re-uses the cached token without consulting the DB.

Two consequences:

1. **Stale grants.** A user who was promoted to platform admin in the DB after sign-in has `token.isPlatformAdmin === false` until they sign out and back in. They pass `PortalShell` (which reads from the same JWT) but might still 403 on individual admin endpoints, OR worse, the inverse — an admin who was demoted in the DB still has `token.isPlatformAdmin === true` until token expiry. Both ship.
2. **Defense-in-depth gap.** PR #79 added graceful 403 cards on `/admin/orgs` and `/admin/users` to mask the symptom of the stale grant. The cards remain useful, but the root cause is the JWT staleness — not the page-level error handling.

The Go side reads the same JWT field at `platform/internal/auth/jwt.go:156-158` and `platform/internal/auth/middleware.go:191-205`. Whatever the TS jwt callback writes, the Go middleware trusts.

## Problem 2: `DEV_SKIP_AUTH` has no production guard

`platform/internal/auth/middleware.go:89-104` honors a `DEV_SKIP_AUTH` env var: any non-empty value bypasses authentication entirely and injects a synthetic `Dev User` claims struct. `DEV_SKIP_AUTH=admin` grants full platform-admin access. There is no `APP_ENV == "production"` check — if the variable accidentally leaks into a staging or production deployment (operator error, secrets-manager mistake, container env misconfiguration), every request is treated as a fully-privileged dev user.

Per review 009-2026-04-30 §P1-2 the recommendation is: at server startup, panic (or refuse to start) when `DEV_SKIP_AUTH != "" && APP_ENV == "production"`. Also explicitly zero `ImpersonatedBy` on the synthetic claims to avoid the impersonator-bypass carve-out in `RequireAdmin`.

## Out of scope

- The `id` field on the token. It's set once on first login and never legitimately changes; refreshing it would be redundant.
- Email/name updates. Those flow through profile-edit handlers; not part of the JWT claims surface for authorization.
- Any change to the Go-side claim parsing or middleware beyond the startup guard. That layer is correct as-is — it just trusts what the TS side writes.
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

**JWT refresh path:**
- Modify: `src/lib/auth.ts` — JWT callback rewritten as above.
- Add: `tests/integration/auth-jwt-refresh.test.ts` — integration test covering:
  - Promotion: user signs in → not admin → DB sets `is_platform_admin=true` → next protected request shows admin role.
  - Demotion: user signs in as admin → DB sets `is_platform_admin=false` → next protected request rejected.
  - Account deletion: user's `users` row deleted → next request behaves as unauthenticated.
- Verify: existing `tests/integration/admin-orgs-api.test.ts` and `admin-users-api.test.ts` still pass — the page-level 403 card from PR #79 remains correct as defense in depth.

**DEV_SKIP_AUTH guard path:**
- Modify: `platform/cmd/api/main.go` — at startup, before wiring any handler, check `os.Getenv("DEV_SKIP_AUTH") != ""` AND `os.Getenv("APP_ENV") == "production"`. If both, `slog.Error(...)` and `os.Exit(1)` (refuse to start). Document the guard inline.
- Modify: `platform/internal/auth/middleware.go:89-104` — explicitly zero `ImpersonatedBy` on the synthetic dev claims so that `RequireAdmin`'s impersonator-bypass carve-out doesn't accidentally accept dev claims as admin.
- Add: `platform/cmd/api/main_test.go` (or a smaller `auth/dev_skip_test.go`) — table-driven test that the startup guard returns/exits when both env vars are set.

**APP_ENV convention:** if Bridge doesn't currently set `APP_ENV`, the guard treats absence-of-`APP_ENV` as "not production" (safe default for dev). Document the convention in `docs/setup.md` so deploys know to set `APP_ENV=production` in prod env.

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

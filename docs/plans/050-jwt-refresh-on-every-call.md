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

Per review 009-2026-04-30 §P1-2 the recommendation is: at server startup, panic (or refuse to start) when `DEV_SKIP_AUTH != "" && APP_ENV == "production"`. Separately, the synthetic dev claims literal will get an explicit `ImpersonatedBy: ""` line — this is **documentation/test hardening only, NOT a behavior change**: the field is already zero-valued via Go's struct-literal omission, but explicit zeroing makes future readers (and grep) see the intent.

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
- Modify: `src/lib/auth.ts` — JWT callback rewritten as above. Export
  the callback (or `authConfig.callbacks.jwt`) so a unit test can drive
  it directly.
- Add: `tests/unit/auth-jwt-refresh.test.ts` — UNIT test of the
  exported jwt callback, NOT an integration test. The existing
  integration harness (`tests/api-helpers.ts:27-43`) mocks `auth()`
  directly and never exercises `callbacks.jwt`; there is no pattern
  for "real cookie session, mutate DB between requests, assert new
  claim". The unit test calls
  `authConfig.callbacks.jwt({ token, user: undefined })` after DB
  mutation and asserts the returned token. Three scenarios:
  - Promotion: `users.is_platform_admin = true` after first sign-in →
    callback returns `token.isPlatformAdmin = true`.
  - Demotion: `users.is_platform_admin = false` after first sign-in →
    callback returns `token.isPlatformAdmin = false`.
  - Account deletion: `users` row removed → callback nulls `id` and
    `isPlatformAdmin`, so NextAuth treats the next request as
    unauthenticated.
- Verify: existing `tests/integration/admin-orgs-api.test.ts` still
  passes (note: `admin-users-api.test.ts` referenced in earlier draft
  does NOT exist — there is no admin-users TS API test in this
  codebase). The page-level 403 card from PR #79 remains correct as
  defense in depth.

**DEV_SKIP_AUTH guard path:**
- Modify: `platform/cmd/api/main.go` — at startup, before wiring any
  handler, check `os.Getenv("DEV_SKIP_AUTH") != ""` AND
  `os.Getenv("APP_ENV") == "production"`. If both, `slog.Error(...)`
  and `os.Exit(1)` (refuse to start). Document the guard inline.
- Modify: `platform/internal/auth/middleware.go:89-104` — **explicit
  zeroing for documentation/test hardening only**, NOT a behavioral
  fix. The synthetic dev claims struct literal already omits
  `ImpersonatedBy`, which means it carries Go's zero value `""` today.
  The change adds an explicit `ImpersonatedBy: ""` line so future
  readers (and grep) can see the field was deliberately zeroed.
  Behavior unchanged.
- Add: `platform/cmd/api/main_test.go` (or a smaller
  `auth/dev_skip_test.go`) — table-driven test that the startup guard
  returns/exits when both env vars are set.

**APP_ENV convention:** APP_ENV is already used at
`platform/internal/auth/middleware.go:45` (cookie-domain diagnostics).
The guard treats absence-of-`APP_ENV` as "not production" (safe default
for dev). Add `APP_ENV=` to `.env.example` (currently absent) and
document the convention in `docs/setup.md` so deploys know to set
`APP_ENV=production` in prod env.

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
- Add `tests/unit/auth-jwt-refresh.test.ts` (UNIT, not integration — see Files section). Cover all three scenarios above by calling the exported `authConfig.callbacks.jwt` directly.
- Run `bun run test` end-to-end. Run `cd platform && go test ./...` (no Go-side changes; smoke).
- Self-review the diff.
- Commit.

### Phase 2: verify in dev DB + post-impl review

- Manually exercise: log in as a non-admin, promote in the DB via `UPDATE users SET is_platform_admin = true WHERE email = '<...>'`, navigate to `/admin/orgs` without re-authenticating. Confirm access works.
- Reverse: demote, navigate again, confirm 403.
- Update `TODO.md` with anything that surfaced.
- Open PR.

## Codex Review of This Plan

### Pass 1 — 2026-05-02: BLOCKED → fixes folded in

Codex flagged 3 blockers, all addressed inline:

1. **`tests/integration/admin-users-api.test.ts` doesn't exist** —
   plan referenced a non-existent file. Fix: removed the reference;
   existing `admin-orgs-api.test.ts` is the only verification target
   for the page-level 403 card regression.

2. **`ImpersonatedBy` "gap" is not real** — the struct literal at
   `platform/internal/auth/middleware.go:89-104` already omits the
   field, so Go's zero-value `""` is in place. The change is now
   documented as "explicit zeroing for documentation/test hardening,
   not a behavioral fix" so future readers don't misread it as a
   security patch.

3. **Test harness can't drive JWT refresh end-to-end** — existing
   `tests/api-helpers.ts:27-43` mocks `auth()` directly; no harness
   exists for "real cookie + DB mutation + re-request". Fix: scope
   to a UNIT test of the exported `authConfig.callbacks.jwt` function
   (call directly, mutate DB, call again, assert returned token).
   Avoids needing new harness while still exercising the proposed
   change.

Confirmed (no blockers):
- `src/lib/auth.ts:181-193` matches plan; impersonation flow is
  cookie-based (not `token.impersonatedBy`), no interaction.
- NextAuth v5 (`next-auth@5.0.0-beta.30`) fires `callbacks.jwt` on
  every authenticated request — plan's premise is correct.
- APP_ENV is already used at `middleware.go:45`; no competing
  convention. `.env.example` needs an `APP_ENV=` entry (now
  documented in the Files section).

### Pass 2 — 2026-05-02: 2 line-level inconsistencies, both fixed

- Plan §Phase 1 line 136 said integration test path; corrected to
  unit test path (matches Files section).
- Plan §Problem 2 line 26 still framed `ImpersonatedBy` as a
  security fix; reworded to "documentation/test hardening only".

### Pass 3 — 2026-05-02: **CONCUR**

Codex confirmed both pass-2 fixes correctly applied; no remaining
issues. Plan is clear to proceed to Phase 1 implementation.

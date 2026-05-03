# Plan 065 — Go owns auth verification (delete dual-JWE seam)

## Status

- **Date:** 2026-05-03
- **Origin:** Architectural review triggered by plan 050 + plan 053 + the
  PR #103 Edge-runtime fix to `refreshJwtFromDb`. All three patched the
  same underlying problem from different angles: Bridge has two
  independent implementations of "decrypt the Auth.js v5 JWE cookie and
  extract identity claims," and they keep drifting.
- **Scope:** Backend + Auth.js wiring. No portal UI changes. No new
  user-visible features. Existing OAuth and credentials flows are
  untouched from the user's perspective.
- **Predecessor plans this builds on:** plan 050 (JWT refresh on every
  call), plan 053 (Hocuspocus signed tokens — establishes the pattern
  this plan generalizes), plan 064 (parent_links — independent, no
  conflict).

## Problem

Bridge currently runs **two independent verifiers** for the same Auth.js
v5 JWE session cookie:

- **Next.js side**: `next-auth@5.0.0-beta.30` parses the cookie via its
  internal `jwt` callback path. `src/lib/auth.ts` + `src/lib/auth-jwt-callback.ts`
  call `refreshJwtFromDb()` on every Node-side request to pull
  `is_platform_admin` from Postgres.
- **Go platform side**: `platform/internal/auth/jwt.go` re-implements
  Auth.js's HKDF-SHA256 key derivation and JOSE A256CBC-HS512 decryption
  (`DecryptAuthJSToken`) so middleware can read claims from the same
  cookie.

The Go side is a hand-port of what the `auth.js` library does
internally. It is treated as a public contract but is not — the Auth.js
maintainers are free to change cookie format, salt info string, claim
names, or expiration semantics in any minor release. Every drift surfaces
as a security or correctness incident:

| Date | Plan | Symptom | Root cause |
|---|---|---|---|
| 2026-04-22 | 050 | Stale `is_platform_admin` after DB promotion | JWT was cached, neither side re-read DB |
| 2026-05-02 | 053 phase 3 | Hocuspocus rejected new tokens | Required separate signing key (correctly), but only because reusing `NEXTAUTH_SECRET` for a Node process to verify was structurally messy |
| 2026-05-03 | PR #103 | `JWTSessionError` on Edge runtime | `refreshJwtFromDb` called drizzle/postgres-js TCP from Edge — drizzle can't run there |

The pattern is clear: every time we add a new claim, every time we move
auth into a new runtime (Edge, Hocuspocus, future mobile), every time a
NextAuth release changes a default — the Go side has to be patched too.
The dev-only `/api/auth/debug` endpoint exists *specifically* to detect
when the two layers disagree, which is itself an admission that the
architecture is fragile.

A second, related problem: `refreshJwtFromDb`'s Edge guard means
middleware-side authorization decisions on Edge fall back to whatever
was in the JWT at the last Node-side write. We accepted this because
real authorization gates run on Node-render anyway, but it's a textbook
case of "the design forced us to be clever in a way that's hard to
reason about."

## Out of scope

- Replacing Auth.js entirely. Auth.js does Google OAuth, credentials,
  email verification, and provider linking well; rewriting that in Go
  would be weeks of work for no behavioral win.
- Mobile clients / non-browser clients. The bearer-token path will work
  for them once this lands, but designing that flow is a separate plan.
- Multi-tab logout propagation, refresh-token rotation, JWT
  blacklisting. None are present today; not introducing them here.
- Any change to how `bridge-impersonate` works. Impersonation overlay
  in Go middleware (`platform/internal/auth/middleware.go:135-151`)
  continues unchanged.
- Any change to `HOCUSPOCUS_TOKEN_SECRET` and the realtime-token mint
  path from plan 053. That is a separate, narrower JWT for a specific
  resource and stays as-is. Plan 065's new cookie is for general
  request authentication.

## Approach

**Go mints a Bridge-issued session JWT after Auth.js completes
authentication. Go middleware verifies that JWT instead of decrypting
the Auth.js JWE. Auth.js stays in charge of OAuth/credentials flows;
the JWE remains the Next-side session cookie.**

Concretely:

1. Add a new HS256 JWT format `BridgeSessionClaims` signed by Go with
   a new env var `BRIDGE_SESSION_SECRET`. Distinct from
   `HOCUSPOCUS_TOKEN_SECRET` (different audience, different blast radius
   on leak).
2. Add `POST /api/internal/sessions` — server-to-server endpoint
   protected by `BRIDGE_INTERNAL_SECRET` bearer (mirrors the existing
   `/api/internal/realtime/auth` pattern). Body: `{ email, name }`.
   Response: `{ token, expiresAt }` after Go looks up the user in the
   DB and embeds `id` + `isPlatformAdmin`.
3. In Auth.js, hook the `events.signIn` event to call the mint endpoint
   immediately after a successful sign-in (Google or credentials). Set
   the resulting JWT as a cookie named `bridge.session` with
   `HttpOnly`, `SameSite=Lax`, `Path=/`, `Secure` matching Auth.js's
   cookie.
4. Behind a feature flag `BRIDGE_SESSION_AUTH=1`, Go middleware
   `RequireAuth` / `OptionalAuth` reads `bridge.session` first; if
   absent or invalid, falls back to the Auth.js JWE path (current
   behavior). When the flag is `0` (default), legacy JWE path is
   primary.
5. Migrate Next-side `isPlatformAdmin` reads off the Auth.js token and
   onto a fetch from `/api/me/identity` (which already exists and is
   the source of truth from Go's perspective). This collapses
   `refreshJwtFromDb` — the Auth.js JWT no longer needs to carry
   `isPlatformAdmin` at all.
6. Once production is stable on `BRIDGE_SESSION_AUTH=1`, delete:
   - `platform/internal/auth/jwt.go` JWE branch (`DecryptAuthJSToken`,
     `decryptWithSalt`, `deriveEncryptionKey`).
   - `src/lib/auth-jwt-callback.ts` (Edge guard becomes unneeded).
   - The `isPlatformAdmin` reads on `session.user.*` in Next route
     handlers — they all switch to a small server-side helper that
     calls Go.
   - `/api/auth/debug` (its purpose was drift detection; with one
     verifier there's nothing to drift).

This is the same shape as plan 053's realtime-token pattern,
generalized: Go is authoritative for short-lived signed access tokens,
Auth.js orchestrates the user-facing flow.

### Why HS256 + a separate secret (not asymmetric)

HS256 keeps the implementation small (one signing path, one verifying
path). Both Go and the Auth.js mint endpoint live in the same trust
boundary — there's no third party that needs to verify without being
able to sign. RS256/EdDSA would buy us key separation, but at the cost
of public-key distribution and rotation logic we don't need today. If
we add a mobile client or external verifier later, we revisit. The
secret is distinct from `NEXTAUTH_SECRET` and `HOCUSPOCUS_TOKEN_SECRET`
so each compromise is independently contained.

### Why behind a flag

Auth changes that ship in a single commit are how outages happen. The
flag lets us:
- Run Bridge session minting in dev with the flag ON.
- Deploy with the flag OFF (mints happen, cookie is set, but Go still
  verifies via JWE — the new cookie is dormant).
- Flip the flag in staging with a safe rollback to OFF.
- Flip in prod after a soak period.
- Delete the legacy code only after a follow-up plan confirms the
  flipped state is stable.

This is the same pattern plan 053 used (`HOCUSPOCUS_REQUIRE_SIGNED_TOKEN`).

## Decisions to lock in (first-draft, open to revision)

1. **Cookie name:** `bridge.session` (short, distinct from
   `authjs.session-token`, no `__Secure-` prefix variant — see #2).
2. **Cookie scheme handling:** Single name, but `Secure` attribute set
   based on `APP_ENV` (production → `Secure=true`, dev → `Secure=false`).
   The dual-cookie-name dance Auth.js does is a NextAuth convention we
   don't need to inherit — the canonical-cookie middleware in Go can
   shrink to one name.
3. **TTL:** 7 days (matches Auth.js default), refreshed on the
   server-to-server mint call. Auth.js triggers a re-mint via a small
   Next API route hit on every authenticated render (or via
   middleware-on-Node, see Phase 2 below) — same cadence as plan 050's
   "always refresh from DB."
4. **Claims:** `sub` (user id, UUID), `email`, `name`,
   `isPlatformAdmin`, `iss=bridge-platform`, `exp`, `iat`, `nbf` (with
   30s skew). Same shape as `RealtimeClaims` from plan 053, minus
   `scope` (not relevant here) and minus `role` (we already have role
   from `/api/me/roles`).
5. **DB lookup ownership:** Go's mint endpoint owns the DB lookup. Next
   never queries `users` for `isPlatformAdmin` again.
6. **Logout:** `signOut` in Auth.js clears both cookies. Add a
   matching `DELETE /api/internal/sessions` endpoint (or just have
   Auth.js's signOut callback call a Set-Cookie with `Max-Age=0` —
   the simpler path).
7. **Session refresh on demote/promote:** Same model as plan 050. The
   admin gate in Go calls `users.is_platform_admin` on every request
   (via a tiny new helper). The cookie's `isPlatformAdmin` claim
   becomes a *hint* used for portal nav rendering, not an authority.
   If hint disagrees with DB, DB wins. This is the cleanest
   interpretation: Go middleware's `RequireAdmin` becomes a DB call,
   not a claim read. Cost: one indexed query per admin request — same
   cost as plan 050's `refreshJwtFromDb`.

Decision #7 is the most important: it means **the JWT becomes
unprivileged — losing it doesn't grant or revoke anything sensitive,
because authorization always reverifies against the DB.** That removes
an entire class of staleness bugs.

## Files

### Phase 1 — Go mint infrastructure

**Add:**
- `platform/internal/auth/bridge_session.go` — `BridgeSessionClaims`,
  `SignBridgeSession`, `VerifyBridgeSession`. Mirrors
  `realtime_jwt.go`'s shape; HS256, issuer `bridge-platform`, 7-day
  TTL clamp, 30s NBF skew.
- `platform/internal/auth/bridge_session_test.go` — sign+verify round
  trip, expired token, wrong secret, wrong issuer, missing claims,
  TTL clamp.
- `platform/internal/handlers/internal_sessions.go` — exposes
  `POST /api/internal/sessions`. Bearer-protected via
  `BRIDGE_INTERNAL_SECRET`. Body validation (email + name required,
  email is a valid format). On success: look up user by email,
  embed `id` + `isPlatformAdmin`, sign, return.
- `platform/internal/handlers/internal_sessions_test.go` — happy
  path, missing bearer, wrong bearer, malformed body, unknown email
  (404), DB error (500). At least 6 cases.

**Modify:**
- `platform/cmd/api/main.go` — wire the new handler under the existing
  internal-route block (NOT under `RequireAuth` — same pattern as
  `/api/internal/realtime/auth`).
- `.env.example` — add `BRIDGE_SESSION_SECRET=` and
  `BRIDGE_INTERNAL_SECRET=` with comments. (The latter may already
  exist for the realtime internal callback — verify and reuse if so.)

### Phase 2 — Auth.js mint integration

**Add:**
- `src/lib/auth-bridge-session.ts` — `mintBridgeSession({ email, name })`
  helper that calls `POST /api/internal/sessions` with the bearer and
  returns `{ token, expiresAt }`. Pure function over `fetch`, easily
  unit-testable. Fails closed: if the call errors, the user remains
  signed in via Auth.js but has no `bridge.session` cookie set —
  middleware then falls back to JWE (legacy path).
- `tests/unit/auth-bridge-session.test.ts` — happy path (mocked
  `fetch`), 500-from-Go, 404-from-Go, network timeout. Asserts the
  helper never throws into Auth.js's signIn callback.

**Modify:**
- `src/lib/auth.ts` — add `events.signIn` (NextAuth v5 has this hook;
  fires after `signIn` callback completes successfully). The event
  handler calls `mintBridgeSession`, then sets `bridge.session` via
  `cookies().set(...)`. Strict cookie attributes per "Decisions" §1-2.
- `src/lib/auth.ts` — the existing `jwt` callback no longer needs to
  refresh `isPlatformAdmin` once Phase 4 lands; until then it remains
  for legacy clients. Mark deprecated in a comment, do not delete yet.
- `tests/integration/auth-signin.test.ts` (new) OR extend an existing
  signin test — assert the cookie is set after a Google sign-in
  succeeds. End-to-end coverage of the wiring.

### Phase 3 — Go middleware reads bridge.session

**Modify:**
- `platform/internal/auth/middleware.go` — add
  `readBridgeSessionToken(r)` helper. In `RequireAuth` and
  `OptionalAuth`, when `BRIDGE_SESSION_AUTH=1`:
  - Read `bridge.session` first.
  - If present and valid → use those claims directly (no JWE
    decryption needed).
  - If present but invalid → 401 (don't fall back; an invalid
    bridge.session is a real problem worth surfacing).
  - If absent → fall back to existing JWE path.
- `platform/internal/auth/middleware_test.go` — extend to cover all
  three branches: bridge.session present + valid, present + invalid,
  absent (legacy fallback). 6 new cases minimum.
- Update `RequireAdmin` to do a DB lookup of `users.is_platform_admin`
  and use the live value (per Decisions §7). Keep the claim-based
  check as a fast-path "is the user even claiming admin?" — only DB
  is authoritative. Add a small `auth.AdminCheck(ctx, userID)` helper
  that reads `users.is_platform_admin`.

### Phase 4 — Next.js stops trusting Auth.js token for `isPlatformAdmin`

**Modify (10 files):**

Replace every `session.user.isPlatformAdmin` read with a server-side
fetch helper. The helper calls `/api/me/identity` (already exists),
caches the result on the request via `React.cache` so SSR and the
route handler share one call.

- `src/lib/identity.ts` (new) — `getIdentity(): Promise<Identity>`,
  React-cached, calls `/api/me/identity`. Returns
  `{ userId, isPlatformAdmin, ... }`.
- `src/app/api/admin/orgs/route.ts:8` — replace.
- `src/app/api/admin/impersonate/route.ts:15` — replace.
- `src/app/api/assignments/[id]/submit/route.ts:26` — replace.
- `src/app/api/courses/route.ts:35,60` — replace.
- `src/app/api/assignments/[id]/route.ts:56,92` — replace.
- `src/app/api/sessions/[id]/broadcast/route.ts:28` — replace.
- `src/app/api/orgs/[id]/members/route.ts:28,76` — replace.
- `src/lib/impersonate.ts:33` — replace.
- `src/app/(portal)/admin/users/page.tsx:78` — already reads from
  Go (no change).

**Add:**
- `tests/unit/identity-helper.test.ts` — happy path, network failure,
  non-admin response, admin response.

After Phase 4, `src/lib/auth-jwt-callback.ts`'s DB call is no longer
needed. The Edge guard from PR #103 can remain (cheap, defensive)
until phase 5 deletes the file entirely.

### Phase 5 — Cutover and cleanup

This is its own follow-up PR after the flag has flipped in production
and stayed stable for ≥7 days.

**Delete:**
- `platform/internal/auth/jwt.go`'s `DecryptAuthJSToken`,
  `decryptWithSalt`, `deriveEncryptionKey`, `verifyPlainJWT` (all the
  JWE plumbing). Keep `extractClaims` if still useful, otherwise
  delete.
- `platform/internal/auth/jwe_test.go`, `jwt_test.go` (the JWE-specific
  tests).
- `src/lib/auth-jwt-callback.ts` and its test
  `tests/unit/auth-jwt-refresh.test.ts`.
- The `jwt` callback in `src/lib/auth.ts` (or shrink to a no-op).
- `src/app/api/auth/debug/route.ts` — drift detection no longer
  needed.
- The `BRIDGE_SESSION_AUTH` env var and all branching it gates.

**Modify:**
- `platform/internal/auth/middleware.go` — remove the JWE fallback
  branch. `bridge.session` is the only path.
- `.env.example` — drop deprecated comments.
- `docs/setup.md` — replace the "Trusted Reverse-Proxy Configuration"
  section's discussion of dual cookie names with the single-cookie
  model.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Mint endpoint is a new attack surface | medium | Bearer-protected, server-to-server only, never exposed publicly. Same pattern as `/api/internal/realtime/auth`. Reject any request without the bearer with 401 BEFORE parsing the body. |
| Cookie size growth | low | HS256 JWT is ~250 bytes; roughly 0.3% of the 8KB cookie header limit. We have headroom. |
| Auth.js `events.signIn` doesn't fire reliably | medium | NextAuth v5 fires `events.signIn` synchronously after `callbacks.signIn` returns true. Verified at https://authjs.dev/reference/nextjs#events. Test: integration test in Phase 2 asserts the cookie is set. |
| Cookie not set during the OAuth redirect dance | medium | Auth.js's flow ends on `/api/auth/callback/google`, which is a Node route — `cookies().set()` works there. Confirmed by reviewing how `cookies()` is used by other Auth.js callbacks today (`signin-google` flow in plan 043). |
| Mint failure breaks login UX | medium | The helper fails closed: if Go is down or the mint fails, the user signs in via Auth.js JWE only; the new cookie isn't set; Go middleware (with flag ON) falls back to JWE. Net: identical to today's behavior. |
| Flag flip rolls forward an admin gate that does new DB queries on every request | low | The query is the same indexed lookup we already do in `refreshJwtFromDb`. No new latency vs. plan 050. |
| Drift between phase 4 and phase 5 | low | Phase 4 lands with the flag OFF. Both Auth.js JWT (with `isPlatformAdmin`) and the new helper coexist; route handlers use the helper. Phase 5 deletes the now-unused claim. No hot-loop window. |
| Cross-domain cookies in prod | medium | If Bridge ever splits Next and Go onto different hostnames, `bridge.session` would need a `Domain=` attribute. Today Bridge runs on a single domain via Next's proxy; verified in `next.config.ts` rewrites. Document this as a Future Work note. |
| Test database parity | low | All new Go tests follow the existing pattern (`TEST_DATABASE_URL`, store integration tests). No new harness needed. |

## Phases

### Phase 0: Pre-impl Codex review of THIS plan

Per CLAUDE.md plan review gate. Dispatch `codex:codex-rescue` to
review against:

- `src/lib/auth.ts` and `src/lib/auth-jwt-callback.ts` (current
  Auth.js wiring).
- `platform/internal/auth/jwt.go` (current Go JWE verifier).
- `platform/internal/auth/middleware.go` (cookie reading + fallback).
- `platform/internal/handlers/realtime_token.go` (the pattern this
  plan generalizes).
- `platform/internal/auth/realtime_jwt.go` (the existing HS256 JWT
  shape we're mirroring).

Specific questions for Codex:

1. Does `events.signIn` actually fire with reliable timing in NextAuth
   v5 beta.30, and can `cookies().set()` from inside the event handler
   actually attach a Set-Cookie header to the OAuth redirect response?
   The plan assumes yes; please verify against the v5 source.
2. Is there a hidden ordering hazard between the Auth.js cookie
   write and the `bridge.session` cookie write that could cause one
   to be lost?
3. Does any existing callsite I missed read `isPlatformAdmin` directly
   off the JWT in a way that wouldn't be caught by Phase 4's
   replacement list? Search for `token.isPlatformAdmin` /
   `session.user.isPlatformAdmin` / equivalents.
4. Is the `RequireAdmin` DB-lookup-on-every-request safe under load?
   Plan 050 already does this for the JWT callback; this plan moves
   it to Go. Is the indexed lookup actually the right one
   (`users_email_idx` vs `users_pkey`)? Note: plan 050 keys on email,
   but this plan keys on `id` (since `bridge.session` carries `sub`
   = user id). Verify the index exists for `id` (it's the PK so
   should be fine, but please confirm the query plan).
5. Cross-runtime gotchas: `cookies()` in Auth.js's `events.signIn` —
   does it work the same as in a route handler? The Edge-runtime
   surprise from PR #103 is a recent reminder to verify.
6. Is there a simpler alternative I missed (e.g., have Go provide a
   verification adapter that Next imports as a Node module, so they
   share code instead of duplicating)?

Iterate until both Claude and Codex concur (no blockers, all
"important" items have explicit resolutions). Capture the verdict
inline in `## Codex Review of This Plan` below.

### Phase 1: Go mint infrastructure (PR 1)

- Implement `bridge_session.go` + tests.
- Implement `internal_sessions.go` handler + tests.
- Wire under `/api/internal/sessions` in `cmd/api/main.go`.
- Add env vars to `.env.example`.
- `cd platform && go test ./... -count=1 -timeout 120s` clean.
- Self-review the diff.
- Codex post-impl review of phase 1.
- PR + merge.

### Phase 2: Auth.js mint integration (PR 2)

- Implement `auth-bridge-session.ts` + unit tests.
- Wire `events.signIn` in `auth.ts`.
- Add integration test asserting the cookie is set.
- `bun run test` clean.
- Self-review.
- Codex post-impl review of phase 2.
- PR + merge with flag OFF in dev `.env`.

### Phase 3: Go middleware reads bridge.session (PR 3)

- Add `readBridgeSessionToken` helper.
- Add flag-gated branching in `RequireAuth` / `OptionalAuth`.
- Add `auth.AdminCheck(ctx, userID)` helper.
- `RequireAdmin` switches to DB lookup for the admin authority check.
- Extend middleware tests (6+ new cases).
- `cd platform && go test ./...` clean.
- Self-review.
- Codex post-impl review.
- PR + merge with `BRIDGE_SESSION_AUTH=0` (default).
- Manual smoke in dev: flip flag locally, sign in, hit `/admin/stats`,
  inspect `/api/auth/debug` to confirm both layers agree.

### Phase 4: Next route handlers stop reading `isPlatformAdmin` from JWT (PR 4)

- Add `src/lib/identity.ts` helper + tests.
- Replace 9 callsites enumerated in Files §Phase 4.
- Run full `bun run test`.
- Self-review.
- Codex post-impl review.
- PR + merge.

### Phase 5: Cutover and cleanup (PR 5)

Filed as a separate plan or as a child PR after the flag has flipped
in production and stayed stable ≥7 days. NOT shipped concurrently
with phases 1–4. The intermediate state (legacy code present, flag
flipped) is the safe rollback point.

- Delete legacy code per Files §Phase 5.
- Run full `bun run test` + `cd platform && go test ./...`.
- Codex post-impl review.
- PR + merge.

## Operational rollout (post-merge of phases 1–4)

1. **Dev**: `BRIDGE_SESSION_AUTH=1` set in dev `.env`. Smoke-test sign
   in, admin nav, an admin API call. Verify `bridge.session` cookie
   present in browser devtools.
2. **Staging**: deploy with flag OFF. Confirm no regressions (Go still
   verifies JWE — pure backward-compat). Then flip flag to ON. Soak
   24h.
3. **Production**: deploy with flag OFF. Soak 24h (forces both teams
   to confirm new code is healthy without auth changes). Flip flag to
   ON. Soak 7 days.
4. **Cleanup**: open phase 5 PR.

## Codex Review of This Plan

_(To be populated by Codex pass — see Phase 0.)_

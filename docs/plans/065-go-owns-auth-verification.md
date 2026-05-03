# Plan 065 — Go owns auth verification (delete dual-JWE seam)

## Status

- **Date:** 2026-05-03
- **Origin:** Architectural review triggered by plan 050 + plan 053 + the
  PR #103 Edge-runtime fix to `refreshJwtFromDb`. All three patched the
  same underlying problem from different angles: Bridge has two
  independent implementations of "decrypt the Auth.js v5 JWE cookie and
  extract identity claims," and they keep drifting.
- **Scope:** Backend + Auth.js wiring + Next middleware. No portal UI
  changes. No new user-visible features. Existing OAuth and credentials
  flows are untouched from the user's perspective.
- **Predecessor plans this builds on:** plan 050 (JWT refresh on every
  call), plan 053 (Hocuspocus signed tokens — establishes the pattern
  this plan generalizes), plan 064 (parent_links — independent, no
  conflict).
- **Pinned dependency context:** `next-auth@5.0.0-beta.30`,
  `@auth/core@0.41.1` (per `bun.lock:118-120`). All claims about
  Auth.js v5 behavior in this plan refer to these resolved versions.

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
  continues unchanged. (Note: the overlay's gate "claims.IsPlatformAdmin"
  becomes "live admin status" — see Phase 3 §"Live admin in claims".)
- Any change to `HOCUSPOCUS_TOKEN_SECRET` and the realtime-token mint
  path from plan 053. That is a separate, narrower JWT for a specific
  resource and stays as-is. Plan 065's new cookie is for general
  request authentication.

## Approach

**Go mints a Bridge-issued session JWT after Auth.js completes
authentication. Go middleware verifies that JWT instead of decrypting
the Auth.js JWE. Auth.js stays in charge of OAuth/credentials flows;
the JWE remains the Next-side session cookie that drives Auth.js's own
internal state.**

Concretely:

1. Add a new HS256 JWT format `BridgeSessionClaims` signed by Go with
   a new env var `BRIDGE_SESSION_SECRET`. Distinct from
   `HOCUSPOCUS_TOKEN_SECRET` and `NEXTAUTH_SECRET` (different audience,
   different blast radius on leak).
2. Add `POST /api/internal/sessions` — server-to-server endpoint
   protected by a bearer token. The bearer is a NEW env var
   `BRIDGE_INTERNAL_SECRET`, distinct from `HOCUSPOCUS_TOKEN_SECRET`
   (which the realtime internal callback uses today). This avoids
   coupling: a leak of the realtime callback bearer must not also
   forge session cookies. Body: `{ email, name }`. Response:
   `{ token, expiresAt }` after Go looks up the user in the DB and
   embeds `id` + `isPlatformAdmin` (the latter is just an *initial*
   value — see Decision §7 below for why it's not authoritative).
3. **Lazy mint in Edge middleware** (NOT `events.signIn`). The Next
   middleware at `src/middleware.ts` is wrapped: after Auth.js's
   `authorized` callback runs, the middleware checks for `bridge.session`
   in the request. If absent or expiring within 24h, the middleware
   calls `POST /api/internal/sessions` via `fetch()` (Edge-safe) and
   attaches the resulting cookie via `NextResponse.cookies.set()`.
   See §"Why lazy middleware mint" below for why this beats
   `events.signIn`.
4. Behind a feature flag `BRIDGE_SESSION_AUTH=1`, Go middleware
   `RequireAuth` / `OptionalAuth` reads `bridge.session` first; if
   absent or invalid, falls back to the Auth.js JWE path (current
   behavior). When the flag is `0` (default), legacy JWE path is
   primary and the cookie is dormant.
5. **Live admin status in claims at the middleware layer.** Today, ~80
   handler sites read `claims.IsPlatformAdmin` across virtually every
   handler file (Codex pass-2 audit). Rather than touching each one
   AND rather than maintaining a brittle path-skip list that has to
   be kept in sync with new handler reads, the Go middleware does ONE
   indexed PK lookup of `users.is_platform_admin` per *authenticated*
   request and overwrites the JWT-carried claim with the live value
   before invoking the handler. All existing reads keep working
   unchanged and become automatically live. Cost: one indexed PK
   lookup per authenticated request — same as plan 050's
   `refreshJwtFromDb`. Unauthenticated traffic (no token) takes zero
   DB cost.
6. Migrate Next-side `isPlatformAdmin` reads off the Auth.js token and
   onto a fetch from `/api/me/identity` (which already exists and,
   after Phase 3, is the live-from-DB source of truth). This
   collapses `refreshJwtFromDb` — the Auth.js JWT no longer needs to
   carry `isPlatformAdmin` at all.
7. Once production is stable on `BRIDGE_SESSION_AUTH=1`, delete:
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

### Why lazy middleware mint (not `events.signIn`)

Codex's pass-1 review flagged a load-bearing ambiguity: NextAuth v5's
`events.signIn` *does* fire after the sign-in callback, but whether
`cookies().set()` from inside that handler reliably attaches a
Set-Cookie header to the OAuth-callback redirect response is **not
verified by any test in this codebase or by an authoritative source
in `@auth/core@0.41.1`**. There's also a real ordering concern:
Auth.js builds its own Set-Cookie array internally; a parallel
`cookies().set()` may or may not be appended to the same response.

The lazy-middleware-mint approach sidesteps all of this:

- **Edge middleware fires on every authenticated request to a portal
  or admin path**, including the very first request the browser makes
  after the OAuth redirect lands.
- Auth.js v5 exposes the resolved session to middleware via `req.auth`
  (Edge-safe — Auth.js itself runs there).
- `fetch()` works on Edge runtime, so the middleware can call Go's
  mint endpoint.
- `NextResponse.cookies.set()` is the documented, Edge-supported way
  to attach Set-Cookie to a middleware response.
- The mint is idempotent + cached: middleware decodes the existing
  `bridge.session` JWT *without verifying signature* (we trust our
  own cookie's payload format) just to check `exp`. If still > 24h
  away, no mint call is made — steady-state cost is one
  cookie-payload-decode per request.
- Refresh is automatic: the same logic handles initial mint AND
  re-mint when expiry is approaching. No separate refresh endpoint
  needed.

This is the same architectural shape as plan 053's lazy realtime-token
mint, applied at the middleware layer instead of inside a React hook.

### Why HS256 + a separate secret (not asymmetric)

HS256 keeps the implementation small (one signing path, one verifying
path). Both Go and the Auth.js mint endpoint live in the same trust
boundary — there's no third party that needs to verify without being
able to sign. RS256/EdDSA would buy us key separation, but at the cost
of public-key distribution and rotation logic we don't need today. If
we add a mobile client or external verifier later, we revisit. The
secret is distinct from `NEXTAUTH_SECRET`, `HOCUSPOCUS_TOKEN_SECRET`,
and `BRIDGE_INTERNAL_SECRET` so each compromise is independently
contained.

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

## Decisions to lock in

1. **Cookie name:** `bridge.session` (short, distinct from
   `authjs.session-token`, no `__Secure-` prefix variant — see #2).
2. **Cookie scheme handling:** Single name, but `Secure` attribute set
   based on `APP_ENV` (production → `Secure=true`, dev → `Secure=false`).
   The dual-cookie-name dance Auth.js does is a NextAuth convention we
   don't need to inherit — the canonical-cookie middleware in Go can
   shrink to one name for `bridge.session`. (The legacy JWE cookie's
   dual-name handling stays in place until Phase 5.)
3. **TTL:** 7 days (matches Auth.js default). Re-minted whenever the
   middleware sees the cookie within 24h of expiry. Steady-state
   sessions get a fresh 7-day cookie roughly every 6 days.
4. **Claims:** `sub` (user id, UUID), `email`, `name`,
   `isPlatformAdmin` (initial value at mint time — NOT authoritative;
   see #7), `iss=bridge-platform`, `exp`, `iat`, `nbf` (with 30s
   skew). Same shape as `RealtimeClaims` from plan 053, minus
   `scope`/`role`.
5. **DB lookup ownership:** Go's mint endpoint owns the user-by-email
   DB lookup at mint time. Go's `RequireAuth` middleware additionally
   does a user-by-id DB lookup of `is_platform_admin` on every
   authenticated request to enforce live admin status (see #7).
6. **Logout:** `signOut` in Auth.js clears the Auth.js cookie. The
   middleware on the next request notices `bridge.session` exists
   without a corresponding Auth.js session and clears it (sets
   `Max-Age=0`). Simpler than coupling logout into Auth.js's signOut
   callback, and self-healing if the user opens a stale tab.
7. **Live admin in claims at the middleware layer.** Most important
   decision in this plan. The JWT's `isPlatformAdmin` field is a
   *cosmetic hint* — useful as a fast-path for Go middleware to skip
   the DB lookup when the JWT clearly says non-admin AND the request
   isn't hitting an admin path. For ALL other paths and for any path
   touching `claims.IsPlatformAdmin` in handler code, the middleware
   does an indexed DB lookup of `users.is_platform_admin` and rewrites
   the claim to the live value before handlers see it. This means:
   - All ~80 existing reads of `claims.IsPlatformAdmin` in
     `platform/internal/handlers/*.go` keep working unchanged.
   - Stale-grant and stale-revocation bugs are eliminated by
     construction.
   - The cost is one indexed PK lookup (`users_pkey`, `WHERE id = $1`)
     per authenticated request — same as plan 050's email-keyed lookup.
   - The fast-path skip for non-admin paths preserves the no-DB-cost
     hot path for the bulk of requests.

   See "Live admin via middleware injection" below for the full
   design.

8. **Refresh path:** No separate refresh endpoint. The middleware's
   lazy-mint logic handles both initial mint AND refresh-when-expiring.
   This addresses Codex pass-1's "session refresh underspecified"
   blocker.

### Live admin via middleware injection

`auth.Middleware` already exists as a struct (`platform/internal/auth/middleware.go:72-78`)
holding `Secret`. We extend it to hold a small `AdminChecker` interface
backed by `UserStore`, and promote the package-level `RequireAdmin` to
a method on the same struct so it shares the same DI surface.

**Construction order matters** (Codex pass-2 finding): today
`cmd/api/main.go:55-64` constructs `authMw` BEFORE stores. Plan 065
moves middleware construction to AFTER stores are built, so the
`AdminChecker` (which wraps `UserStore`) can be passed in at
construction time. Phase 1 includes this reorder explicitly.

```go
// New: platform/internal/auth/admin_check.go
type AdminChecker interface {
    IsAdmin(ctx context.Context, userID string) (bool, error)
}

// Middleware is restructured.
type Middleware struct {
    Secret         string  // unchanged
    BridgeSecret   string  // BRIDGE_SESSION_SECRET, new
    AdminChecker   AdminChecker  // injected
    BridgeAuthOn   bool    // BRIDGE_SESSION_AUTH flag
}
```

Inside `RequireAuth`, after token verification produces `claims`:

```go
// Always do the live-admin DB lookup for any authenticated request.
// Codex pass-2 audit confirmed claims.IsPlatformAdmin is read across
// virtually every handler file (~80 sites in classes, sessions,
// courses, problems, attempts, units, collections, documents,
// annotations, ai, parent, teacher, me, etc.) — a path-based skip
// list would be impossible to keep in sync. The unconditional
// lookup is cheap (indexed PK) and matches plan 050's
// always-refresh model.
isAdmin, err := m.AdminChecker.IsAdmin(r.Context(), claims.UserID)
if err != nil {
    // Log and fail-closed: claims.IsPlatformAdmin = false. We
    // would rather temporarily 403 a real admin than silently
    // grant admin to a stale JWT during a DB hiccup. Real admin
    // gates pages re-render anyway; this is a transient inconvenience.
    slog.Warn("admin check DB lookup failed, fail-closing IsPlatformAdmin",
        "err", err, "userID", claims.UserID)
    claims.IsPlatformAdmin = false
} else {
    claims.IsPlatformAdmin = isAdmin
}
```

This unconditional lookup is the same cost shape as plan 050 and is
the simpler, drift-proof alternative to a path-based skip list.
Unauthenticated traffic (no token) bypasses this whole block, so static
assets and public pages take zero DB cost.

`auth.RequireAdmin` becomes a method on `Middleware`:

```go
func (m *Middleware) RequireAdmin(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        claims := GetClaims(r.Context())
        if claims == nil {
            writeJSONError(w, http.StatusForbidden, "Platform admin required")
            return
        }
        // claims.IsPlatformAdmin is already live by the time we get
        // here (RequireAuth ran before RequireAdmin on this chain).
        if !claims.IsPlatformAdmin && claims.ImpersonatedBy == "" {
            writeJSONError(w, http.StatusForbidden, "Platform admin required")
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

Callsites that today reference the package-level `auth.RequireAdmin`
become `mw.RequireAdmin` where `mw` is the middleware instance wired
up in `cmd/api/main.go`. This is a mechanical rename — no behavior
change beyond DI. Phase 3 enumerates the callsites.

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
- `platform/internal/auth/admin_check.go` — `AdminChecker` interface
  + a default impl backed by `*store.UserStore` that does
  `SELECT is_platform_admin FROM users WHERE id = $1`. Trivial
  delegation; one method.
- `platform/internal/auth/admin_check_test.go` — happy path, missing
  user (returns false + nil error so middleware doesn't 500), DB
  error.
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
  `/api/internal/realtime/auth`). **Reorder construction**: build
  stores BEFORE constructing `auth.Middleware`, then pass the
  `AdminChecker` into the middleware constructor. Remove any
  `auth.RequireAdmin` package-level references — they all become
  method calls on the middleware instance (`mw.RequireAdmin`).
- `platform/internal/config/config.go` — add `BridgeSessionSecret`,
  `BridgeInternalSecret`, and `BridgeSessionAuth` fields to the
  `Config` struct. Read from env in the same loader that already
  reads `NEXTAUTH_SECRET` (`config.go:78-80`) and
  `HOCUSPOCUS_TOKEN_SECRET` (`config.go:94-96`). All three default
  empty in dev; production startup guard rejects empty for the two
  secrets when `BRIDGE_SESSION_AUTH=1`.
- `.env.example` — add `BRIDGE_SESSION_SECRET=` and
  `BRIDGE_INTERNAL_SECRET=` with comments explaining they are
  distinct from `NEXTAUTH_SECRET` and `HOCUSPOCUS_TOKEN_SECRET`.
  Also add `BRIDGE_SESSION_AUTH=` (default off, set to 1 to enable
  bridge.session reading in Go middleware).
- `next.config.ts` — verify `/api/internal/*` is NOT in
  `GO_PROXY_ROUTES` (it isn't today — confirmed at
  `next.config.ts:10-43`). Add an explanatory comment near
  `GO_PROXY_ROUTES` stating that `/api/internal/sessions` is
  server-to-server only, called from `src/lib/bridge-session-mint.ts`
  with the `BRIDGE_INTERNAL_SECRET` bearer, and must never be
  exposed to browser traffic.

### Phase 2 — Lazy mint in Edge middleware

**Add:**
- `src/lib/bridge-session-mint.ts` — Edge-safe helper. Two functions:
  - `bridgeSessionExpiringSoon(token: string | undefined): boolean`
    — base64-decodes the JWT payload (no signature verify), reads
    `exp`, returns true when missing OR within 24h.
  - `mintBridgeSession({ email, name }): Promise<{ token, expiresAt }>`
    — calls `POST /api/internal/sessions` with the bearer. Returns
    null on any error (caller falls back gracefully).
- `tests/unit/bridge-session-mint.test.ts` — happy path, expiry
  threshold (already-expired, 23h-from-expiry, 25h-from-expiry,
  missing token, malformed token), mint failure, mint timeout.
  These tests run in vitest's jsdom env and use `fetch` mocks.

**Modify:**
- `src/middleware.ts` — wrap the existing `auth as middleware` export
  with a function that, after the auth check, conditionally mints
  `bridge.session` and attaches it via `NextResponse.cookies.set`.
  Auth.js v5's pattern for this is documented:
  ```ts
  export default auth(async (req) => {
    const response = NextResponse.next();
    if (req.auth?.user) {
      const existing = req.cookies.get("bridge.session")?.value;
      if (bridgeSessionExpiringSoon(existing)) {
        const minted = await mintBridgeSession({
          email: req.auth.user.email!,
          name: req.auth.user.name ?? "Unknown",
        });
        if (minted) {
          response.cookies.set("bridge.session", minted.token, {
            httpOnly: true,
            sameSite: "lax",
            path: "/",
            secure: process.env.APP_ENV === "production",
            expires: new Date(minted.expiresAt),
          });
        }
      }
    } else {
      // No auth → make sure stale bridge.session is cleared.
      if (req.cookies.get("bridge.session")) {
        response.cookies.delete("bridge.session");
      }
    }
    return response;
  });
  ```
- `src/middleware.ts` matcher — extend to cover EVERY authenticated
  API path that proxies to Go, so the lazy mint fires before the
  request reaches Go. The canonical list is `GO_PROXY_ROUTES` in
  `next.config.ts:10-43`. The middleware matcher must be a strict
  superset of that list (Codex pass-2 finding Q6).
  - existing portal paths (keep): `/teacher/:path*`, `/student/:path*`,
    `/parent/:path*`, `/org/:path*`, `/admin/:path*`.
  - existing API paths (keep): `/api/admin/:path*`, `/api/orgs/:path*`.
  - add (mirror of next.config.ts proxy list, in same order):
    `/api/auth/register`, `/api/courses/:path*`, `/api/classes/:path*`,
    `/api/sessions/:path*`, `/api/documents/:path*`,
    `/api/assignments/:path*`, `/api/submissions/:path*`,
    `/api/annotations/:path*`, `/api/ai/:path*`, `/api/parent/:path*`,
    `/api/me/:path*`, `/api/teacher/:path*`, `/api/org/:path*`,
    `/api/schedule/:path*`, `/api/topics/:path*`,
    `/api/problems/:path*`, `/api/test-cases/:path*`,
    `/api/attempts/:path*`, `/api/s/:path*`, `/api/units/:path*`,
    `/api/collections/:path*`, `/api/uploads/:path*`,
    `/api/realtime/:path*`.
  - The matcher list lives in `src/middleware.ts` (literal, per
    plan 047 phase 1 constraint) AND in
    `src/lib/portal/middleware-matcher.ts` (used by unit tests).
    The existing parity test at
    `tests/unit/middleware-matcher.test.ts` enforces no drift between
    those two locations. Phase 2 ADDS a new parity test
    `tests/unit/middleware-proxy-parity.test.ts` that asserts every
    entry in `next.config.ts:GO_PROXY_ROUTES` has a matching entry
    in the middleware matcher (modulo trailing `:path*`). This
    prevents future proxy additions from silently bypassing the
    lazy mint.
- `tests/unit/middleware-bridge-session.test.ts` — integration test
  that simulates an authenticated request with no `bridge.session`,
  asserts the response sets the cookie. Mocks `mintBridgeSession`
  to avoid hitting Go in the test.

### Phase 3 — Go middleware: read bridge.session + inject live admin

**Modify:**
- `platform/internal/auth/middleware.go`:
  - Promote `RequireAuth` and `RequireAdmin` to methods on `Middleware`
    (DI for `AdminChecker`, `BridgeSecret`, `BridgeAuthOn`).
  - Add `readBridgeSessionToken(r *http.Request) string` that reads
    the `bridge.session` cookie.
  - In `RequireAuth`/`OptionalAuth`, when `m.BridgeAuthOn`:
    1. Read `bridge.session`. If present + valid → use those claims.
    2. If present but invalid → 401 unconditionally. **No JWE fallback
       on invalid bridge.session.** Codex pass-2 flagged that allowing
       JWE fallback creates a downgrade attack surface: once Bridge
       sessions become authoritative and we ship session
       revocation/rotation, an attacker holding an old un-revoked JWE
       could pair it with a forged-invalid `bridge.session` to bypass
       revocation. The benign "stale cookie" race (cookie expired but
       middleware hasn't re-minted yet) is also fixed by the next
       Edge-middleware request — the user retries and Edge mint
       attaches a fresh cookie before the request reaches Go.
       Net: prefer the security guarantee over the marginal robustness.
    3. If absent → fall back to existing JWE path. (Absent ≠ invalid.
       Absent means "lazy mint hasn't run yet" or "this is a
       direct-to-Go request from a non-browser client" — JWE is still
       the right fallback during rollout.)
  - After token verification (either path), invoke
    `m.AdminChecker.IsAdmin(ctx, claims.UserID)` per the path-based
    check from §"Live admin via middleware injection" and overwrite
    `claims.IsPlatformAdmin` with the live value.
- `platform/internal/auth/middleware_test.go`:
  - Add `TestRequireAuth_BridgeSession_PreferredOverJWE` — both
    cookies present, bridge.session wins.
  - Add `TestRequireAuth_BridgeSession_InvalidReturns401` — invalid
    bridge.session → 401 unconditionally, even when JWE is also
    present and valid (downgrade-attack defense).
  - Add `TestRequireAuth_BridgeSession_AbsentFallsBackToJWE` —
    cookie not set at all → JWE legacy path runs.
  - Add `TestRequireAuth_BridgeSession_FlagOff_IgnoresBridgeSession`
    — when `BRIDGE_SESSION_AUTH=0`, bridge.session is ignored even
    when present.
  - Add `TestRequireAuth_LiveAdminInjection_PromotesViaDB` — DB says
    admin, JWT says non-admin → handler sees admin.
  - Add `TestRequireAuth_LiveAdminInjection_DemotesViaDB` — DB says
    non-admin, JWT says admin → handler sees non-admin.
  - Add `TestRequireAuth_LiveAdminInjection_DBErrorFailsClosed` — DB
    lookup errors → claims.IsPlatformAdmin = false (fail-closed).
  - Add `TestRequireAuth_LiveAdminInjection_DeletedUserFailsClosed`
    — user row missing (deleted) → IsPlatformAdmin = false.
- `platform/cmd/api/main.go` — pass `Middleware` instance to all
  handler `Routes(...)` calls; replace `auth.RequireAdmin` with
  `mw.RequireAdmin`. Audit:
  ```bash
  grep -rn "auth.RequireAdmin\b" platform/ --include='*.go'
  ```
  All hits must be either method calls on `mw` or test fixtures.

**Verify (no code change but explicit):**
- All ~80 reads of `claims.IsPlatformAdmin` in
  `platform/internal/handlers/*.go` continue to read the live value
  without modification, because middleware overwrote the claim
  before the handler ran. Phase 3 includes a self-review checklist:
  1. Run `grep -rn "claims.IsPlatformAdmin\|c\.IsPlatformAdmin"
     platform/internal/handlers/*.go | wc -l` and confirm count
     unchanged (no accidental deletions).
  2. Sample 5 randomly-chosen sites; for each, trace from the route
     mounting in `cmd/api/main.go` through `RequireAuth` and confirm
     the claim is the live-injected value.
  3. Add an integration test
     `platform/internal/handlers/admin_live_admin_test.go` that
     creates two requests against any handler reading
     `claims.IsPlatformAdmin`: one with `is_platform_admin=true`
     in DB, one with `false`. The handler's behavior must match the
     DB row, not the JWT-carried value.

### Phase 4 — Next.js stops trusting Auth.js token for `isPlatformAdmin`

**Modify (10 files):**

Replace every `session.user.isPlatformAdmin` read with a server-side
fetch helper. The helper calls `/api/me/identity` (already exists
and, post-Phase-3, returns the live-from-DB value). Cached on the
request via `React.cache` so SSR and route handlers share one call.

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

## Rejected alternatives

### Shared verification adapter (Go imports a TS module, or vice versa)

Codex pass-1 raised the question: instead of porting JWE decryption
into Go, could Go and Next share a single verification adapter?

**Rejected.** This keeps the Auth.js v5 JWE format as our internal
contract. The drift problem is the format itself — Auth.js can change
HKDF salt strings, claim names, expiration semantics, or cookie
encoding in any release, and any shared adapter has to track those
changes. We've already paid that cost three times in the last 30 days
(plans 050, 053, PR #103). Moving to a Bridge-issued JWT means our
contract is OUR JWT shape, which we control end-to-end.

### Mint via Auth.js `events.signIn` cookie write

**Rejected.** Codex pass-1 flagged that `cookies().set()` from inside
`events.signIn` is unproven against the OAuth-callback redirect
response in `next-auth@5.0.0-beta.30`. There's also a real ordering
hazard with Auth.js's internal cookie-array building. The lazy
middleware mint sidesteps both concerns and gives us free refresh.

### Path-based skip for live-admin DB lookup

**Rejected** (Codex pass-2). An earlier draft of this plan kept a
`needsLiveAdminCheck(path)` skip list to avoid the DB lookup on
"non-admin" paths. Codex audit found `claims.IsPlatformAdmin` reads
in *virtually every handler file* — `/api/classes`, `/api/submissions`,
`/api/problems`, `/api/test-cases`, `/api/attempts`, `/api/units`,
`/api/collections`, `/api/me`, `/api/documents`, `/api/annotations`,
`/api/ai`, `/api/parent`, `/api/teacher`, plus the originally-listed
admin/orgs/courses/sessions/topics/assignments. A skip list that
needs to enumerate "every authenticated path" is identical to no
skip list at all, but with maintenance overhead. Drop the skip;
always do the lookup. Cost is one indexed PK query per
authenticated request — same as plan 050. Unauthenticated traffic
takes zero DB cost since it never enters the post-token-verify
block.

### Asymmetric JWT (RS256 / EdDSA) for `bridge.session`

**Rejected for now.** No third-party verifier today; HS256 with a
single shared secret is sufficient. Revisit when mobile clients or
external verifiers come online.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Mint endpoint is a new attack surface | medium | Bearer-protected via `BRIDGE_INTERNAL_SECRET`, server-to-server only, never exposed publicly. Same pattern as `/api/internal/realtime/auth`. Reject any request without the bearer with 401 BEFORE parsing the body. |
| Cookie size growth | low | HS256 JWT is ~250 bytes; roughly 0.3% of the 8KB cookie header limit. We have headroom. |
| Lazy mint adds latency to authenticated requests | low | Mint is a single localhost HTTP call (~5ms in dev, similar in prod when Next and Go are colocated). Only fires when cookie is missing or within 24h of expiry — at most once per 6 days for steady-state users. |
| Edge runtime restrictions break the mint | low | The mint helper uses only `fetch()` and base64 decoding — both fully Edge-supported. No DB calls, no Node-specific APIs. The Go mint endpoint runs in Node where DB access is fine. |
| Live admin DB lookup hammers Postgres | low | Same indexed PK lookup plan 050 already does on every authenticated request. The path-based skip removes the cost from non-admin-related traffic. Add a basic load test in Phase 3 that confirms p99 latency is unchanged. |
| Live-admin DB lookup adds latency to every authenticated request | low | Same indexed PK lookup plan 050 already does. Sub-millisecond. Add a basic load test in Phase 3 that confirms p99 latency is unchanged. |
| Auth.js v5 `req.auth` semantics in Edge | low | Verified by Codex pass-1: `req.auth` is the Edge-safe surface Auth.js v5 documents and uses internally. Phase 2's middleware integration test exercises this directly. |
| Mint failure on first request after sign-in | medium | Helper fails closed: if Go is unreachable, no cookie is set and Go middleware falls back to JWE (legacy path is still active during rollout). User experience identical to today. |
| Cross-domain cookies in prod | medium | If Bridge ever splits Next and Go onto different hostnames, `bridge.session` would need a `Domain=` attribute. Today Bridge runs on a single domain via Next's proxy; verified in `next.config.ts` rewrites. Document this as a Future Work note. |
| Test database parity | low | All new Go tests follow the existing pattern (`TEST_DATABASE_URL`, store integration tests). No new harness needed. |

## Phases

### Phase 0: Pre-impl Codex review of THIS plan (current pass)

Per CLAUDE.md plan review gate. Dispatch `codex:codex-rescue` to
review against the context anchors enumerated below. Iterate until
both Claude and Codex concur. Capture the verdict inline in
`## Codex Review of This Plan` below.

Specific questions for Codex pass-2 (since pass-1 already raised the
big six):

1. Does the lazy-middleware-mint approach close the "Set-Cookie
   reliability" question? Specifically: does
   `NextResponse.cookies.set` from inside an `auth(...)` wrapper in
   `src/middleware.ts` reliably attach to the response that goes
   back to the browser, including for the very first request after
   the OAuth callback redirect?
2. Is the path-based skip in `needsLiveAdminCheck` exhaustive for
   the current handler grep? List any handler file that reads
   `claims.IsPlatformAdmin` whose path is NOT in the proposed skip
   list.
3. Is the DI restructure of `Middleware` (adding `AdminChecker`,
   `BridgeSecret`, `BridgeAuthOn` fields) the right shape, or is
   there a cleaner way to plumb them?
4. Is there any race between Phase 3's middleware overwriting
   `claims.IsPlatformAdmin` and `claims` being passed to a handler
   that later reads it? (Go's `http.Handler` chain is sequential
   per-request, so I don't think so, but please verify.)
5. The "stale bridge.session with valid JWE → fall back to JWE"
   special case in Phase 3. Is this safe, or does it create a
   downgrade attack surface where an attacker with a stolen JWE
   could pair it with a forged-but-invalid bridge.session to bypass
   the bridge-session check? (My read: no, because the JWE is the
   strictly stronger credential — if it verifies, the user is
   authenticated; the bridge.session "downgrade" is just falling
   back to today's behavior. But please sanity-check.)
6. Anything else surprising in the revised plan that pass-1 didn't
   surface.

### Phase 1: Go mint infrastructure (PR 1)

- Implement `bridge_session.go` + tests.
- Implement `admin_check.go` + tests.
- Implement `internal_sessions.go` handler + tests.
- Wire under `/api/internal/sessions` in `cmd/api/main.go`.
- Promote `RequireAuth`/`RequireAdmin`/`OptionalAuth` to methods on
  `Middleware`. Update all callsites in `cmd/api/main.go`.
- Add env vars to `.env.example`.
- Update `next.config.ts` comment confirming `/api/internal/*` is
  not browser-proxied.
- `cd platform && go test ./... -count=1 -timeout 120s` clean.
- Self-review the diff.
- Codex post-impl review of phase 1.
- PR + merge with `BRIDGE_SESSION_AUTH=0` (no behavior change for
  users yet — endpoint exists but unused).

### Phase 2: Lazy mint in Edge middleware (PR 2)

- Implement `bridge-session-mint.ts` + unit tests.
- Wrap `src/middleware.ts` with the lazy-mint logic.
- Extend the matcher list in `src/middleware.ts` AND
  `src/lib/portal/middleware-matcher.ts`.
- Add `tests/unit/middleware-bridge-session.test.ts` integration
  test (mocked Go mint call).
- Add `tests/unit/middleware-proxy-parity.test.ts` parity test
  asserting middleware matcher is a strict superset of
  `next.config.ts:GO_PROXY_ROUTES`.
- **Auth.js cookie-attach integration test** (Codex pass-2 mandate
  for Q1): add `e2e/bridge-session-mint.spec.ts` that drives a
  real OAuth callback (via the Auth.js test stub or a mocked
  provider) and asserts the response back to the browser carries
  BOTH `authjs.session-token` AND `bridge.session` Set-Cookie
  headers. This proves that `NextResponse.cookies.set` from the
  `auth(...)`-wrapped middleware reliably attaches Set-Cookie even
  on the first post-OAuth response. If this test cannot pass, the
  whole approach pivots — without it, Q1 remains unproven.
- `bun run test` + `bun run test:e2e` (the new spec) clean.
- Self-review.
- Codex post-impl review of phase 2.
- PR + merge with `BRIDGE_SESSION_AUTH=0` (cookie is now being set
  on every authenticated request but Go is still verifying JWE —
  cookie is dormant).
- Manual smoke in dev: sign in, inspect browser dev tools to confirm
  `bridge.session` is set after the first portal request.

### Phase 3: Go middleware reads bridge.session + injects live admin (PR 3)

- Add `readBridgeSessionToken` helper.
- Add flag-gated branching in `RequireAuth` / `OptionalAuth`.
- Add live-admin injection in middleware (path-based skip + DB
  lookup + claim overwrite).
- Add path-skip-list audit test.
- Extend middleware tests (8+ new cases per Files §Phase 3).
- `cd platform && go test ./...` clean.
- Self-review.
- Codex post-impl review.
- PR + merge with `BRIDGE_SESSION_AUTH=0` (default — flag drives
  whether bridge.session is read, but live-admin injection is ON
  unconditionally; the latter is the plan-050 successor and has no
  back-compat concern).
- Manual smoke in dev: flip flag locally, sign in, hit `/admin/stats`,
  inspect `/api/auth/debug` to confirm both layers agree. Then
  manually `UPDATE users SET is_platform_admin=false WHERE
  email='m2chrischou@gmail.com'` in dev DB and confirm next
  `/admin/stats` request 403s without re-signing-in (live admin
  works).

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
   present in browser devtools. Verify live-admin demote/promote works
   without re-sign-in.
2. **Staging**: deploy with flag OFF. Confirm no regressions (Go still
   verifies JWE — pure backward-compat). Then flip flag to ON. Soak
   24h. Watch error rates on `/api/internal/sessions`.
3. **Production**: deploy with flag OFF. Soak 24h (forces both teams
   to confirm new code is healthy without auth changes). Flip flag to
   ON. Soak 7 days.
4. **Cleanup**: open phase 5 PR.

## Codex Review of This Plan

### Pass 1 — 2026-05-03: BLOCKED → all 6 findings folded in

Codex flagged 6 substantive blockers + 3 non-blocking concerns. All
addressed in this revision:

1. **`events.signIn` cookie write was unproven** (Q1+Q2+Q6) → pivoted
   approach to lazy mint in Edge middleware, which uses the documented
   `NextResponse.cookies.set` API and Auth.js v5's `req.auth`
   surface. New §"Why lazy middleware mint" explains the choice.
2. **Phase 4's enumerated list missed Go-side reads of
   `claims.IsPlatformAdmin`** (Q3) — there are ~80 reads scattered
   across handler files, not just `RequireAdmin`. Resolved by adding
   middleware-layer claim injection (§"Live admin via middleware
   injection") so every existing read becomes automatically live.
3. **Shared-adapter alternative not explicitly rejected** (Q4) — added
   to new §"Rejected alternatives" with rationale.
4. **`auth.RequireAdmin` package-level lacked DI** (Q5) — restructured
   to method on `Middleware` struct holding `AdminChecker`,
   `BridgeSecret`, `BridgeAuthOn`. Phase 1 includes the rename of
   all callsites.
5. **Session refresh underspecified** — addressed by lazy-middleware
   approach: same logic that initial-mints also re-mints when
   approaching expiry. No separate refresh route.
6. **`BRIDGE_INTERNAL_SECRET` config wiring not explicit** — added
   as a new env var distinct from `HOCUSPOCUS_TOKEN_SECRET`. Phase 1
   adds it to `.env.example` with a comment.
7. **Auth.js version pinning** (non-blocking) — pinned in §Status:
   `next-auth@5.0.0-beta.30`, `@auth/core@0.41.1`.
8. **`/api/internal/*` proxy behavior** (non-blocking) — added Phase 1
   step that updates `next.config.ts` comment to confirm browser
   requests to `/api/internal/*` are NOT proxied.

### Pass 2 — 2026-05-03: BLOCKED → 2 blockers + 3 important folded in

Codex pass-2 returned BLOCKED with two structural issues. Both addressed:

1. **`needsLiveAdminCheck` skip list incomplete (Q2 + Section 1 #1)**
   — Codex audited every handler file and found `claims.IsPlatformAdmin`
   reads at paths the original skip list missed: `/api/classes`,
   `/api/submissions`, `/api/problems`, `/api/test-cases`,
   `/api/attempts`, `/api/units` (NOT `/api/teaching-units` — Codex
   also caught the path-name error), `/api/collections`, `/api/me`,
   `/api/documents`, `/api/annotations`, `/api/ai`, `/api/parent`,
   `/api/teacher`. Resolution: drop the skip list entirely. Always
   do the live-admin DB lookup for any authenticated request. See
   §"Live admin via middleware injection" updated pseudocode and
   §"Rejected alternatives" updated rationale.

2. **JWE downgrade on invalid `bridge.session` (Q5 + Section 1 #2)**
   — original Phase 3 said "stale bridge.session with valid JWE →
   fall back to JWE". Codex correctly flagged this as a downgrade
   attack surface once Bridge sessions become authoritative
   (revocation/rotation). Resolution: invalid `bridge.session` is
   ALWAYS 401, regardless of JWE. Only ABSENT `bridge.session`
   falls back to JWE during rollout. Updated Phase 3 §`RequireAuth`
   logic.

Important non-blocking concerns folded in:

3. **Q1 cookie-attach reliability still unproven** — added a
   mandatory e2e test in Phase 2 (`e2e/bridge-session-mint.spec.ts`)
   that asserts both Auth.js and `bridge.session` Set-Cookie headers
   are present on the response after OAuth callback. If this test
   can't pass, the approach pivots.

4. **DI construction order** — Phase 1 now explicitly states the
   `cmd/api/main.go:55-64` reorder: build stores BEFORE constructing
   `auth.Middleware`, so `AdminChecker` can be passed at construction
   time.

5. **Config wiring** — Phase 1 adds explicit
   `platform/internal/config/config.go` field additions for
   `BridgeSessionSecret`, `BridgeInternalSecret`, `BridgeSessionAuth`.
   Production startup guard rejects empty secrets when flag is on.

Confirmed by Codex (no resolution needed):

- `auth.Middleware` is already a struct with method-based
  `RequireAuth`/`OptionalAuth` (pass-2 Q3 confirmed).
- No race between middleware claim overwrite and handler read
  (pass-2 Q4 confirmed — Go's http.Handler chain is sequential).
- `BRIDGE_INTERNAL_SECRET` doesn't conflict with existing env vars
  (pass-2 Section 2 #3 confirmed).

### Pass 3 — pending re-dispatch

Will dispatch after this commit lands. Pass-3 questions: confirm
the always-DB-lookup approach is actually safe for production load,
confirm the absent-vs-invalid `bridge.session` distinction in Phase 3
is robust, confirm the e2e test approach is feasible given Auth.js
v5's test stubbing surface.

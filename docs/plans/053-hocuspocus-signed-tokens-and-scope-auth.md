# Plan 053 — Hocuspocus signed tokens + per-scope authorization (P0)

## Status

- **Date:** 2026-05-01
- **Severity:** P0 (token forgery, full WebSocket bypass)
- **Origin:** Reviews `008-...:9-15`, `008-...:17-23`, `009-...:17-23`. Plus the deferred-but-unfiled gates from plan 030b/030c/035.

## Problem

The Hocuspocus server treats the client-supplied connection token as a plain `userId:role` string and trusts whatever the browser sends.

| Layer | File:Line | Failure mode |
|---|---|---|
| Token construction (session teacher) | `src/components/session/teacher/teacher-dashboard.tsx:135` | `${userId}:teacher` literal |
| Token construction (session student) | `src/components/session/student/student-session.tsx:49` | `${userId}:user` literal |
| Token construction (parent viewer) | `src/components/parent/live-session-viewer.tsx:31` | `${parentId}:parent` literal |
| Token construction (problem attempt — student) | `src/components/problem/problem-shell.tsx:64` | `${userId}:user` literal |
| Token construction (problem attempt — teacher watch) | `src/components/problem/teacher-watch-shell.tsx:55` | `${teacherId}:teacher` literal — **6th site, missed in earlier draft** |
| Token construction (unit editor) | `src/lib/yjs/use-yjs-tiptap.ts:99` | `${userId}:teacher` literal |
| Token validation | `server/hocuspocus.ts:20-23` | `token.split(":")` — no signature, no DB check |
| Scope branching | `server/hocuspocus.ts:46-90` | Trusts the parsed `role` for `session:*`, `attempt:*`, `broadcast:*`, `unit:*` rooms |

**Document name format** (for the JWT scope claim): the actual Hocuspocus doc names are NOT one-flat-id-per-scope. They are:

- session: `session:{sessionId}:user:{studentId}` (one doc per student per session — confirmed at `server/hocuspocus.ts:46-47`)
- broadcast: `broadcast:{sessionId}` (teacher's broadcast doc)
- attempt: `attempt:{attemptId}` (one doc per attempt)
- unit: `unit:{unitId}` (one doc per unit)

The JWT's `scope` claim must be the **exact `documentName` string** the client will request, not a shortened scope-only key. `onAuthenticate` compares `claims.scope === documentName` byte-for-byte. The mint endpoint accepts the full doc-name from the caller and gates each path through the corresponding HTTP-side helper before signing.

A malicious client connects to `ws://hocuspocus/`, sends `{ token: "<victim-uuid>:teacher", documentName: "unit:<any-uuid>" }`, and gets read+write access to that document. Or `attempt:<victim-attempt-uuid>` works the same way. The forged role bypasses the role gating at lines 48-51.

Plans 030b and 030c promised to gate session-room access through the same class-membership-or-token rules as HTTP. Plan 035 explicitly deferred per-unit `canEditUnit` validation. Neither shipped.

## Out of scope

- Replacing Hocuspocus with a different realtime layer.
- Changing the Yjs document shape.
- Hocuspocus persistence (still in-memory; tracked separately in `TODO.md`).

## Approach

Two layers, both required:

1. **Sign the token.** Replace `userId:role` with a short-lived JWT minted by the Go API after a normal authenticated request. Sign with HMAC-SHA256 using a new `HOCUSPOCUS_TOKEN_SECRET` env var (separate from `NEXTAUTH_SECRET` so a leak of the WebSocket signing key doesn't compromise sessions). The token claim set:
   ```
   { sub: userId, role: "teacher"|"user"|"parent", scope: "<exact-documentName>", iat, exp }
   ```
   The `scope` claim is the **full Hocuspocus documentName** (`session:{sessionId}:user:{studentId}`, `attempt:{attemptId}`, `unit:{unitId}`, `broadcast:{sessionId}`) — see the doc-name format note above. `exp` ≤ 30 minutes; the client refreshes via a new `POST /api/realtime/token` endpoint.
2. **Verify on connect AND on document open.** `server/hocuspocus.ts::onAuthenticate` verifies the JWT signature and rejects mismatched `documentName` (exact-string compare with the JWT's `scope`). `onLoadDocument` does a defense-in-depth DB check via the Go API:
   - `session:{sessionId}:user:{studentId}` → caller passes `canJoinSession(sessionID)` AND (`sub == studentId` OR caller is the session's teacher/class-staff).
   - `attempt:{attemptId}` → owner-or-class-staff (new helper if needed).
   - `unit:{unitId}` → `canEditUnit` (existing helper).
   - `broadcast:{sessionId}` → class instructor of the session's class.

The Go-side helpers already exist for HTTP. Expose them via a single internal endpoint (`POST /api/internal/realtime/auth` taking `{documentName, sub}`) that Hocuspocus calls from Node; gate the endpoint to the Hocuspocus shared secret via `Authorization: Bearer <HOCUSPOCUS_TOKEN_SECRET>` so it isn't user-facing.

**Multi-pod / key rotation (Codex Phase 0 follow-up):** `HOCUSPOCUS_TOKEN_SECRET` is shared between Go and the Hocuspocus Node process. For multi-pod deployments, the secret must be sourced from the same secrets-manager entry by both. Initial implementation supports a single key; rotation is documented as a follow-up — when added, the JWT carries a `kid` header, the Hocuspocus verifier consults a small key map, and the Go side mints with the current key. Plan 053 ships single-key; key-rotation is a future plan (058+).

## Files

- Create: `platform/internal/handlers/realtime_token.go` — `POST /api/realtime/token` handler that mints a Hocuspocus JWT for the authenticated caller, scoped to a single `documentName` passed in the body. Verifies the caller can access that doc-name before signing (uses the same per-scope checks as the internal auth endpoint).
- Create: `platform/internal/handlers/realtime_auth.go` — internal-only `POST /api/internal/realtime/auth` (POST not GET because the body carries `documentName + sub` not query params) called by Hocuspocus during `onLoadDocument`. Gated by `Authorization: Bearer <HOCUSPOCUS_TOKEN_SECRET>`.
- Modify: `next.config.ts:10-36` — add a rewrite for `/api/realtime/:path*` so the client-side `POST /api/realtime/token` proxies to Go (currently absent — Phase 3 client fetch would 404 without it).
- Modify: `server/hocuspocus.ts` — verify JWT in `onAuthenticate`, call internal auth endpoint in `onLoadDocument`, reject on mismatch.
- Modify: every token-construction site listed above (six files) — fetch a token from `/api/realtime/token` instead of building the raw string. Cache token in-memory client-side; refresh on `exp - 60s`. Implement as a small shared helper `src/lib/realtime/get-token.ts` so all six sites share one cache.
- Add: `HOCUSPOCUS_TOKEN_SECRET` to `.env.example`, `docs/setup.md`, and the dev runbook.

**Test infrastructure (Codex Phase 0 follow-up):** the codebase has no Hocuspocus test harness today — neither vitest nor Go tests exercise the live WebSocket path. Plan 053 introduces it incrementally per phase (see the Phases section below for which test lands in which phase). Net additions across the four phases:
- A small Go in-process JWT verifier helper so subsequent phases can reuse parsing without booting Hocuspocus (Phase 1).
- `platform/internal/handlers/realtime_token_test.go` (Phase 1).
- `platform/internal/handlers/realtime_auth_test.go` (Phase 1).
- `tests/integration/realtime-token-mint.test.ts` — vitest end-to-end for `POST /api/realtime/token` (Phase 2).
- `e2e/hocuspocus-auth.spec.ts` — Playwright spec for the live mint → connect → verify path (Phase 3).

Update: `TODO.md:9` (Hocuspocus auth note).

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Token-mint endpoint adds latency to every realtime connect | low | Cache token in-memory client-side; refresh on `exp - 60s`. |
| Internal auth endpoint becomes a bypass surface | medium | Bearer-token gate + bind to a dedicated server-only port if needed. Document that the secret is environment-only, never client-shipped. |
| Existing in-flight sessions break when this rolls out | medium | Ship behind a feature flag (`HOCUSPOCUS_REQUIRE_SIGNED_TOKEN=1`). Stage off in dev first; flip on after a soak. |
| Parent viewer flow is fragile already (read-only stream) | low | Token claim has `role: "parent"` and the Go side returns the same `canJoinSession` answer; existing parent tests cover the flow. |

## Phases

### Phase 0: pre-impl Codex review

Dispatch `codex:codex-rescue` on this plan focusing on (a) the JWT claim shape's compatibility with the existing Hocuspocus extension, (b) the internal-auth endpoint's bearer-token gate, (c) the feature-flag rollout. Capture verdict below. Iterate until concur.

### Phase 1: server-side mint + verify (Go)

- Implement `POST /api/realtime/token` and `POST /api/internal/realtime/auth` (POST on both — auth endpoint takes `{documentName, sub}` in the body, not query params).
- Add HMAC signing helpers reusing `golang-jwt/jwt/v5`.
- Tests added in this phase:
  - `platform/internal/handlers/realtime_token_test.go` — happy path + 403 for unauthorized scope + 401 for missing claims + boundary cases per doc-name shape (session/attempt/unit/broadcast).
  - `platform/internal/handlers/realtime_auth_test.go` — Bearer-token gate + per-doc-name dispatch + signature verification.
  - A small Go in-process JWT verifier helper so subsequent phases can reuse the parsing without booting Hocuspocus.
- Commit + push (no client changes yet — feature flag off).

### Phase 2: server-side verify (Hocuspocus)

- Update `server/hocuspocus.ts` to verify JWT signature when `HOCUSPOCUS_REQUIRE_SIGNED_TOKEN=1`.
- Defense-in-depth `onLoadDocument` DB check via the new internal endpoint.
- **Backward-compat parsing (Codex Phase 0 follow-up):** with the flag OFF, the parser must accept BOTH the legacy `userId:role` shape (current behavior) AND a signed JWT (newly-deployed clients during the rollout window). Detect by string shape: if the token starts with `ey` (base64-encoded JWT header `{"alg":"HS256"}` always starts with `ey`) treat as JWT and verify; otherwise fall back to `split(":")`. With the flag ON, rejecting any unsigned token is unconditional.
- Tests added in this phase:
  - `tests/integration/realtime-token-mint.test.ts` — vitest end-to-end for `POST /api/realtime/token` (mocked Go via existing Go-proxy stub).

### Phase 3: client-side token fetch

- Add `src/lib/realtime/get-token.ts` — small shared helper that posts to `/api/realtime/token` with the desired `documentName`, caches the token in-memory, and refreshes on `exp - 60s`. Single source of truth so all six sites share one cache.
- Replace each of the six token-construction sites with a call to the helper.
- Tests added in this phase:
  - `e2e/hocuspocus-auth.spec.ts` — Playwright spec hitting the live Hocuspocus server with (a) a forged token (expect close), (b) a valid token (expect open), (c) an expired token (expect close). Integration ratchet for the full mint → connect → verify path.

### Phase 4: enable the flag in dev → staging → prod, retire the legacy path

- Soak in dev for 24h with `HOCUSPOCUS_REQUIRE_SIGNED_TOKEN=1`.
- Enable in staging.
- After a clean week in staging, enable in prod.
- Remove the unsigned fallback parser branch (and the `ey`-prefix sniffing) in a follow-up commit. Bump the e2e spec to assert unsigned tokens are unconditionally rejected.

## Codex Review of This Plan

### Pass 1 — 2026-05-01: BLOCKED → fixes folded in

Codex Phase 0 review found 3 blockers + 3 [IMPORTANT] items, all addressed inline:

- `[CRITICAL]` Missing 6th token construction site at `src/components/problem/teacher-watch-shell.tsx:55`. **Fix:** added to the surface table.
- `[IMPORTANT/blocker]` Doc-name format mismatch — actual session doc names are `session:{sessionId}:user:{studentId}` not `session:{sessionId}`. **Fix:** Approach section now spells out the four exact doc-name shapes; JWT `scope` is the full doc-name string, byte-for-byte compared against `documentName`.
- `[IMPORTANT/blocker]` `next.config.ts` missing `/api/realtime/:path*` rewrite — Phase 3 client fetch would 404. **Fix:** Files section now lists the rewrite addition.
- `[IMPORTANT]` Phase 3 backward-compat: legacy `split(":")` can't parse a JWT during the rollout window. **Fix:** Phase 2 now describes a shape-sniffing parser (JWT starts with `ey`; else legacy split) for the flag-off period.
- `[IMPORTANT]` Multi-pod `HOCUSPOCUS_TOKEN_SECRET` rotation. **Fix:** Approach section documents single-key initial impl + `kid`-based rotation as a follow-up plan.
- `[IMPORTANT]` E2E test harness for Hocuspocus doesn't exist. **Fix:** Files section now describes the test infrastructure landed across Phases 1–3: Go in-process verifier helper + per-handler tests in Phase 1, vitest mint integration test in Phase 2, Playwright Hocuspocus auth e2e in Phase 3. Each phase's bullet list calls out which tests it adds.

**Status:** Pass-2 dispatch will confirm the resolutions land cleanly.

### Pass 5 — 2026-05-01: **PHASE-0 CONCUR**

After Passes 2-4 caught (a) test-harness placement inconsistency, (b) `GET` vs `POST` mismatch on the internal endpoint, (c) leftover "Phase 2" test-harness attribution, and (d) a stale-snapshot false alarm about "replay rejection," Pass 5 returned a clean concur.

**Status: ready for Phase 1 implementation** (server-side mint + verify in Go). PR-1 of plan 053 starts on a fresh branch.

---

## Phase 1 Post-Implementation Review (2026-05-02)

Phase 1 shipped on `fix/053-1-realtime-token-mint`. Three commits total:
`265432a` (initial), `d172c91` (pass-1 fixes), `cb91839` (pass-2 status-code fix).

### Pass 1 — 5 blockers, all fixed in `d172c91`

1. **Internal endpoint behind RequireAuth** — `/api/internal/realtime/auth`
   was registered inside the user-auth group, so the server-to-server
   callback got 401'd before its bearer-token check could run. **Fix:**
   split route registration: `Routes()` for the public mint endpoint
   stays under `RequireAuth`, `InternalRoutes()` registers the bearer-
   gated callback at the top level.

2. **Attempt scope had admin/impersonator bypass** — Phase 1 promised
   strict owner-only for attempt docs (teacher-watch path is deferred
   to Phase 2 alongside attempt → class resolution). **Fix:** removed
   the bypass. Test now asserts both admin and impersonator are
   denied.

3. **Broadcast scope was broader than the REST gate** —
   `authorizeBroadcastDoc` granted access to any class staff with
   `AccessMutate`, but `SessionHandler.ToggleBroadcast` only allows
   platform admin OR the session's teacher. **Fix:** dropped the
   class-staff path. New test `TestMintToken_BroadcastDoc_OrgAdminDenied`
   locks the parity down.

4. **Internal endpoint synthetic claims didn't carry admin status** —
   With the route split (blocker 1), the internal endpoint runs outside
   user auth, so it got only `body.Sub`. An admin demoted between mint
   and recheck would still pass because synthetic claims had
   `IsPlatformAdmin: false`. **Fix:** added `Users` field to
   `RealtimeHandler` and rebuild claims via `Users.GetUserByID`,
   reading `is_platform_admin` from the DB. `ImpersonatedBy`
   intentionally NOT rehydrated — impersonation is a session-level
   superpower; the recheck should evaluate the underlying user's
   actual permissions. New test
   `TestInternalAuth_RehydratesPlatformAdminFromDB` verifies it.

5. **docs/setup.md path mismatch** — doc said
   `/api/internal/realtime/authorize`; actual path is `/auth`. **Fix:**
   doc corrected and amended with the mount-location rationale.

### Pass 2 — 1 blocker, fixed in `cb91839`

1. **InternalAuth collapsed all errors into 200/Allowed:false** —
   Every `authDecision` failure (400 malformed doc-name, 404 missing
   resource, 500 DB error) turned into "200 + Allowed:false", hiding
   infrastructure problems as ordinary auth denials. **Fix:** split
   dispatch by `decision.Status`: only `403 Forbidden` collapses to
   `200 Allowed:false`; everything else surfaces via `writeError`.
   User-not-found is now `404` (not `200/Allowed:false`). Four new
   tests cover the paths:
   `TestInternalAuth_{UnknownSub_404, BadDocName_400,
   MissingResource_404, NilUsersStore_500}`.

### Pass 3 — **CONCUR**

> All four verification points confirmed: status dispatch correct,
> leaked-info acceptable (bearer gate filters non-Hocuspocus
> callers), test coverage sufficient, no regression on pass-1/2
> fixes. Phase 1 is clear to merge.

Two non-blocking gaps Codex noted: no test for `GetUserByID`
returning a non-nil error (vs nil result) and no test for
`authorizeDocument` returning `StatusInternalServerError`. Both are
exercised at the dispatcher level by `TestInternalAuth_NilUsersStore_500`;
producer-level coverage is belt-and-suspenders. Filed as a Phase 1
follow-up if a future regression demands it.

### Final test counts

- `platform/internal/auth`: 8 unit tests on `SignRealtimeToken` /
  `VerifyRealtimeToken` (round-trip, wrong secret, wrong issuer,
  expired, malformed, TTL clamp, ey prefix lock-in).
- `platform/internal/handlers`: 22 mint/internal-auth tests
  (auth required, all 4 doc-name shapes, owner/teacher/admin/
  impersonator matrices, bearer gate, status-code dispatch, DB
  rehydrate, missing user/resource).
- Full Go suite: green.

**Phase 1 status: ready to merge.** Phase 2 (server-side verify in
Hocuspocus + backward-compat parser) is the next plan-053 unit.

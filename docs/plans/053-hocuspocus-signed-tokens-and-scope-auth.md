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
| Token construction (problem attempt) | `src/components/problem/problem-shell.tsx:64` | `${userId}:user` literal |
| Token construction (unit editor) | `src/lib/yjs/use-yjs-tiptap.ts:99` | `${userId}:teacher` literal |
| Token validation | `server/hocuspocus.ts:20-23` | `token.split(":")` — no signature, no DB check |
| Scope branching | `server/hocuspocus.ts:46-90` | Trusts the parsed `role` for `session:*`, `attempt:*`, `broadcast:*`, `unit:*` rooms |

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
   { sub: userId, role: "teacher"|"user"|"parent", scope: "session:<id>"|"unit:<id>"|"attempt:<id>"|"broadcast:<id>", iat, exp }
   ```
   `exp` ≤ 30 minutes; the client refreshes via a new `POST /api/realtime/token` endpoint.
2. **Verify on connect AND on document open.** `server/hocuspocus.ts::onAuthenticate` verifies the JWT signature and rejects mismatched `documentName`/`scope`. `onLoadDocument` does a defense-in-depth DB check via the Go API:
   - `session:<id>` → `canJoinSession` (existing helper)
   - `unit:<id>` → `canEditUnit` (existing helper)
   - `attempt:<id>` → owner-or-class-staff (new helper if needed)
   - `broadcast:<id>` → class instructor

The Go-side helpers already exist for HTTP. Expose them via small read-only endpoints (`GET /api/internal/realtime/auth?scope=...&user=...`) that Hocuspocus calls from Node; gate those endpoints to the Hocuspocus secret so they aren't user-facing.

## Files

- Create: `platform/internal/handlers/realtime_token.go` — `POST /api/realtime/token` handler that mints a Hocuspocus JWT for the authenticated caller, scoped to a single document name passed in the body. Verifies the caller can access that scope before signing.
- Create: `platform/internal/handlers/realtime_auth.go` — internal-only `GET /api/internal/realtime/auth` that the Hocuspocus server calls during `onLoadDocument`. Gated by `Authorization: Bearer <HOCUSPOCUS_TOKEN_SECRET>`.
- Modify: `server/hocuspocus.ts` — verify JWT in `onAuthenticate`, call internal auth endpoint in `onLoadDocument`, reject on mismatch.
- Modify: every token-construction site listed above — fetch a token from `/api/realtime/token` instead of building the raw string.
- Add: `HOCUSPOCUS_TOKEN_SECRET` to `.env.example`, `docs/setup.md`, and the dev runbook.
- Tests:
  - `platform/internal/handlers/realtime_token_test.go` — happy path + 403 for unauthorized scope + 401 for missing claims.
  - `platform/internal/handlers/realtime_auth_test.go` — signature gate + scope checks.
  - New e2e: `e2e/hocuspocus-auth.spec.ts` — forged token rejected; valid token accepted; expired token rejected.
- Update: `TODO.md:9` (Hocuspocus auth note).

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

- Implement `POST /api/realtime/token` and `GET /api/internal/realtime/auth`.
- Add HMAC signing helpers reusing `golang-jwt/jwt/v5`.
- Tests: token-mint, internal-auth verification, expiration, replay rejection.
- Commit + push (no client changes yet — feature flag off).

### Phase 2: server-side verify (Hocuspocus)

- Update `server/hocuspocus.ts` to verify JWT signature when `HOCUSPOCUS_REQUIRE_SIGNED_TOKEN=1`.
- Defense-in-depth `onLoadDocument` DB check.
- Backward-compat: with the flag OFF, old behavior (warn but allow). With ON, reject unsigned tokens.

### Phase 3: client-side token fetch

- Replace each of the five token-construction sites with a call to `/api/realtime/token` (cached client-side).
- e2e regression.

### Phase 4: enable the flag in dev → staging → prod, retire the legacy path

- Soak in dev for 24h.
- Enable in staging.
- After a clean week in staging, enable in prod and remove the unsigned fallback.

## Codex Review of This Plan

(Filled in after Phase 0.)

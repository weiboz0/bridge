# Plan 081 — Realtime-config health check + smoke

## Problem

Browser review 011 flagged live collaboration as a P0 release blocker because every live collaboration surface reported `Live collaboration is unavailable` and token minting failed with `HOCUSPOCUS_TOKEN_SECRET` unset.
The environment fix is to set the same `HOCUSPOCUS_TOKEN_SECRET` for the Go API and Hocuspocus processes, but operators currently do not have a single visible health surface that distinguishes Go reachability, realtime token configuration, Hocuspocus expectations, and Bridge session auth configuration.

Plan 078 already changed the realtime E2E token checks so a `/api/realtime/token` 503 is a hard failure rather than a skip.
Plan 081 adds the remaining code work from `TODO.md`: `/api/health/realtime` plus an in-app operator indicator.

## Scope

Add a Go-owned public health endpoint exposed through Next at `/api/health/realtime`.
Render that health state on the platform-admin dashboard.
Keep this as an operator/configuration diagnostic, not a replacement for authenticated realtime token mint authorization.

Out of scope:

- Setting deploy secrets.
- Adding Hocuspocus persistence.
- Changing realtime token authorization.
- Starting browser infrastructure inside unit tests.

## Files

Modify:

- `platform/internal/handlers/realtime_token.go`
- `platform/internal/handlers/realtime_token_test.go`
- `platform/cmd/api/main.go`
- `next.config.ts`
- `src/app/(portal)/admin/page.tsx`
- `TODO.md`

Create:

- `docs/plans/081-realtime-health.md`
- `tests/unit/admin-realtime-health.test.ts`

## Phases

### Phase 1 — Go health endpoint

Add `GET /api/health/realtime` outside user auth.

Response shape:

```json
{
  "status": "ok" | "degraded",
  "goApi": { "status": "ok" },
  "realtime": {
    "tokenMinting": "ok" | "misconfigured",
    "hocuspocus": "requires_matching_secret" | "blocked",
    "hocuspocusTokenSecret": "set" | "missing",
    "hocuspocusProcess": "not_checked"
  },
  "bridgeSession": {
    "authFlag": "on" | "off",
    "secrets": "set" | "missing",
    "internalBearer": "set" | "missing"
  }
}
```

The endpoint must not expose secret values.
It reports whether Go is configured to mint realtime tokens and whether the Hocuspocus side must be configured with the matching shared secret.
It does not probe the separate Hocuspocus Node process.

Tests:

- `platform/internal/handlers/realtime_token_test.go`
  - missing realtime secret returns `status: degraded`, token minting `misconfigured`, Hocuspocus `blocked`
  - configured realtime secret returns `status: ok`, token minting `ok`, Hocuspocus `requires_matching_secret`
  - Bridge session auth flags are reported as set/missing without values
  - raw response body does not include configured secret values

### Phase 2 — Next proxy + admin indicator

Expose the Go endpoint through Next rewrites by adding `/api/health/:path*` to the Go proxy list.

Update `/admin` to fetch `/api/health/realtime` and render an operator health card:

- Healthy state: Go realtime token minting is ready and the Hocuspocus process must use the same `HOCUSPOCUS_TOKEN_SECRET`.
- Degraded state: show concise remediation text naming `HOCUSPOCUS_TOKEN_SECRET`.
- Fetch failure: show an operator-visible card saying realtime health could not be checked.

Tests:

- `tests/unit/admin-realtime-health.test.ts`
  - source-level guard that `/admin` fetches `/api/health/realtime`
  - degraded copy names `HOCUSPOCUS_TOKEN_SECRET`
  - healthy/degraded labels are present

### Phase 3 — Verify, review, and handoff

Run:

```bash
(cd platform && env GOCACHE=/tmp/magicburg-go-build-cache go test ./internal/handlers -count=1 -run TestRealtimeHealth)
/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run tests/unit/admin-realtime-health.test.ts
```

Then run broader useful checks:

```bash
(cd platform && env GOCACHE=/tmp/magicburg-go-build-cache go test ./internal/handlers -count=1)
/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/typescript/bin/tsc --noEmit
```

Record any existing baseline failures rather than hiding them.
Run mandatory external code review before shipping and resolve all `[OPEN]` findings.

## Code Review

External review completed with Codex, GLM 5.1, DeepSeek V4 Flash, and Kimi K2.6.
After resolving review findings, final GLM 5.1, DeepSeek V4 Flash, and Kimi K2.6 passes returned `ALLOW`.

- [RESOLVED] Hocuspocus readiness overclaim. The initial response/admin copy implied the Go endpoint could prove the Node process was configured. The contract now reports `hocuspocus: "requires_matching_secret"` and `hocuspocusProcess: "not_checked"`, and the admin copy says the Node process must run with the same `HOCUSPOCUS_TOKEN_SECRET`.
- [RESOLVED] Plan handoff sections were pending and `TODO.md` still showed Plan 081 open. This report records the review and execution evidence, and `TODO.md` is checked off.
- [RESOLVED] Admin health fetch failure rendered raw `Error.message`, which could expose an internal Go URL from a server-side network error. The health card now renders only `API <status>` for structured API errors or `request failed` for other failures.
- [RESOLVED] Tests did not assert raw configured values were absent from the health JSON. `TestRealtimeHealth_ConfiguredIsOK` now checks that the configured Go token, bridge-session secret, and internal bearer strings are not present in the body.
- [ACCEPTED] `/api/health/realtime` returns HTTP 200 for degraded configuration. This endpoint is an operator diagnostic; callers must parse the JSON `status` field. Generic liveness remains `/healthz`.
- [ACCEPTED] `/api/health/realtime` is public and exposes only coarse set/missing/on/off configuration state. This matches the plan scope for an operator-visible diagnostic and never returns secret values.
- [ACCEPTED] `/api/health/:path*` remains in middleware matcher lists. It preserves this repo's proxy/middleware parity checks; the Auth.js callback passes API paths through before user-page redirects.
- [ACCEPTED] Admin coverage remains a source-level guard. Rendering the server component would require broader Next server-component test harness work; the plan intentionally scoped this test to fetch path, labels, and remediation copy.
- [ACCEPTED] No live rewrite integration test was added. Existing parity tests verify the rewrite/matcher configuration, and the admin server component calls the Go API directly through `api()`.

## Post-execution Report

Implemented:

- Added Go `GET /api/health/realtime` through `RealtimeHandler.HealthRoutes`, mounted outside user auth.
- Added set/missing/on/off realtime and Bridge Session configuration reporting without exposing secret values.
- Added Next rewrite and middleware matcher parity for `/api/health/:path*`.
- Added a platform-admin realtime health card with healthy, degraded, and fetch-failure states.
- Added focused Go health tests and a source-level admin health guard.
- Marked Plan 081 complete in `TODO.md`.

Verification:

- RED: `/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run tests/unit/admin-realtime-health.test.ts` failed before implementation because `/admin` did not fetch or render realtime health.
- RED: `cd platform && env GOCACHE=/tmp/magicburg-go-build-cache go test ./internal/handlers -count=1 -run TestRealtimeHealth` failed before implementation because the health handler/types did not exist.
- PASS: `cd platform && env GOCACHE=/tmp/magicburg-go-build-cache go test ./internal/handlers -count=1 -run TestRealtimeHealth`.
- PASS: `/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run tests/unit/admin-realtime-health.test.ts`.
- PASS: `/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run tests/unit/middleware-proxy-parity.test.ts tests/unit/admin-realtime-health.test.ts`.
- PASS: `cd platform && env GOCACHE=/tmp/magicburg-go-build-cache go test ./internal/handlers -count=1`.
- PASS: `cd platform && env GOCACHE=/tmp/magicburg-go-build-cache go test ./... -count=1 -timeout 120s`.
- PASS: `env DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test /home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run` with escalated local database access: 85 files, 666 tests passed; 2 files and 11 tests skipped.
- BASELINE FAIL: `/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/typescript/bin/tsc --noEmit` still fails on existing unrelated errors in `src/app/(portal)/teacher/units/new/page.tsx`, `src/components/admin/user-actions.tsx`, and `tests/unit/identity-assert.test.ts`.

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
    "hocuspocus": "configured" | "blocked",
    "hocuspocusTokenSecret": "set" | "missing"
  },
  "bridgeSession": {
    "authFlag": "on" | "off",
    "secrets": "set" | "missing",
    "internalBearer": "set" | "missing"
  }
}
```

The endpoint must not expose secret values.
It reports whether Go is configured to mint realtime tokens and whether the Hocuspocus side can possibly be configured with the required shared secret.

Tests:

- `platform/internal/handlers/realtime_token_test.go`
  - missing realtime secret returns `status: degraded`, token minting `misconfigured`, Hocuspocus `blocked`
  - configured realtime secret returns `status: ok`, token minting `ok`, Hocuspocus `configured`
  - Bridge session auth flags are reported as set/missing without values

### Phase 2 — Next proxy + admin indicator

Expose the Go endpoint through Next rewrites by adding `/api/health/:path*` to the Go proxy list.

Update `/admin` to fetch `/api/health/realtime` and render an operator health card:

- Healthy state: realtime token minting and Hocuspocus shared secret are configured.
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
env GOCACHE=/tmp/magicburg-go-build-cache go test ./platform/internal/handlers -count=1 -run TestRealtimeHealth
/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run tests/unit/admin-realtime-health.test.ts
```

Then run broader useful checks:

```bash
env GOCACHE=/tmp/magicburg-go-build-cache go test ./platform/internal/handlers -count=1
/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/typescript/bin/tsc --noEmit
```

Record any existing baseline failures rather than hiding them.
Run mandatory external code review before shipping and resolve all `[OPEN]` findings.

## Code Review

Pending implementation.

## Post-execution Report

Pending implementation.

# Plan 072 — Realtime auth cutover completion

## Status

- **Date:** 2026-05-06
- **Origin:** Comprehensive architectural review 011 §1.1 (BLOCKER, Codex). Plan 053 introduced the JWT path for Hocuspocus realtime auth (Go-minted scoped tokens, doc-name-bound, with an in-Go `/api/internal/realtime/auth` recheck on document load) and a kill-switch flag `HOCUSPOCUS_REQUIRE_SIGNED_TOKEN=1` to cutover from the legacy `userId:role` string-token path. The flag was intended to be flipped to `1` after a compatibility window. **It was never flipped.** The legacy branches at `server/hocuspocus.ts:84-162` are still the production default and carry documented forged-token risks (`server/hocuspocus.ts:97, 104, 144, 147`).
- **Scope:** Make signed tokens the production default (invert the env flag), fail Hocuspocus startup when a signing secret is missing, delete the legacy auth branches outright. Single PR.

## Problem

Today's realtime-auth state in `server/hocuspocus.ts`:

| Layer | Status |
|---|---|
| Client mints JWT via `/api/realtime/token` | ✅ shipped (plan 053 phase 3) |
| All client callers use `useRealtimeToken` / `getRealtimeToken` | ✅ confirmed via `grep`: no surviving legacy `${userId}:${role}` string construction in `src/` |
| Hocuspocus accepts signed JWTs and rechecks via Go | ✅ shipped (plan 053 phase 2) |
| Hocuspocus signed-tokens-required flag exists | ✅ `HOCUSPOCUS_REQUIRE_SIGNED_TOKEN=1` |
| Hocuspocus signed-tokens-required is the production default | ❌ flag defaults to OFF — legacy path is the live default |
| Legacy `userId:role` parser branches deleted | ❌ ~80 lines still present |
| Hocuspocus boot fails without a signing secret in production | ❌ `TOKEN_SECRET` empty just disables the JWT path silently |

Concrete failure modes from the legacy code path that still ships:

1. **Forged session-doc takeover** (`server/hocuspocus.ts:97, 104, 111-117`): a forged `userId:role` token where `userId === docOwner` is accepted with no DB recheck. The TODO at line 95-110 explicitly documents this gap.
2. **Forged unit collaboration** (`server/hocuspocus.ts:144-156`): the `unit:*` legacy path checks only `role === "teacher"` — no per-unit org/ownership validation. Any forged token claiming `role: "teacher"` opens any unit document.
3. **Silent JWT disable** (`server/hocuspocus.ts:25, 56`): when `HOCUSPOCUS_TOKEN_SECRET` is empty, the JWT path throws "Realtime tokens not configured" — but only AFTER recognising a JWT. A legacy token with the secret unset and the require-flag unset slides through the legacy path instead.

The intent of plan 053 was to land the JWT path, run a compatibility window, then flip the flag and delete the legacy branches. Phase 4 of that plan was queued but never executed — and no client callers actually still need legacy.

## Out of scope

- **Schema migrations.** No DB changes.
- **Go-side token mint changes.** Plan 053 phase 1 already covers it; mint contract stays as-is.
- **Client-side fetch logic.** `getRealtimeToken` already implements caching, retry, and 503-as-unavailable surfacing.
- **E2E test coverage gaps** (review 011 §4.1) — separate plan 080.
- **Per-unit auth gate hardening** — replacing the `role === "teacher"` heuristic with a Go-side `canEditUnit` call is plan 053b material; this plan only deletes the still-legacy branch, leaving the JWT path's per-doc recheck (already implemented) as the load-bearing gate.

## Approach

Three small phases on a single branch:

### Phase 1 — Invert the env flag default + boot-time secret check

The current contract is opt-in cutover (`HOCUSPOCUS_REQUIRE_SIGNED_TOKEN=1` flips to JWT-only). Switch the default polarity: signed tokens are required UNLESS an explicit `HOCUSPOCUS_ALLOW_LEGACY_TOKEN=1` opt-out is set (and even then, only honored when `BRIDGE_HOST_EXPOSURE` is `local-only`, mirroring plan 068's startup guards). This:

1. Means new deployments are secure-by-default.
2. Keeps a small dev/test escape hatch that's gated by the same exposure-aware mechanism plan 068 introduced.
3. Adds a hard fail at boot when the secret is missing AND legacy is not explicitly allowed — exactly the case Codex flagged (the silent JWT-disable hole).

Specifically:
- Change `REQUIRE_SIGNED_TOKEN` to `ALLOW_LEGACY_TOKEN = process.env.HOCUSPOCUS_ALLOW_LEGACY_TOKEN === "1"`.
- New boot check (top of `server/hocuspocus.ts` server construction): if `!TOKEN_SECRET && !ALLOW_LEGACY_TOKEN`, log a fatal and `process.exit(1)`. If `ALLOW_LEGACY_TOKEN` is set in a non-`local-only` exposure, log a fatal and exit.
- Add an explicit startup log line stating which mode is active so ops can confirm at boot.

This phase is the minimum viable BLOCKER fix — even with the legacy code still present, the production default is now JWT-only.

### Phase 2 — Delete legacy auth branches

After Phase 1 is in place, the legacy branches at `server/hocuspocus.ts:84-162` are unreachable in any production deployment (they require `HOCUSPOCUS_ALLOW_LEGACY_TOKEN=1` AND `BRIDGE_HOST_EXPOSURE=local-only`). Delete them outright:

- The `parts[0] === "session"` legacy branch (lines 90-117)
- The `parts[0] === "attempt"` legacy branch (lines 119-137)
- The `parts[0] === "broadcast"` legacy branch (lines 139-142)
- The `parts[0] === "unit"` legacy branch (lines 144-156)
- The `documentName === "noop"` legacy branch (lines 158-160)
- The legacy-token parsing helper (lines 84-89)

The `tokenKind: "legacy"` field on `AuthContext` is removed; only `"jwt"` remains, which means the field becomes redundant — drop it from the type entirely.

The dev/test escape hatch from Phase 1 also goes away in this phase. With the legacy code deleted, `ALLOW_LEGACY_TOKEN=1` in a `local-only` exposure logs a deprecation warning at boot but otherwise no-ops; the JWT-only path is the only path.

### Phase 3 — Tests + docs

- `e2e/hocuspocus-auth.spec.ts`: tighten the secret-required guard. Currently the entire suite skips when `HOCUSPOCUS_TOKEN_SECRET` is unset (review 011 §4.1 flagged this as a separate concern, but with Phase 1's change the missing-secret case is now a boot failure, not a soft skip). Replace the `test.skip` with an `assert.ok(secretPresent)` that fails CI when the secret isn't configured. (Plan 080 will go further; this PR just closes the loophole the cutover introduces.)
- `tests/unit/`: any Hocuspocus auth tests that exercised the legacy `userId:role` shape need updates. Replace with JWT-shape fixtures that the test signs with a test secret.
- `docs/setup.md`: update the env-var section to mark `HOCUSPOCUS_TOKEN_SECRET` as required-in-production.

## Decisions to lock in

1. **Secure-by-default polarity flip.** New deployments are JWT-only out of the box. Legacy is opt-in via `HOCUSPOCUS_ALLOW_LEGACY_TOKEN=1` AND `BRIDGE_HOST_EXPOSURE=local-only`. Anyone not running with the explicit dev escape hatch gets the safe path.
2. **Phase 2 deletes the legacy branches outright** rather than leaving them behind a kill switch. The escape hatch in Phase 1 is for the BRIEF window between Phase 1 merging and Phase 2 landing in the same PR — it's not a long-lived feature.
3. **Boot-time fail-fast** (not log-and-degrade). Misconfigured production should not start, full stop. Mirrors plan 068 phase 1's pattern for `BRIDGE_HOST_EXPOSURE`.
4. **`tokenKind` removal**: with the legacy path gone, the `tokenKind: "jwt" | "legacy"` discriminant in `AuthContext` is dead code. Drop the field; the `onLoadDocument` recheck (`server/hocuspocus.ts:174`) currently gates on `context.tokenKind === "jwt"` — change to unconditional recheck since JWT is the only path.
5. **No client changes.** The client is already 100% on JWT.
6. **No Go changes.** The Go mint endpoint and recheck endpoint stay as-is.

## Files

### Phase 1 — Env flag invert + boot fail-fast

**Modify:**
- `server/hocuspocus.ts` — replace `REQUIRE_SIGNED_TOKEN` with `ALLOW_LEGACY_TOKEN`. Add boot-time check that exits when `!TOKEN_SECRET && !ALLOW_LEGACY_TOKEN`, plus check that `ALLOW_LEGACY_TOKEN=1` is only honored when `BRIDGE_HOST_EXPOSURE=local-only`. Add startup log line.
- `docs/setup.md` — env-var section: `HOCUSPOCUS_TOKEN_SECRET` is required in production; `HOCUSPOCUS_ALLOW_LEGACY_TOKEN=1` is dev-only and gated by `BRIDGE_HOST_EXPOSURE=local-only`.

### Phase 2 — Delete legacy branches

**Modify:**
- `server/hocuspocus.ts` — delete lines 84-162 (legacy parsing + per-room legacy auth). Drop `tokenKind` from `AuthContext`. Update `onLoadDocument` to recheck unconditionally (was gated on `tokenKind === "jwt"`).

### Phase 3 — Tests + docs

**Modify:**
- `e2e/hocuspocus-auth.spec.ts` — replace soft `test.skip(!secretPresent)` with hard fail when `HOCUSPOCUS_TOKEN_SECRET` isn't set in CI. Adjust any tests that exercised the legacy `userId:role` shape.
- `tests/unit/realtime-jwt.test.ts` (or equivalent) — confirm the legacy-shape fixtures are gone.
- `docs/setup.md` — already updated in Phase 1; reconfirm.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Old browser tabs (open >25 min, never refreshed) hold a pre-cutover token | low | Plan 053 already shipped 25-minute JWT TTL + automatic re-mint. Tabs that connect with an expired token re-mint via `useRealtimeToken`. The legacy path was the fallback for "tabs that pre-date the client rollout entirely" — that rollout was 7+ months ago. Acceptable to drop. |
| Dev/test environments without `HOCUSPOCUS_TOKEN_SECRET` start failing | medium | Phase 1's `ALLOW_LEGACY_TOKEN=1 + BRIDGE_HOST_EXPOSURE=local-only` escape hatch covers the dev-without-secret case for the brief window before Phase 2. After Phase 2 the legacy branch is gone, so `ALLOW_LEGACY_TOKEN` no-ops; dev needs the secret. Document in `docs/setup.md`. Local dev usually generates one via the existing `BRIDGE_SESSION_SECRETS` rotation list pattern. |
| `e2e/hocuspocus-auth.spec.ts` flakes on CI when secret is missing | medium | Phase 3 converts the soft skip to a hard fail. The test infrastructure already provisions secrets via env; CI failure is the correct signal. |
| Plan 068's `validateDevAuthEnv` startup guard might conflict | low | Plan 068's guard handles `DEV_SKIP_AUTH` + `BRIDGE_HOST_EXPOSURE`. Plan 072's check for `HOCUSPOCUS_TOKEN_SECRET` lives in the Hocuspocus server boot, not the Go boot — no overlap. |
| Hidden Go-side caller still constructing legacy tokens (e.g., a server-rendered fallback) | low | grep audit before Phase 2 against `platform/internal/` for any string concatenation matching `userId:role` shape. None expected. |

## Phases

### Phase 1 — Env flag + boot fail-fast (commit 1)

- Replace `REQUIRE_SIGNED_TOKEN` with `ALLOW_LEGACY_TOKEN`.
- Add boot-time secret check + `BRIDGE_HOST_EXPOSURE` cross-check.
- Startup log line.
- Update `docs/setup.md`.
- Self-test: `bun run hocuspocus` with various env permutations (no secret + default → fails; no secret + ALLOW_LEGACY=1 + local-only → starts with warning; secret set + default → starts JWT-only).
- Commit: `plan 072 phase 1: invert env flag, fail-fast on missing secret`.

### Phase 2 — Delete legacy branches (commit 2)

- Delete the 6 legacy branches (~80 lines) in `server/hocuspocus.ts`.
- Drop `tokenKind` from `AuthContext`.
- Update `onLoadDocument` recheck to unconditional.
- `bun run hocuspocus` smoke: starts cleanly with JWT-only path.
- Commit: `plan 072 phase 2: delete legacy userId:role auth branches`.

### Phase 3 — Tests + audit-trail (commit 3)

- Tighten `e2e/hocuspocus-auth.spec.ts` skip-condition to a hard fail.
- Update any vitest tests that referenced the legacy shape.
- Run full `bun run test` + Go `go test ./...` to confirm no regressions.
- Commit: `plan 072 phase 3: tests + docs`.

After Phase 3, run the 4-way code review against the consolidated branch diff (single-PR-per-plan policy), fold findings, open the PR.

## Plan Review

(pending — 4-way: self / Codex / DeepSeek V4 Pro / GLM 5.1)

## Code Review

(pending — at PR-open time per the new policy)

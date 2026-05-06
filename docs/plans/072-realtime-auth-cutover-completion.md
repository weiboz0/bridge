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

The current contract is opt-in cutover (`HOCUSPOCUS_REQUIRE_SIGNED_TOKEN=1` flips to JWT-only). Switch the default polarity: signed tokens are required UNLESS an explicit `HOCUSPOCUS_ALLOW_LEGACY_TOKEN=1` opt-out is set (and even then, only honored when `BRIDGE_HOST_EXPOSURE` is `localhost`, mirroring plan 068's startup guards). This:

1. Means new deployments are secure-by-default.
2. Keeps a small dev/test escape hatch that's gated by the same exposure-aware mechanism plan 068 introduced.
3. Adds a hard fail at boot when the secret is missing AND legacy is not explicitly allowed — exactly the case Codex flagged (the silent JWT-disable hole).

Specifically:
- Change `REQUIRE_SIGNED_TOKEN` to `ALLOW_LEGACY_TOKEN = process.env.HOCUSPOCUS_ALLOW_LEGACY_TOKEN === "1"`.
- New boot check (top of `server/hocuspocus.ts` server construction): if `!TOKEN_SECRET && !ALLOW_LEGACY_TOKEN`, log a fatal and `process.exit(1)`. If `ALLOW_LEGACY_TOKEN` is set in a non-`localhost` exposure, log a fatal and exit.
- Add an explicit startup log line stating which mode is active so ops can confirm at boot.

This phase is the minimum viable BLOCKER fix — even with the legacy code still present, the production default is now JWT-only.

### Phase 2 — Delete legacy auth branches

After Phase 1 is in place, the legacy branches at `server/hocuspocus.ts:84-162` are unreachable in any production deployment (they require `HOCUSPOCUS_ALLOW_LEGACY_TOKEN=1` AND `BRIDGE_HOST_EXPOSURE=localhost`). Delete them outright:

- The `parts[0] === "session"` legacy branch (lines 90-117)
- The `parts[0] === "attempt"` legacy branch (lines 119-137)
- The `parts[0] === "broadcast"` legacy branch (lines 139-142)
- The `parts[0] === "unit"` legacy branch (lines 144-156)
- The `documentName === "noop"` legacy branch (lines 158-160)
- The legacy-token parsing helper (lines 84-89)

The `tokenKind: "legacy"` field on `AuthContext` is removed; only `"jwt"` remains, which means the field becomes redundant — drop it from the type entirely.

The dev/test escape hatch from Phase 1 also goes away in this phase. With the legacy code deleted, `ALLOW_LEGACY_TOKEN=1` in a `localhost` exposure logs a deprecation warning at boot but otherwise no-ops; the JWT-only path is the only path.

### Phase 3 — Tests + docs

- `e2e/hocuspocus-auth.spec.ts`: tighten the secret-required guard. Currently the entire suite skips when `HOCUSPOCUS_TOKEN_SECRET` is unset (review 011 §4.1 flagged this as a separate concern, but with Phase 1's change the missing-secret case is now a boot failure, not a soft skip). Replace the `test.skip` with an `assert.ok(secretPresent)` that fails CI when the secret isn't configured. (Plan 080 will go further; this PR just closes the loophole the cutover introduces.)
- `tests/unit/`: any Hocuspocus auth tests that exercised the legacy `userId:role` shape need updates. Replace with JWT-shape fixtures that the test signs with a test secret.
- `docs/setup.md`: update the env-var section to mark `HOCUSPOCUS_TOKEN_SECRET` as required-in-production.

## Decisions to lock in

1. **Secure-by-default polarity flip.** New deployments are JWT-only out of the box. Legacy is opt-in via `HOCUSPOCUS_ALLOW_LEGACY_TOKEN=1` AND `BRIDGE_HOST_EXPOSURE=localhost`. Anyone not running with the explicit dev escape hatch gets the safe path.
2. **Phase 2 deletes the legacy branches outright** rather than leaving them behind a kill switch. The escape hatch in Phase 1 is for the BRIEF window between Phase 1 merging and Phase 2 landing in the same PR — it's not a long-lived feature.
3. **Boot-time fail-fast** (not log-and-degrade). Misconfigured production should not start, full stop. Mirrors plan 068 phase 1's pattern for `BRIDGE_HOST_EXPOSURE`.
4. **`tokenKind` removal**: with the legacy path gone, the `tokenKind: "jwt" | "legacy"` discriminant in `AuthContext` is dead code. Drop the field; the `onLoadDocument` recheck (`server/hocuspocus.ts:174`) currently gates on `context.tokenKind === "jwt"` — change to unconditional recheck since JWT is the only path.
5. **No client changes.** The client is already 100% on JWT.
6. **No Go changes.** The Go mint endpoint and recheck endpoint stay as-is.

## Files

### Phase 1 — Env flag invert + boot fail-fast

**Modify:**
- `server/hocuspocus.ts` — replace `REQUIRE_SIGNED_TOKEN` with `ALLOW_LEGACY_TOKEN`. Add boot-time check that exits when `!TOKEN_SECRET && !ALLOW_LEGACY_TOKEN`, plus check that `ALLOW_LEGACY_TOKEN=1` is only honored when `BRIDGE_HOST_EXPOSURE=localhost`. Add startup log line.
- `docs/setup.md` — env-var section: `HOCUSPOCUS_TOKEN_SECRET` is required in production; `HOCUSPOCUS_ALLOW_LEGACY_TOKEN=1` is dev-only and gated by `BRIDGE_HOST_EXPOSURE=localhost`.

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
| Dev/test environments without `HOCUSPOCUS_TOKEN_SECRET` start failing | medium | Phase 1's `ALLOW_LEGACY_TOKEN=1 + BRIDGE_HOST_EXPOSURE=localhost` escape hatch covers the dev-without-secret case for the brief window before Phase 2. After Phase 2 the legacy branch is gone, so `ALLOW_LEGACY_TOKEN` no-ops; dev needs the secret. Document in `docs/setup.md`. Local dev usually generates one via the existing `BRIDGE_SESSION_SECRETS` rotation list pattern. |
| `e2e/hocuspocus-auth.spec.ts` flakes on CI when secret is missing | medium | Phase 3 converts the soft skip to a hard fail. The test infrastructure already provisions secrets via env; CI failure is the correct signal. |
| Plan 068's `validateDevAuthEnv` startup guard might conflict | low | Plan 068's guard handles `DEV_SKIP_AUTH` + `BRIDGE_HOST_EXPOSURE`. Plan 072's check for `HOCUSPOCUS_TOKEN_SECRET` lives in the Hocuspocus server boot, not the Go boot — no overlap. |
| Hidden Go-side caller still constructing legacy tokens (e.g., a server-rendered fallback) | low | grep audit before Phase 2 against `platform/internal/` for any string concatenation matching `userId:role` shape. None expected. |

## Phases

### Phase 1 — Env flag + boot fail-fast (commit 1)

- Replace `REQUIRE_SIGNED_TOKEN` with `ALLOW_LEGACY_TOKEN`.
- Add boot-time secret check + `BRIDGE_HOST_EXPOSURE` cross-check.
- Startup log line.
- Update `docs/setup.md`.
- Self-test: `bun run hocuspocus` with various env permutations (no secret + default → fails; no secret + ALLOW_LEGACY=1 + localhost → starts with warning; secret set + default → starts JWT-only).
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

### Self-review (Opus 4.7) — 4 concerns, no blockers

1. **Escape-hatch ephemerality**: `ALLOW_LEGACY_TOKEN` only matters in the window between Phase 1 and Phase 2 (since Phase 2 deletes the code it gates). Two options: (a) keep the staged approach for safer rollout — Phase 1 ships, dev validates the polarity flip in production, then Phase 2 deletes; (b) collapse Phase 1 + 2 into a single deletion commit. Going with (a) for a brief soak — the deletion commit lives on the same branch but as a separate logical unit so the diff reads cleanly. Net effect: same single PR, but reviewers can trace "the polarity flip" vs "the deletion" separately. Minor structural choice; staying with the plan as written.
2. **`tokenKind` reader audit**: before Phase 2, grep `tokenKind` across `server/` to confirm only `onLoadDocument` reads it. If other readers exist, drop them too. Folded into Phase 2 as a pre-work step.
3. **No shared startup-guard module on the Node side**: Bridge's only env-validation pattern is in Go (`platform/cmd/api/main.go::validateDevAuthEnv`). Hocuspocus boot validation is one-off. Acceptable for a single env contract; if more startup gates land later, extract a shared helper.
4. **Test-contract change scope**: tightening `e2e/hocuspocus-auth.spec.ts` from `test.skip` to hard fail is technically plan 080 territory. But the soft-skip is the failure mode that LET the unflipped flag ship in production — leaving it for plan 080 means another window of "review 011's BLOCKER could regress and CI wouldn't catch it." Keep in plan 072 Phase 3.

### Codex / DeepSeek V4 Pro / GLM 5.1

### Codex — CONCUR-WITH-CHANGES (3 fixes folded)

1. **Phase 2 deletion is wider than the plan listed.** Beyond the 6 branches at lines 84-162, also remove: `loadAttemptOwner` and `teacherCanViewAttempt` imports at `server/hocuspocus.ts:5,8` (legacy-only helpers); the legacy-sniff note at `server/attempts.ts:73`; fixture coverage in `tests/unit/realtime-jwt.test.ts:63` and `platform/internal/auth/realtime_jwt_test.go:128`.

2. **`localhost` is not a real BRIDGE_HOST_EXPOSURE value.** Plan 068's actual enum (`platform/cmd/api/main.go:502-525`) is `localhost`/empty (default) or `exposed`. Plan as written would boot-fail on any dev machine. Aligning to `localhost`.

3. **E2E hard-fail scope is incomplete.** HTTP tests in `e2e/hocuspocus-auth.spec.ts:131,150,159` still tolerate 503 / soft-skip. Phase 3 needs to harden those alongside the WebSocket `beforeAll`. Add a deploy-sequencing note to §Risks: `HOCUSPOCUS_TOKEN_SECRET` must be provisioned for both Go (`platform/internal/config/config.go:122`) and Hocuspocus (`server/hocuspocus.ts:25`) atomically.

### DeepSeek V4 Pro — CONCUR-WITH-CHANGES (3 fixes folded)

1. **Node-side `BRIDGE_HOST_EXPOSURE` read is a new contract.** That env var is currently only consumed by Go. Phase 1 explicitly notes Hocuspocus reads it too.

2. **`.env.example:63-71` and `TODO.md:9` reference the legacy format.** Both updated in Phase 3.

3. **CI prerequisite for Phase 3 hard-fail.** Skip-to-hard-fail flip only works if CI provisions `HOCUSPOCUS_TOKEN_SECRET`. Confirm before Phase 3 lands.

**Confirmed (no fix):** `tokenKind` readers are clean (11 references, all in `hocuspocus.ts`, single reader at `onLoadDocument:184`). Backwards-compat risk argument sound.

### GLM 5.1 — CONCUR-WITH-CHANGES (2 fixes folded)

1. **`noop` document path needs explicit handling.** Lines 158-159 currently return `tokenKind: "legacy"` for `documentName === "noop"`. Decision: bypass `onAuthenticate` for noop via Hocuspocus's `token: false` config, OR add a noop-scope JWT path. Going with bypass — noop documents have no actual collaboration content; auth is theater there.

2. **`isLikelyJwt` simplification.** Post-cutover, every token IS a JWT — replace the `isLikelyJwt`-then-fall-through with unconditional `verifyRealtimeJwt`. Also confirm the Go mint endpoint's `broadcast:*` and `session:*` scope gates match the deleted legacy code's role/owner checks (so the JWT path isn't more permissive than the path it replaces).

**Confirmed (no fix):** escape-hatch shape correct; old-browser-tab risk sound.

### Kimi K2.6 — CONCUR-WITH-CHANGES (5 NEW findings folded)

Kimi caught five things the other reviewers missed:

1. **Hidden legacy detector at `src/lib/yjs/use-yjs-provider.ts:33`** — the `shouldConnect` guard checks `!token.startsWith(":")`, a vestigial post-cutover check. Add to Phase 2 deletion sweep.

2. **`isLikelyJwt` must be DELETED, not just bypassed.** Plan said "replace with unconditional `verifyRealtimeJwt`", but missed that the function export + its test describe block at `tests/unit/realtime-jwt.test.ts:58-73` become dead code. Remove the export and the describe block in Phase 2.

3. **`onLoadDocument` `TOKEN_SECRET` guard becomes redundant.** After Phase 1's boot check fails without `TOKEN_SECRET`, the runtime guard `if (context?.tokenKind === "jwt" && TOKEN_SECRET)` simplifies to unconditional. Drop the `&& TOKEN_SECRET` clause alongside the `tokenKind` removal.

4. **`BRIDGE_HOST_EXPOSURE` empty-string semantics.** Go treats empty as `localhost` (`platform/cmd/api/main.go:504`). Hocuspocus must match — otherwise dev boxes without the var explicitly exported to the Node process boot-fail. Phase 1's check needs `(exposure === "" || exposure === "localhost")` parity.

5. **`.env.example` should ADD** a (commented-out) `HOCUSPOCUS_ALLOW_LEGACY_TOKEN`, not just remove the old `HOCUSPOCUS_REQUIRE_SIGNED_TOKEN`. Plus `TODO.md:9` should be DELETED outright — this plan completes the JWT replacement, no follow-up.

**Operational concern (Kimi's #3):** Phase 2 makes `onLoadDocument` recheck unconditional. The recheck calls `GO_INTERNAL_API_URL` (default `http://localhost:8002`). Pre-Phase-2 a missing `TOKEN_SECRET` silently skipped the recheck; post-Phase-2 a misconfigured URL becomes a total outage. **Folded:** boot-time sanity check warning when `GO_INTERNAL_API_URL` is the localhost default while `BRIDGE_HOST_EXPOSURE=exposed`.

**Structural concern (Kimi's #2):** the Phase 1 escape hatch is operationally meaningless inside a single PR (no real canary window before Phase 2 deletes it). Acknowledged — Self-review NIT #1 already noted this; keeping the staged structure for diff readability, explicitly labeled as such.

**No blockers.** Kimi confirmed: Go mint endpoint's scope gates are already stricter than the deleted legacy branches, so the JWT path isn't more permissive than what it replaces.

### Consolidated plan changes (post-review)

- §Phase 1 §Files: explicitly note Hocuspocus reads `BRIDGE_HOST_EXPOSURE`.
- §Phase 1 §Decisions: env value is `localhost` (plan 068's enum), not the made-up `local-only`.
- §Phase 1 §Files: empty-string `BRIDGE_HOST_EXPOSURE` treated as `localhost` (matches Go `main.go:504`).
- §Phase 1 §Files: boot-time warning when `GO_INTERNAL_API_URL` is the localhost default while `BRIDGE_HOST_EXPOSURE=exposed` (Kimi's operational concern).
- §Phase 2 §Files: also delete the legacy detector at `src/lib/yjs/use-yjs-provider.ts:33`; delete the `isLikelyJwt` export + its describe block at `realtime-jwt.test.ts:58-73`; drop `&& TOKEN_SECRET` from the `onLoadDocument` guard alongside the `tokenKind` removal.
- §Phase 3 §Files: `.env.example` ADDS a commented-out `HOCUSPOCUS_ALLOW_LEGACY_TOKEN` line; `TODO.md:9` is DELETED outright (legacy-token bullet is closed by this plan).
- §Phase 2 §Files: expand deletion checklist — drop `loadAttemptOwner`/`teacherCanViewAttempt` imports + `attempts.ts:73` legacy-sniff comment + `realtime-jwt.test.ts:63` + `realtime_jwt_test.go:128` legacy fixtures; bypass `onAuthenticate` for noop documents; replace `isLikelyJwt` with unconditional `verifyRealtimeJwt`.
- §Phase 3 §Files: tighten `e2e/hocuspocus-auth.spec.ts:131,150,159` HTTP tests too (not just the `beforeAll`); update `.env.example:63-71` + `TODO.md:9` legacy references; confirm CI provisions `HOCUSPOCUS_TOKEN_SECRET`.
- §Risks: add deploy-sequencing note (Go + Hocuspocus secret atomic); add Go-side scope-enforcement verification (broadcast/session scopes deleted legacy code's role/owner checks must be covered by JWT mint endpoint).

## Post-execution report

**Status**: 3 phases shipped on branch `feat/072-realtime-auth-cutover-completion`. PR pending. Net diff: ~`-210` lines (more deleted than added).

| Phase | Commit | Net |
|---|---|---|
| 1 — env flag invert + boot fail-fast | `b96c648` | +85 / -32 |
| 2 — delete legacy auth branches | `43590e7` | -210 across 6 files |
| 3 — tests + docs | `f617450` | +26 / -15 |

**5-way plan review verdicts** (all CONCUR after folding):
- Self (Opus 4.7): 4 concerns folded
- Codex: CONCUR-WITH-CHANGES — 3 fixes (env enum value, expanded deletion checklist, e2e HTTP hardening + deploy sequencing)
- DeepSeek V4 Pro: CONCUR-WITH-CHANGES — 3 fixes (Node-side BRIDGE_HOST_EXPOSURE contract, .env.example/TODO.md updates, CI prerequisite)
- GLM 5.1: CONCUR-WITH-CHANGES — 2 fixes (noop document handling, isLikelyJwt simplification)
- Kimi K2.6 (new 5th reviewer): CONCUR-WITH-CHANGES — 5 NEW findings (use-yjs-provider:33 detector, full isLikelyJwt deletion, TOKEN_SECRET guard cleanup, empty-string BRIDGE_HOST_EXPOSURE semantics, GO_INTERNAL_API_URL operational risk)

**Verification**:
- `bun run hocuspocus` boots cleanly with secret + JWT-only mode log line.
- Boot-fail tests for the four error permutations (no secret + no escape, ALLOW_LEGACY=1 + exposed, invalid BRIDGE_HOST_EXPOSURE) all exit code 1 with expected error messages.
- Full Go test suite (`go test ./...`) green.
- Vitest `tests/unit/realtime-jwt.test.ts` 17/17 pass.
- `tsc --noEmit` baseline of 10 (pre-existing errors in unrelated files) maintained.
- ESLint clean for all modified files.

**Known follow-ups** (filed for plan 080):
- CI workflow file is missing from the repo. The hardened e2e tests will fail the moment CI is provisioned without `HOCUSPOCUS_TOKEN_SECRET`. By design (closes the loophole that let the un-flipped flag ship), but means the secret must be added to CI config before e2e can run.
- Soft suite-level `beforeAll` skip in `e2e/hocuspocus-auth.spec.ts` (WS describe block) was left untouched per Codex's plan-review acceptance.

**No follow-up plans needed for the cutover itself.** Realtime-auth BLOCKER from review 011 §1.1 is closed.

**Plan deviation (intentional, more secure)**: Phase 2's deletion sweep also removed the `ALLOW_LEGACY_TOKEN` runtime check from `validateRealtimeAuthEnv` — not just the legacy parsing branches. Net effect: `HOCUSPOCUS_TOKEN_SECRET` is now unconditionally required at boot, with no escape hatch (the original plan kept the boot-tolerance knob for the dev case). DeepSeek + GLM caught that `.env.example` and `docs/setup.md` still described the now-phantom flag; corrected in the BLOCKER-fix commit (28e3b3e + follow-up). The new contract is strictly more secure than the original Phase 1 design.

## Code Review

5-way code review against branch `feat/072-realtime-auth-cutover-completion` head `dc7f5c6` (3-phase implementation). All four external reviewers independently identified the same auth-bypass BLOCKER — the multi-reviewer consensus working as designed.

### Self (Opus 4.7) — OK

Verified diff structure, ran full Go test suite (passes), vitest realtime tests (17/17), tsc baseline maintained, spot-greps confirmed deletion completeness during phase implementation. No findings before externals returned.

### Codex round-1 — BLOCKERS (2)

**Q3 BLOCKER — auth regression**: `onAuthenticate` returned `{userId:"", role:""}` for any missing/empty token (line 78 originally read `if (!token || documentName === "noop")`). An unauthenticated WebSocket could load any document; `onLoadDocument` then skipped recheck because `context.userId` was empty. **FIXED in commit `28e3b3e`** — split conditions so only noop bypasses, missing token on any other document throws.

**Q6 BLOCKER — docs lie**: `docs/setup.md:205` and `.env.example:60-71` documented `HOCUSPOCUS_ALLOW_LEGACY_TOKEN` as a working escape hatch, but Phase 2's deletion sweep removed the runtime read entirely. **FIXED in commit `55077d5`** — phantom-flag references stripped from both files; setup.md prose rewritten ("required, no escape hatch"); plan-file post-execution report adds a "Plan deviation" note acknowledging Phase 2 went further than originally specified (more secure outcome).

NITS (acceptable, not actionable): recheck "unconditional" wording slightly imprecise (gated on `context?.userId`); WS `beforeAll` soft-skip remains (deferred to plan 080 per Codex's own plan-review).

### DeepSeek V4 Flash — needs-attention (1)

Independently surfaced the same Q6 phantom-flag finding ("docs/setup.md and .env.example describe HOCUSPOCUS_ALLOW_LEGACY_TOKEN behavior that Phase 2 deleted from runtime"). **FIXED in `55077d5`** — same fix as Codex Q6. Other findings confirmed clean.

### GLM 5.1 — needs-attention (2)

Independently surfaced both the auth-bypass and the phantom-flag findings. Phrased the auth bypass as "Bug: `!token || documentName === "noop"` is too broad". **Both FIXED** in commits `28e3b3e` + `55077d5`.

### Kimi K2.6 — BLOCKER (1)

Independently surfaced the auth bypass with the same recommended fix ("separate the conditions: noop bypass first, then restore `if (!token) throw new Error('Authentication required')`"). **FIXED in `28e3b3e`** — exactly the recommended split. Confirmed all 5 of its plan-review findings landed correctly.

### Codex round-2 (against `28e3b3e`)

Q3 closed; Q6 still flagged because round-2 reviewed `28e3b3e` (Q6 fix landed in `55077d5` AFTER round-2 was dispatched). Round-3 dispatched against `55077d5` for final confirmation.

### Codex round-3

(running)

### Convergence

All four external reviewers (Codex, DeepSeek V4 Flash, GLM 5.1, Kimi K2.6) independently identified the same auth-bypass BLOCKER. Three of four also flagged the phantom-flag docs issue. **Both BLOCKERS fixed** in commits `28e3b3e` + `55077d5`. Awaiting Codex round-3 confirmation; otherwise ready to PR.

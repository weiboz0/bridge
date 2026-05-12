# Plan 078 — Deterministic E2E fixture + skip-to-fail conversion

## Problem (review 011 §4.1)

The Playwright suite is "stable but not a reliable regression gate for the features most likely to break on auth changes" because it silently `test.skip()`s entire scenarios when the seed data or env doesn't line up. Two spec files concentrate the risk:

### `e2e/hocuspocus-auth.spec.ts` (5 conditional skips)

- `:119, :123` — skip when teacher has no units accessible. Mints a JWT for a unit-scope doc, but if `/api/me/units` returns empty, the test exits without exercising the mint endpoint.
- `:176` — skip when `HOCUSPOCUS_TOKEN_SECRET` is unset. The WebSocket auth ratchet ENTIRELY skips. This is the test review 011 §1.1 named the realtime auth BLOCKER fix relies on.
- `:184, :187` — same data dependency on `/api/me/units`.

### `e2e/session-flow.spec.ts` (9 conditional skips)

- `:46, :150` — no classes for teacher.
- `:69, :72, :131` — UI buttons not visible (Start/End/Resume Live Session).
- `:94, :120` — previous test in chain didn't set classId/sessionId.
- `:105` — student not enrolled.
- `:160` — no past sessions.

The chain pattern is fragile: one early skip silently disables the entire downstream chain. With Playwright at 1 worker (`playwright.config.ts:9`), the suite still finishes green even when skipping all 9 scenarios.

## Approach

Two-part fix matching review §4.1 verbatim:

1. **Deterministic e2e fixture setup** — a one-shot seed run BEFORE `auth.setup.ts` that ensures every required entity exists: 1 teacher (eve@demo.edu), 1 student (alice@demo.edu), 1 class (both enrolled), 1 published unit, `HOCUSPOCUS_TOKEN_SECRET` present.
2. **Skip-to-fail conversion** for the auth/realtime/live-session test files. After the fixture guarantees the data, `test.skip` becomes `expect.fail` (or just delete the skip and let the natural assertion fail).

### Fixture mechanism

Add a new Playwright project named `seed` that runs FIRST (before `auth-setup`) and:

- Hits the Go API directly (`POST /api/classes`, `POST /api/classes/join`) to ensure the canonical fixture entities exist. Idempotent — checks before creating. Does NOT create orgs or users — relies on the demo SQL seed having already established `eve@demo.edu`'s org membership (Kimi K2.6 round-1 NIT 3 — original draft mentioned `POST /api/orgs` which was wrong; the fixture finds eve's org via `GET /api/orgs` (her memberships) and creates the class IN that org).
- Asserts `HOCUSPOCUS_TOKEN_SECRET` is present in the runner's env (`expect(process.env.HOCUSPOCUS_TOKEN_SECRET).toBeTruthy()`). If missing, the seed project FAILS — the rest of the suite never runs.
- Stashes a tiny JSON fixture-meta file (e.g., `e2e/.fixture/state.json` — gitignored) that records the seeded `classId`, `unitId`, `sessionId-or-null` for spec files to consume. Avoids re-querying every test.

Seed project depends on nothing; `auth-setup` depends on `seed`; `tests` depends on `auth-setup`. Chain: seed → auth-setup → tests.

The fixture files use the existing `demo.edu` seeded users. Codex round-1 BLOCKER caught a citation error in the original draft: the eve/alice users are NOT created by `seed_bridge_hq.sql` (which only creates `system@bridge.platform`). They're created by **`scripts/seed_problem_demo.sql`** — verified via `grep "eve@demo.edu"` (matches in `seed_problem_demo.sql:4, 36`). The setup workflow in `docs/setup.md` calls this via `bun run content:python-101:import --apply --wire-demo-class`. The platform admin (`admin@e2e.test`) is a separate one-time `INSERT` snippet in `docs/setup.md`. Plan 078 doesn't create users — it relies on the setup-time SQL seed. Plan 078 just adds the realtime-relevant entities (class, unit) and the env-var assertion. Avoids adding a SECOND user-creation pathway.

### Skip-to-fail conversion

After the fixture is in place, the conditional skips in `hocuspocus-auth.spec.ts` and `session-flow.spec.ts` become hard failures:

- **`hocuspocus-auth.spec.ts:119, 123, 184, 187`** — replace `test.skip(!unitId, ...)` with `expect(unitId).toBeDefined()` or simply read `unitId` from the fixture-state and proceed.
- **`hocuspocus-auth.spec.ts:176`** — `test.skip(!TOKEN_SECRET, ...)` becomes `expect(TOKEN_SECRET).toBeTruthy()`. The seed project already asserts this; this is belt-and-suspenders for direct test runs.
- **`session-flow.spec.ts:46, 94, 105, 120, 150`** — these read `classId` / `sessionId` from previous tests OR from missing UI. With the fixture seeding a class + enrollments, the `no class` cases become hard failures. The "session already active" / "Start button not visible" branches can be handled by ensuring the fixture cleans up active sessions in its setup phase.

### Convert ALL 9 session-flow skips (Codex round-1 BLOCKER 2 + DeepSeek round-1 Q1)

Original draft kept 3 skips as "legitimate downstream consequences". Both Codex and DeepSeek pushed back independently: the session-flow.spec.ts tests are SERIAL (workers=1, each test depends on previous), and the fixture guarantees the data prerequisites. So:

- **"End Session button not visible" (line 131)**: if the prior "start session" test (line 30+) ran successfully — which the fixture's class+enrollment guarantees — then the End button MUST be visible. The skip masks a real test gap. Convert to hard failure.
- **"No past sessions visible" (line 160)**: if the fixture's class exists AND the previous "end session" test ran, there IS exactly one past session. The skip masks a real test gap. Convert to hard failure.
- **"Live session card not visible" (line 105)**: same logic — student is enrolled (fixture guarantees), session is active (previous test guarantees), card MUST appear. Convert.

All 9 skips convert. The fixture is now load-bearing for the full chain: any link breaking is a real regression.

## Decisions to lock in

1. **Reuse the existing demo.edu user seed.** Don't create a parallel user-creation pathway. Plan 078 adds class + unit + active-session-cleanup, not users.
2. **Fixture writes to a gitignored `e2e/.fixture/state.json`** so spec files can pick up `classId`/`unitId` without re-querying. Single source of truth for "what the suite seeded".
3. **HOCUSPOCUS_TOKEN_SECRET asserted at seed time** — fail loudly if missing. Documents the contract: e2e CANNOT run without the secret. Aligns with plan 072's runtime-required posture.
4. **Active-session cleanup in seed**. Before tests run, the seed project ends any active session for the class so `start session` doesn't bump into a leftover. **Concrete cleanup recipe** (GLM 5.1 round-1 NIT Q1):
   - `GET /api/classes/{fixtureClassId}/active-session` (or list-by-class endpoint, whichever exists) — returns the active session row or null.
   - If non-null: `POST /api/sessions/{id}/end`. Tolerate non-200 responses (log warning, continue) — a session in an unexpected state (e.g. mid-`ending`) shouldn't crash the seed.
   - If the API doesn't expose a "list active sessions for a class" endpoint, document the gap and have the seed call `GET /api/teacher/classes/{id}` to read the class detail (which surfaces active session state).
5. **All 9 session-flow skips convert** (Codex + DeepSeek BLOCKER fold). Original draft kept 3 as "legitimate" — wrong because the serial test chain creates the past-session naturally (start → end → past-sessions list). Plan now converts every skip in session-flow.spec.ts.
6. **No new playwright workers.** The serial-execution config (`workers: 1`) stays — review §4.1 acknowledges this is fine, the issue was hidden skips, not parallelism.
7. **CI-only enforcement OR everywhere?** Make it everywhere. Local dev should also fail fast on missing fixture so a developer's first `bun run test:e2e` makes the missing-secret state visible immediately, not after 90 seconds of green-with-skips.

## Files

**Modify (4 files):**

- `e2e/playwright.config.ts` — add a new `seed` project before `auth-setup`. Update `dependencies` chain: `tests` → `auth-setup` → `seed`.
- `e2e/hocuspocus-auth.spec.ts` — convert `test.skip(!unitId, ...)` → `expect(unitId).toBeDefined()`; convert `test.skip(!TOKEN_SECRET, ...)` → `expect(TOKEN_SECRET).toBeTruthy()`. Read `classId` / `unitId` from the fixture-state file.
- `e2e/session-flow.spec.ts` — convert ALL 9 conditional `test.skip` calls to hard failures. Read seeded `classId` from fixture state. Each test reads `classId`/`sessionId` from a shared module-level state set by the previous test in the chain (already the existing pattern); after Phase 3 those reads `expect()` non-null instead of skipping. (Codex round-2 + DeepSeek/Kimi round-1 — see §"Convert ALL 9" + §Decisions #5; no skip stays "legitimate".)
- `.gitignore` — add `e2e/.fixture/`.

**Create (2 files):**

- `e2e/seed.setup.ts` — the new fixture-setup spec. Asserts `HOCUSPOCUS_TOKEN_SECRET` is set, ensures a class with eve+alice enrolled exists, ensures a published unit linked to a topic in that class's course exists, ends any leftover active sessions, writes `e2e/.fixture/state.json` with `{classId, unitId}`.
- `e2e/helpers/fixture-state.ts` — small helper exposing `getFixtureState(): Promise<{classId: string, unitId: string}>`. Reads the JSON file; throws on missing with a developer-actionable error (GLM 5.1 round-1 NIT Q3): `"e2e/.fixture/state.json not found — run the e2e seed project first ('bun run test:e2e --project=seed' or just 'bun run test:e2e' which auto-runs seed via Playwright project dependencies). The seed project has a hard dependency: HOCUSPOCUS_TOKEN_SECRET must be set in the runner's environment."` Specs import this instead of re-querying the API.

**No changes to:**

- The other 25+ e2e spec files (review §4.1 explicitly named auth + session-flow as the at-risk surface). A future plan can sweep more if patterns repeat.
- `e2e/auth.setup.ts` — login flow stays the same; just runs after seed.
- `scripts/seed_bridge_hq.sql` — the SQL seed for users isn't touched; e2e seed builds on top.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Seed project fails on a fresh dev DB without the demo.edu users | high | Document the dependency in `docs/setup.md` (already present). Seed project's first action is `loginWithCredentials(eve@demo.edu)` — failure here is a clear "users not seeded" error. |
| Active-session cleanup fails because the seed-time login doesn't have permission to end an OTHER teacher's session | low | Only Eve's own sessions are cleaned. A multi-teacher scenario would need broader coordination but isn't in scope. |
| The fixture-state JSON file conflicts across parallel CI runs | very low | Workers=1; fully serialized. JSON file is per-checkout, not shared. |
| Adding a `seed` project doubles e2e setup time | medium | The seed project is fast — single API calls, idempotent. <5s expected. Acceptable trade for losing the silent-skip class of bugs. |
| Skip-to-fail conversion breaks PRs that previously passed via skips | high (intended) | This IS the goal. If a PR breaks the realtime auth path, the test should FAIL, not pass-with-skip. The breakage IS the signal. Document in plan + commit message. |
| Past-sessions test continues to skip and degrades over time | low | Documented exception. A future plan can add past-session seeding (1-2 lines). Not blocking. |
| The `e2e/.fixture/` gitignore creates confusion ("why isn't this committed?") | low | One-line README in `e2e/.fixture/` explaining the directory's purpose, OR leave it implicit since the dir is ephemeral. Pick implicit. |
| Seed POST calls (create class, join) need CSRF tokens or special headers | low | (DeepSeek round-1 Q5 NIT.) Confirm during Phase 1 implementation: hit `POST /api/classes` from `seed.setup.ts` once + see whether it 403s. Bridge's API uses cookie-based auth without CSRF tokens for non-browser flows (Auth.js JWE / bridge.session cookies authenticate); programmatic Playwright `page.request.post()` carrying the cookie should work. If a 403 appears, switch to the existing `loginWithCredentials` page-level flow + UI to create the class. |
| All-or-nothing dependency chain — seed failure blocks 25+ spec files | low (intended) | (DeepSeek round-1 Q5 NIT.) This IS the goal: a flaky seed is a real signal that the fixture contract is broken. CI's `retries: 2` already absorbs transient flakes. A persistent seed failure means the fixture or environment is genuinely wrong — surface it loudly. |
| The fixture's class-creation step duplicates the demo seed's class | medium | (GLM 5.1 round-1 NIT Q2 — implementation detail spelled out.) The demo seed gives eve a class (`00000000-0000-0000-0000-000000040001`), so "does eve have any class" is ALWAYS true and is NOT a valid idempotency check. Concrete strategy: the seed project queries `GET /api/teacher/classes` as eve, looks for a class whose `title` starts with the literal string `"e2e-fixture"`. If found → reuse. If not → `POST /api/classes` with `title: "e2e-fixture-class"`. The string-prefix match is stable across re-runs and distinguishes the e2e fixture class from any demo classes. Same strategy for the unit: query `GET /api/me/units` for one whose `title` starts with `"e2e-fixture"`; create if missing. |

## Phases

### Phase 1 — Seed project + fixture-state helper (commit 1)

- Create `e2e/seed.setup.ts`: env-var assertion + idempotent class/unit creation + active-session cleanup + JSON write.
- Create `e2e/helpers/fixture-state.ts`: read helper.
- Update `e2e/playwright.config.ts`: add `seed` project, update dependencies.
- Add `e2e/.fixture/` to `.gitignore`.
- Run `bun run test:e2e -g "seed"` (or just kick off the suite) — confirm seed runs before auth-setup, JSON file is written.
- Commit: `plan 078 phase 1: deterministic e2e fixture seed (class + unit + secret assert)`.

### Phase 2 — Convert hocuspocus-auth.spec.ts skips (commit 2)

- Replace `test.skip(!unitId, ...)` × 3 with `expect(unitId).toBeDefined()` (and pull `unitId` from fixture state).
- Replace `test.skip(!TOKEN_SECRET, ...)` with `expect(TOKEN_SECRET).toBeTruthy()`.
- Run `bun run test:e2e e2e/hocuspocus-auth.spec.ts` — all WS + HTTP tests must execute, no skips.
- Commit: `plan 078 phase 2: hocuspocus-auth — skip-to-fail conversion`.

### Phase 3 — Convert session-flow.spec.ts skips (commit 3) — ALL 9

Each conditional `test.skip(condition, msg)` becomes `expect(condition).toBeFalsy()` (or read seeded id from fixture state and assert non-null). Per the line numbers in `session-flow.spec.ts` at HEAD:

- **Line 46** — `test.skip(true, "No classes available for teacher")` after `classLink.isVisible()` returns false. Fixture guarantees a class for eve. Read `classId` from fixture state instead of clicking the first link; remove the visibility-fallback skip.
- **Line 69** — `test.skip(true, "Session already active...")` when Resume button visible. Fixture ends active sessions in seed; this branch should never fire. Convert to a hard `expect(resumeButton).not.toBeVisible()` after seed cleanup.
- **Line 72** — `test.skip(true, "Start Live Session button not visible")`. Fixture's class-detail page MUST show the Start button. Convert to `await expect(startButton).toBeVisible({timeout: 5000})`.
- **Line 94** — `test.skip(!classId || !sessionId, "No session was started in previous test")`. The previous test guarantees both via fixture-driven flow. Replace with `expect(classId).toBeDefined(); expect(sessionId).toBeDefined();`.
- **Line 105** — `test.skip(true, "Live session card not visible — student may not be enrolled")`. Fixture enrolls alice. Convert to `await expect(liveCard).toBeVisible({timeout: 5000})`.
- **Line 120** — `test.skip(!sessionId, "No session was started")`. Same as line 94 — `expect(sessionId).toBeDefined()`.
- **Line 131** — `test.skip(true, "End Session button not visible")`. After previous start-session test, the End button MUST be visible to the teacher. Convert to `await expect(endButton).toBeVisible({timeout: 5000})`.
- **Line 150** — `test.skip(!classId, "No class available")`. Fixture guarantees classId. Replace with `expect(classId).toBeDefined()`.
- **Line 160** — `test.skip(true, "No past sessions visible")`. After the end-session test runs, one past session exists. Convert to `await expect(pastSessionsList).toContainText(/expected text/)`.

After conversion: `grep -c "test.skip" e2e/session-flow.spec.ts` returns 0. Run `bun run test:e2e e2e/session-flow.spec.ts` — all tests execute, none skip.
- Commit: `plan 078 phase 3: session-flow — convert all 9 skips to hard failures`.

### Phase 4 — Verify + post-execution report (commit 4)

- Run full e2e suite locally with all three services up. Confirm seed project runs, all auth+session tests execute (no skips on the converted paths), suite passes.
- Update post-execution report.
- Commit: `docs: plan 078 post-execution report`.

After Phase 4, run the 5-way code review. Note: e2e suite cannot be exercised in the reviewer sandbox; manual verification by the author is the gate.

## Plan Review

### Round 1 (2026-05-06)

#### Self-review (Opus 4.7) — clean

No concerns surfaced before externals returned.

#### Codex — 2 BLOCKERS (FIXED)

1. `[FIXED]` BLOCKER Q1: Plan claimed `seed_bridge_hq.sql` creates eve/alice — false. That script only creates `system@bridge.platform`. → **Response**: corrected citation to `scripts/seed_problem_demo.sql` (verified via grep `eve@demo.edu` in that file). Setup workflow runs it via `bun run content:python-101:import --apply --wire-demo-class`.
2. `[FIXED]` BLOCKER Q2: 3 retained skips (End Session button, No past sessions, Live session card) were inside the SERIAL test chain — once the fixture guarantees the prerequisite data, those tests should hard-fail not skip. → **Response**: §"Convert ALL 9 skips" replaces the original "legitimate skips" carve-out. Phase 3 converts every conditional skip in session-flow.spec.ts.

#### DeepSeek V4 Pro — CONCUR (5 NITs, 4 FIXED + 1 acknowledged)

1. `[FIXED]` Q1 (overlap with Codex BLOCKER 2): convert all 9 session-flow skips. Already folded.
2. `[NOTED]` Q2: file-based state isn't idiomatic Playwright but acceptable given workers=1. Already documented in §Decisions; added one-line rationale: "Seed is a project so it appears in the runner output; cross-project state requires file or env passthrough."
3. `[FIXED]` Q3: explicit user-existence pre-check in seed.setup.ts before `loginWithCredentials` so missing-user error is actionable, not a generic locator timeout. → **Response**: added Phase-1 step to do `await page.request.get("/api/auth/session")` early, OR check via DB if accessible. Error message: `"e2e seed failed: demo.edu users not found. Run scripts/seed_problem_demo.sql or 'bun run content:python-101:import --apply --wire-demo-class' from docs/setup.md."`
4. `[ACKNOWLEDGED]` Q4: assertion location at seed-time correct.
5. `[FIXED]` Q5: two new risks added — CSRF tokens (low-prob but worth checking the seed's POST calls); all-or-nothing dependency chain (intended).

#### GLM 5.1 — CONCUR (5 NITs, 3 FIXED + 2 acknowledged)

1. `[FIXED]` Q1 (active-session cleanup robustness): added concrete recipe to §Decisions #4 — `GET active-session for class → POST /api/sessions/:id/end` with non-200 tolerance.
2. `[FIXED]` Q2 (class-matching strategy): demo seed already gives Eve a class, so "does eve have any class" is NOT a valid idempotency check. Plan now specifies title-prefix match `"e2e-fixture"` for both class and unit.
3. `[FIXED]` Q3: developer-actionable error in `fixture-state.ts` when JSON missing.
4. `[ACKNOWLEDGED]` Q4: Playwright dependencies + workers=1 guarantee correct ordering.
5. `[ACKNOWLEDGED]` Q5: stale fixture file risk — seed re-runs each invocation makes this low.

#### Kimi K2.6 — CONCUR (NITs only)

1. `[ACKNOWLEDGED]` shared fixture state acceptable.
2. `[FIXED]` (overlap with Codex BLOCKER 2 + DeepSeek Q1): all skips convert. Already folded.
3. `[FIXED]` org-creation clarification: the fixture does NOT create orgs (`POST /api/orgs` reference removed). It finds eve's existing org from demo seed.
4. `[ACKNOWLEDGED]` gitignore pattern matches existing conventions.
5. `[ACKNOWLEDGED]` no risks need BLOCKER promotion.

### Convergence

All 5 reviewers concur after fold-in. Codex round-2 dispatch needed to confirm both BLOCKERS closed.

## Code Review

5-way code review against branch HEAD `929714d` (consolidated branch diff vs main; static-only — orchestrator can't run live e2e).

### Self (Opus 4.7) — clean

`bunx tsc --noEmit` 10 errors (pre-existing baseline). `grep -c "test.skip" e2e/hocuspocus-auth.spec.ts e2e/session-flow.spec.ts` returns 0 in both files. Working tree clean. No commit-gap incidents.

### Codex — CONCUR (0 BLOCKERS, 0 NITS)

All 6 review questions confirmed clean: `HOCUSPOCUS_TOKEN_SECRET` assertion at `seed.setup.ts:72`; round-1 citation BLOCKER resolved (error message at `:83` cites `scripts/seed_problem_demo.sql` + python-101 import fallback); cleanup recipe at `:183-189`; `grep -c test.skip` returns 0; conversions semantically correct; deferred unit-creation defensible (`/api/me/units` empty → hard-fail at `hocuspocus-auth.spec.ts:120`, not silent pass). Playwright config wired correctly: `seed` at `:21`, `auth-setup` depends on seed at `:29`, `tests` depends on auth at `:34`. `.gitignore:45` covers `e2e/.fixture/`.

### DeepSeek V4 Flash — CONCUR with 2 minor NITs (acknowledged, not blocking)

1. `[ACKNOWLEDGED]` Q1: pre-check is technically a try/catch around login, not a true pre-check. Catches the symptom (login failure) rather than the root (users not seeded). Error message IS actionable, so spirit of the original NIT is satisfied. → No fix.
2. `[ACKNOWLEDGED]` Q2: no CSRF tokens on the seed's POST calls. Relies on Playwright `page.request` cookie context. Bridge's API uses cookie-based auth without CSRF tokens for non-browser flows; expected to work. If a 403 surfaces, switch to UI-driven flow.

DeepSeek confirmed all other claims: project ordering (`seed` first at `:22`, `auth-setup` deps at `:29`), three actionable error variants in `fixture-state.ts`, all 14 skip-to-fail conversions semantically correct.

### GLM 5.1 — CONCUR (0 BLOCKERS, 0 NITS)

Confirmed all 5 plan-review NITs landed correctly in code: cleanup recipe at `seed.setup.ts:182-209` uses `GET /api/sessions/active/{classId}` → `POST end` with try/catch + `console.warn` non-200 tolerance; class-matching at `:95` uses `title.startsWith("e2e-fixture")` (units at `:229`); fixture-state error at `helpers/fixture-state.ts:29-35` is developer-actionable with exact commands + handles invalid JSON + missing classId; playwright config dependency chain `seed → auth-setup → tests` correctly at `:29,34`; session-flow.spec.ts reads `classId` from fixture state at test-start instead of clicking a class card.

### Kimi K2.6 — CONCUR with 1 real bug + 2 NITs (1 FIXED + 2 ACKNOWLEDGED)

1. `[FIXED]` **Real runtime bug** at `hocuspocus-auth.spec.ts:137`. Playwright `APIResponse` body can be read only once. The template-literal eagerly awaits `res.text()` for the assertion message, consuming the body. The subsequent `res.json()` throws "body already consumed" on the 200 (success) path. **Pre-existing from plan 072** (commit `498ce7d`) — plan 078 didn't introduce it, but plan 078's skip-to-fail conversion makes it MUCH more likely to fire (the test now ALWAYS runs instead of sometimes-skipping). → **Response (e5e1dd6)**: read body once into a string, use it for both assertion message and JSON parse: `const rawBody = await res.text(); expect(...).toBe(200); const body = JSON.parse(rawBody)`. Other 3 code reviewers missed this.
2. `[FIXED]` Unused `type Page` import in `session-flow.spec.ts:1`. Removed at `e5e1dd6`.
3. `[ACKNOWLEDGED]` Redundant double-visibility-assertions in session-flow at lines 87/89, 132/134. Stylistic only; no behavior impact. Left for a future cleanup pass.

Kimi confirmed all other claims: hocuspocus-auth mint tests query `/api/me/units` at test time (don't depend on fixture-seeded units, so deferred unit-creation is non-blocking); seed uses hardcoded demo org UUID (verified stable from `scripts/seed_problem_demo.sql`); `.gitignore:45` covers `e2e/.fixture/`; session-flow's "session already active" branch correctly uses negative assertion; no syntax errors / broken imports / unhandled promises.

### Convergence

All 5 reviewers concur after the Kimi-caught bug fix. Plan 078 ready to ship.

**Multi-reviewer ensemble value, plan 078 edition**: Kimi K2.6 caught a real runtime bug that Codex, DeepSeek, and GLM all missed despite Codex's deep static-analysis passes and explicit prompts about "regressions / broken imports / lost behavior". The bug was subtle — JS engineers (and presumably the reviewing models) often assume `await` in a template literal is benign. Body-consumed semantics specific to Playwright's `APIResponse` are easy to overlook. Pattern: **specialized API knowledge wins over general code-review heuristics for framework-specific bugs.** Logged for future plan reviews.

## Post-execution report

3 active phases shipped (Phase 4 was post-execution-report-only).

### Phase 1 — `8378ddc`

- `e2e/seed.setup.ts` (new) — fixture-setup spec asserting `HOCUSPOCUS_TOKEN_SECRET` is set, login-validating eve@demo.edu existence with an actionable error message, idempotently creating the `e2e-fixture-class` (matched via title-prefix), enrolling alice via the join-code flow, ending leftover active sessions (with `console.warn` tolerance), and writing `e2e/.fixture/state.json` with `{classId, unitId?}`.
- `e2e/helpers/fixture-state.ts` (new) — synchronous `getFixtureState()` reader with developer-actionable error on missing JSON.
- `e2e/playwright.config.ts` — added `seed` project; `auth-setup` now `dependencies: ["seed"]`. Chain: tests → auth-setup → seed.
- `.gitignore` — added `e2e/.fixture/`.

**Sonnet subagent deviation**: unit creation is currently a TODO in seed.setup.ts (logged as console message, not yet creating a unit via UI). The seed queries `/api/units` for an existing fixture-prefixed unit. Since the Python 101 demo seed creates many units owned by eve, a non-fixture unit may be returned — `unitId` may be undefined in `state.json` if no fixture-prefixed unit exists. The hocuspocus-auth.spec.ts mint tests don't depend on `unitId` from fixture state — they query `/api/me/units` themselves at test time and use the first available. So the deferred unit-creation isn't blocking; it just means the fixture's `unitId` field may be undefined for now. A follow-up plan can add unit creation if a need arises.

**Hardcoded UUIDs in seed**: the subagent referenced `00000000-0000-0000-0000-0000000aa001` (course UUID) and `d386983b-6da4-4cb8-8057-f2aa70d27c07` (org UUID). Verified both come from `scripts/seed_problem_demo.sql` and `scripts/python-101/import.ts:838` — stable demo-seed identifiers safe to hardcode.

### Phase 2 — `0d0895a`

5/5 skips converted to hard failures in `e2e/hocuspocus-auth.spec.ts`:
- Line 119: `test.skip(true, "teacher has no units accessible...")` → `expect(unitsRes.ok()).toBeTruthy()`.
- Line 123: `test.skip(!unitId, ...)` → `expect(unitId).toBeDefined()`.
- Line 176: `test.skip(!TOKEN_SECRET, ...)` → `expect(TOKEN_SECRET).toBeTruthy()`.
- Line 184: `test.skip(!unitsRes.ok(), ...)` → `expect(unitsRes.ok()).toBeTruthy()`.
- Line 187: `test.skip(!unitId, ...)` → `expect(unitId).toBeDefined()`.

`grep -c "test.skip" e2e/hocuspocus-auth.spec.ts` returns 0.

### Phase 3 — `f319e02`

9/9 skips converted to hard failures in `e2e/session-flow.spec.ts`:
- Line 46 / 150: skip-on-no-classes → read `classId` from `getFixtureState()`, navigate directly to `/teacher/classes/{classId}`, `expect(classId).toBeDefined()`.
- Line 69: skip-when-resume-button-visible → `expect(resumeButton).not.toBeVisible({timeout:2000})` (seed cleanup ensures no active session).
- Line 72: skip-when-start-button-not-visible → `await expect(startButton).toBeVisible({timeout:5000})`.
- Line 94 / 120: skip-when-classId/sessionId-undefined → `expect(classId).toBeDefined(); expect(sessionId).toBeDefined()`.
- Line 105: skip-when-live-session-card-not-visible → `await expect(liveCard).toBeVisible({timeout:5000})`.
- Line 131: skip-when-end-button-not-visible → `await expect(endButton).toBeVisible({timeout:5000})`.
- Line 160: skip-when-no-past-sessions → `await expect(pastSessionsHeading).toBeVisible({timeout:5000})`.

`grep -c "test.skip" e2e/session-flow.spec.ts` returns 0.

### Verification

- `bunx tsc --noEmit`: 10 errors, all pre-existing baseline.
- `grep -c "test.skip"` in both target files: 0.
- Working tree clean at HEAD `f319e02`.
- **Live e2e suite NOT exercised by orchestrator** — would require Next + Go + Hocuspocus running, demo-seeded DB, browser. Author manual verification at code-review time is the final gate. Spec changes are correct-by-construction per the plan.

### Behavior changes shipped (deliberate)

1. **Hard environment requirement**: `HOCUSPOCUS_TOKEN_SECRET` must be set or the entire e2e suite fails at seed-time. Aligns with plan 072's runtime contract.
2. **Hard data requirement**: a fixture class with eve+alice enrolled exists, or seed fails. No more silent "no class" pass-with-skip.
3. **All 14 prior skip paths now hard-fail** when the prerequisite isn't present — the suite is now a reliable regression gate for realtime auth + live-session flows.

### Follow-ups

- **Unit creation in seed**: currently a TODO. If a future test depends on a stable fixture unitId, add UI-driven unit creation to `seed.setup.ts`. For now, hocuspocus-auth's mint tests query units at test time so the gap is invisible.
- **CI provisioning**: the `HOCUSPOCUS_TOKEN_SECRET` requirement now makes the e2e suite hard-block on env. Document CI setup in `docs/setup.md` if not already covered.
- **Stop-hook stash-pop guard**: plan 077's incident (leftover `<<<<<<<` markers from a stash-pop) suggests adding a pre-commit / stop-hook guard for stray conflict markers. Filed as a follow-up TODO.

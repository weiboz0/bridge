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

- Hits the Go API directly (`POST /api/orgs`, `POST /api/classes`, `POST /api/orgs/{id}/members`, `POST /api/classes/join`) to ensure the canonical fixture entities exist. Idempotent — checks before creating.
- Asserts `HOCUSPOCUS_TOKEN_SECRET` is present in the runner's env (`expect(process.env.HOCUSPOCUS_TOKEN_SECRET).toBeTruthy()`). If missing, the seed project FAILS — the rest of the suite never runs.
- Stashes a tiny JSON fixture-meta file (e.g., `e2e/.fixture/state.json` — gitignored) that records the seeded `classId`, `unitId`, `sessionId-or-null` for spec files to consume. Avoids re-querying every test.

Seed project depends on nothing; `auth-setup` depends on `seed`; `tests` depends on `auth-setup`. Chain: seed → auth-setup → tests.

The fixture files use the existing `demo.edu` seeded users (created by `psql -f scripts/seed_bridge_hq.sql` per `docs/setup.md`). The seed project doesn't create users — it relies on the setup-time SQL seed. Plan 078 just adds the realtime-relevant entities (class, unit) and the env-var assertion. Avoids adding a SECOND user-creation pathway.

### Skip-to-fail conversion

After the fixture is in place, the conditional skips in `hocuspocus-auth.spec.ts` and `session-flow.spec.ts` become hard failures:

- **`hocuspocus-auth.spec.ts:119, 123, 184, 187`** — replace `test.skip(!unitId, ...)` with `expect(unitId).toBeDefined()` or simply read `unitId` from the fixture-state and proceed.
- **`hocuspocus-auth.spec.ts:176`** — `test.skip(!TOKEN_SECRET, ...)` becomes `expect(TOKEN_SECRET).toBeTruthy()`. The seed project already asserts this; this is belt-and-suspenders for direct test runs.
- **`session-flow.spec.ts:46, 94, 105, 120, 150`** — these read `classId` / `sessionId` from previous tests OR from missing UI. With the fixture seeding a class + enrollments, the `no class` cases become hard failures. The "session already active" / "Start button not visible" branches can be handled by ensuring the fixture cleans up active sessions in its setup phase.

### What stays as legitimate skip

A few skip patterns ARE valid and stay:
- "End Session" button not visible AFTER a session ended — that's a legitimate downstream consequence, not a missing prerequisite.
- "No past sessions" if the fixture itself doesn't seed a past session — the fixture is forward-looking, not backwards-looking. Hard-failure here is wrong. Either add past-session seeding (more scope) or accept this skip.

The plan keeps the legitimate skips, converts the data-prerequisite ones to hard failures, and asserts env at fixture-setup time.

## Decisions to lock in

1. **Reuse the existing demo.edu user seed.** Don't create a parallel user-creation pathway. Plan 078 adds class + unit + active-session-cleanup, not users.
2. **Fixture writes to a gitignored `e2e/.fixture/state.json`** so spec files can pick up `classId`/`unitId` without re-querying. Single source of truth for "what the suite seeded".
3. **HOCUSPOCUS_TOKEN_SECRET asserted at seed time** — fail loudly if missing. Documents the contract: e2e CANNOT run without the secret. Aligns with plan 072's runtime-required posture.
4. **Active-session cleanup in seed**. Before tests run, the seed project ends any active session for the class so `start session` doesn't bump into a leftover. Use `POST /api/sessions/:id/end` from a previous run.
5. **Past-sessions skip stays.** Adding past-session seeding is out of scope; that test's assertion ("teacher can see past sessions list") legitimately needs historical data the seed doesn't provide.
6. **No new playwright workers.** The serial-execution config (`workers: 1`) stays — review §4.1 acknowledges this is fine, the issue was hidden skips, not parallelism.
7. **CI-only enforcement OR everywhere?** Make it everywhere. Local dev should also fail fast on missing fixture so a developer's first `bun run test:e2e` makes the missing-secret state visible immediately, not after 90 seconds of green-with-skips.

## Files

**Modify (4 files):**

- `e2e/playwright.config.ts` — add a new `seed` project before `auth-setup`. Update `dependencies` chain: `tests` → `auth-setup` → `seed`.
- `e2e/hocuspocus-auth.spec.ts` — convert `test.skip(!unitId, ...)` → `expect(unitId).toBeDefined()`; convert `test.skip(!TOKEN_SECRET, ...)` → `expect(TOKEN_SECRET).toBeTruthy()`. Read `classId` / `unitId` from the fixture-state file.
- `e2e/session-flow.spec.ts` — convert "no classes" / "student not enrolled" / "no session was started" skips to hard failures. Keep "End Session button not visible" (downstream) and "No past sessions visible" (out-of-scope) as legitimate skips with a clarifying comment. Read seeded entity ids from fixture state.
- `.gitignore` — add `e2e/.fixture/`.

**Create (2 files):**

- `e2e/seed.setup.ts` — the new fixture-setup spec. Asserts `HOCUSPOCUS_TOKEN_SECRET` is set, ensures a class with eve+alice enrolled exists, ensures a published unit linked to a topic in that class's course exists, ends any leftover active sessions, writes `e2e/.fixture/state.json` with `{classId, unitId}`.
- `e2e/helpers/fixture-state.ts` — small helper exposing `getFixtureState(): Promise<{classId: string, unitId: string}>`. Reads the JSON file; throws if missing (suite has a bug). Specs import this instead of re-querying the API.

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
| The fixture's class-creation step duplicates the demo seed's class | medium | Idempotency: query existing classes for eve first; create only if missing. The plan's "1 class" is the canonical e2e fixture class — distinguished from any demo classes by a known prefix (e.g., `e2e-fixture-class`). |

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

### Phase 3 — Convert session-flow.spec.ts skips (commit 3)

- Read `classId` from fixture state; remove "no classes" skip at line 46.
- Remove "session already active" skip — fixture ends active sessions before tests run.
- Keep "End Session button not visible" with a clarifying comment (downstream, legitimate).
- Keep "No past sessions" with a comment about future fixture extension.
- Run `bun run test:e2e e2e/session-flow.spec.ts` — confirm 6 of 9 skips convert; 3 remain as documented exceptions.
- Commit: `plan 078 phase 3: session-flow — convert data-prerequisite skips to hard failures`.

### Phase 4 — Verify + post-execution report (commit 4)

- Run full e2e suite locally with all three services up. Confirm seed project runs, all auth+session tests execute (no skips on the converted paths), suite passes.
- Update post-execution report.
- Commit: `docs: plan 078 post-execution report`.

After Phase 4, run the 5-way code review. Note: e2e suite cannot be exercised in the reviewer sandbox; manual verification by the author is the gate.

## Plan Review

(pending — 5-way before implementation)

## Code Review

(pending — 5-way at branch-diff time)

## Post-execution report

(pending)

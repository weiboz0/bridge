# Plan 074 — Shadow Next API cleanup (`/api/orgs`) + contract-parity lint

## Problem (review 011 §1.2)

`next.config.ts` proxies `/api/orgs/:path*` (and ~25 other path prefixes) to the Go API. But the repo still carries overlapping Next API route files at the same paths, with stale Drizzle writes and their own auth decisions. The most acute case is `/api/orgs/[id]/members/[memberId]/route.ts` — its PATCH/DELETE handlers do NOT have the self-action guard that plan 069 phase 4 added on the Go side. If the Next proxy were ever bypassed (misconfig, server-action import, future routing change), an `org_admin` could suspend or remove their own membership and lock themselves out of the org.

The shadow routes are unreachable in normal operation — Next.js's proxy beats them — but they sit there as latent code that grows stale, drifts from the Go contract, and represents a real security hazard on the day the proxy ever changes.

Review 011 §1.2 recommendation, verbatim:

> Delete or quarantine the migrated Next route files in one cleanup plan. Until deletion, add a test that fails when proxied Go routes still have executable Next handlers under the same path.

## Current state

- 4 shadow files under the proxied `/api/orgs/:path*` prefix:
  - `src/app/api/orgs/route.ts` (64 lines)
  - `src/app/api/orgs/[id]/route.ts` (86 lines)
  - `src/app/api/orgs/[id]/members/route.ts` (83 lines, line 53 = direct `addOrgMember` Drizzle write)
  - `src/app/api/orgs/[id]/members/[memberId]/route.ts` (71 lines, lines 37 + 65 = PATCH/DELETE without self-action guard)

- 2 Vitest integration test files exercise these shadow routes (importing `PATCH`, `DELETE`, etc. directly):
  - `tests/integration/orgs-api.test.ts`
  - `tests/integration/org-members-api.test.ts`

- 39 total Next API route files (verified: `find src/app/api -name route.ts | wc -l = 39`). Breakdown after the /api/orgs deletion:
  - 31 shadow files under proxied prefixes: `/api/admin` (4), `/api/ai` (2), `/api/assignments` (4), `/api/classes` (3 — including `classes/join` which has Go parity at `platform/internal/handlers/classes.go:239`), `/api/courses` (6), `/api/documents` (3), `/api/parent` (1), `/api/sessions` (7), `/api/submissions` (1).
  - 4 legitimate-Next files NOT under any GO_PROXY_ROUTES prefix: `/api/auth/[...nextauth]`, `/api/auth/debug`, `/api/auth/logout-cleanup`, `/api/auth/signup-intent`. (`/api/auth/register` IS proxied but no Next file exists at that path, so nothing to allowlist there.)
  - The next.config.ts comment estimate of "~42" predates plan 069's drift work; actual is 39.

- Go has full parity for the `/api/orgs/...` surface:
  - Routes registered at `platform/internal/handlers/orgs.go:29-34` (ListMembers GET, AddMember POST, UpdateMember PATCH, RemoveMember DELETE).
  - Self-action guards landed in plan 069 phase 4 at `orgs.go:409` (UpdateMember) and `orgs.go:470` (RemoveMember).
  - Test coverage in `platform/internal/handlers/org_self_action_guard_test.go` (4 tests: SelfSuspendForbidden, SelfActivateAllowed, SelfRemoveForbidden, PlatformAdminBypass) plus existing handler + store tests.

- No source code in `src/` imports from any shadow `route.ts` file (verified: `grep -rn "from.*api/orgs" src/ --include="*.ts"` returns 0). Route files only export HTTP method handlers consumed by Next.js's router; tests are the only importers.

## Approach

Three-phase, single-PR cleanup that fixes the immediate `/api/orgs` security hazard, locks in a reusable contract-parity guard, and leaves a paper trail for the broader sweep:

### Phase 1 — Delete the `/api/orgs` shadow routes + their tests

Per review §1.2's primary scope. Delete:
- All 4 files under `src/app/api/orgs/`.
- Both Vitest integration tests that import them.
- Empty parent directories left over (`src/app/api/orgs/` should be removed).

Go-side test coverage (`org_self_action_guard_test.go` + handler/store tests) is the canonical suite from now on.

### Phase 2 — Contract-parity Vitest test (shadow-routes guard)

Add `tests/unit/shadow-routes.test.ts` that:
1. Reads `GO_PROXY_ROUTES` from `next.config.ts` (parse the array once at import time — `next.config.ts` is a TypeScript module, importable from a vitest test).
2. Walks `src/app/api/` recursively for `route.ts` files.
3. For each route file, derives its URL path (`src/app/api/orgs/[id]/route.ts` → `/api/orgs/:id`) and checks whether it matches any proxied prefix from `GO_PROXY_ROUTES`.
4. Fails if a route file sits under a proxied prefix UNLESS it is in an explicit `KNOWN_SHADOW_ALLOWLIST` array.
5. The allowlist enumerates only shadow files (~31 paths) — files that sit under a proxied prefix but haven't been deleted yet. Each entry has a one-line comment noting "shadow: pending plan-NNN cleanup". Files NOT under any proxied prefix (e.g., `/api/auth/[...nextauth]`, `/api/auth/debug`) are never inspected by the test, so they don't need allowlist entries.

This makes the existing tech debt grep-able and reducible plan-by-plan: future plans (075+) shrink the allowlist as they migrate each surface. NEW shadow routes added by mistake fail the test immediately.

### Phase 3 — Update `next.config.ts` comment + TODO.md

- Update the TODO comment at the top of `next.config.ts` (currently says "~42 Next.js API route files") to reflect the new count and reference plan 074.
- Add a TODO.md entry under "Tech debt" linking to the allowlist as the canonical worklist.

## Decisions to lock in

1. **Narrow scope per review 011 §1.2.** Plan 074 deletes ONLY the `/api/orgs` shadow routes — the ones with the self-action-guard gap that motivated the review finding. The remaining ~33 shadow files go in the allowlist, not the delete list. Reasons: (a) review estimated 0.5-day scope, (b) each path needs Go-side parity verification before its shadow can be deleted (different surfaces, different contracts), (c) bigger sweeps mean larger PRs with worse reviewability.
2. **Allowlist as code, not as a comment.** Future plans grep the allowlist to find the next migration target; a comment in `next.config.ts` would be invisible to tooling.
3. **Vitest, not eslint.** The contract-parity check is a behavior assertion (does the file system match the proxy config?), not a syntactic lint. Vitest already runs in CI; a custom eslint rule would need plumbing.
4. **Delete tests, don't migrate them to Go.** The shadow-route Vitest tests exercise unreachable code paths. Their assertions are duplicated by Go-side handler/store tests with better coverage of the self-action guard. Migration adds work without value.
5. **No new Go tests.** Go-side parity already exists; this plan does not touch `platform/`.

## Files

**Delete (6 files):**

- `src/app/api/orgs/route.ts`
- `src/app/api/orgs/[id]/route.ts`
- `src/app/api/orgs/[id]/members/route.ts`
- `src/app/api/orgs/[id]/members/[memberId]/route.ts`
- `tests/integration/orgs-api.test.ts`
- `tests/integration/org-members-api.test.ts`

(After deletion, also remove the now-empty `src/app/api/orgs/` directory tree.)

**Create (1 file):**

- `tests/unit/shadow-routes.test.ts` — the contract-parity test described in Phase 2. Includes `KNOWN_SHADOW_ALLOWLIST` array enumerating remaining shadow paths.

**Modify (2 files):**

- `next.config.ts` — update the TODO comment at top to reflect the new shadow-file count and reference plan 074 as the cleanup harness.
- `TODO.md` — add "shadow-route allowlist shrinkage" as a tracked item under tech-debt section, linking to `tests/unit/shadow-routes.test.ts`.

**No changes to:**

- `platform/` (Go API has full parity already).
- `src/lib/org-memberships.ts`, `src/lib/db/schema.ts`, `src/lib/db.ts` — these are still consumed by other code in `src/lib/` (auth, parent-reports, etc.); only the route handlers go away.
- `src/app/api/auth/[...nextauth]/route.ts` and other legitimately-Next-only routes (NextAuth, debug, logout-cleanup, signup-intent — not under any GO_PROXY_ROUTES prefix).

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| A future Next.js feature (server action, Edge route) mistakenly imports `@/app/api/orgs/...` after deletion | low | TypeScript catches the missing module for static imports. Dynamic `import()` and string-literal `fetch("/api/orgs/...")` aren't caught by TS — the Phase 1 pre-impl grep covers them; ongoing protection comes from `fetch("/api/orgs/...")` continuing to work because the proxy still routes to Go (the route handler going away doesn't break callers, it removes the dead-code shadow). |
| The contract-parity test produces false positives on legitimately-Next routes | medium | The allowlist explicitly includes legitimate-Next routes (e.g., `/api/auth/[...nextauth]`) with a "legitimate-next" tag distinct from the "shadow-pending-cleanup" tag. Reviewers can tell the two cases apart. |
| `next.config.ts` parsing in the Vitest test is fragile (re-runs vitest config every test) | low | Import the rewrites function once in the test setup; cache the parsed array at module load. |
| Deleting the Vitest integration tests reduces overall test count and looks like coverage regression | low | Document in the post-execution report that the tests exercised dead code; Go-side coverage is the canonical suite. |
| The allowlist grows stale (reviewers add new shadow files with allowlist entries to silence the test) | medium | Allowlist entries require a comment with a plan number reference. PR review catches "added without justification" cases. Same pattern Bridge already uses for skipped tests. |
| Other test files (e2e, etc.) reference the deleted files | low | Pre-impl grep planned for Phase 1: `grep -rln "@/app/api/orgs" tests/ e2e/` returns only the two files being deleted (verified during plan drafting). |
| Dynamic / catch-all path matcher in the contract-parity test breaks on edge cases (`[...slug]`, `[[id]]`, multiple `[id]` segments, nested dynamic) | low | (Codex round-1 NIT, downgraded after DeepSeek round-1: prefix matching handles `[...slug]` / `[[id]]` correctly because they don't appear in the current GO_PROXY_ROUTES set or in `src/app/api/`. The test still includes inline fixtures for static, single-dynamic `[id]`, and `:path*` patterns — the three patterns that DO appear today.) |
| Regex parsing of `next.config.ts` is brittle if the GO_PROXY_ROUTES array format changes (variable interpolation, spreads, dynamic construction) | medium | (DeepSeek V4 Pro round-1 NIT.) The test fails loudly on parse failure rather than silently passing. `tests/unit/middleware-proxy-parity.test.ts` shares this fragility and serves as a canary — both tests would fail on the same refactor, surfacing the issue before merge. The future `src/lib/go-proxy-routes.ts` extraction (TODO comment in the test header) eliminates this entire risk class. |

## Phases

### Phase 1 — Delete `/api/orgs` shadow routes + their tests (commit 1)

- Pre-impl verification (two greps — Kimi K2.6 round-1 NIT extends the second one):
  - `grep -rln "@/app/api/orgs" tests/ e2e/ src/` — confirms zero static-import callers (TypeScript catches these).
  - `grep -rn "/api/orgs/" src/ --include="*.tsx" --include="*.ts"` — confirms NO client-side `fetch("/api/orgs/...")` or dynamic `import()` references that would 404 silently after the route handlers are gone. Any hits here must be evaluated: a `fetch` call still works because the request is proxied to Go, but stale imports of helpers from the route file would break.
- **Coverage equivalence inventory** (Codex round-1 NIT): for each `it(...)` block in `tests/integration/orgs-api.test.ts` and `tests/integration/org-members-api.test.ts`, document the equivalent Go test that covers the same behavior. Acceptable forms: same-test-name in `platform/internal/handlers/orgs_test.go`, same-behavior in `platform/internal/handlers/org_self_action_guard_test.go`, or same-behavior in `platform/internal/store/orgs_test.go`. If any vitest assertion has NO Go counterpart, surface it before deleting — either add the missing Go test as a same-PR side-effect or carve the assertion out for separate follow-up. Document the inventory inline in the Phase 1 commit message body.
- Delete the 4 route files + 2 vitest integration tests listed in §Files.
- Remove now-empty `src/app/api/orgs/` directory tree.
- Run `bun run test` (Vitest) — expect the two deleted test files gone from the run, all other suites pass.
- Run `bunx tsc --noEmit` — expect 0 errors (no TS reference to the deleted files anywhere).
- Run `cd platform && go test ./... -count=1 -timeout 120s` — confirm the Go-side coverage cited in the inventory is actually green.
- Commit: `plan 074 phase 1: delete /api/orgs shadow Next routes + their vitest integration tests` (commit body includes the coverage inventory).

### Phase 2 — Add contract-parity test (commit 2)

- Create `tests/unit/shadow-routes.test.ts`.
- Implementation: read `next.config.ts` as TEXT via `readFileSync` and extract `GO_PROXY_ROUTES` entries via regex (same pattern as the existing `tests/unit/middleware-proxy-parity.test.ts:21-33`). Direct module import fails because it pulls in `NextConfig` types and the Turbopack runtime, which doesn't load cleanly outside the Next.js build (DeepSeek V4 Pro round-1 BLOCKER). Strip `//` line comments before regex matching to avoid false positives from quoted paths in deprecation notes (precedent from plan-065 review-2). Walk `src/app/api/` for `route.ts` files using `node:fs`; convert each path to a URL pattern (prefix match: strip trailing `:path*` from the proxy entry, then check whether the derived URL path starts with the result); assert each match is in `KNOWN_SHADOW_ALLOWLIST`.
- **Typed allowlist entries** (GLM 5.1 round-1 NIT): use a typed shape, not bare strings.
  ```ts
  type ShadowEntry = {
    path: string;        // canonical URL path, e.g. "/api/courses/:id"
    cleanupPlan: string; // future plan that will delete this, e.g. "075" or "TBD"
    note?: string;       // optional clarification (e.g., complex Go parity status)
  };
  const KNOWN_SHADOW_ALLOWLIST: ShadowEntry[] = [...];
  ```
  Prevents typos and makes the structure self-documenting. Allowlist contains ONLY shadow files (under proxied prefixes); legitimate-Next files (e.g., `/api/auth/[...nextauth]`) are never inspected by the test.
- **Matcher fixtures** (Codex round-1 NIT): include explicit fixtures inside the same test file covering: (a) static path, (b) single dynamic `[id]`, (c) nested dynamic, (d) catch-all `[...slug]`, (e) `:path*` matcher. The fixtures live next to the production assertions so any conversion bug fails CI immediately.
- **TODO comment** (GLM 5.1 round-1 NIT, low-urgency): note in the test file's header comment that a future plan should extract `GO_PROXY_ROUTES` to `src/lib/go-proxy-routes.ts` so both `next.config.ts` and the test import the same source. Not in scope for plan 074 — it's a config refactor that touches Next's build pipeline.
- **Pin test to `environment: 'node'`** (Kimi K2.6 round-1 NIT): the test uses `node:fs` to walk `src/app/api/`. Default Vitest environment may be jsdom (per `vitest.config.ts`); pin this file explicitly via `// @vitest-environment node` comment at the top.
- **(Kimi K2.6 NIT about regex fallback obsoleted by DeepSeek BLOCKER fix.)** Phase 2 now uses regex parsing as the PRIMARY path (per the established pattern), so a fallback is moot. The parse is still brittle in the same way `middleware-proxy-parity.test.ts` is — see the new §Risks row.
- **Bidirectional allowlist check** (Kimi K2.6 round-1 NIT — this is the load-bearing one): the test asserts in BOTH directions:
  - Forward: every shadow file under a proxied prefix MUST be in `KNOWN_SHADOW_ALLOWLIST`. Catches added shadow routes.
  - Reverse: every entry in `KNOWN_SHADOW_ALLOWLIST` MUST point to a real file on disk. Catches stale ghosts when future plans delete a shadow file but forget the allowlist sweep.
  The reverse check makes the allowlist self-cleaning across the lifetime of the worklist.
- Allowlist initial population: enumerate remaining 31 shadow paths verbatim, each with `cleanupPlan: "TBD"` until follow-up plans claim them.
- Run `bun run test tests/unit/shadow-routes.test.ts` — expect PASS (allowlist exhaustively covers current state).
- Sanity check: temporarily add a fake route at `src/app/api/orgs/test-shadow/route.ts`, re-run the test, expect FAIL with a clear "unallowlisted shadow path" message; remove the fake route, expect PASS.
- Commit: `plan 074 phase 2: add shadow-routes contract-parity test + initial allowlist`.

### Phase 3 — next.config.ts comment + TODO.md (commit 3)

- Update the comment at the top of `next.config.ts` to reflect "33 shadow Next handlers remain under proxied prefixes; see `tests/unit/shadow-routes.test.ts` allowlist for the worklist. Plan 074 deleted the /api/orgs surface."
- Add to `TODO.md` under tech debt: "Shadow Next API allowlist shrinkage — see `tests/unit/shadow-routes.test.ts:KNOWN_SHADOW_ALLOWLIST`. Each entry needs a follow-up plan to delete or replace."
- Run full `bun run test` one more time + `bunx tsc --noEmit`.
- Commit: `plan 074 phase 3: doc cleanup + TODO.md`.

After Phase 3, run the 5-way code review against the consolidated branch diff (single-PR-per-plan policy), fold findings, open the PR via Step 6.

## Plan Review

### Round 1 (2026-05-06)

#### Self-review (Opus 4.7) — clarification

Folded one precision update at `ce48069`: tightened §Current state file counts (39 total, 31 shadow + 4 legitimate-Next after /api/orgs deletion); confirmed `/api/auth/register` has no Next file (Go-only); verified `/api/classes/join` has Go parity at `platform/internal/handlers/classes.go:239`.

#### Codex — CONCUR (3 NITs, all FIXED)

1. `[FIXED]` Coverage equivalence inventory — Phase 1 needs to enumerate which Go test covers each deleted vitest assertion. → **Response**: added "Coverage equivalence inventory" step at the top of Phase 1 with explicit inline-in-commit-body documentation; pre-impl now requires per-`it()` mapping to a Go test, with fail-loud if any vitest assertion has no Go counterpart.
2. `[FIXED]` Allowlist framing contradiction — earlier text mixed "shadow files only" with "shadow + legitimate-Next" framing. → **Response**: tightened §Approach Phase 2 description to "allowlist contains ONLY shadow files (~31 paths); files NOT under any proxied prefix are never inspected by the test". Removes the contradiction.
3. `[FIXED]` Missing risk for dynamic/catch-all matcher edge cases. → **Response**: added new risk row covering `[...slug]`, `[[id]]`, nested dynamic, `:path*`; mitigation is in-test fixtures for each pattern type.

Codex round-1 also confirmed direction: "Phase order is correct. Deleting the acute /api/orgs files first, then adding the allowlist guard, then updating docs follows a sound dependency order and keeps the 0.5-day scope intact."

#### DeepSeek V4 Pro — BLOCKER (FIXED) + 2 NITs (FIXED)

1. `[FIXED]` BLOCKER: Phase 2 §1 claimed `next.config.ts` is importable from a Vitest test. False — `tests/unit/middleware-proxy-parity.test.ts:14-16` documents the opposite (Turbopack runtime doesn't load cleanly outside the Next build). → **Response**: revised Phase 2 to use `readFileSync` + regex extraction (same pattern as the existing test), with `//` comment stripping (precedent from plan 065 review-2). Kimi's "regex fallback" NIT became moot since regex is now the primary path.
2. `[FIXED]` NIT: §Phases path-matching description should say "prefix match" rather than implying full pattern algebra. → **Response**: tightened Phase 2 description; `[...slug]` / `[[id]]` corner cases don't appear in current GO_PROXY_ROUTES or `src/app/api/`, so the §Risks row was downgraded medium → low.
3. `[FIXED]` NIT: missing risk row for "regex parsing of next.config.ts is brittle if array format changes". → **Response**: added new risk row; mitigation cites `middleware-proxy-parity.test.ts` as canary (both tests fail on the same refactor).

DeepSeek V4 Pro round-1 also confirmed direction: "Allowlist-as-array is defensible. Co-located with the enforcing test, grep-able, each entry requires a comment with a plan reference."

#### GLM 5.1 — CONCUR (2 NITs, 1 FIXED, 1 deferred-to-test-comment)

1. `[FIXED]` Typed allowlist entries (`{path, cleanupPlan, note?}`) instead of bare strings — prevents typos, machine-checkable. → **Response**: added typed `ShadowEntry` shape to Phase 2.
2. `[DEFERRED]` Extract `GO_PROXY_ROUTES` to a shared `src/lib/go-proxy-routes.ts` module — eliminates import-coupling risk between next.config.ts and the test. GLM marked as low urgency. → **Response**: added a TODO comment in the test file header pointing to a future config-refactor plan; the extraction itself is out of scope (touches Next's build pipeline; deserves its own plan).

GLM round-1 also confirmed direction: "Defensible. The allowlist makes tech debt grep-able and shrinkable per-plan; a 31-file sweep would need individual Go-parity verification for each surface and bloats review."

#### Kimi K2.6 — CONCUR (4 NITs, 3 FIXED, 1 obsoleted)

1. `[FIXED]` Coverage mapping in commit message — same as Codex round-1 NIT 1, already covered by Phase 1 inventory step.
2. `[FIXED]` Hoist allowlist to top-level `const` with `// baseline: plan-074` header — incorporated into the typed `ShadowEntry` shape from GLM round-1.
3. `[FIXED]` Pin test to `environment: 'node'` (not jsdom) for `node:fs` walking + add a regex fallback for next.config.ts parse. → **Response**: pinned to node environment via `// @vitest-environment node` comment. Regex fallback obsoleted because DeepSeek BLOCKER resolution made regex the PRIMARY path.
4. `[FIXED]` Two missing risks:
   - **Stale allowlist ghosts** (load-bearing): when future plans delete a shadow file, the allowlist entry can rot if not also deleted. → **Response**: Phase 2 now does a BIDIRECTIONAL check — forward (every shadow file must be in allowlist) AND reverse (every allowlist entry must point to a real file). Self-cleaning across the lifetime of the worklist.
   - **Runtime string references**: `fetch("/api/orgs/...")` and dynamic `import()` aren't caught by TypeScript. → **Response**: extended Phase 1 pre-impl grep to `--include="*.tsx" --include="*.ts"` for string literals. Updated the related risk row to clarify that proxy routing keeps `fetch` calls working even after handler deletion.

Kimi round-1 also confirmed direction: "Test deletion is defensible... allowlist design is the right call... no phase reordering needed."

### Convergence

All 5 reviewers concur after fold-in (DeepSeek round-2 confirmed BLOCKER closed at `fd90894`). Plan 074 ready for implementation.

## Code Review

5-way code review against branch `feat/074-shadow-next-api-cleanup` HEAD `9ba9493` (consolidated branch diff vs main; 3 phases).

### Self (Opus 4.7) — clean

`bun run test` 634 passed / 11 skipped / 0 failed. `cd platform && go test ./... -count=1 -timeout 120s` all packages OK. `bunx tsc --noEmit` returns 10 errors, all pre-existing (verified via `git stash` comparison against main). Sanity-checked `tests/unit/shadow-routes.test.ts` forward + reverse directions during Phase 2 implementation.

### Codex — pending

### DeepSeek V4 Flash — pending

### GLM 5.1 — CONCUR (0 BLOCKERS, 0 NITS)

Confirmed all 4 round-1 plan-review NITs landed: typed `ShadowEntry` shape (lines 123-129 of test file), TODO comment on `GO_PROXY_ROUTES` extraction (lines 3-8), `// @vitest-environment node` pin (line 1), no broken imports. Allowlist contents match on-disk routes exactly (31 entries). Phase 1 test deletions defensible — coverage inventory in commit body comprehensive.

### Kimi K2.6 — pending

## Post-execution report

3-phase implementation shipped on a single branch.

### Phase 1 — `a478d7a`

Deleted 4 shadow Next route files under `/api/orgs/` + 2 vitest integration tests that exercised them. Removed empty parent directories. Coverage equivalence inventory captured in the commit body — Go has strictly more coverage (Vitest tests had no self-action guard assertions; Go has 4 dedicated tests for it from plan 069 phase 4).

Pre-impl greps (both clean):
- `grep -rln "@/app/api/orgs" tests/ e2e/ src/` — only the 2 deleted vitest files.
- `grep -rn "/api/orgs/" src/ --include="*.tsx" --include="*.ts"` — ~15 fetch/string refs, all proxy-routed to Go (proxy works without the Next route file existing).

Verification:
- `bunx tsc --noEmit`: 10 errors, all pre-existing in unrelated files (verified via `git stash` comparison); 0 new errors from this change.
- `bun run test`: 621 passed, 11 skipped, 0 failed.
- `cd platform && go test ./... -count=1 -timeout 120s`: all packages OK.

### Phase 2 — `d797920`

Added `tests/unit/shadow-routes.test.ts` (294 lines) — a Vitest contract-parity test with bidirectional check + matcher fixtures. Implementation by Sonnet subagent following the exact design from §Files (typed `ShadowEntry`, text-parse with `//` comment stripping, `// @vitest-environment node` pin, both forward + reverse assertions, inline fixtures for `routeFileToUrlPath` / `stripPathWildcard` / `isUnderProxiedPrefix` cross-prefix collision check).

13 tests, all passing in 620ms. Sanity checks (verified by subagent):
- Forward: temporarily added `src/app/api/orgs/test-shadow/route.ts` → test FAILED with clear "unallowlisted shadow path" naming `/api/orgs/test-shadow`. Removed → PASS.
- Reverse: temporarily added stale entry `/api/orgs/nonexistent` to allowlist → test FAILED naming the ghost. Removed → PASS.

Initial allowlist contains exactly 31 shadow paths matching the §Approach inventory (4 admin, 2 ai, 4 assignments, 3 classes, 6 courses, 3 documents, 1 parent, 7 sessions, 1 submissions). All entries have `cleanupPlan: "TBD"`.

One implementation note from the subagent: legitimate-Next catch-all routes (`[...nextauth]`) are skipped via an `includes("[...")` check before `routeFileToUrlPath` is called, since the helper throws on catch-alls. If a future change adds a catch-all UNDER a proxied prefix, the skip would silently exempt it — acceptable for current state (no proxied catch-alls today, Next router conventions discourage them in API routes), but worth flagging if that pattern ever appears.

### Phase 3 — `a210523`

- `next.config.ts`: rewrote the stale `TODO(plan-038)` comment ("~42 Next.js API route files...") to reference plan 074, the actual current count (31 shadow files remain), and the canonical worklist location (`tests/unit/shadow-routes.test.ts:KNOWN_SHADOW_ALLOWLIST`).
- `TODO.md`: added "Shadow Next API allowlist shrinkage" under Technical Debt with a pointer to the allowlist as the canonical worklist.

Final verification: `bun run test` → 634 passed (up from 621 in Phase 1; the +13 are the new shadow-routes tests), 11 skipped, 0 failed. `bunx tsc --noEmit` still at 10 pre-existing errors, no regression.

### No deviations from plan

Phase boundaries, file lists, and verification steps shipped exactly as specified after the 5-way plan review's fold-in. The implementation reflects all NITs from Codex (matcher fixtures, coverage inventory), DeepSeek (text-parse pattern), GLM (typed entries, future GO_PROXY_ROUTES extraction noted as TODO), and Kimi (env:node pin, bidirectional check, fetch-string grep).

### Follow-ups

- Each of the 31 allowlist entries needs a future plan to verify Go parity and delete the shadow file. Allowlist is grep-friendly to find migration targets; reverse check makes the cleanup self-cleaning.
- Future plan should consider extracting `GO_PROXY_ROUTES` from `next.config.ts` to a shared `src/lib/go-proxy-routes.ts` so both `next.config.ts` and `tests/unit/shadow-routes.test.ts` (plus the existing `tests/unit/middleware-proxy-parity.test.ts`) import from a single source instead of re-parsing config text. GLM round-1 + Kimi round-1 both flagged the parsing fragility; the extraction touches Next's build pipeline so it deserves its own scoped plan.

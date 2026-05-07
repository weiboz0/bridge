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
5. The allowlist enumerates every remaining shadow file as of this PR (~35 paths total: 31 shadow + 4 legitimate-Next), each with a one-line comment noting "shadow: pending plan-NNN cleanup" or "legitimate-next: <reason>".

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
| A future Next.js feature (server action, Edge route) mistakenly imports `@/app/api/orgs/...` after deletion | low | TypeScript catches the missing module. |
| The contract-parity test produces false positives on legitimately-Next routes | medium | The allowlist explicitly includes legitimate-Next routes (e.g., `/api/auth/[...nextauth]`) with a "legitimate-next" tag distinct from the "shadow-pending-cleanup" tag. Reviewers can tell the two cases apart. |
| `next.config.ts` parsing in the Vitest test is fragile (re-runs vitest config every test) | low | Import the rewrites function once in the test setup; cache the parsed array at module load. |
| Deleting the Vitest integration tests reduces overall test count and looks like coverage regression | low | Document in the post-execution report that the tests exercised dead code; Go-side coverage is the canonical suite. |
| The allowlist grows stale (reviewers add new shadow files with allowlist entries to silence the test) | medium | Allowlist entries require a comment with a plan number reference. PR review catches "added without justification" cases. Same pattern Bridge already uses for skipped tests. |
| Other test files (e2e, etc.) reference the deleted files | low | Pre-impl grep planned for Phase 1: `grep -rln "@/app/api/orgs" tests/ e2e/` returns only the two files being deleted (verified during plan drafting). |

## Phases

### Phase 1 — Delete `/api/orgs` shadow routes + their tests (commit 1)

- Pre-impl verification: re-run `grep -rln "@/app/api/orgs" tests/ e2e/ src/` to confirm zero unexpected importers.
- Delete the 4 route files + 2 vitest integration tests listed in §Files.
- Remove now-empty `src/app/api/orgs/` directory tree.
- Run `bun run test` (Vitest) — expect the two deleted test files gone from the run, all other suites pass.
- Run `bunx tsc --noEmit` — expect 0 errors (no TS reference to the deleted files anywhere).
- Commit: `plan 074 phase 1: delete /api/orgs shadow Next routes + their vitest integration tests`.

### Phase 2 — Add contract-parity test (commit 2)

- Create `tests/unit/shadow-routes.test.ts`.
- Implementation: import the rewrites array from next.config.ts; walk `src/app/api/` for `route.ts` files using `node:fs`; convert each path to a URL pattern; match against `GO_PROXY_ROUTES`; assert each match is in `KNOWN_SHADOW_ALLOWLIST`.
- Allowlist initial population: enumerate remaining ~33 shadow paths verbatim, each with a `// shadow: pending plan-NNN cleanup` or `// legitimate-next: <reason>` comment.
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

(pending — 5-way before implementation)

## Code Review

(pending — 5-way at branch-diff time)

## Post-execution report

(pending)

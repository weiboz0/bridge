# Project Instructions

## CRITICAL RULES (never skip)

- **Always create a feature branch** before implementation. Never commit directly to main. Use `git checkout -b feat/NNN-description`.
- **Always code review** before merging a PR. Write findings to the plan file's `## Code Review` section.
- **Always write a post-execution report** in the plan file before shipping.
- **Always run the full test suite** before pushing. Do not push with failing tests.

## Project Structure

- `src/` — Next.js 16 App Router frontend + legacy TypeScript API routes
- `platform/` — Go backend (API server, LLM/agent/sandbox layers, stores)
- `server/hocuspocus.ts` — Yjs collaboration server (port 4000)
- `e2e/` — Playwright E2E tests
- `tests/` — Vitest unit + integration tests
- `drizzle/` — Drizzle migration files
- `docs/` — Documentation
- `docs/plans/` — Executed implementation plans (numbered, `NNN-feature-name.md`)
- `docs/specs/` — Design specs for large or novel features

## Running the App

- **Next.js dev**: `PORT=3003 bun run dev`
- **Go platform** (hot-reload): `cd platform && air` (port 8002)
- **Go platform** (manual): `cd platform && go run ./cmd/api/`
- **Hocuspocus**: `bun run hocuspocus` (port 4000)
- **DB studio**: `bun run db:studio`

Frontend proxies Go routes via `next.config.ts` rewrites (`GO_PROXY_ROUTES`). All three services must be running for E2E tests.

## Coding Conventions

### TypeScript / Next.js
- App Router, React 19, shadcn/ui, Monaco editor, Yjs
- Linting: `bun run lint`
- Type-check: `bunx tsc --noEmit` (or `node_modules/.bin/tsc --noEmit`)

### Go
- Chi router, `database/sql` via pgx, parameterized queries only
- Module path: `github.com/weiboz0/bridge/platform`
- Stores accept `*db.DB`; handlers use `ValidateUUIDParam` middleware for path IDs
- Error handling: return `(result, error)`, log with `slog`
- Timestamps: `time.RFC3339`
- **Production-quality code required.** Go code must cover the SAME business logic, edge cases, fallback chains, defensive patterns, and generalization as the Python implementation — not just the happy path. Do NOT strip defensive logic, pre-validation, health probes, retry paths, or fallback chains in the name of Go minimalism or YAGNI. When porting from Python, read the Python implementation's INTERNAL logic (not just the API shape) and port ALL defensive paths. When writing new Go code with no Python counterpart, handle: what happens on failure? partial input? upstream down? concurrent access? empty collections? Think like a production SRE, not a tutorial writer.

### General
- Follow existing patterns in the codebase — check how similar features are implemented before writing new code.
- Never hardcode secrets or credentials. Secrets go in `.env`, non-secret config in config files.
- When modifying any logic, proactively search the codebase for similar patterns that should receive the same change. Do not wait to be asked — audit related handlers, components, and utilities for consistency.
- **Never defer fixes without a follow-up plan.** If a known issue is identified during implementation, fix it NOW in the same commit/PR. Do NOT label it "acceptable trade-off", "follow-up", "TODO", or "deferred" unless a concrete follow-up plan has been drafted in `docs/plans/` with a plan number, scope, and phases. Unfiled deferrals rot — they become invisible tech debt that surfaces only during live user testing.
- **Always implement the long-term solution, not the short-term workaround.** When two approaches exist — a quick hack that unblocks now vs a proper fix that solves the root cause — choose the proper fix. Short-term workarounds create architectural debt that compounds across plans (e.g., dual agentic loops, dual streaming states, dual event channels). If the proper fix is genuinely too large for the current scope, draft the follow-up plan immediately and get user approval before shipping the workaround.


## Testing

- When code is added or modified, write or update test cases covering the changes.
- Run the relevant tests to verify they pass before committing.
- Test both happy paths and error/edge cases (invalid input, missing data, unauthorized access).
- Aim for maximum test coverage: every public function, every branch, every error path. If a function has 3 code paths, write at least 3 tests. Do not skip edge cases or treat them as "obvious".
- **Unit / integration tests (Vitest)**: `bun run test` — uses the `bridge_test` database (see `docs/setup.md`).
- **Go tests**: `cd platform && go test ./... -count=1 -timeout 120s`.
  - Store integration tests require `TEST_DATABASE_URL`.
  - Every new Go API endpoint must have an integration test covering happy path, auth, error cases, cross-user isolation.
- **E2E tests (Playwright)**: `bun run test:e2e` — requires Next.js + Go platform + Hocuspocus all running.

## Documentation

When code changes affect behavior, APIs, architecture, or configuration, update the relevant documentation in `docs/` and any affected `README.md` files to stay in sync. This includes: new endpoints, changed request/response schemas, new features, modified environment variables, and updated setup steps.

## Git

- **Never commit directly to `main`.** Always create a feature branch and PR via `gh pr create`.
- Before merging any PR, check CI status with `gh pr checks <number>`. If any checks fail, fix the errors and push before merging. Never merge a PR with failing checks.
- Do not push to remote unless explicitly asked.
- Write clear, descriptive commit messages — lead with what changed and why, not how.
- Do not commit `.env`, credentials, or large binary files.
- Do not commit every small change individually. Batch related small fixes into a single meaningful commit. Only commit when a logical unit of work is complete.

## Plans

For substantial code changes — new features, re-architecting, multi-file refactors, new integrations, etc. — always enter plan mode first and write a detailed plan before any implementation. Get user approval on the plan before proceeding.

**Execution plans** (phased implementation with file lists and verification steps) go in `docs/plans/`. Numbered sequentially (001, 002, ...). Sub-documents use letter suffixes (021a, 021b).

**Design specs** go in `docs/specs/` — but only for large, novel, or cross-cutting designs that need a standalone reference document. When brainstorming produces a concrete design (components, file lists, phases decided), skip the spec and go straight to an execution plan in `docs/plans/`.

Before writing a new plan, review existing plans in `docs/plans/` to ensure consistency. Check for: reusable patterns and utilities already established, architectural decisions that must be respected, and existing implementations that the new work should build on rather than duplicate. Avoid introducing duplicate code — reuse existing implementations and keep logic in a single source of truth.

## Development Workflow

Follow `docs/development-workflow.md` exactly for every plan (Steps 1–6: Design → Plan → Build → Verify → Review → Ship). Do not skip steps or batch them. Key points:
- **Design before plan** — explore the problem, brainstorm approaches, align with user
- **Plan before code** — write and commit plan file before any implementation
- **Build phase by phase** — implement, test, self-review, document, commit each phase separately
- **Verify before review** — full test suite, cross-phase consistency check
- **Review before ship** — code review findings in plan file, all [OPEN] items resolved
- **Ship cleanly** — post-execution report, update `TODO.md`, then PR

## Code Review

Follow `docs/code-review.md` for the review process. Key points:
- Reviews go in the plan file's `## Code Review` section
- Reviewers: append findings with `[OPEN]` status and file:line references
- Authors: respond inline with `→ Response:` and `[FIXED]`/`[WONTFIX]`
- **Use Codex for code review** when available. Dispatch via `/codex:rescue` with a review prompt targeting the branch diff. Codex provides an independent second opinion with access to the full repo context. The Codex review gate (enabled in this project) also triggers automatically at session stop.

## Multi-Agent Coordination

Multiple Claude Code sessions share the same local repository. Only one agent should work at a time. To avoid lost work:

- **Always commit before ending a session.** Even partial work — use a `WIP:` prefix. Uncommitted changes are invisible to the next session and will be lost.
- **Check `git status` at session start.** Look for untracked or modified files left by a previous session. Ask the user before discarding them.
- **Each plan uses a feature branch.** Pull before starting work, push before ending. Never leave unpushed commits.
- **Never assume prior session completed its work.** Verify by reading the plan file's post-execution report and checking git log — don't trust claims in conversation summaries alone.

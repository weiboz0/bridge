# Codex Project Instructions

This file is the Codex-parity version of `CLAUDE.md`. It keeps the same project rules and intent, but phrases them for Codex-style execution: inspect the repo first, make bounded edits, verify changes locally, and leave a clear handoff trail.

Bridge's highest-value Codex use cases are:

1. **Go platform work** - extend and harden `platform/` without drifting from the frontend/API contracts.
2. **Plan-driven implementation** - execute work from `docs/plans/` with tests, review notes, and post-execution reporting.
3. **Frontend and classroom workflow changes** - keep Next.js, real-time collaboration, auth, and teacher/student flows consistent across the app.

## Critical Rules

- **Always create a feature branch before implementation.** Do not work directly on `main`. Use `feature/plan-NNN-description` for plan work when practical.
- **Plan review before code.** For substantial plan-driven work, confirm the plan has passed the five-way review gate before implementation begins.
- **Review before ship.** For plan-driven work, record findings in the plan file's `## Code Review` section.
- **Write a post-execution report** in the plan file before shipping substantial work.
- **Run the relevant tests before handoff.** For plan work, run the full applicable suite before shipping unless a blocker is documented in the plan.
- **Do not push unless explicitly asked.**
- **Do not trust old plan claims without code verification.** Comments, plans, and prior reviews are hints, not proof.
- **Do not ship unplanned deferrals.** If a known issue is not fixed now, create a concrete follow-up plan in `docs/plans/` with scope and phases before treating it as deferred.
- **Prefer the long-term fix over a workaround.** If the proper fix is too large, write the plan first and get approval before shipping a containment-only change.

## Working Style For Codex

- **Read before editing.** Inspect the relevant files, nearby implementations, tests, and docs before making changes.
- **Prefer minimal, correct diffs.** Reuse existing helpers and patterns instead of introducing parallel implementations.
- **Preserve user changes.** Never revert unrelated work in a dirty tree.
- **Use repo conventions, not generic ones.** Bridge has established patterns for handlers, stores, API clients, auth, Yjs/Hocuspocus flows, and tests.
- **Treat docs as part of the implementation.** If behavior, architecture, configuration, or API shape changes, update docs in the same unit of work.
- **Trace cross-file behavior.** In this repo, a route or component is rarely the whole feature. Follow handlers, stores, API clients, server components, auth checks, realtime events, and tests before deciding something is done.

## Project Structure

- `src/` - Next.js 16 App Router frontend plus legacy TypeScript API routes
- `platform/` - Go backend API server, LLM/agent/sandbox layers, stores, and contract tests
- `server/hocuspocus.ts` - Yjs collaboration server
- `e2e/` - Playwright E2E tests
- `tests/` - Vitest unit and integration tests
- `drizzle/` - Drizzle migration files
- `docs/` - Project docs
- `docs/plans/` - Execution plans
- `docs/specs/` - Design specs for large or novel features
- `docs/reviews/` - Standalone reviews, audits, and handoffs when there is no plan file to embed findings in

## Run Commands

### Frontend

- Dev server: `PORT=3003 bun run dev`
- Lint: `bun run lint`
- Type-check: `bunx tsc --noEmit` or `node_modules/.bin/tsc --noEmit`
- Unit/integration tests: `bun run test`
- DB studio: `bun run db:studio`

### Go Platform

- Hot reload: `cd platform && air` or `cd platform && make dev`
- API direct: `cd platform && go run ./cmd/api/`
- Build: `cd platform && make build`
- Tests: `cd platform && go test ./... -count=1 -timeout 120s`
- Short tests: `cd platform && make test`
- Integration tests: `cd platform && make test-integration`
- Contract tests: `cd platform && make test-contract`

### Realtime And E2E

- Hocuspocus: `bun run hocuspocus`
- E2E: `bun run test:e2e`

All three services are required for E2E tests: Next.js on port 3003, Go platform on port 8002, and Hocuspocus on port 4000. Frontend requests to Go routes are proxied through `next.config.ts` rewrites.

## Coding Conventions

### TypeScript / Next.js

- App Router, React 19, shadcn/ui, Monaco editor, Yjs
- Follow existing server/client component boundaries.
- Keep API client behavior consistent with proxied Go routes.
- Use existing UI primitives and role-based portal patterns.

### Go

- Chi router, `database/sql` via pgx, parameterized queries only
- Module path: `github.com/weiboz0/bridge/platform`
- Stores accept `*db.DB`; handlers use `ValidateUUIDParam` middleware for path IDs
- Return `(result, error)`, log with `slog`
- Use `time.RFC3339` consistently
- Match the production behavior, not just the API shape. When porting from Python, carry over defensive logic, validation, fallback chains, retries, health checks, and edge-case handling rather than reimplementing a happy-path-only version. When building new Go code from scratch, think through failure modes, partial input, upstream outages, concurrency, and empty-data cases up front.
- Every new Go API endpoint should have integration coverage for happy path, auth, error cases, and cross-user isolation.

### General

- Search for similar implementations before changing logic.
- If a fix applies to one handler, picker, store, or UI flow, check siblings proactively.
- Never hardcode secrets or credentials. Secrets belong in `.env`; non-secret config belongs in config files.
- Keep shared logic in one source of truth.
- Do not defer known issues without a concrete follow-up plan. If you identify a real bug or gap during implementation, either fix it in the same unit of work or document the follow-up immediately in a numbered plan under `docs/plans/` before shipping.
- Prefer the long-term fix over the short-term workaround. Avoid shipping tactical hacks that create parallel code paths or architectural debt unless the user has explicitly accepted that tradeoff and the follow-up plan is already written.

## Testing Expectations

- **Codex is the default independent test implementer when Claude delegates test work.** When acting in that role, write the test cases, keep production-code changes out of scope unless explicitly requested, and report exactly what was verified.
- Add or update tests whenever behavior changes.
- Cover happy paths, invalid input, missing data, unauthorized access, and isolation checks where relevant.
- Prefer integration tests for API behavior.
- Run the narrowest useful tests during implementation, then broader verification before handoff.

Primary commands:

- Frontend tests: `bun run test`
- Frontend lint: `bun run lint`
- Type-check: `bunx tsc --noEmit`
- Go tests: `cd platform && go test ./... -count=1 -timeout 120s`
- E2E tests: `bun run test:e2e`

Do not mark work complete if relevant tests were not run. If something could not be verified, say so explicitly.

## Documentation Expectations

Update relevant docs when changing:

- API routes or schemas
- behavior visible to users or operators
- architecture or runtime behavior
- environment/configuration
- setup or workflow steps

Likely doc targets:

- matching plan/spec files
- `docs/reviews/` when producing standalone audit or handoff material
- relevant `README.md` files
- `docs/api.md`
- `docs/setup.md`
- `docs/development-workflow.md` and `docs/code-review.md` for workflow changes

## Plans, Specs, And Reviews

For substantial work, use plan-first execution.

- **Execution plans** go in `docs/plans/`
- **Large design docs** go in `docs/specs/` only when a standalone design reference is actually needed
- **Standalone reviews** go in `docs/reviews/` only when there is no plan file to hold the findings, or when producing a retrospective audit/handoff.

Plan- and code-review verdicts for numbered plans are embedded inside the plan file (`## Plan Review` / `## Code Review`), not split into standalone review docs.

Use semantic line breaks for new prose in new plans/specs: one sentence per line.
Existing hard-wrapped plans can stay as-is to avoid churn; new sections in existing plans should use semantic line breaks where practical.

Before creating a new plan:

- read related existing plans
- read `TODO.md` for relevant outstanding work
- reuse established utilities and patterns
- avoid duplicate architecture

For plan-driven work, follow `docs/development-workflow.md`.

### When To Go Straight To Code

It is reasonable to skip a new plan only when the task is clearly bounded, low-risk, and local, for example:

- typo or doc fixes
- single-file bug fixes
- small test-only updates

For multi-file features, migrations, refactors, new integrations, or anything touching runtime architecture, use plan-first execution.

## Code Review

Follow `docs/code-review.md`.

- Findings belong in the plan file's `## Code Review` section.
- Use explicit statuses: `[OPEN]`, `[FIXED]`, `[WONTFIX]`.
- Include concrete file and line references where possible.
- Resolve every `[OPEN]` item before shipping.
- When acting as the Codex reviewer in the five-way gate, focus on correctness, security, authorization/isolation, test gaps, and consistency with the plan and repo conventions.
- Tag Codex-sourced findings clearly when the plan/review format asks for source tags.

## Git And Handoff

- Do not commit unrelated noise.
- Group related changes into meaningful commits.
- Do not commit every small edit individually; commit logical units.
- Do not commit secrets, `.env`, or large binaries.
- Do not amend commits unless explicitly requested.
- Do not push to remote unless explicitly requested.
- If ending a session with useful unfinished work, leave the tree and docs in a state the next engineer can understand.

For multi-session continuity:

- check `git status` at start
- commit useful work before ending a session when the repo workflow requires a handoff commit; use `WIP:` only for genuinely unfinished handoff commits
- inspect existing plan files and recent review notes
- do not assume prior work is complete just because a conversation said so
- do not revert unrelated changes from another session

Recommended handoff artifacts for substantial work:

- updated plan file
- updated docs if behavior changed
- review findings in the plan file
- fresh review doc in `docs/reviews/` for audit-style work
- clear note on what was verified and what was not

## Codex-Specific Best Practices

- Prefer small, auditable changes over wide speculative refactors.
- Treat comments and plan claims as hints, not proof. Verify in code.
- When delegated test implementation, write tests that would fail on the old behavior and exercise the public contract, not just the new helper path.
- When reviewing, lead with concrete findings and file:line references; avoid broad style commentary unless it hides a real risk.
- If an earlier audit is stale, write a fresh handoff in `docs/reviews/` rather than mutating history into ambiguity.
- When editing, optimize for the next engineer reading the diff: clear names, minimal blast radius, tests near the behavior that changed, and docs that explain why behavior exists.

## Codex Checklists

### Before Editing

- Read the relevant implementation files fully.
- Read nearby tests.
- Read related plans/specs/docs.
- Check sibling handlers, stores, API clients, and UI flows for the same behavior.
- For plan-driven work, confirm the current branch and whether plan review has already passed.

### Before Claiming Completion

- Re-read all modified files.
- Run the relevant tests.
- State what was verified.
- State what remains unverified.
- Update plans/docs/reviews as needed.

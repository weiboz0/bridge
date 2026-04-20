# Codex Project Instructions

This file is the Codex-parity version of `CLAUDE.md`. It keeps the same project rules and intent, but phrases them for Codex-style execution: inspect the repo first, make bounded edits, verify changes locally, and leave a clear handoff trail.

Bridge's highest-value Codex use cases are:

1. **Go platform work** - extend and harden `platform/` without drifting from the frontend/API contracts.
2. **Plan-driven implementation** - execute work from `docs/plans/` with tests, review notes, and post-execution reporting.
3. **Frontend and classroom workflow changes** - keep Next.js, real-time collaboration, auth, and teacher/student flows consistent across the app.

## Critical Rules

- **Do not work directly on `main`.** Use a feature branch for implementation work.
- **Review before ship.** For plan-driven work, record findings in the plan file's `## Code Review` section.
- **Write a post-execution report** in the plan file before shipping substantial work.
- **Run the relevant tests before handoff.** Do not claim completion without verification.
- **Do not push unless explicitly asked.**
- **Do not trust old plan claims without code verification.** Comments, plans, and prior reviews are hints, not proof.

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
- Every new Go API endpoint should have integration coverage for happy path, auth, error cases, and cross-user isolation.

### General

- Search for similar implementations before changing logic.
- If a fix applies to one handler, picker, store, or UI flow, check siblings proactively.
- Never hardcode secrets or credentials. Secrets belong in `.env`; non-secret config belongs in config files.
- Keep shared logic in one source of truth.

## Testing Expectations

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
- relevant `README.md` files
- `docs/api.md`
- `docs/setup.md`
- `docs/development-workflow.md` and `docs/code-review.md` for workflow changes

## Plans And Specs

For substantial work, use plan-first execution.

- **Execution plans** go in `docs/plans/`
- **Large design docs** go in `docs/specs/` only when a standalone design reference is actually needed

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

## Git And Handoff

- Do not commit unrelated noise.
- Group related changes into meaningful commits.
- Do not commit secrets, `.env`, or large binaries.
- If ending a session with useful unfinished work, leave the tree and docs in a state the next engineer can understand.

For multi-session continuity:

- check `git status` at start
- inspect existing plan files and recent review notes
- do not assume prior work is complete just because a conversation said so
- do not revert unrelated changes from another session

Recommended handoff artifacts for substantial work:

- updated plan file
- updated docs if behavior changed
- review findings in the plan file
- clear note on what was verified and what was not

## Codex Checklists

### Before Editing

- Read the relevant implementation files fully.
- Read nearby tests.
- Read related plans/specs/docs.
- Check sibling handlers, stores, API clients, and UI flows for the same behavior.

### Before Claiming Completion

- Re-read all modified files.
- Run the relevant tests.
- State what was verified.
- State what remains unverified.
- Update plans/docs/reviews as needed.

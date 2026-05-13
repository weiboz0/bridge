# Project Instructions

## Critical Rules

- **Always create the feature branch BEFORE drafting the plan** (`git checkout -b feat/NNN-description`). All commits — plan file, review verdicts, implementation — land there. Never on `main`.
- **Spawn subagents for coding work — domain-based dispatch.** Orchestrator stays on Opus 4.7 for planning + review + coordination. For implementation: **Codex** for backend (Go in `platform/`, Hocuspocus server); **Sonnet** for frontend (Next.js, React, `src/`); **Sonnet** for all test code regardless of domain; **Opus subagent** for complex cross-domain or new-pattern work. DeepSeek and GLM remain review-only. Details: `docs/coding-agent.md`.
- **Run the 4-way plan review** on every plan before any implementation. Capture all four verdicts in the plan's `## Plan Review` section. Gate + dispatch table: `docs/reviewers.md`.
- **Run the 4-way code review** before merging any PR. Findings go in the plan's `## Code Review` section. Gate + dispatch table: `docs/reviewers.md`.
- **Always write a post-execution report** in the plan file before shipping.
- **Run the full test suite before pushing.** Never push with failing tests.
- **Never commit directly to `main`.** Always feature branch + PR via `gh pr create`.

## References

| Topic | Doc |
|-------|-----|
| Project layout + service ports + run commands | `docs/project-structure.md` |
| Dev setup (PostgreSQL, env, auth, migrations) | `docs/setup.md` |
| Subagent dispatch + tier policy | `docs/coding-agent.md` |
| Plan + code review gates, external reviewers | `docs/reviewers.md` |
| Workflow (Design → Plan → Build → Verify → Review → Ship) | `docs/development-workflow.md` |
| Code-review format | `docs/code-review.md` |

## Coding Conventions

### TypeScript / Next.js
- App Router, React 19, shadcn/ui, Monaco editor, Yjs.
- Lint: `bun run lint`. Type-check: `bunx tsc --noEmit`.

### Go
- Chi router, `database/sql` via pgx, parameterized queries only.
- Module path: `github.com/weiboz0/bridge/platform`.
- Stores accept `*db.DB`; handlers use `ValidateUUIDParam` middleware for path IDs.
- Errors: return `(result, error)`; log with `slog`. Timestamps: `time.RFC3339`.
- **Production-quality code required.** Cover failure, partial input, upstream down, concurrent access, empty collections. Don't strip defensive logic in the name of YAGNI. When porting from Python, port the internal logic (defensive paths, fallbacks, validation, retries), not just the API shape.

### General
- Follow existing patterns. When modifying logic, audit related handlers/components for consistency.
- Never hardcode secrets. `.env` for secrets, config files for non-secrets.
- **Never defer fixes without a follow-up plan.** No "TODO", "follow-up", "deferred" unless a numbered plan exists in `docs/plans/` with scope and phases.
- **Always implement the long-term solution, not the workaround.** If the proper fix is genuinely too large for current scope, draft the follow-up plan first and get user approval before shipping the workaround.

## Testing

- Test code is Sonnet's job by default — see `docs/coding-agent.md`.
- Write/update tests for every code change. Cover happy + error/edge paths. Every branch, every error path.
- **Vitest**: `bun run test` (uses `bridge_test` DB; see `docs/setup.md`).
- **Go**: `cd platform && go test ./... -count=1 -timeout 120s`. Store integration tests require `TEST_DATABASE_URL`. Every new Go API endpoint needs an integration test (happy + auth + error + cross-user isolation).
- **E2E (Playwright)**: `bun run test:e2e` — requires Next.js + Go platform + Hocuspocus all running.

## Documentation

When code affects behavior, APIs, architecture, or config, update `docs/` and affected `README.md` files in the same commit. Includes new endpoints, changed schemas, new features, env-var changes, setup steps.

## Git

- Always feature branch + PR via `gh pr create`. Never push directly to `main`.
- Before merging: `gh pr checks <number>`. Don't merge with failing checks.
- Do not push to remote unless explicitly asked.
- Commit messages lead with WHAT changed and WHY.
- Never commit `.env`, credentials, or large binaries.
- Batch related small fixes into one meaningful commit. Don't commit every small change individually.
- **Don't open separate PRs for trivial / doc-only changes.** Batch into the next adjacent substantive PR.

## Plans

- Substantial code changes (new features, re-architecting, multi-file refactors, integrations) start with a plan in `docs/plans/` (sequential `NNN-feature-name.md`; sub-docs use letter suffixes `021a`, `021b`).
- Design specs go in `docs/specs/` — only for large, novel, or cross-cutting designs that need a standalone reference. When brainstorming produces a concrete design (components, files, phases decided), skip the spec and go straight to a plan.
- Before writing a new plan, review existing plans in `docs/plans/` for reusable patterns, architectural decisions, and existing implementations the new work should build on rather than duplicate.
- Plans must pass the 4-way review before implementation. See `docs/reviewers.md`.

## Multi-Agent Coordination

Multiple Claude Code sessions share the same local repo sequentially. Only one agent works at a time.

- **Commit before ending a session.** Use `WIP:` prefix for partial work. Uncommitted changes are invisible to the next session.
- **Check `git status` at session start.** Flag orphaned changes to the user before discarding.
- **Each plan uses a feature branch.** Pull before starting, push before ending. Never leave unpushed commits.
- **Verify prior work** via plan post-execution reports + `git log`. Don't trust conversation summaries alone.

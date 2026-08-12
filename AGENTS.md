# Project Instructions

Bridge is a teaching platform.
Teachers author problems and lessons, run live collaborative sessions with students, and review submitted work;
students write and execute code in the browser.
It holds **real teacher and student data** — that constraint drives several rules below that would be over-engineering elsewhere.

Three components:

1. **`src/`** — Next.js 16 App Router frontend, plus the remaining legacy TypeScript API routes.
2. **`platform/`** — Go backend: API server, LLM/agent/sandbox layers, stores.
3. **`server/hocuspocus.ts`** — Yjs collaboration server for live sessions.

**This file (`AGENTS.md`) is the canonical, agent-agnostic instruction set.**
Claude Code, Codex, opencode, and any other coding agent read the SAME rules from here.
The repo-root `CLAUDE.md` is a thin pointer that `@`-imports this file — **edit `AGENTS.md`, never the pointer.**

## CRITICAL RULES (never skip)

- **Autopilot is the DEFAULT operating mode for design, plans, and bug fixes.**
  When asked to design something, write or run a plan, or fix a bug,
  run the full lifecycle autonomously through merge —
  design → plan-review gate → phase-by-phase implementation → code-review gate → post-execution report → PR → `pre-merge-guard.sh --pr` → squash-merge —
  without stopping for per-step approval.
  The gates below are NOT skipped by autopilot; autopilot *conducts* them.
  The user can redirect at any time; "stop at PR" caps a run at the PR.

- **Hard safeguards — always pause, regardless of mode.** See `## Hard safeguards` below. These are not advisory.

- **Delegate coding work by DOMAIN, not by complexity.**
  Backend (Go in `platform/`, `server/hocuspocus.ts`) → Codex `gpt-5.6-terra`.
  Frontend (`src/`) → Sonnet 5.
  All tests → Opus 5 by default.
  An explicit user model pin overrides these implementation defaults for the named work.
  Cross-cutting refactors, new patterns, and hard multi-system debugging stay inline on the orchestrator.
  Dispatch table: `docs/coding-agent.md`.

- **Branch BEFORE drafting or reviewing a plan.**
  `git checkout -b feat/NNN-description` runs first.
  Plan file, every review verdict, and every implementation commit lives on that branch.
  Never commit directly to `main`.

- **Every plan declares a `## File scope`** listing the files it may modify.
  It is fixed when the plan clears its gate.
  Changing files outside it is a hard safeguard pause, and widening the scope afterward is itself a pause.

- **Run the plan-review gate before any implementation.**
  Width is risk-tiered — see `docs/reviewers.md`.
  A passing gate IS authorization to begin; no separate user-approval pause is required.
  Capture every verdict in the plan's `## Plan Review` section.

- **Run the code-review gate before opening a PR.**
  Findings go in the plan's `## Code Review` section with `[OPEN]` / `[FIXED]` / `[WONTFIX]` tags,
  file:line references, and source tags.
  All `[OPEN]` items resolve before merge.

- **Always write a post-execution report** in the plan file before shipping.

- **Local is the gate.** `bash scripts/ci-local.sh` must be green before any merge.
  Cloud CI runs the same script, so the two cannot drift.
  Never `--admin` past a failing gate.

## Hard safeguards

Always pause and surface to the user, regardless of operating mode.

- **Secrets** — `.env*` except `.env.example`, `.gh-token`, `.oauth-state-secret.local`,
  and any path matching `*credential*` / `*api_key*` / `*token*` / `*secret*`.

- **Database and student data** — any drizzle migration against a non-test database,
  `db:push` / `db:migrate`, `scripts/seed_*.sql`, and content imports.
  Bridge has no down-migrations and holds real student work.

  This one is *enforced*, not trusted: `drizzle.config.ts` reads `process.env.DATABASE_URL`,
  so there is no separate test-only migration path and an inherited `DATABASE_URL` silently targets production.
  `scripts/check-test-database-url.mjs`, `scripts/tests/test-guards.sh`, and
  `scripts/ci-local.sh` require a parsed and live-validated `_test` database name before tests run.
  Migrating that throwaway container is the one narrow exception.

- **Auth and tenancy** — `platform/internal/middleware` session verification, org-tenancy scoping,
  Hocuspocus signed tokens, admin impersonation.
  A permissive change here leaks cross-org student data while every test still passes.

- **Processes and ports** — never kill a process.
  **Never run E2E without a pinned `E2E_BASE_URL`**: `e2e/playwright.config.ts` declares no `webServer`
  and defaults to `http://localhost:3003`, which on the primary dev machine is a *different* service,
  while `e2e/seed.setup.ts` creates classes and enrolls users.
  Bridge's own stack is on `NEXTJS_PORT` / `PLATFORM_PORT` per `.env`.

- **History and remote** — `git push --force`, `reset --hard` on shared history,
  a direct commit to `main`, `git branch -D` with unmerged commits,
  **`gh pr merge --admin`**, and any `gh` write against a PR the agent does not own.
  Autopilot merges with `gh pr merge --squash` only.

- **Process** — file changes outside the plan's `## File scope`,
  unresolved `[OPEN]` review findings,
  and a failing `pre-merge-guard.sh` or `ci-local.sh`.

- **Governance docs** — `AGENTS.md`, the `CLAUDE.md` pointer,
  `docs/{coding-agent,development-workflow,reviewers}.md`, `.githooks/`,
  `scripts/check-test-database-url.mjs`, `scripts/tests/test-guards.sh`, and `scripts/ci-local.sh`,
  unless declared in the plan's `## File scope` at gate time.
  The hook and the gate script are governance: weakening either removes the only pre-merge check Bridge has.

- **Judgment forks** — surface genuine scope, architecture, trust-model, or breaking-change decisions
  via `AskUserQuestion`.
  Under autopilot a fork **halts the run**; it does not proceed on a default.
  Mechanical or clear-cut decisions proceed without asking — don't manufacture questions.

## Permanent review-gate contract

Every committed design spec under `docs/specs/**` passes a design-review gate before an implementation plan is drafted or revised from it.
The design gate has exactly two required reviewers: Codex Sol (`gpt-5.6-sol`, reasoning effort high) and Claude Code (`claude-fable-5`).
Design reviews are read-only and bind every verdict to the exact substantive commit.
Both receive read-only prompts and must approve the same exact substantive commit with no open blocker.
Verdicts and findings are recorded in the spec with `[sol]` and `[fable]` tags; author responses remain `[OPEN]` until the flagging reviewer confirms them.

The design gate, plan-review gate, and code-review gate are uncapped consensus loops with no numeric round cap.
Consensus requires every required reviewer to return `APPROVE` or `APPROVE WITH NITS` with no open blocker.
Only flagging reviewers are re-dispatched after a response; a material revision invalidates approval of the prior substantive commit.
An unavailable required reviewer pauses the gate; the reviewer is never silently substituted, replaced, or waived.
After every three consecutive non-converged substantive rounds, record and surface a concise checkpoint; repeated reopening or two checkpoints without net blocker reduction becomes a genuine user-decision pause.

## References

| Topic | Doc |
|-------|-----|
| Project layout, service ports, run commands | `docs/project-structure.md` |
| Dev setup (PostgreSQL, env, auth, migrations) | `docs/setup.md` |
| Cross-cutting architecture decisions | `docs/architecture/decisions.md` |
| Test tiers, commands, env vars | `docs/testing.md` |
| Subagent dispatch policy | `docs/coding-agent.md` |
| Review gates + reviewer dispatch | `docs/reviewers.md` |
| Workflow (Design → Plan → Build → Verify → Review → Ship) | `docs/development-workflow.md` |
| Code-review format | `docs/code-review.md` |
| Bug-investigation gate | `docs/bug-investigation-gate.md` |

## Architecture decisions

**`docs/architecture/decisions.md` is the single source of truth for cross-cutting rules** —
auth, tenancy, persistence, error shaping, realtime, LLM backends, the Go/Next.js boundary.
Read it before changes touching those areas; update it when a plan introduces a new decision.

## Coding conventions

### TypeScript / Next.js

- App Router, React 19, shadcn/ui, Monaco editor, Yjs. Runtime is **bun**.
- Lint: `bun run lint`. Type-check: `bunx tsc --noEmit`.
- Talks to the Go backend through the proxy-route list in `next.config.ts` — see `decisions.md`.

### Go

- Chi router, `database/sql` via pgx, parameterized queries only.
- Module path: `github.com/weiboz0/bridge/platform`.
- Stores accept `*db.DB`; handlers use `ValidateUUIDParam` middleware for path IDs.
- Errors: return `(result, error)`; log with `slog`. Timestamps: `time.RFC3339`.
- **Production-quality code required.**
  Cover failure, partial input, upstream down, concurrent access, empty collections.
  Don't strip defensive logic in the name of YAGNI.
  When porting, port the internal logic — defensive paths, fallbacks, validation, retries — not just the API shape.

### General

- Follow existing patterns. When modifying logic, audit related handlers and components for consistency.
- Never hardcode secrets. `.env` for secrets, config files for non-secrets.
- **Never defer fixes without a follow-up plan.**
  No "TODO", "follow-up", or "deferred" unless a numbered plan exists in `docs/plans/` with scope and phases.
- **Always implement the long-term solution, not the workaround.**
  If the proper fix is genuinely too large for current scope,
  draft the follow-up plan first and get user approval before shipping the workaround.

## Testing

Full tier descriptions, commands, and gating env vars: `docs/testing.md`.

- Test code goes to Opus 5 by default — see `docs/coding-agent.md`.
- Write or update tests for every code change. Cover happy, error, and edge paths.
- **Every new Go API endpoint needs an integration test** — happy path, auth check, error cases, cross-user isolation.
- **Every feature plan needs a named integration-tests phase.**
  Required when the plan touches the Go API surface, the realtime protocol, or cross-cutting plumbing
  (auth, persistence, org scoping).
  Reviewers MUST reject a plan that omits it; exempt plans must say so explicitly in `## Out of scope`.
- **LLM-touching tests bill real money.**
  `tests/llm/*.test.ts` call live provider endpoints and are gated only by the presence of an API key —
  and bun auto-loads `.env`, which carries real keys.
  Never run the full suite against live providers casually;
  `scripts/ci-local.sh` explicitly exports `ANTHROPIC_API_KEY=`, `OPENAI_API_KEY=`,
  `GEMINI_API_KEY=`, `DASHSCOPE_API_KEY=`, and `OPENROUTER_API_KEY=` for Vitest.

## Documentation

When code affects behavior, APIs, architecture, or config,
update `docs/` and affected `README.md` files in the same commit.
Includes new endpoints, changed schemas, new features, env-var changes, and setup steps.

## Git

- Always feature branch + PR via `gh pr create`. Never push directly to `main`.
- Before merging, run `bash scripts/pre-merge-guard.sh` (or `--pr <number>` for the simulated post-merge state).
  It catches plan-number, spec-number, and migration-prefix collisions that parallel sessions introduce,
  plus conflict markers and semantic breaks.
  Renumber the newer artifact; never silence the guard.
- **Bridge runs no cloud CI — the local gate is the only gate.**
  `bash scripts/ci-local.sh` must pass on the exact commit being merged.
  A passing run writes an attestation naming that commit; a run against different code is not evidence.
  The `pre-push` hook enforces this — install it once per clone with `bash scripts/install-hooks.sh`.
  `--no-verify` exists for when you have already run the gate yourself, not for getting past a red one.
- Do not push to remote unless explicitly asked.
- Commit messages lead with WHAT changed and WHY.
- Never commit `.env`, credentials, or large binaries.
- Batch related small fixes into one meaningful commit.
- **Don't open separate PRs for trivial or doc-only changes.** Batch into the next adjacent substantive PR.

## Plans

Substantial code changes — new features, re-architecting, multi-file refactors, integrations — start with a plan.

- `docs/plans/NNN-feature-name.md`, sequential; sub-plans use letter suffixes (`079b`, `030a`).
- Design specs go in `docs/specs/` — only for large, novel, or cross-cutting designs needing a standalone reference.
  When brainstorming produces a concrete design (components, files, phases decided), skip the spec and write the plan.
- Before writing a new plan, review existing plans for reusable patterns and existing implementations to build on.
- Every plan has a `## File scope` section. Every feature plan has a named integration-tests phase.
- Plans must pass the plan-review gate before implementation. See `docs/reviewers.md`.
- **Markdown formatting:** semantic line breaks — one sentence per line.
  Long sentences may break at a clause boundary.
  Tables, code blocks, lists, and URLs follow their natural format.

## Development workflow

Follow `docs/development-workflow.md` for every plan (Steps 1–6).
Do not skip steps or batch them.

- **Design before plan** — explore the problem, align with the user.
- **Plan before code** — write and commit the plan file first.
- **Build phase by phase** — implement, test, self-review, document, commit each phase separately.
- **Verify before review** — full suite plus cross-phase consistency check.
- **Review before ship** — findings in the plan file, all `[OPEN]` resolved.
- **Ship cleanly** — post-execution report, update `decisions.md` and `TODO.md`, then PR.

## Multi-agent coordination

Multiple agent sessions share this local repository sequentially. Only one works at a time.

- **Commit before ending a session.** `WIP:` prefix is fine. Uncommitted changes are invisible to the next session.
- **Check `git status` at session start.** Flag orphaned changes to the user before discarding.
- **Each plan uses a feature branch.** Pull before starting, push before ending.
- **Never assume a prior session finished its work.**
  Verify via the plan's post-execution report and `git log`, not a conversation summary.

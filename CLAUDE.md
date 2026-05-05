# Project Instructions

## CRITICAL RULES (never skip)

- **Spawn subagents for coding work; pick the model per task.** The orchestrator stays on Opus 4.7 for planning + review + coordination, then dispatches a subagent (`general-purpose`, `model: "sonnet"`) for routine implementation (CRUD, tests, refactors, copy) or `model: "opus"` for complex implementation (new patterns, hard debugging, large-codebase reasoning). Do not delegate code generation to Codex, DeepSeek, GLM, or any other external model — those are review-only. See "Coding Agent" below for dispatch patterns and tier escalation.
- **Always create the feature branch BEFORE drafting the plan.** Use `git checkout -b feat/NNN-description` first. The plan file, review verdicts, and iterative revisions all commit to this branch — never to `main`. (The opencode session-continuity helper also keys on `plan-NNN`-style branch names, so this is a precondition for reviewer session reuse.)
- **Always run the 4-way plan review before any implementation.** After drafting or revising any plan in `docs/plans/`, dispatch ALL four reviewers in parallel (see "Plan review gate" below). Do NOT begin implementation until all four concur. Capture each verdict in the plan's `## Plan Review` section, committed to the feature branch as they arrive.
- **Always run the 4-way code review** before merging a PR. Dispatch all four reviewers in parallel (see "Code review gate" below). Write findings to the plan file's `## Code Review` section.
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

- **Test code is Sonnet's job.** Default to Claude Sonnet 4.6 for writing and maintaining all test code (Vitest unit + integration, Go test files, Playwright e2e, fixtures, helpers). Tests are pattern-heavy, repetitive, and benefit from Sonnet's faster turnaround. Only escalate to Opus 4.7 when the test itself is hard — e.g., diagnosing a flaky timing-dependent test, designing a new fixture pattern that other suites will copy, or test code that drives a multi-system integration through unfamiliar territory. Implementation code may run on Opus while the matching tests run on Sonnet — that's fine and often faster.
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

### Plan review gate (mandatory — 4-way)

Every plan — new or revised — must pass a 4-way review before any code is written. The four reviewers run **in parallel** (dispatch all simultaneously via multiple Agent/subagent calls in a single message):

| # | Reviewer | How to dispatch | Model |
|---|----------|-----------------|-------|
| 1 | **Self-review (Opus 4.7)** | Claude reads its own plan critically on Opus and lists concerns inline | claude-opus-4-7 |
| 2 | **Codex** | `codex:codex-rescue` subagent with the plan path + review questions | Codex default |
| 3 | **DeepSeek V4 Pro** | `opencode:opencode-review` subagent with `--model deepseek/deepseek-v4-pro` | deepseek/deepseek-v4-pro |
| 4 | **GLM 5.1** | `opencode:opencode-review` subagent with `--model volcengine-plan/glm-5.1` | volcengine-plan/glm-5.1 |

Steps:

1. **Create the feature branch FIRST** — `git checkout -b feat/NNN-description` before any plan drafting. This guarantees every commit (the plan file, the review verdicts, the iterative revisions) lands on the feature branch, never on `main`. The opencode session-continuity helper also keys on the branch name (`plan-NNN`), so reviewer history scopes correctly when the branch matches `feat/NNN-*`.
2. **Draft or revise the plan** in `docs/plans/`. Commit it to the feature branch so the reviewers can see the same on-disk file Claude is iterating on.
3. **Run self-review** — Claude reads the plan with fresh eyes and records any concerns.
4. **Dispatch reviewers 2-4 in parallel** — same prompt to each (plan path + explicit review questions: blockers, hidden assumptions, scope, ordering, missing risks). Keep the prompt focused — under 500 words, time-bounded. If any reviewer needs a remote read (e.g., GitHub-connector access), push the branch first so they can fetch the plan file by branch.
5. **Capture all four verdicts** in the plan's `## Plan Review` section. Include the date, the reviewer name, the verdict, the blockers, and the resolution for each blocker. Commit the verdicts to the feature branch as they arrive — don't batch the audit trail.
6. **If ANY reviewer flags blockers, revise the plan** to address them on the same feature branch. Re-dispatch only the flagging reviewer(s) on the revised plan. Iterate until all four concur.
7. **Only then** begin implementation on the same branch. The plan file with the 4-way review summary is already committed; the next commits are the implementation.

A plan that hasn't passed all four reviews is not ready for execution, regardless of how confident Claude is in it. The multi-model consensus catches blind spots no single model can see.

## Development Workflow

Follow `docs/development-workflow.md` exactly for every plan (Steps 1–6: Design → Plan → Build → Verify → Review → Ship). Do not skip steps or batch them. Key points:
- **Design before plan** — explore the problem, brainstorm approaches, align with user
- **Plan before code** — write and commit plan file before any implementation
- **One branch + one PR per plan** — all phases of a plan land on a single feature branch (`feat/NNN-description`) and ship together as a single PR. Phases are commit-level subdivisions of the same branch, not separate PRs. (Exception: a plan with a genuinely independent backend-infrastructure phase that other phases don't depend on may ship as a separate PR — document the deviation in the plan file. Default is single-PR-per-plan.)
- **Build phase by phase, commit per phase** — implement, test, self-review, document, commit each phase separately on the same plan branch. The phase boundaries help the reviewer trace logical units in the diff; the single PR keeps the audit trail and CI history consolidated.
- **Verify before review** — full test suite, cross-phase consistency check
- **Review before ship** — 4-way code review fires once per plan PR (not per phase). Findings go in the plan file's `## Code Review` section, all [OPEN] items resolved before merge.
- **Ship cleanly** — post-execution report, update `TODO.md`, then PR

## Code Review

Follow `docs/code-review.md` for the review process. Reviews use the same 4-way pattern as plans but with a **flash-tier model for DeepSeek** (code reviews are more frequent and latency-sensitive):

| # | Reviewer | How to dispatch | Model |
|---|----------|-----------------|-------|
| 1 | **Self-review (Opus 4.7)** | Claude reads the diff critically on Opus before dispatching | claude-opus-4-7 |
| 2 | **Codex** | `codex:codex-rescue` subagent with branch diff + review questions | Codex default |
| 3 | **DeepSeek V4 Flash** | `opencode:opencode-review` subagent with `--model deepseek/deepseek-v4-flash` | deepseek/deepseek-v4-flash |
| 4 | **GLM 5.1** | `opencode:opencode-review` subagent with `--model volcengine-plan/glm-5.1` | volcengine-plan/glm-5.1 |

Key points:
- **Timing**: the 4-way code review fires once per plan PR — at PR-open time after all phases are implemented and verified. Not after each phase. (Exception: a one-phase plan or a single-PR-deviation plan reviews at the same point — once, before merge.)
- All four reviewers dispatch **in parallel** (multiple Agent calls in one message) after the self-review pass.
- Reviews go in the plan file's `## Code Review` section.
- Reviewers: append findings with `[OPEN]` status and file:line references.
- Authors: respond inline with `→ Response:` and `[FIXED]`/`[WONTFIX]`.
- If ANY reviewer flags a blocker, fix it before merging. Re-dispatch only the flagging reviewer(s) to confirm the fix.

## Coding Agent

Bridge uses a **two-tier Claude coding policy** — Sonnet 4.6 for the bulk of routine work, Opus 4.7 for the genuinely hard parts. The model in use should match the difficulty of the task, not the prestige of the plan.

### Spawn subagents for coding work (preferred)

The orchestrator Claude (the session talking to the user) stays on Opus 4.7 for **planning, review, and coordination**. Actual implementation work — file edits, code generation, test writing — should be delegated to a subagent so the implementation can run on whichever model fits the task. This pattern has three benefits:

1. **Right-sized model per task** — routine implementation runs on Sonnet (faster + cheaper), complex implementation runs on Opus, both are explicit choices rather than whatever model the orchestrator happens to be on.
2. **Context isolation** — the subagent gets a focused brief, does its work in its own context window, and returns a summary. The orchestrator's context stays clean for review and coordination.
3. **Parallelizable** — independent coding tasks (e.g., backend handler + frontend component + tests) can dispatch as parallel subagents in a single message.

How to dispatch:

```
Agent tool with:
  subagent_type: "general-purpose"   # or a specific persona where applicable
  model: "sonnet"                     # or "opus" for complex work
  prompt: <complete brief — files to touch, what to do, verification steps>
```

The `model` parameter on the Agent tool **overrides** the subagent definition's default model, so the orchestrator picks per-call. Valid values: `"sonnet"`, `"opus"`, `"haiku"`.

When you spawn a subagent, give it a self-contained brief:
- The exact files to read / modify / create
- The acceptance criteria (what tests should pass, what type-check / lint must be clean)
- Any relevant context the subagent can't infer from the codebase alone (e.g., "use the existing pattern in `X` rather than inventing a new one")
- Whether the subagent should commit + push, or hand back to the orchestrator for review first

After the subagent returns, the orchestrator must **verify the actual changes** (read the diff, run tests if relevant) before reporting the work as done. The subagent's summary describes intent, not necessarily reality.

### Claude Sonnet 4.6 — routine coding (default subagent model)

Spawn a Sonnet subagent for:
- **All test code** (Vitest, Go tests, Playwright e2e, fixtures, helpers) — see "Testing" above. Test code is the single most consistent fit for Sonnet: pattern-heavy, repetitive, fast feedback loop.
- New components / pages following established patterns
- CRUD endpoints on top of existing handlers + stores
- Refactors with a clear before/after shape
- Doc updates, dependency bumps, copy edits
- Small bug fixes where the root cause is already known

### Claude Opus 4.7 — complex coding (orchestrator + complex subagents)

Keep work in the orchestrator (or spawn an Opus subagent) when:
- Designing a new architectural pattern or cross-cutting abstraction
- Debugging a non-obvious issue with multi-system surface area (race conditions, state-machine bugs, cross-service auth, distributed correctness)
- Implementing the first phase of a plan that establishes patterns later phases will follow
- Reading a large unfamiliar codebase to plan a refactor
- Tasks that genuinely benefit from the 1M context window (e.g., reasoning over the whole `platform/internal/store` at once)
- Anything where a Sonnet subagent has already attempted and gotten stuck

When in doubt, dispatch a Sonnet subagent first. Promote to Opus only when the work clearly warrants it — Opus is more expensive, slower, and shouldn't be the reflex choice. Promotion mid-task is fine: if a Sonnet subagent hits a wall, the orchestrator (already on Opus) can take over directly or spawn an Opus subagent with the previous attempt's context as input.

### When to skip the subagent and stay inline

Spawning a subagent has overhead — context to write, results to verify. Skip it when:
- The change is a single-line edit or a trivial fix.
- You're mid-debugging and need the conversation context to continue (the subagent loses everything not in its prompt).
- You're iterating rapidly on a small file and the back-and-forth would dominate the time spent.

Otherwise, default to the subagent.

### Self-review always runs on Opus 4.7

The **self-review stage** of both gates (plan review #1 and code review #1) ALWAYS runs on Opus 4.7, regardless of which tier wrote the plan or code. Review is judgment-heavy and load-bearing — a routine implementation deserves a careful review, and the cost gap between Sonnet and Opus on a single review pass is dwarfed by the cost of shipping a flaw that the gate should have caught. Switch to Opus before invoking the self-review step.

### Both tiers — same rules

- Do NOT delegate coding tasks to Codex, DeepSeek, or GLM. Those models are review-only (see below).
- Both tiers follow the 4-way review gates for plans and code (with Opus on self-review, see above).
- Both tiers respect the branch-first rule and the development workflow.

## External Reviewers (Review Only)

Three external review models complement Claude's self-review. None of them implement code — they review only.

### Codex
- Dispatch: `codex:codex-rescue` subagent.
- Use for: plan reviews, code reviews, spec reviews, post-impl reviews.
- Prompt style: under 500 words, focused questions, time-bounded.

### DeepSeek (via opencode)
- Plan reviews: `opencode:opencode-review` subagent with `--model deepseek/deepseek-v4-pro`.
- Code reviews: `opencode:opencode-review` subagent with `--model deepseek/deepseek-v4-flash` (faster for frequent post-impl passes).
- Prompt style: same prompt as Codex (plan path + questions, or branch diff + questions). The opencode subagent accepts free-form prompt text forwarded to the model.

### GLM 5.1 (via opencode)
- All reviews: `opencode:opencode-review` subagent with `--model volcengine-plan/glm-5.1`.
- Prompt style: same as above.

Do NOT use any of these for implementation, debugging, refactoring, or coding. Those belong to Claude Sonnet 4.6 (default) or Opus 4.7 (complex) — see "Coding Agent" above.

## Multi-Agent Coordination

Multiple Claude Code sessions share the same local repository. Only one agent should work at a time. To avoid lost work:

- **Always commit before ending a session.** Even partial work — use a `WIP:` prefix. Uncommitted changes are invisible to the next session and will be lost.
- **Check `git status` at session start.** Look for untracked or modified files left by a previous session. Ask the user before discarding them.
- **Each plan uses a feature branch.** Pull before starting work, push before ending. Never leave unpushed commits.
- **Never assume prior session completed its work.** Verify by reading the plan file's post-execution report and checking git log — don't trust claims in conversation summaries alone.

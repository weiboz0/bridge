# Coding Agent Policy

Bridge uses a **two-tier Claude coding policy** — Sonnet 4.6 for the bulk of routine work, Opus 4.7 for the genuinely hard parts. The model in use should match the difficulty of the task, not the prestige of the plan.

## Spawn subagents for coding work (preferred)

The orchestrator Claude (the session talking to the user) stays on Opus 4.7 for **planning, review, and coordination**. Actual implementation — file edits, code generation, test writing — should be delegated to a subagent so it runs on whichever model fits the task. Benefits:

1. **Right-sized model per task** — routine implementation on Sonnet (faster, cheaper), complex implementation on Opus.
2. **Context isolation** — the subagent gets a focused brief, does its work in its own context window, returns a summary. The orchestrator's context stays clean for review.
3. **Parallelizable** — independent coding tasks (backend handler + frontend component + tests) can dispatch as parallel subagents in a single message.

### How to dispatch

```
Agent tool with:
  subagent_type: "general-purpose"   # or a specific persona where applicable
  model: "sonnet"                     # or "opus" for complex work
  prompt: <complete brief — files to touch, what to do, verification steps>
```

The `model` parameter on the Agent tool **overrides** the subagent definition's default model, so the orchestrator picks per-call. Valid values: `"sonnet"`, `"opus"`, `"haiku"`.

### Subagent brief checklist

When you spawn a subagent, give it a self-contained brief:
- The exact files to read / modify / create.
- The acceptance criteria (what tests should pass, lint/type-check clean).
- Any context the subagent can't infer from the codebase alone (e.g., "use the existing pattern in `X` rather than inventing a new one").
- Whether the subagent should commit + push, or hand back to the orchestrator for review first.

After the subagent returns, the orchestrator must **verify the actual changes** (read the diff, run tests if relevant) before reporting the work as done. The subagent's summary describes intent, not necessarily reality.

## Tier criteria

### Sonnet 4.6 — routine coding (default subagent model)

Spawn a Sonnet subagent for:
- **All test code** — Vitest, Go tests, Playwright e2e, fixtures, helpers. Tests are pattern-heavy, repetitive, fast-feedback. Only escalate to Opus when the test itself is hard (flaky timing-dependent test, new fixture pattern other suites will copy, test driving multi-system integration through unfamiliar territory).
- New components / pages following established patterns.
- CRUD endpoints on top of existing handlers + stores.
- Refactors with a clear before/after shape.
- Doc updates, dependency bumps, copy edits.
- Small bug fixes where the root cause is already known.

### Opus 4.7 — complex coding (orchestrator + complex subagents)

Keep work in the orchestrator (or spawn an Opus subagent) when:
- Designing a new architectural pattern or cross-cutting abstraction.
- Debugging a non-obvious multi-system issue (race conditions, state-machine bugs, cross-service auth, distributed correctness).
- Implementing the first phase of a plan that establishes patterns later phases will follow.
- Reading a large unfamiliar codebase to plan a refactor.
- Tasks that genuinely benefit from the 1M context window (e.g., reasoning over the whole `platform/internal/store` at once).
- Anything where a Sonnet subagent has already attempted and gotten stuck.

When in doubt, dispatch Sonnet first. Promote to Opus only when the work clearly warrants it. Promotion mid-task is fine.

## When to skip the subagent and stay inline

Spawning a subagent has overhead — context to write, results to verify. Skip it when:
- The change is a single-line edit or trivial fix.
- You're mid-debugging and need the conversation context to continue (the subagent loses everything not in its prompt).
- You're iterating rapidly on a small file and the back-and-forth would dominate.

Otherwise, default to the subagent.

## Self-review always runs on Opus 4.7

The **self-review stage** of both gates (plan review #1 and code review #1) ALWAYS runs on Opus 4.7, regardless of which tier wrote the plan or code. Review is judgment-heavy and load-bearing — a routine implementation deserves a careful review, and the cost gap between Sonnet and Opus on a single review pass is dwarfed by the cost of shipping a flaw the gate should have caught. Switch to Opus before invoking the self-review step.

## Both tiers — same rules

- Do NOT delegate coding tasks to Codex, DeepSeek, GLM, or Kimi. Those are review-only. See `docs/reviewers.md`.
- Both tiers follow the 5-way review gates for plans and code (with Opus on self-review).
- Both tiers respect the branch-first rule and the development workflow.

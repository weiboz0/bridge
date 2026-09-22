# Coding agent policy

Bridge dispatches coding work by **domain**, not by complexity.
The orchestrator (Opus 5) stays in session for planning, review, and coordination, then hands
implementation to a subagent whose model matches what the code *is* — not how hard it looks.

## Permanent review-gate contract

Every committed design spec under `docs/specs/**` passes a design-review gate before its implementation plan is drafted or revised from it.
The design gate has exactly two required reviewers: Codex Sol (`gpt-5.6-sol`, reasoning effort high) and Claude Code (`claude-fable-5`).
Design reviews are read-only and bind every verdict to the exact substantive commit.
Both reviewers receive read-only prompts and must approve the same exact substantive commit with no open blocker.
Record `[sol]` and `[fable]` findings and exact-commit verdicts in the spec; a material revision invalidates both approvals.

The design gate, plan-review gate, and code-review gate are uncapped consensus loops with no numeric round cap.
Consensus requires every required reviewer to return `APPROVE` or `APPROVE WITH NITS` with no open blocker.
An unavailable required reviewer pauses the gate and is never silently substituted, replaced, or waived.
After every three consecutive non-converged substantive rounds, surface a checkpoint; repeated reopening or two checkpoints without net blocker reduction requires a genuine user-decision pause.

## Dispatch table

| Domain | Agent | Dispatch | Why |
|--------|-------|----------|-----|
| **Backend** — Go in `platform/`, `server/hocuspocus.ts` | Codex `gpt-5.6-terra` | `codex:codex-rescue` subagent | Go's conventions here are tight (Chi, store/handler split, parameterized SQL, `slog`, RFC3339). Codex follows them well from one brief. |
| **Frontend** — Next.js App Router, React, `src/` | Sonnet 5 | `Agent`, `model: "sonnet"` | Pattern-heavy work across many similar components, fast feedback loop. |
| **Tests** (all domains) | Opus 5 | `Agent`, `model: "opus"` | Tests are the correctness contract; the deeper model is worth it to catch the cases a pattern-matcher would rubber-stamp. |
| **Cross-domain / new patterns / hard debugging** | Opus 5 | inline, or `Agent`, `model: "opus"` | The cross-cutting reasoning IS the value; splitting by domain would lose it. |
| **Review** | see `docs/reviewers.md` | — | Reviewer slots are a separate roster from implementer slots. Don't conflate them. |

GLM is **review-only**. Never delegate implementation to it.

## Why domain rather than complexity

Complexity-based dispatch requires predicting task difficulty up front, and that prediction is often
wrong — work tagged "routine" turns out to need depth, work tagged "complex" turns out mechanical.
Domain-based dispatch keys on the code's idiom and cost profile instead, which is observable rather
than predicted. Genuinely cross-cutting cases stay with the orchestrator.

## How to dispatch

```
# Backend
Agent tool with:
  subagent_type: "codex:codex-rescue"
  prompt: <complete brief>

# Frontend
Agent tool with:
  subagent_type: "general-purpose"
  model: "sonnet"          # Sonnet 5
  prompt: <complete brief>

# Tests (any domain)
Agent tool with:
  subagent_type: "general-purpose"
  model: "opus"            # Opus 5 — tests are the correctness contract
  prompt: <complete brief>
```

The `model` parameter overrides the subagent definition's default. Valid: `"sonnet"`, `"opus"`, `"haiku"`, `"fable"`.
`codex:codex-rescue` uses its own model (set `gpt-5.6-terra` in `~/.codex/config.toml`) regardless of the `model` parameter.

## Brief checklist

Every dispatch, every route:

- The exact files to read, modify, or create — and the plan's `## File scope`, which the subagent must not exceed.
- Acceptance criteria: which tests must pass, lint and type-check clean.
- Context the subagent can't infer from the codebase ("use the existing pattern in `X`, don't invent one").
- Whether to commit and push, or hand back to the orchestrator first.
- For anything touching the database, the `DATABASE_URL` constraint from `AGENTS.md` — subagents
  inherit the environment and can migrate a real database by accident.

## Trust but verify

After a subagent returns, the orchestrator MUST verify the actual changes — read the diff, run the
tests, grep for cross-references it claims to have touched — before reporting the work done.
**A subagent's summary describes intent, not necessarily reality.**
This has caught real discrepancies; treat it as a required step, not a formality.

## When to stay inline

Spawning a subagent costs context to write and results to verify. Skip it when:

- The change is a single-line edit or trivial fix.
- You're mid-debugging and need the conversation context — the subagent loses everything not in its prompt.
- You're iterating rapidly on a small file and the round-trip would dominate.

Otherwise, default to the domain dispatch.

## Parallelism

- **Parallelize** when subtasks touch different files with no ordering dependency.
  Cross-domain is the common case: dispatch Codex for the Go handler and Sonnet for the React
  component in a single message.
- **Don't** when subtasks share state, when the work is small enough that overhead exceeds the gain,
  or when correctness depends on ordering.

## Promotion mid-task

If the domain default hits a wall — Codex stuck on a race, Sonnet stuck on a hook interaction — take
it over inline with the context intact, or spawn an Opus subagent carrying the previous attempt.
A user `/model` pin overrides all of this; respect it.

## Self-review always runs on Opus 5

The self-review slot of both gates runs on Opus 5 regardless of who wrote the code.
Review is judgment-heavy and load-bearing: the cost gap on a single pass is dwarfed by the cost of
shipping a flaw the gate should have caught.

## Codex's dual role

Codex both implements backend code and sits on the review gates.
When it wrote the code, its own review carries familiarity bias — the ensemble supplies independence
via the fresh-context Claude reviewer and GLM.
Note also that Codex runs with `sandbox_mode = "danger-full-access"`: a "read-only" instruction is
prompt-enforced only, so state it explicitly every time.

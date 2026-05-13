# Coding Agent Policy

Bridge uses **domain-based dispatch** for coding work. The orchestrator (Opus 4.7) stays in the session for planning, review, and coordination, then dispatches a subagent whose model matches the work's domain — not its complexity.

## Default dispatch by domain

| Domain | Default agent | Dispatch | Notes |
|--------|---------------|----------|-------|
| **Backend** — Go in `platform/`, Hocuspocus server, Go-adjacent server logic | Codex | `codex:codex-rescue` subagent | Codex is a strong fit for Go and server-side TypeScript. |
| **Frontend** — Next.js App Router, React components, `src/` UI code | Claude Sonnet 4.6 | `Agent` tool, `model: "sonnet"` | Pattern-heavy work with fast feedback loops. |
| **Tests** (all domains) | Claude Sonnet 4.6 | `Agent` tool, `model: "sonnet"` | Tests are pattern-heavy + repetitive across backend and frontend alike. |
| **Complex / cross-domain / new patterns** | Claude Opus 4.7 | Orchestrator inline OR `Agent` tool, `model: "opus"` | See "When to escalate to Opus" below. |
| **Review** | Claude Opus 4.7 + Codex + DeepSeek + GLM + Kimi | See `docs/reviewers.md` | Codex reviews even when it also wrote the code (other reviewers carry the independence). |

DeepSeek, GLM, and Kimi remain **review-only**. Do NOT delegate implementation to them.

## Why domain instead of complexity

The previous policy split by complexity (Sonnet routine / Opus complex) and reserved Codex for review. The new policy delegates by what the code is, not how hard it is:

- **Codex on backend** — Go has tight conventions (Chi router, store/handler separation, parameterized SQL, `slog`, RFC3339 timestamps, defensive paths) that Codex tends to follow well. The orchestrator briefs once and Codex executes within the convention.
- **Sonnet on frontend** — React + Next.js work benefits from fast iteration and pattern recognition across many similar components. Sonnet's speed-cost profile beats Opus for the bulk of UI work.
- **Opus for orchestration + complex cases** — Cross-domain reasoning, brand-new patterns, and stuck-state debugging still warrant the orchestrator's context or an Opus subagent.

## How to dispatch

### Codex (backend)

```
Agent tool with:
  subagent_type: "codex:codex-rescue"
  prompt: <complete brief — files to touch, what to do, verification steps>
```

Codex briefs should include:
- Exact files / package paths.
- Test commands (`cd platform && go test ./... -count=1 -timeout 120s`).
- Conventions the change must respect (see `CLAUDE.md` ## Coding Conventions → Go).
- Whether to commit/push or hand back to orchestrator.

### Sonnet (frontend, tests)

```
Agent tool with:
  subagent_type: "general-purpose"   # or a specific persona where applicable
  model: "sonnet"
  prompt: <complete brief — files to touch, what to do, verification steps>
```

The `model` parameter on the Agent tool **overrides** the subagent definition's default model. Valid values: `"sonnet"`, `"opus"`, `"haiku"`.

### Opus (complex, escalation)

Same dispatch shape as Sonnet but `model: "opus"`. Or keep the work inline in the orchestrator session.

## Subagent brief checklist (all dispatch routes)

- The exact files to read / modify / create.
- The acceptance criteria (tests that should pass, lint/type-check clean).
- Context the subagent can't infer from the codebase alone ("use the existing pattern in `X`").
- Whether to commit + push, or hand back to the orchestrator for review first.

After the subagent returns, the orchestrator must **verify the actual changes** (read the diff, run tests if relevant) before reporting work as done. The subagent's summary describes intent, not necessarily reality.

## When to escalate to Opus

Even within a domain, some work needs the orchestrator (or an Opus subagent):

- Designing a new architectural pattern or cross-cutting abstraction.
- Debugging a multi-system issue (race conditions, state-machine bugs, cross-service auth).
- Implementing the first phase of a plan that establishes patterns later phases will follow.
- Reading a large unfamiliar codebase to plan a refactor.
- Tasks that benefit from the 1M context window (e.g., reasoning over the whole `platform/internal/store` at once).
- Anything where the domain-default subagent has already attempted and gotten stuck.

When in doubt, dispatch the domain default first. Promote to Opus only when the work clearly warrants it. Promotion mid-task is fine — the orchestrator can take over directly or spawn an Opus subagent with the previous attempt's context.

## When to skip the subagent and stay inline

Spawning a subagent has overhead — context to write, results to verify. Skip it when:
- The change is a single-line edit or trivial fix.
- You're mid-debugging and need the conversation context to continue (the subagent loses everything not in its prompt).
- You're iterating rapidly on a small file and the back-and-forth would dominate.

Otherwise, default to the domain dispatch.

## Self-review always runs on Opus 4.7

The **self-review stage** of both gates (plan review #1 and code review #1) ALWAYS runs on Opus 4.7, regardless of which agent wrote the plan or code. Review is judgment-heavy and load-bearing — a routine implementation deserves a careful review, and the cost gap between models on a single review pass is dwarfed by the cost of shipping a flaw the gate should have caught.

## Codex's dual role

Codex implements backend code AND participates in plan + code reviews (see `docs/reviewers.md`). When Codex wrote the code, its own review naturally has familiarity bias — but Codex still runs as one of the five reviewers because the gate's strength comes from the ensemble: GLM, DeepSeek, Kimi, and Opus-self provide independent perspectives on Codex-authored code.

## All routes — same rules

- Both Codex and Sonnet follow the 5-way review gates for plans and code (with Opus on self-review).
- Both respect the branch-first rule and the development workflow.
- Both must honor the conventions in `CLAUDE.md` ## Coding Conventions.

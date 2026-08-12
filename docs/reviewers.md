# Reviewers

Bridge runs **risk-tiered** review gates.
External reviewers complement Claude's self-review; none of them implement code at the gate.
Implementation dispatch is a separate policy — see `docs/coding-agent.md`.

## Risk tier routing

Tier is a function of the plan's `## File scope`, not a judgment call.

- **Tier A → 4-way.** Any scope touching
  `platform/internal/{middleware,store,llm,tools,skills}/**`, `drizzle/**`, `server/hocuspocus.ts`,
  `scripts/**`, `.githooks/**`, or the governance docs.
- **Tier B → 2-way.** Scope lying *entirely* within `src/**` with no API-contract change,
  `content/**`, or `docs/**` excluding governance docs.
- **Precedence.** One Tier-A path makes the whole plan Tier A. Plans are never split across tiers.
- **Default.** Any path matching neither list is Tier A.

*Worked example.* Plan 091's scope includes `scripts/**` and `.githooks/**` → Tier A → 4-way.

## Reviewer roster

| # | Reviewer | Dispatch | Model |
|---|----------|----------|-------|
| 1 | Claude self-review | inline by orchestrator | Opus 5 |
| 2 | Codex | `codex:codex-rescue` subagent | `gpt-5.6-sol`, reasoning effort high |
| 3 | Independent Claude | `Agent` (general-purpose, `model: opus`) — fresh context, read-only | Opus 5 |
| 4 | GLM | `opencode:opencode-review` subagent — read-only | `volcengine-plan/glm-5.2` |

Tier B runs slots 1 and 2. Tier A runs all four.
Dispatch the externals **in parallel with** the inline self-review — one message, multiple blocks.

Slot 3 is a *separate, fresh-context* subagent, not the inline self-review.
Its value is having no conversation history, so it cannot inherit the author's assumptions.
Fable 5 may substitute for Opus in slot 1 or 3 when credits allow;
it is not the default, because a Fable-pinned slot failed mid-gate on quota exhaustion during plan 091's own review.

### Reviewer environment caveats

- **Codex runs `approval_policy = "never"` with `sandbox_mode = "danger-full-access"`.**
  "Read-only, do not write code" is enforced by prompt text only — there is no sandbox behind it.
  Write it explicitly in every review prompt.
- **GLM's opencode `limit.output` must stay ≥ 32000.**
  At the default 4096 it exhausts its budget on reasoning and returns *zero* text parts,
  which surfaces as an empty review rather than an error.
  This cost two round-2 attempts during plan 091 before it was diagnosed.
- Keep external prompts under ~500 words and time-bounded.
  If a reviewer needs a remote read, push the branch first.

## Design-review gate

Every committed design spec under `docs/specs/**` passes a design-review gate before an implementation plan is drafted or revised from it.
The design gate has exactly two required reviewers: Codex Sol (`gpt-5.6-sol`, reasoning effort high) and Claude Code (`claude-fable-5`).
Design reviews are read-only and bind every verdict to the exact substantive commit.
Both receive read-only prompts and must approve the same exact substantive commit with no open blocker.
Record verdicts and findings in the spec with `[sol]` and `[fable]` source tags.
An author response remains `[OPEN]` until the flagging reviewer confirms it; a material revision invalidates both approvals and re-dispatches both reviewers.

The design gate, plan-review gate, and code-review gate are uncapped consensus loops with no numeric round cap.
Consensus requires every required reviewer to return `APPROVE` or `APPROVE WITH NITS` with no open blocker.
An unavailable required reviewer pauses the gate; do not silently substitute, replace, or waive that reviewer.
After every three consecutive non-converged substantive rounds, record and surface a concise checkpoint.
If the same finding reopens twice after claimed resolutions, or two consecutive checkpoints show no net reduction in open blockers, pause for a genuine user decision.

## Plan-review gate

Every plan — new or revised — passes its tier's gate before any code is written.

1. **Create the feature branch FIRST** — `git checkout -b feat/NNN-description`, before any plan drafting.
2. **Draft the plan** in `docs/plans/`, including its `## File scope`. Commit it so reviewers see the on-disk file.
3. **Run the self-review** on Opus 5 — read the plan with fresh eyes, record concerns inline.
4. **Dispatch the externals in parallel** with the plan path and explicit review questions:
   blockers, hidden assumptions, scope, ordering, missing risks.
5. **Capture every verdict** in the plan's `## Plan Review` section, tagged
   `[claude-self]` / `[codex]` / `[opus]` / `[glm]`, with date and blockers. Commit verdicts as they arrive.
6. **Iterate to consensus.** Re-dispatch only the flagging reviewers.
7. **Then implement**, on the same branch.

**Consensus is full-blocking:** the gate passes only when every reviewer in the tier returns
APPROVE or APPROVE WITH NITS with no open blockers.
A passing gate IS user approval — no separate approval pause follows it.

**Reviewer duty on integration tests.** Every reviewer MUST reject a plan that lacks a named
integration-tests phase when it touches the Go API surface, the realtime protocol, or cross-cutting
plumbing (auth, persistence, org scoping). One reject blocks the gate.
Exempt plans (docs-only, behavior-preserving refactors, plan-design plans) must call the omission out
explicitly in `## Out of scope`; silent omission is a blocker.

## Code-review gate

Same roster and tier rules, run against the consolidated branch diff.

- Fires **once per plan**, after Step 4 (Verify) and before the PR is opened in Step 6 — not per phase.
- All reviewers dispatch in parallel after the self-review pass.
- Findings go in the plan's `## Code Review` section per `docs/code-review.md`, with `[OPEN]` status and file:line refs.
- Authors respond inline with `→ Response:` and `[FIXED]` / `[WONTFIX]`.
- All `[OPEN]` items resolve before merge. Re-dispatch only the flagging reviewers to confirm.

Plan and code review retain the risk-tiered roster above.
They iterate under the same uncapped consensus and no-open-blocker contract as the design gate.

## Notes on reviewer independence

Codex both implements backend code and reviews.
When Codex wrote the code its own review carries familiarity bias — the ensemble is what supplies
independence, via the fresh-context Claude reviewer and GLM.
Do not treat a Codex approval of Codex-authored code as an independent signal on its own.

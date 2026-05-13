# Reviewers

Bridge runs a **4-way review** on every plan and every PR. Three external models complement Claude's self-review. None of the externals implement code — they review only. See `docs/coding-agent.md` for the implementation tier policy.

## Plan review gate (mandatory — 4-way)

Every plan — new or revised — must pass a 4-way review before any code is written. The four reviewers run **in parallel** (dispatch all simultaneously via multiple Agent/subagent calls in a single message):

| # | Reviewer | How to dispatch | Model |
|---|----------|-----------------|-------|
| 1 | **Self-review (Opus 4.7)** | Claude reads its own plan critically on Opus and lists concerns inline | claude-opus-4-7 |
| 2 | **Codex** | `codex:codex-rescue` subagent with the plan path + review questions | Codex default |
| 3 | **DeepSeek V4 Pro** | `opencode:opencode-review` subagent with `--model opencode-go/deepseek-v4-pro` | opencode-go/deepseek-v4-pro |
| 4 | **GLM 5.1** | `opencode:opencode-review` subagent with `--model opencode-go/glm-5.1` | opencode-go/glm-5.1 |

### Steps

1. **Create the feature branch FIRST** — `git checkout -b feat/NNN-description` before any plan drafting. Every commit (plan file, review verdicts, revisions, implementation) lands on the feature branch, never on `main`. The opencode session-continuity helper keys on `plan-NNN`-style branch names, so reviewer history scopes correctly when the branch matches `feat/NNN-*`.
2. **Draft or revise the plan** in `docs/plans/`. Commit it to the feature branch so reviewers see the on-disk file Claude is iterating on.
3. **Run self-review** on Opus — Claude reads the plan with fresh eyes and records concerns.
4. **Dispatch reviewers 2-4 in parallel** — same prompt to each (plan path + explicit review questions: blockers, hidden assumptions, scope, ordering, missing risks). Keep prompts under 500 words, time-bounded. If a reviewer needs a remote read (e.g., GitHub connector), push the branch first.
5. **Capture all four verdicts** in the plan's `## Plan Review` section. Include date, reviewer, verdict, blockers, resolution. Commit verdicts to the feature branch as they arrive — don't batch the audit trail.
6. **If ANY reviewer flags blockers, revise** on the same feature branch. Re-dispatch only the flagging reviewer(s). Iterate until all four concur.
7. **Only then** begin implementation on the same branch.

A plan that hasn't passed all four reviews is not ready for execution, regardless of how confident Claude is in it.

## Code review gate (mandatory — 4-way)

Code reviews use the same 4-way pattern but with a **flash-tier model for DeepSeek** (code reviews are more frequent and latency-sensitive):

| # | Reviewer | How to dispatch | Model |
|---|----------|-----------------|-------|
| 1 | **Self-review (Opus 4.7)** | Claude reads the diff critically on Opus before dispatching | claude-opus-4-7 |
| 2 | **Codex** | `codex:codex-rescue` subagent with branch diff + review questions | Codex default |
| 3 | **DeepSeek V4 Flash** | `opencode:opencode-review` subagent with `--model deepseek/deepseek-v4-flash` | deepseek/deepseek-v4-flash |
| 4 | **GLM 5.1** | `opencode:opencode-review` subagent with `--model opencode-go/glm-5.1` | opencode-go/glm-5.1 |

### Timing + format

- The 4-way fires **once per plan** against the consolidated branch diff after Step 4 (Verify), before the PR is opened in Step 6. Not per phase. (Exception: a one-phase plan or a single-PR-deviation plan reviews at the same point — once, before the PR is opened.)
- All four dispatch **in parallel** (multiple Agent calls in one message) after the self-review pass.
- Findings go in the plan file's `## Code Review` section, following `docs/code-review.md` format.
- Reviewers append findings with `[OPEN]` status and file:line references.
- Authors respond inline with `→ Response:` and `[FIXED]` / `[WONTFIX]`.
- If ANY reviewer flags a blocker, fix before merging. Re-dispatch only the flagging reviewer(s) to confirm.

## External reviewer dispatch details

### Codex
- Dispatch: `codex:codex-rescue` subagent.
- Use for: plan reviews, code reviews, spec reviews, post-impl reviews.
- **Also the default backend implementer** — see `docs/coding-agent.md`. When Codex wrote the code, it still runs as one of the four reviewers; ensemble independence comes from the other three.
- Prompt style: under 500 words, focused questions, time-bounded.

### DeepSeek (via opencode)
- Plan reviews: `--model opencode-go/deepseek-v4-pro` (opencode-go provider).
- Code reviews: `--model deepseek/deepseek-v4-flash` (deepseek provider — flash tier, faster for frequent post-impl passes).
- Prompt style: same as Codex (plan path + questions, or branch diff + questions). The opencode subagent accepts free-form prompt text forwarded to the model.

### GLM 5.1 (via opencode)
- All reviews: `--model opencode-go/glm-5.1`.
- Prompt style: same as above.

**DeepSeek and GLM are review-only — never use them for implementation.** Implementation is split by domain: Codex for backend, Sonnet for frontend + tests, Opus for complex/cross-domain — see `docs/coding-agent.md`.

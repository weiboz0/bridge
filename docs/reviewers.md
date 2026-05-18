# Reviewers

Bridge runs a **2-way plan review** and a **3-way code review** on every plan and PR. External reviewers complement Claude's self-review. None of the externals implement code — they review only. See `docs/coding-agent.md` for the implementation tier policy.

## Plan review gate (mandatory — 2-way)

Every plan — new or revised — must pass a 2-way review before any code is written. Both reviewers run **in parallel** (dispatch simultaneously via Agent calls in a single message):

| # | Reviewer | How to dispatch | Model |
|---|----------|-----------------|-------|
| 1 | **Self-review (Opus 4.7)** | Claude reads its own plan critically on Opus and lists concerns inline | claude-opus-4-7 |
| 2 | **Codex** | `codex:codex-rescue` subagent with the plan path + review questions | Codex default |

### Steps

1. **Create the feature branch FIRST** — `git checkout -b feat/NNN-description` before any plan drafting. Every commit (plan file, review verdicts, revisions, implementation) lands on the feature branch, never on `main`. The opencode session-continuity helper keys on `plan-NNN`-style branch names, so reviewer history scopes correctly when the branch matches `feat/NNN-*`.
2. **Draft or revise the plan** in `docs/plans/`. Commit it to the feature branch so reviewers see the on-disk file Claude is iterating on.
3. **Run self-review** on Opus — Claude reads the plan with fresh eyes and records concerns.
4. **Dispatch Codex** — plan path + explicit review questions: blockers, hidden assumptions, scope, ordering, missing risks. Keep prompts under 500 words, time-bounded. If Codex needs a remote read (e.g., GitHub connector), push the branch first.
5. **Capture both verdicts** in the plan's `## Plan Review` section. Include date, reviewer, verdict, blockers, resolution. Commit verdicts to the feature branch as they arrive.
6. **If either reviewer flags blockers, revise** on the same feature branch. Re-dispatch only the flagging reviewer(s). Iterate until both concur.
7. **Only then** begin implementation on the same branch.

A plan that hasn't passed both reviews is not ready for execution, regardless of how confident Claude is in it.

## Code review gate (mandatory — 3-way)

Code reviews use a 3-way ensemble. GLM joins for code review (not plan review) — independent ensemble on diff is more valuable post-impl than on plan text:

| # | Reviewer | How to dispatch | Model |
|---|----------|-----------------|-------|
| 1 | **Self-review (Opus 4.7)** | Claude reads the diff critically on Opus before dispatching | claude-opus-4-7 |
| 2 | **Codex** | `codex:codex-rescue` subagent with branch diff + review questions | Codex default |
| 3 | **GLM 5.1** | `opencode:opencode-review` subagent with `--model volcengine-plan/glm-5.1` | volcengine-plan/glm-5.1 |

### Timing + format

- The 3-way fires **once per plan** against the consolidated branch diff after Step 4 (Verify), before the PR is opened in Step 6. Not per phase. (Exception: a one-phase plan or a single-PR-deviation plan reviews at the same point — once, before the PR is opened.)
- All three dispatch **in parallel** (multiple Agent calls in one message) after the self-review pass.
- Findings go in the plan file's `## Code Review` section, following `docs/code-review.md` format.
- Reviewers append findings with `[OPEN]` status and file:line references.
- Authors respond inline with `→ Response:` and `[FIXED]` / `[WONTFIX]`.
- If any reviewer flags a blocker, fix before merging. Re-dispatch only the flagging reviewer(s) to confirm.

## External reviewer dispatch details

### Codex
- Dispatch: `codex:codex-rescue` subagent.
- Use for: plan reviews, code reviews, spec reviews, post-impl reviews.
- **Also the default backend implementer** — see `docs/coding-agent.md`. When Codex wrote the code, it still runs as one of the reviewers; ensemble independence comes from the other reviewers.
- Prompt style: under 500 words, focused questions, time-bounded.

### GLM 5.1 (via opencode)
- Code reviews only: `--model volcengine-plan/glm-5.1`.
- Prompt style: under 500 words, focused questions, branch diff + explicit review questions.

**GLM is review-only — never use it for implementation.** Implementation is split by domain: Codex for backend, Sonnet for frontend + tests, Opus for complex/cross-domain — see `docs/coding-agent.md`.

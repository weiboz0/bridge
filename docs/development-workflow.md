# Development Workflow

Follow this process for every plan. Do NOT skip steps or batch them.

For trivial changes (typo fixes, one-line patches), use your judgement — not every change needs the full workflow. But any multi-file feature, refactor, or integration must follow it.

> **Superpowers integration:** Each step lists applicable superpowers skills. These are *tools for meeting the step's requirements*, not replacements. The step's requirements are mandatory whether or not superpowers is installed.

---

## Step 1 — Design

Explore the problem space before committing to an approach.

1. Clarify the user's intent — what problem are we solving, what does success look like?
2. Research constraints: read `docs/architecture/decisions.md`, check relevant code, identify affected systems.
3. Brainstorm approaches. Compare trade-offs. Identify risks.
4. Get user alignment on the approach before writing a plan.

**Output:** Design spec in `docs/superpowers/specs/` (for non-trivial designs), or verbal alignment for simpler work.

**Skip when:** The task is well-defined with an obvious approach (e.g., "add field X to endpoint Y").

| Superpowers skill | When to use |
|---|---|
| `brainstorming` | Always for new features, architecture changes, or ambiguous requirements |

---

## Step 2 — Plan

Write a concrete execution plan with phases, files, tests, and verification steps.

1. Read `docs/architecture/decisions.md`. Ensure the plan respects all existing rules.
2. Read existing plans in `plan/` for reusable patterns and established conventions.
3. Read `TODO.md` for outstanding items relevant to this work.
4. Write a detailed plan with: phases, file lists, testing plan per phase (files, functions, expected coverage), and verification steps.
5. Self-review the plan: check for inconsistencies, missing files, stale references, blast radius, naming conflicts, edge cases.
6. Get user approval.
7. Save to `plan/` with the next sequential number (e.g., `plan/102-feature-name.md`). Reference the design spec if one exists.
8. **Commit the plan file before any implementation code.**

**Output:** Committed plan file in `plan/`.

| Superpowers skill | When to use |
|---|---|
| `writing-plans` | For structuring the plan with phases and verification steps |

---

## Step 3 — Build

Implement the plan phase by phase. Do not batch phases.

For **each phase**:

1. **Implement** — write the code for this phase only.
2. **Test** — write or update tests following the Testing guidelines. Run all relevant tests. Fix failures before proceeding.
3. **Self-review** — re-read all modified files. Compare against the plan and `docs/architecture/decisions.md`. Check for consistency, dead code, duplicate logic.
4. **Document** — update `docs/` and affected `README.md` files for this phase's changes.
5. **Commit** — only after tests pass and self-review is clean. Commit code, tests, and docs together.

| Superpowers skill | When to use |
|---|---|
| `executing-plans` | Single-agent plan execution, following each task step by step |
| `subagent-driven-development` | Multi-agent parallel execution when tasks are independent |
| `test-driven-development` | When implementing each unit of work (write test → implement → pass) |
| `systematic-debugging` | When a test fails or unexpected behavior is encountered |
| `using-git-worktrees` | When isolation from the current workspace is needed |

---

## Step 4 — Verify

After all phases are complete, verify the whole before moving on.

1. Run the **full** test suite (not just changed tests). All must pass.
2. Check cross-phase consistency: duplicated code, inconsistent patterns, missed edge cases.
3. Compare actual test coverage against the plan's testing plan. Add any missing tests.
4. If issues are found, fix them following the Build step's per-phase process.

| Superpowers skill | When to use |
|---|---|
| `verification-before-completion` | Before claiming work is done — runs verification and confirms output |

---

## Step 5 — Review

Code review catches what self-review misses.

1. Request code review. Reviewer appends findings to the plan file's `## Code Review` section, following `docs/code-review.md` format.
2. Each finding is numbered with `[OPEN]` status, file:line references, and Must Fix / Should Fix / Nice to Have priority.
3. Author addresses each finding: fix the code or explain why not. Respond inline with `→ Response:` and update status to `[FIXED]` or `[WONTFIX]`.
4. All `[OPEN]` items must be resolved before shipping.

| Superpowers skill | When to use |
|---|---|
| `requesting-code-review` | Dispatch the code-reviewer subagent with commit range and context |
| `receiving-code-review` | When acting on review feedback — ensures rigorous evaluation before implementing suggestions |

---

## Step 6 — Ship

Wrap up, push, and create the PR.

1. **Update the plan file** with a post-execution report: implementation details, deviations from plan, known limitations, follow-up work.
2. **Update `docs/architecture/decisions.md`** if new architectural decisions were made.
3. **Update `TODO.md`** — mark completed items, add new follow-up items.
4. **Commit** the updated plan and docs.
5. **Run ALL tests** one final time. Do not proceed if any test fails.
6. **Push** the branch and **create a PR** via `gh pr create`.

| Superpowers skill | When to use |
|---|---|
| `finishing-a-development-branch` | Presents structured options (merge / PR / keep / discard) and handles cleanup |

---

## Session Handoff Rules

Multiple Claude Code sessions share the same local repository sequentially. To prevent lost work:

1. **Commit everything before ending.** Every file change — code, plan files, docs, review findings — must be committed before the session ends. Use `WIP:` prefix for incomplete work. Uncommitted changes are invisible to the next session.
2. **Push the branch.** Don't leave unpushed commits. The next session may start on a different branch or after a `git pull`.
3. **Check `git status` on session start.** Look for untracked or modified files from a prior session. These are orphaned changes that need to be committed or discarded (ask the user).
4. **Don't trust conversation summaries.** Verify prior work by checking `git log`, reading plan files, and confirming post-execution reports exist. A summary may claim work was done that was never committed.

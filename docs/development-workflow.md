# Development Workflow

Follow this process for every plan. Do NOT skip steps or batch them.

For trivial changes (typo fixes, one-line patches), use your judgement — not every change needs the full workflow. But any multi-file feature, refactor, or integration must follow it.

---

## Step 1 — Design

Explore the problem space before committing to an approach.

1. Clarify the user's intent — what problem are we solving, what does success look like?
2. Research constraints: check relevant code, identify affected systems, review existing plans in `docs/plans/`.
3. Brainstorm approaches. Compare trade-offs. Identify risks.
4. Get user alignment on the approach before writing a plan.

**Output:** One of the following, depending on complexity:
- **Verbal alignment** — for simple, well-defined tasks. Proceed directly to Plan.
- **Straight to plan** — when brainstorming produces a concrete design (components, file lists, phases, testing strategy). No spec needed.
- **Design spec** in `docs/specs/` — only for large, novel, or cross-cutting designs that will be referenced by multiple plans or need a standalone reference document. Use numbered prefixes (e.g., `001-feature-name.md`).

**Skip when:** The task is well-defined with an obvious approach (e.g., "add field X to endpoint Y").

---

## Step 2 — Plan

Write a concrete execution plan with phases, files, tests, and verification steps.

1. Read existing plans in `docs/plans/` for reusable patterns and established conventions.
2. Read `TODO.md` for outstanding items relevant to this work.
3. Write a detailed plan with: phases, file lists, testing plan per phase (files, functions, expected coverage), and verification steps.
4. Self-review the plan: check for inconsistencies, missing files, stale references, blast radius, naming conflicts, edge cases.
5. Get user approval.
6. Save to `docs/plans/` with the next sequential number (e.g., `docs/plans/024-feature-name.md`). Reference the design spec if one exists.
7. **Commit the plan file before any implementation code.**

**Output:** Committed plan file in `docs/plans/`.

---

## Step 3 — Build

Implement the plan phase by phase on a **single plan branch** (`feat/NNN-description`). All phases share one branch and one PR — phase boundaries are commit-level subdivisions, not separate PRs.

For **each phase**:

1. **Implement** — write the code for this phase only. Stay on the plan branch.
2. **Test** — write or update tests following the Testing guidelines. Run all relevant tests. Fix failures before proceeding.
3. **Self-review** — re-read all modified files. Compare against the plan. Check for consistency, dead code, duplicate logic.
4. **Document** — update `docs/` and affected `README.md` files for this phase's changes.
5. **Commit** — only after tests pass and self-review is clean. Commit code, tests, and docs together with a clear `plan NNN phase M: …` message so the reviewer can trace logical units in the squashed diff.

Do NOT open a PR after each phase. After Step 4 (Verify), the 5-way code review (Step 5) runs against the consolidated branch diff. The PR is opened in Step 6, after all reviewers concur.

**Single-PR exception**: when a plan has a genuinely independent backend-infrastructure phase that other phases don't depend on (e.g., a schema migration that downstream UI doesn't need to coordinate with), it MAY ship as a separate PR. Document the deviation in the plan file's `## Phases` section. Default is one PR per plan.

When tasks within a phase are independent and can be implemented without shared state, dispatch them to subagents in parallel when the current coding environment supports it. When tasks have sequential dependencies, execute them in order in the current session.

When a test fails or behavior is unexpected, debug systematically: reproduce the failure deterministically, isolate the smallest case, form a hypothesis, verify by changing one thing at a time. Don't guess at fixes.

---

## Step 4 — Verify

After all phases are complete, verify the whole before moving on.

1. Run the **full** test suite (not just changed tests). All must pass.
2. Check cross-phase consistency: duplicated code, inconsistent patterns, missed edge cases.
3. Compare actual test coverage against the plan's testing plan. Add any missing tests.
4. If issues are found, fix them following the Build step's per-phase process.

Before claiming work is done, run the verification commands and confirm the actual output. Don't assert success without evidence.

---

## Step 5 — Review

Code review catches what self-review misses. The 5-way review fires **once per plan** against the consolidated branch diff, after Step 4 (Verify) and before the PR is opened. Findings go in the plan file's `## Code Review` section. Not after each phase.

1. Request code review (5-way: self on Opus, Codex, DeepSeek V4 Flash, GLM 5.1, Kimi K2.6 — see CLAUDE.md `## Code Review`). Reviewers append findings to the plan file's `## Code Review` section, following `docs/code-review.md` format.
2. Each finding is numbered with `[OPEN]` status, file:line references, and Must Fix / Should Fix / Nice to Have priority.
3. Author addresses each finding: fix the code or explain why not. Respond inline with `→ Response:` and update status to `[FIXED]` or `[WONTFIX]`.
4. All `[OPEN]` items must be resolved before shipping.

When acting on review feedback, evaluate each finding rigorously before implementing — don't blindly apply suggestions. If a finding is unclear or technically questionable, push back with reasoning rather than agreeing performatively.

---

## Step 6 — Ship

Wrap up, push, and create the **single plan PR**. All phases of the plan ship together (default), or per the documented single-PR-deviation if applicable.

1. **Update the plan file** with a post-execution report: implementation details per phase, deviations from plan, known limitations, follow-up work.
2. **Update `TODO.md`** — mark completed items, add new follow-up items.
3. **Commit** the updated plan and docs.
4. **Run ALL tests** one final time. Do not proceed if any test fails.
5. **Push** the branch and **create the PR** via `gh pr create`. PR title is `Plan NNN: <description>` (one PR per plan); body summarizes the phases shipped + cross-phase test plan + 5-way review summary.

---

## Session Handoff Rules

Multiple Claude Code and Codex sessions may share the same local repository sequentially. To prevent lost work:

1. **Commit everything before ending.** Every file change — code, plan files, docs, review findings — must be committed before the session ends. Use `WIP:` prefix for incomplete work. Uncommitted changes are invisible to the next session.
2. **Push the branch.** Don't leave unpushed commits. The next session may start on a different branch or after a `git pull`.
3. **Check `git status` on session start.** Look for untracked or modified files from a prior session. These are orphaned changes that need to be committed or discarded (ask the user).
4. **Don't trust conversation summaries.** Verify prior work by checking `git log`, reading plan files, and confirming post-execution reports exist. A summary may claim work was done that was never committed.

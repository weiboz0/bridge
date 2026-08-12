# Development Workflow

Follow this process for every plan. Do NOT skip steps or batch them.

For trivial changes — a typo, a one-line patch — use judgement.
Any multi-file feature, refactor, or integration follows the whole thing.

## Permanent review-gate contract

Every committed design spec under `docs/specs/**` passes a design-review gate before its implementation plan is drafted or revised from it.
The design gate has exactly two required reviewers: Codex Sol (`gpt-5.6-sol`, reasoning effort high) and Claude Code (`claude-fable-5`).
Design reviews are read-only and bind every verdict to the exact substantive commit.
Both reviewers receive read-only prompts and must approve the same exact substantive commit with no open blocker.
Record `[sol]` and `[fable]` findings and verdicts in the spec; a material revision invalidates both approvals.

The design gate, plan-review gate, and code-review gate are uncapped consensus loops with no numeric round cap.
Consensus requires every required reviewer to return `APPROVE` or `APPROVE WITH NITS` with no open blocker.
An unavailable required reviewer pauses the gate and is never silently substituted, replaced, or waived.
After every three consecutive non-converged substantive rounds, surface a checkpoint; repeated reopening or two checkpoints without net blocker reduction requires a genuine user-decision pause.

---

## Step 1 — Design

Explore the problem space before committing to an approach.

1. Clarify intent — what problem are we solving, what does success look like?
2. Research constraints: read `docs/architecture/decisions.md`, check the relevant code, identify affected systems, review existing plans in `docs/plans/`.
3. Brainstorm approaches. Compare trade-offs. Identify risks.
4. Align with the user on the approach before writing a plan.

**Output**, depending on complexity:

- **Verbal alignment** — simple, well-defined task. Go straight to Plan.
- **Straight to plan** — brainstorming produced a concrete design (components, file lists, phases, testing strategy). No spec needed.
- **Design spec** in `docs/specs/NNN-…` — only for large, novel, or cross-cutting designs referenced by multiple plans.

**Skip when** the task is well-defined with an obvious approach ("add field X to endpoint Y").

---

## Step 2 — Plan

Write a concrete execution plan with phases, files, tests, and verification steps.

1. Read `docs/architecture/decisions.md`. The plan must respect every existing rule or explicitly change one.
2. Read existing plans in `docs/plans/` for reusable patterns and established conventions.
3. Read `TODO.md` for outstanding items relevant to this work.
4. Write the plan: phases, file lists, per-phase testing plan, verification steps.
5. **Declare `## File scope`** — every file the plan may modify.
   This is load-bearing, not paperwork: it fixes the blast radius at gate time and is what the
   scope-drift safeguard compares against. A plan without it cannot be gated.
6. **Include a named integration-tests phase** — not folded into "build" or "verify", not implicit.
   Required when the plan touches the Go API surface, the realtime protocol, or cross-cutting
   plumbing (auth, persistence, org scoping). The phase specifies:
   - **Scenarios** — happy path, auth check, error cases, cross-org isolation.
   - **Fixtures** the tests will use.
   - **Live vs fast split** — which assertions need a live service vs pure wiring.
   - **Acceptance criteria** — exact test names that must exist and pass before the phase is done.

   **Exempt** (state it in `## Out of scope`): documentation-only plans, behavior-preserving
   refactors, plan-design plans. Silent omission is a blocker at the gate.

   Backfilling tests for an existing surface gets its own plan.
7. Self-review: inconsistencies, missing files, stale references, blast radius, naming conflicts, edge cases.
8. **Run the plan-review gate** for the plan's risk tier — `docs/reviewers.md`.
   A passing gate (every reviewer APPROVE / APPROVE WITH NITS, no open blockers) **is** user approval.
   No separate approval pause follows it; the user can redirect any time.
9. Save as `docs/plans/NNN-feature-name.md` with the next free number.
   Run `bash scripts/check-plan-uniqueness.sh` — parallel sessions collide, and the number that looks
   free on your branch may not be free on `main`.
10. **Commit the plan file before any implementation code.**

**Output:** committed plan file with `## File scope` and a named integration-tests phase (or an explicit exemption).

---

## Step 3 — Build

Implement phase by phase on a **single plan branch** (`feat/NNN-description`).
All phases share one branch and one PR — phase boundaries are commit-level subdivisions.

For **each phase**:

1. **Implement** this phase only. Dispatch by domain — `docs/coding-agent.md`.
2. **Test** — write or update tests per `docs/testing.md`. Run them. Fix failures before proceeding.
3. **Self-review** — re-read every modified file against the plan and `decisions.md`.
   Check consistency, dead code, duplicate logic.
4. **Document** — update `docs/` and affected `README.md` files for this phase.
5. **Commit** — only after tests pass and self-review is clean.
   Code, tests, and docs together, message `plan NNN phase M: …` so the reviewer can trace logical
   units inside the squashed diff.

Do NOT open a PR after each phase.

**Single-PR exception:** when a plan has a genuinely independent backend-infrastructure phase that
other phases don't depend on — a schema migration downstream UI needn't coordinate with — it MAY ship
separately. Document the deviation in the plan's `## Phases`. Default is one PR per plan.

Independent tasks within a phase can go to parallel subagents. Sequentially dependent ones don't.

When a test fails or behavior surprises you, debug systematically: reproduce deterministically,
isolate the smallest case, form a hypothesis, change one thing at a time.
Don't guess at fixes. For anything non-obvious, use the bug-investigation gate.

---

## Step 4 — Verify

After all phases, verify the whole.

1. Run `bash scripts/ci-local.sh` — the full suite, not just changed tests. All green.
2. **Confirm the integration-tests phase shipped what it promised.**
   Every test name listed in its acceptance criteria exists and passes.
   A missing one is a phase regression, not a follow-up.
3. Check cross-phase consistency: duplicated code, inconsistent patterns, missed edge cases.
4. Compare actual coverage against the plan's testing plan. Add what's missing.
5. Fix any issues using the Build step's per-phase process.

Run the verification commands and confirm the actual output before claiming done.
Evidence before assertions.

---

## Step 5 — Review

Code review catches what self-review misses.
Fires **once per plan** against the consolidated branch diff, after Verify and before the PR — not per phase.

1. Run the code-review gate for the plan's tier — `docs/reviewers.md`.
   Reviewers append findings to the plan's `## Code Review` section per `docs/code-review.md`.
2. Each finding: numbered, `[OPEN]`, file:line refs, Must Fix / Should Fix / Nice to Have, source tag.
3. Author addresses each — fix it or explain why not — inline with `→ Response:` and `[FIXED]` / `[WONTFIX]`.
4. All `[OPEN]` items resolve before shipping.

Evaluate each finding rigorously before implementing.
Don't blindly apply suggestions; if one is unclear or technically questionable, push back with
reasoning rather than agreeing performatively.
A reviewer can be wrong — but verify before concluding that, because they are often right in ways
that look wrong at first read.

---

## Step 6 — Ship

1. **Update the plan file** with a post-execution report: what was implemented per phase, deviations
   from plan, known limitations, follow-up work.
2. **Update `docs/architecture/decisions.md`** if the plan made a new cross-cutting decision.
3. **Update `TODO.md`** — mark completed items, add follow-ups.
4. **Commit** the updated plan and docs.
5. **Run `bash scripts/ci-local.sh`** one final time. Do not proceed if anything fails.
6. **Run `bash scripts/pre-merge-guard.sh`** to catch collisions parallel sessions introduced.
7. **Push** and **create the PR** via `gh pr create`, title `Plan NNN: <description>`.
   Body: phases shipped, cross-phase test plan, review summary.
8. **Confirm the gate attestation names this exact commit** — `.claude/ci-local-attestation.json`.
   Bridge has no cloud CI, so this is the merge evidence, and a run against an earlier commit does not count.
   Merge with `gh pr merge --squash`.
   Never `--admin` past a failing gate — that is a hard safeguard.

---

## Session handoff

Multiple agent sessions share this repository sequentially. To prevent lost work:

1. **Commit everything before ending.** Code, plan files, docs, review findings.
   `WIP:` prefix for incomplete work. Uncommitted changes are invisible to the next session.
2. **Push the branch.** Don't leave unpushed commits.
3. **Check `git status` on session start.** Untracked or modified files from a prior session are
   orphaned changes — surface them to the user before committing or discarding.
4. **Don't trust conversation summaries.** Verify via `git log`, the plan file, and its post-execution
   report. A summary may claim work that was never committed.

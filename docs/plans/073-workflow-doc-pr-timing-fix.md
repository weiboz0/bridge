# Plan 073 — Workflow doc PR-timing + 5-way drift fix

## Problem

Review 011 §11 caught two related issues in `docs/development-workflow.md`:

### Issue A — PR-timing contradiction

Three lines disagree on WHEN the plan PR is created:

- **Step 3, line 55** (build phase footer): "Open the PR once after Step 4 (Verify) when ALL phases are implemented; the 4-way code review (Step 5) runs against the consolidated branch diff." → PR opens BEFORE Step 5.
- **Step 5, line 80** (review header): "The 4-way review fires once per plan PR at PR-open time, against the consolidated branch diff (all phases). Not after each phase." → review fires at PR-open, which is consistent with Step 3 if PR opens before Step 5.
- **Step 6, line 99** (ship): "Push the branch and create the PR via `gh pr create`. PR title is `Plan NNN: ...`." → PR is created HERE, in Step 6, AFTER Step 5.

So Steps 3 and 5 say "PR opens before review", but Step 6 says "PR opens after review". Codex flagged this in review 011 and an author following the doc literally would either (a) open the PR twice or (b) skip the review entirely if they read Step 6 first.

### Issue B — 4-way → 5-way drift

PR #133 (merged 2026-05-04) added Kimi K2.6 as a fifth reviewer in `CLAUDE.md`. `docs/development-workflow.md` was missed in that update. Four locations still say "4-way" or list four reviewers:

- Line 55: "the 4-way code review (Step 5)"
- Line 80: "The 4-way review fires"
- Line 82: "(4-way: self on Opus, Codex, DeepSeek V4 Flash, GLM 5.1 ..."  ← also missing Kimi from the list
- Line 99: "4-way review summary"

## How the workflow actually runs (lived behavior across plans 067-072)

Reviewing what's actually been done across the last 6 plans:

1. Branch is created (`feat/NNN-...`), plan drafted + committed.
2. Plan review fires against the branch — reviewers fetch `docs/plans/NNN-...md` from the feature branch by name. Findings go in the plan file's `## Plan Review` section.
3. Implementation phase by phase, commits to the branch.
4. Step 4 verify: full test suite.
5. Step 5 code review: reviewers fetch the branch diff (`git diff main..HEAD` on the feature branch). Findings go in the plan file's `## Code Review` section. Iterate until all 5 concur. **No PR is open during this step.**
6. Step 6 ship: post-execution report, then `gh pr create` opens the PR with the consolidated review trail in the body.

The actual lived behavior is "branch-diff review THEN PR open", not "PR open THEN review". Step 6's text is correct; Step 3 and Step 5 use a "PR-open time" framing that's a misnomer for what actually happens.

## Approach

Single-phase doc cleanup. No code changes. No tests change. Update three sentences across two locations in `docs/development-workflow.md` to match reality + sweep "4-way" → "5-way".

**Resolution choice**: keep Step 6 as the PR-open step (matches lived behavior + the consolidated post-execution report flows naturally into the PR body). Update Steps 3 and 5 to say "review fires against the branch diff before the PR is opened; the PR is opened in Step 6 after all reviewers concur". This is the smaller change AND matches how every plan since 067 has actually shipped.

Alternative considered: flip the order so PR opens at the START of Step 5 and reviewers comment on the PR directly. Rejected because (a) findings already live in the plan file (single source of truth, audit trail survives squash-merge), (b) some reviewers (DeepSeek, GLM, Kimi) read by branch name not PR number, and (c) it would be a bigger behavioral change vs a doc-text fix.

## Decisions to lock in

1. **Lived behavior is the source of truth.** Doc updates to match what plans 067-072 actually did.
2. **PR opens in Step 6, after review converges.** Step 3 and Step 5 reframed to say "branch diff" instead of "PR".
3. **5-way everywhere.** Sweep `4-way` → `5-way` across the file. Add Kimi K2.6 to the reviewer list at line 82.
4. **Single-PR-per-plan policy unchanged.** This plan does not change phase/PR structure rules.

## Files

**Modify (1 file):**

- `docs/development-workflow.md`
  - Line 55: rewrite Step 3 footer paragraph: "Do NOT open a PR after each phase. After Step 4 (Verify), the 5-way code review (Step 5) runs against the consolidated branch diff. The PR is opened in Step 6, after all reviewers concur."
  - Line 80: rewrite Step 5 lead: "Code review catches what self-review misses. The 5-way review fires **once per plan** against the consolidated branch diff, after Step 4 (Verify) and before the PR is opened. Findings go in the plan file's `## Code Review` section. Not after each phase."
  - Line 82: rewrite reviewer list to "5-way: self on Opus, Codex, DeepSeek V4 Flash, GLM 5.1, Kimi K2.6 — see CLAUDE.md `## Code Review`".
  - Line 99: change "4-way review summary" → "5-way review summary".

**No code, test, or other doc changes.** `docs/code-review.md` was already audited (no `4-way` / `5-way` references; reviewer-agnostic format).

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Authors who already memorized the old workflow get confused by the change | low | The change matches lived behavior — anyone who has shipped a plan since 067 is already doing the new flow. The doc just catches up to reality. |
| Reviewers expecting a PR to comment on can't find one during Step 5 | very low | Reviewers fetch by branch name, not PR (verified across plans 067-072). All 5 external reviewers operate on branch diff. |
| `4-way` references elsewhere in the repo I missed | low | Single grep sweep before commit (planned for the implementation phase). |
| Future workflow refactor (e.g., PR-first review) gets blocked by this fix | very low | This plan documents lived behavior, not a new direction. A future plan can revisit the order if desired. |

## Phases

### Phase 1 — sweep + commit (single phase)

- `grep -n "4-way\|5-way" docs/` to confirm scope (expect: 4 hits in `docs/development-workflow.md`, none in `docs/code-review.md`).
- `grep -rn "4-way" .` once before the edit to catch any stragglers I missed.
- Apply the four edits above.
- Verify `grep -n "4-way" docs/development-workflow.md` returns 0 hits.
- Verify `grep -n "5-way" docs/development-workflow.md` returns 4 hits matching the new text.
- Verify the reviewer list at the new line 82 includes all 5 reviewers in the same order as `CLAUDE.md` `## Code Review`.
- Commit: `plan 073: workflow doc — match PR-timing to lived behavior + 5-way drift`.

After Phase 1, run the 5-way code review against the consolidated branch diff (single-PR-per-plan policy), fold findings, open the PR via Step 6.

## Plan Review

(pending — 5-way before implementation)

## Code Review

(pending — 5-way at branch-diff time per the new policy this plan codifies)

## Post-execution report

(pending)

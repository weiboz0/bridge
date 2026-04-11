# Project Instructions

## Coding Conventions

- Follow existing patterns in the codebase — check how similar features are implemented before writing new code.
- Never hardcode secrets or credentials. Secrets go in `.env`, non-secret config in config files.
- When modifying any logic, proactively search the codebase for similar patterns that should receive the same change. Do not wait to be asked — audit related handlers, components, and utilities for consistency.

## Testing

- When code is added or modified, write or update test cases covering the changes.
- Run the relevant tests to verify they pass before committing.
- Test both happy paths and error/edge cases (invalid input, missing data, unauthorized access).
- Aim for maximum test coverage: every public function, every branch, every error path. If a function has 3 code paths, write at least 3 tests. Do not skip edge cases or treat them as "obvious".

## Git

- Do not push to remote unless explicitly asked.
- Write clear, descriptive commit messages — lead with what changed and why, not how.
- Do not commit `.env`, credentials, or large binary files.
- Never merge directly to `main` with `git merge`. Always push the feature branch and create a pull request via `gh pr create`.
- Before merging any PR, check CI status with `gh pr checks <number>`. If any checks fail, fix the errors and push before merging. Never merge a PR with failing checks.
- Do not commit every small change individually. Batch related small fixes into a single meaningful commit. Only commit when a logical unit of work is complete.

## Plans

For substantial code changes — new features, re-architecting, multi-file refactors, new integrations, etc. — always enter plan mode first and write a detailed plan before any implementation. Get user approval on the plan before proceeding.

Before writing a new plan, review existing plans to ensure consistency. Avoid introducing duplicate code — reuse existing implementations and keep logic in a single source of truth.

## Development Workflow

Follow `docs/development-workflow.md` exactly for every plan (Steps 1 through 6). Do not skip steps or batch them. Key points:
- **Design before plan** — explore the problem, brainstorm approaches, get alignment
- **Plan before code** — write and commit plan file before any implementation
- **Phase by phase** — implement, test, review, document, commit each phase separately
- **Verify after all phases** — full test suite, cross-phase consistency check
- **Review** — code review findings go in plan file's `## Code Review` section
- **Ship** — update plan with post-execution report, update TODO.md, push and create PR

## Code Review

Follow `docs/code-review.md` for the review process. Key points:
- Reviews go in the plan file's `## Code Review` section
- Reviewers: append findings with `[OPEN]` status and file:line references
- Authors: respond inline with `→ Response:` and `[FIXED]`/`[WONTFIX]`

## Multi-Agent Coordination

Multiple Claude Code sessions share the same local repository. Only one agent should work at a time. To avoid lost work:

- **Always commit before ending a session.** Even partial work — use a `WIP:` prefix. Uncommitted changes are invisible to the next session and will be lost.
- **Check `git status` at session start.** Look for untracked or modified files left by a previous session. Ask the user before discarding them.
- **Each plan uses a feature branch.** Pull before starting work, push before ending. Never leave unpushed commits.
- **Never assume prior session completed its work.** Verify by reading the plan file's post-execution report and checking git log — don't trust claims in conversation summaries alone.

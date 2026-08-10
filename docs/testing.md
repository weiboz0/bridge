# Testing

Tier descriptions, exact commands, and gating env vars.
`docs/development-workflow.md` names *when* to verify; this doc owns *how*.

## Tiers

| Tier | Command | Needs |
|------|---------|-------|
| Lint | `bun run lint` | — |
| Type-check | `bunx tsc --noEmit` | — |
| Vitest (unit + integration) | `bun run test` | PostgreSQL, `bridge_test` DB |
| Go | `cd platform && go test ./... -count=1 -timeout 120s` | `TEST_DATABASE_URL` for store tests |
| E2E (Playwright) | `bun run test:e2e` | **pinned `E2E_BASE_URL`** + all three services |
| Live LLM | subset of `bun run test` | provider API keys — **bills real money** |
| Everything | `bash scripts/ci-local.sh` | the above, orchestrated safely |

`scripts/ci-local.sh` is the authoritative local gate.
Any future CI must run the same script, so the two cannot drift.

## The live-LLM hazard

`tests/llm/providers.test.ts` and `tests/llm/guardrails.test.ts` call **real** Anthropic, OpenAI,
Gemini, and DashScope endpoints.
They are gated only by `describe.skipIf(!process.env.<PROVIDER>_API_KEY)`,
and they sit inside vitest's default `include: ["tests/**/*.test.ts"]`.

**Bun auto-loads `.env`, which carries real provider keys.**
So `unset ANTHROPIC_API_KEY` does *not* disable them — the key is re-read from `.env`:

```
$ unset ANTHROPIC_API_KEY
$ bun --eval 'console.log(process.env.ANTHROPIC_API_KEY?.slice(0,12))'
sk-ant-api03            # still present
```

To run the suite without billing anything:

```
bun run --env-file=/dev/null test
```

`ci-local.sh` explicitly exports all five provider keys as empty values for Vitest:
`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`, `DASHSCOPE_API_KEY`,
`OPENROUTER_API_KEY`.
The empty values take precedence over values Bun auto-loads from `.env` and make the corresponding
`skipIf` checks fire.
Any future CI uses the same five-key mechanism.

`platform/internal/llm/` (Go) is separately mock-only — no live tier, no `CI` gating.

## The E2E hazard

`e2e/playwright.config.ts` declares **no `webServer`** and defaults to:

```ts
baseURL: process.env.E2E_BASE_URL || "http://localhost:3003"
```

On the primary dev machine, port 3003 is a **different service** — Bridge's own stack runs on
`NEXTJS_PORT` / `PLATFORM_PORT` from `.env`.
And `e2e/seed.setup.ts` is not read-only: it signs in as `eve@demo.edu`,
creates a fixture class via `POST /api/classes`, and enrolls students.

So an unpinned E2E run points **mutating** setup logic at whatever happens to be listening on 3003.

**Always export `E2E_BASE_URL` explicitly.** `ci-local.sh` fails closed if it is unset rather than
falling back to the default. Never put E2E in an automated gate without it.

## Database

`scripts/ci-local.sh` first validates any inherited `DATABASE_URL` independently.
It then resolves one nonempty `GATE_DATABASE_URL` from `TEST_DATABASE_URL`, `DATABASE_URL`, or the
`bridge_test` fallback and validates it again.
The Node 18 validator parses only PostgreSQL URLs and requires `_test`.
Its one supported encoded database-name form is a trailing `%5Ftest` or `%5ftest`, which it canonicalizes
to `_test` before live validation and runner pinning.
Its live probe makes one bounded `SELECT current_database()` call and requires the connected database name
to end in `_test` too.
The gate pins that validated URL as both `DATABASE_URL` and `TEST_DATABASE_URL` for Vitest, Go, and E2E.
Parser-only mode exists exclusively for `scripts/tests/test-guards.sh` selftests and never gates a run.

**Migrations read `DATABASE_URL`, not `TEST_DATABASE_URL`** — `drizzle.config.ts` has one URL and no
test-only path, so an inherited `DATABASE_URL` silently targets production.
Migrating a real database is a hard safeguard pause (`AGENTS.md`).

## What every change owes

- Happy path, error paths, edge cases. Every branch, every error path.
- **Every new Go API endpoint:** an integration test covering happy path, auth check, error cases,
  and cross-user isolation. No exceptions.
- **Every feature plan:** a named integration-tests phase, specifying scenarios covered, fixtures used,
  the live-vs-fast split, and the exact test names that must exist and pass before the phase is done.
  Reviewers reject plans that omit it — see `docs/reviewers.md`.

Backfilling tests for an existing surface gets its own plan, not a section buried in a feature plan.
Burying it makes the feature PR unreviewable and gives the test work the feature's review schedule.

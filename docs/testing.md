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

To run the suite without billing anything, blank all five provider keys explicitly:

```
ANTHROPIC_API_KEY= OPENAI_API_KEY= GEMINI_API_KEY= DASHSCOPE_API_KEY= OPENROUTER_API_KEY= bun run test
```

`ci-local.sh` explicitly exports all five provider keys as empty values for Vitest and E2E:
`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`, `DASHSCOPE_API_KEY`,
`OPENROUTER_API_KEY`.
The empty values take precedence over values Bun auto-loads from `.env` and make the corresponding
`skipIf` checks fire.
Any future CI uses the same five-key mechanism.

`platform/internal/llm/` (Go) is separately mock-only — no live tier, no `CI` gating.

## The E2E hazard

`e2e/playwright.config.ts` declares **no `webServer`** and rejects configuration evaluation unless it has:

```ts
process.env.E2E_BASE_URL
```

On the primary dev machine, port 3003 is a **different service** — Bridge's own stack runs on
`NEXTJS_PORT` / `PLATFORM_PORT` from `.env`.
And `e2e/seed.setup.ts` is not read-only: it signs in as `eve@demo.edu`,
creates a fixture class via `POST /api/classes`, and enrolls students.

This avoids an unpinned E2E run directing **mutating** setup logic at whatever happens to be listening on
port 3003.

`e2e/playwright.config.ts` loads the repository `.env` through `dotenv`, so a persistent
`E2E_BASE_URL` and the E2E-only control-failure flag work when Playwright runs under Node.
An explicit shell value still wins.
For a full gate, `ci-local.sh` safely reads only `E2E_BASE_URL` through the same dotenv parser (it never
shell-sources `.env` or prints its contents), then fails closed if neither source supplies it.

After the destructive Vitest and Go suites and immediately before a full pinned E2E run, `ci-local.sh`
reapplies `scripts/seed_problem_demo.sql` with `psql -v ON_ERROR_STOP=1` using only the already parsed
and live-validated `GATE_DATABASE_URL`.
The seed is skipped for `--fast`, an absent E2E URL, a rejected database target, and a failed restore;
a failed restore blocks Playwright.
This recovery is limited to the gate's `_test` target and restores the documented demo login identities,
course, and memberships that `e2e/auth.setup.ts` requires before its own fixture class/enrollment setup.

### The stack must prove it is on the test database

Validating the gate's own URL says nothing about the stack Playwright drives: pinning `DATABASE_URL` for
the Playwright process does not reconfigure a running service, and the seed setup creates classes, enrolls
users, and ends sessions.
So before the seed restore, `ci-local.sh` runs `scripts/check-e2e-stack.mjs`, which:

1. opens one reserved connection through the validated `GATE_DATABASE_URL`, begins a transaction, and takes a
   one-key `pg_advisory_xact_lock` whose key carries a reserved Bridge lock class and a fresh random nonce;
2. asks each service — five times, on fresh connections — for an attestation, and accepts it only if that
   service saw the lock **through its own pool** and returned `sha256(nonce ‖ 0 ‖ current_database())`;
3. rolls back. A transaction-scoped lock is released by rollback or disconnect, so it cannot be stranded.

Each service is asked on the origin E2E traffic really uses: Next.js at `/api/e2e-stack`, the Go API at
`/api/health/e2e-stack` *through* the Next.js proxy, and Hocuspocus at `/e2e-stack` on the websocket origin
that the attested Next.js reports from its build-time `NEXT_PUBLIC_HOCUSPOCUS_URL`.
A cloned database, a standby, or another cluster with its own `bridge_test` cannot show the gate's lock.

To run a full gate, start the stack yourself like this:

- `BRIDGE_E2E_STACK=1` on **all three** services; without it the endpoints do not exist.
- The same `_test` database the gate uses, on all three.
- `NEXT_PUBLIC_HOCUSPOCUS_URL` set to the real `ws://` or `wss://` URL when Next.js is built or started.
  Unset, browsers fall back to port 4000, which on the primary dev machine is a foreign service.
- Exactly **one process per service**, reached without load balancing, with live reload **off**
  (no `air`, no file-watch restarts). Every attestation carries a per-process instance id; a changing id
  fails the gate as *multiple instances*. A single-upstream reverse proxy is fine.
- On a host declared `BRIDGE_HOST_EXPOSURE=exposed`, also `ALLOW_E2E_STACK_OVER_TUNNEL=true`.

A refusal is deliberately indistinguishable from the endpoint not existing (Go and Next.js answer their
ordinary 404; Hocuspocus its ordinary default response), so the gate prints a failure class and a
remediation line per service — *not attested*, *mismatch*, *multiple instances*, *no realtime origin*,
*unchecked*, *lock lost*, *database timeout*, *unreachable*, *redirect*, *timeout*.
A service started **without** the flag has no endpoint at all and logs nothing, so a silent service most
likely lacks the flag; a service started with it logs the refusal *category* (database name or lock) at
most once per ten seconds, never the URL, the nonce, or the database name itself.
`e2e/seed.setup.ts` repeats the check with a fresh nonce before its first write, and compares instance ids
with the gate's. Load-balanced stacks and restarts during a run are unsupported, not merely discouraged.
The protocol is pinned in `scripts/tests/e2e-stack-vector.json` and asserted by Go, Bun, Vitest, and the gate
selftests.

## Session whiteboard tiers

The whiteboard contract is split by what each tier can prove.

- **Go** (`platform/internal/{handlers,store,realtime}`) owns authorization and lifecycle:
  the mint matrix (`TestMintToken_Canvas_*`), creator and settings authorization with no administrator
  or impersonator bypass, the status-first end with confirmed and degraded persistence, implicit
  replacement ends, and the advisory-lock ordering proofs. These need the `_test` database.
- **Bun** (`server/canvas-lifecycle.test.ts`, `server/hocuspocus.canvas.test.ts`) owns the realtime
  process: the freeze fence, the eight-admission cap, the 500 ms authorization deadline, size and ledger
  bounds, JWT-expiry closes for readers and writers, and the control listener. No database or network.
  These assume the supported topology of exactly one Hocuspocus process.
- **Vitest** (`tests/unit/whiteboard-*.test.tsx`, `tests/unit/excalidraw-yjs.test.ts`,
  `src/lib/whiteboard/**`) owns the client: echo suppression, local-only `appState`, the host floor
  control, loosening confirmation, image and file rejection, and the replacement and degraded-archive
  warnings.
- **Playwright** (`e2e/session-whiteboard.spec.ts`) owns the live path across all three services.
  It needs the control secret provisioned on both server processes; the degraded-archive assertion
  additionally needs the stack started with `BRIDGE_E2E_CANVAS_CONTROL_FAILURE=1` and is skipped otherwise.

## Database

`scripts/ci-local.sh` first validates any inherited `DATABASE_URL` independently.
It then resolves one nonempty `GATE_DATABASE_URL` from `TEST_DATABASE_URL`, `DATABASE_URL`, or the
`bridge_test` fallback and validates it again.
The Node 18 validator parses only PostgreSQL URLs and requires `_test`.
Its one supported encoded database-name form is a trailing `%5Ftest` or `%5ftest`, which it canonicalizes
to `_test` before live validation and runner pinning.
Its live probe makes one bounded `SELECT current_database()` call and requires the connected database name
to end in `_test` too.
Before that probe, the parser rejects every libpq routing override in the URL query—`host`, `hostaddr`,
`port`, `dbname`/`database`, `user`, `password`, `service`, `servicefile`, `target_session_attrs`, and
`load_balance_hosts`—case-insensitively after URL decoding.
This keeps the Node validator and the later `psql` seed consumer bound to the same pathname/host target;
ordinary application and SSL options remain allowed.
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

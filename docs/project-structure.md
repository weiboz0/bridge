# Project Structure

## Directory Map

| Path | What it is |
|------|------------|
| `src/` | Next.js 16 App Router frontend + legacy TypeScript API routes |
| `platform/` | Go backend (API server, LLM/agent/sandbox layers, stores) |
| `server/hocuspocus.ts` | Yjs collaboration server |
| `e2e/` | Playwright E2E tests |
| `tests/` | Vitest unit + integration tests |
| `drizzle/` | Drizzle migration files |
| `docs/` | Documentation |
| `docs/plans/` | Executed implementation plans (numbered `NNN-feature-name.md`; sub-docs use letter suffixes `021a`, `021b`) |
| `docs/specs/` | Design specs for large or novel features |

## Service Ports

| Service | Port | Notes |
|---------|------|-------|
| Next.js | 3003 | Frontend; proxies Go routes via `next.config.ts` rewrites (`GO_PROXY_ROUTES`) |
| Go platform | 8002 | API server |
| Hocuspocus | 4000 | Yjs collaboration |

All three services must be running for E2E tests.

## Running the Services

| Service | Command |
|---------|---------|
| Next.js dev | `PORT=3003 bun run dev` |
| Go platform (hot-reload) | `cd platform && air` |
| Go platform (manual) | `cd platform && go run ./cmd/api/` |
| Hocuspocus | `bun run hocuspocus` |
| DB studio | `bun run db:studio` |

Full dev setup (PostgreSQL, env vars, auth, migrations) lives in `docs/setup.md`.

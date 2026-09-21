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

| Service | Code default | Override env var | Notes |
|---------|------|------|-------|
| Next.js | 3003 | `NEXTJS_PORT` | Frontend; proxies Go routes via `next.config.ts` rewrites (`GO_PROXY_ROUTES`) |
| Go platform | 8002 | `PLATFORM_PORT` | API server |
| Hocuspocus | 4000 | `HOCUSPOCUS_PORT` | Yjs collaboration |
| Hocuspocus control | 4001 | `HOCUSPOCUS_CONTROL_PORT` | Server-only canvas freeze listener; Go derives `http://127.0.0.1:4001` by default |

> **The code defaults are not what any given machine runs — read `.env`, don't assume.**
> On the primary dev machine the stack is relocated (`NEXTJS_PORT=3101`, `PLATFORM_PORT=8100`,
> fronted by nginx on 3100) because **other, unrelated services occupy 3003 and 8002 there**.
> Never kill a process on those ports assuming it's a stale Bridge instance.
>
> This matters most for E2E: its seed fixture *creates classes and enrolls users*, so an unpinned run
> would aim mutating setup logic at whatever is listening on the default port.
> `e2e/playwright.config.ts` therefore refuses to evaluate without an `E2E_BASE_URL` from the shell or
> `.env`. See `docs/testing.md`.

All three services must be running for E2E tests.

Ports are configurable via `.env` (see the override env vars above). On
localhost the port vars are self-contained: `GO_API_URL` / `GO_INTERNAL_API_URL`
derive from `PLATFORM_PORT` automatically. The only port-paired var you must set by
hand for local dev is `NEXTAUTH_URL` (it carries the full host:port and feeds
the OAuth redirect). For non-localhost setups (staging, tunnel, separate host)
set the explicit URL vars: `GO_API_URL` / `GO_INTERNAL_API_URL` (Go host) and
`NEXT_PUBLIC_HOCUSPOCUS_URL` (browser-reachable Hocuspocus host).

The canvas lifecycle listener is separate from the WebSocket port.
`HOCUSPOCUS_CONTROL_SECRET` is required and must differ from `HOCUSPOCUS_TOKEN_SECRET`.
The Go API uses the numeric-loopback URL derived from `HOCUSPOCUS_CONTROL_PORT` unless `HOCUSPOCUS_INTERNAL_URL` overrides it.
An override may use HTTP only on a canonical numeric IPv4 or IPv6 loopback address; non-loopback deployments require normally verified HTTPS.
This control URL and bearer are server-only and must never be exposed through Next.js or a browser configuration variable.
Hocuspocus is a single-instance service: run one process per deployment.

## Running the Services

| Service | Command |
|---------|---------|
| **All three at once** | `bun run dev:all` (parallel, prefixed output; Go via `go run`) |
| Next.js dev | `bun run dev` (port from `NEXTJS_PORT`, default 3003) |
| Go platform (manual) | `bun run dev:go` — or `cd platform && go run ./cmd/api/` |
| Go platform (hot-reload) | `cd platform && air` (or `make dev`) |
| Hocuspocus | `bun run hocuspocus` |
| DB studio | `bun run db:studio` |

`dev:all` is the quickest start; it runs all three services in one terminal
with Foreman-style prefixed logs and shuts them all down on Ctrl+C. It uses
`go run` for the Go API (no hot-reload) — run `air` / `make dev` in a separate
terminal when you want Go to rebuild on save.

Full dev setup (PostgreSQL, env vars, auth, migrations) lives in `docs/setup.md`.

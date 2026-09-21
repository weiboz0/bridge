# Architecture decisions

**Single source of truth for Bridge's cross-cutting rules.**
Read the relevant section before changing auth, tenancy, persistence, error shaping, realtime, or the
Go/Next.js boundary. Update this file when a plan introduces a new cross-cutting decision, in the
same commit as the code.

Entries are numbered and stable — cite them as `decisions.md §N`.
Numbers are never reused; a superseded entry is marked and kept.

---

## §1 — Runtime is bun

`bun install`, `bun run`, `bun run test`. Vitest is the unit/integration runner, Playwright the E2E runner.

**Consequence that has bitten us:** bun auto-loads `.env`. Env vars cannot be suppressed by `unset`
alone — see `docs/testing.md` on the live-LLM hazard.

## §2 — Two backends, one boundary

Next.js (`src/`) owns rendering, session/auth, and the legacy TypeScript API routes.
Go (`platform/`) owns the current API surface.

The boundary is the **`GO_PROXY_ROUTES` list in `next.config.ts`**, applied as rewrites.
A route on that list is served by Go; anything else stays in Next.

Adding a Go endpoint means adding its prefix there, or it is unreachable from the browser.

**`/api/internal/*` must never appear in `GO_PROXY_ROUTES`.**
It is the server-to-server surface (e.g. the `bridge.session` mint endpoint) reached by Next's Edge
middleware with a `BRIDGE_INTERNAL_SECRET` bearer.
Proxying it would put a bearer-protected internal surface on public browser traffic.
Server-side callers use `GO_API_URL` directly.

## §3 — Auth is NextAuth, verified in Go

`src/lib/auth.ts` configures two providers: Google OAuth and email/password credentials.
Next.js issues the session; Go verifies it in `platform/internal/middleware`.

Auth and tenancy changes are a **hard safeguard** (`AGENTS.md`) — a permissive change here leaks
cross-org student data while every test still passes.

## §4 — Org-scoped tenancy

Users, classes, courses, and content belong to an organization.
Every query that reads user-owned data scopes by org.
Cross-org isolation is a required test case for every new endpoint, not an optional one.

## §5 — Persistence is PostgreSQL via drizzle

Migrations live in `drizzle/` as `NNNN_name.sql`, generated with `bun run db:generate`.

**`drizzle.config.ts` reads `process.env.DATABASE_URL` and nothing else.**
There is no separate test-only migration path, so an inherited `DATABASE_URL` silently targets
production. Bridge has **no down-migrations**.

Migrating a non-test database is a hard safeguard pause, enforced by a `_test` suffix check in
`scripts/ci-local.sh` rather than left to discipline.

Go code uses `database/sql` via pgx with parameterized queries only.
Stores accept `*db.DB`; handlers validate path IDs through `ValidateUUIDParam`.

## §6 — Realtime is Yjs over Hocuspocus

`server/hocuspocus.ts` runs the collaboration server; documents sync via Yjs.
Access is gated by signed tokens minted server-side — treated as auth surface under §3.

## §7 — LLM access goes through the backend factory

`platform/internal/llm/` exposes a backend interface with Anthropic, OpenAI, Gemini, and Ollama
implementations behind `factory.go`, plus an agent loop, a tool registry
(`platform/internal/tools/`), and skills (`platform/internal/skills/`).

Application code depends on the interface, never a concrete provider.

Go-side LLM tests are mock-only today.
The TypeScript live-provider tests in `tests/llm/` are the only tier that hits real endpoints, and
they bill real money — see `docs/testing.md`. Plan 092 addresses the gating.

## §8 — Errors return, they don't panic

Go: return `(result, error)`; log with `slog`; timestamps as `time.RFC3339`.
Handlers shape errors into consistent JSON responses rather than leaking internals.

Production-quality means covering failure, partial input, upstream down, concurrent access, and empty
collections. Don't strip defensive logic in the name of YAGNI.

## §9 — Ports are configurable, and the defaults are not what this machine runs

`NEXTJS_PORT`, `PLATFORM_PORT`, `HOCUSPOCUS_PORT` in `.env`.
`GO_API_URL` / `GO_INTERNAL_API_URL` derive from `PLATFORM_PORT` on localhost.
`NEXTAUTH_URL` carries the full host:port and must be set by hand.

The documented defaults (3003 / 8002) are **not** what the primary dev machine runs — other services
occupy those ports there. Never assume; read `.env`.
This is why E2E requires a pinned `E2E_BASE_URL` (`docs/testing.md`).

## §10 — Session whiteboards are persisted, visibility-floored realtime documents

Each whiteboard has a durable owner and a `canvas:{uuid}` Yjs document.
Canvas realtime minting also carries a canonical session-ID hint.
The hint grants no access: Go acquires that session's shared lifecycle lock first, then authorizes only the exact canvas/session pair and signs the authoritative binding into a canvas-only JWT claim used by every Hocuspocus recheck.
Canvas tokens missing the binding fail closed; non-canvas JWTs omit it.
Canvas visibility is ordered `private < host < participants < session`; PostgreSQL enum ordering is part of the persistence contract.
The enum is append-only: adding a level between existing levels would change `<` comparisons, so a new level belongs at an end or the comparison must move to explicit ranks.

The session host controls a minimum floor up to `participants`; owners may only loosen, never tighten, a canvas visibility.
All floor and visibility writes lock the session row so no concurrent mutation can persist a canvas below its floor.

Live `session` visibility follows the live session's access rule, including authenticated outsiders for public class-less sessions.
After end, no public admission remains: the archive permits the owner, the teacher at `host` or wider, and `present`/`left` former participants at `participants` or wider.
All archive tokens are read-only.
The Hocuspocus connection receives that signed `readOnly` claim and rechecks mutation-bearing canvas frames before Yjs applies or relays them, so a token minted before the end transition cannot write afterward.

The neutral `/sessions/{id}/whiteboards` archive is deliberately client-read-only even while the session remains live: it suppresses local binding writes and mutation controls, while the Go mint and Hocuspocus checks remain authoritative.
The accepted MVP limitations are that an owner cannot tighten an accidental share, a departed live viewer can retain a read token until its short TTL, and the host has no per-canvas takedown control.
The custom binding writes the whole scene last-writer-wins, which is sound because a canvas has exactly one writer; the same owner drawing in two tabs at once will overwrite rather than merge.
That retention is bounded by the token, not by the socket: Hocuspocus closes every established canvas connection, writable or read-only, when its JWT expires, so a reader must re-mint and pass current authorization to continue.

Canvas authorization has no independent platform-administrator or impersonator bypass, unlike other realtime document types.
An administrator or impersonator receives exactly the represented user's canvas access, for creation, the settings routes, minting, and every recheck.
A private student canvas is therefore not an oversight surface; the host's supervision lever is the floor.
Only the session teacher or a currently `present` participant may create a canvas, so an authenticated outsider admitted to a public class-less session can read `session`-visibility boards but cannot spend the per-session cap.

Whiteboards do not persist Excalidraw binary files.
Image insertion, image paste, and file drop are disabled in the client with a visible explanation, and only `viewBackgroundColor` is shared from `appState`; viewport, zoom, selection, and tool state stay local to each viewer.

## §11 — Canvas lifecycle control uses a separate bearer and private transport

The Go API ends a session status-first: it durably leases and lists canvases, requests a same-token Hocuspocus freeze, then records either an atomic confirmed snapshot/end or a separate degraded false/no-snapshot end.
The database result is authoritative; terminal complete/unfreeze calls are best effort and events or schedule completion occur only after a durable end.
PostgreSQL session status outranks Hocuspocus availability: an unreachable, slow, rejecting, or malformed freeze never keeps a session live, it only downgrades the end to degraded.
A confirmed end guarantees that the archive holds the final state present in the responding Hocuspocus process under an active freeze fence.
A degraded end guarantees only that no new write authorization is granted; already-authorized in-flight frames may still fan out to connected peers without reaching the archive, and the teacher is told the latest changes may not have been archived.
Starting a session that implicitly replaces a live one ends the replaced session through the same lifecycle, and the create and scheduled-start responses always carry `replacedSessions` with each durable `whiteboardServerArchiveComplete` flag so the teacher sees a replacement warning without depending on realtime.
No TypeScript code writes session status; the Go lifecycle path is the only producer.

Mutation authorization is never cached in Hocuspocus, because session status must stay authoritative for every accepted frame.
The cost is bounded instead of cached: client writes coalesce on a 100 ms trailing debounce and identical scenes are skipped, each canvas document admits at most eight active-plus-queued mutations (the ninth closes retryably), each authorization recheck has a 500 ms abortable deadline, and a canvas update above 1 MiB is rejected after parsing while the shared 100 MiB websocket ceiling keeps other document types compatible.
Persisted updates and current state are capped at 4 MiB, a snapshot at 8 MiB, a freeze bundle at 50 snapshots, 32 MiB decoded, and a 48 MiB response, under process-wide 128 MiB resident-document and 256 MiB capture ledgers.

Bridge supports exactly one Hocuspocus process.
The freeze fence, admission counters, and memory ledgers are in-process state, so a second realtime process would accept canvas mutations outside the fence and void the confirmed-end guarantee.
Scaling the realtime tier horizontally requires a new decision, not a configuration change.

`HOCUSPOCUS_CONTROL_SECRET` is distinct from `HOCUSPOCUS_TOKEN_SECRET` and is required on both server processes.
The Go API derives `http://127.0.0.1:4001` from `HOCUSPOCUS_CONTROL_PORT` by default.
Plain HTTP is limited to canonical numeric 127/8 or `::1` loopback; IPv4-mapped IPv6 and DNS names (including `localhost`) are rejected. Every non-loopback control endpoint requires verified HTTPS and redirects are refused.

---

## Adding an entry

Append with the next free number, state the decision and the rationale, and note any consequence that
is non-obvious from the code. If a plan changes an existing decision, edit that entry and note what
superseded it — don't silently rewrite history.

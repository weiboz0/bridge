# Development Setup

## PostgreSQL Configuration

Bridge requires PostgreSQL 15+ with two databases:

- `bridge` — development database
- `bridge_test` — test database (cleaned between test runs)

### Creating the databases

```bash
# As a PostgreSQL superuser
createdb bridge
createdb bridge_test
```

### Authentication

The development setup uses `trust` authentication for the `work` PostgreSQL user. Add these lines to the **top** of your `pg_hba.conf`:

```
local   all   work   trust
host    all   work   127.0.0.1/32   trust
host    all   work   ::1/128        trust
```

Then reload PostgreSQL:

```bash
sudo systemctl reload postgresql
```

> **Note:** Use `127.0.0.1` (not `localhost`) in connection strings to avoid IPv6 resolution issues where `localhost` may resolve to `::1`.

### Running Migrations

```bash
# Generate migration from schema changes
bun run db:generate

# Apply migrations to dev database
bun run db:migrate

# Apply migrations to test database
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bun run db:migrate
```

### Drizzle Studio

To browse the database visually:

```bash
bun run db:studio
```

### Seeding sample content

```bash
# 1. Bootstrap the Bridge HQ org + system user (one-time, idempotent).
psql postgresql://work@127.0.0.1:5432/bridge -f scripts/seed_bridge_hq.sql

# 2. Bring up Piston for solution verification (one-time).
docker run -d --rm --privileged -v /piston \
  --name piston -p 2000:2000 ghcr.io/engineer-man/piston
curl -X POST http://localhost:2000/api/v2/packages \
  -H 'Content-Type: application/json' \
  -d '{"language":"python","version":"3.10.0"}'

# 3. Import Python 101 + clone the curriculum into Bridge Demo School
#    (validates YAML, runs all reference solutions against Piston,
#    writes to the DB in one transaction, then clones the course tree
#    into the demo org so eve@demo.edu owns her copy and can edit).
bun run content:python-101:import --apply --wire-demo-class
```

The `--wire-demo-class` step is idempotent: re-runs detect the
existing clone (well-known UUID `00000000-0000-0000-0000-de7000000001`)
and just re-point the demo class at it. The cloned units are
`scope='org'`, `scope_id=Bridge Demo School`, owned by eve, so
`canEditUnit` in `platform/internal/handlers/teaching_units.go` lets
eve edit them. Bridge HQ's canonical platform-scope library stays
untouched as the publishing source.

To skip the Piston pre-flight (e.g., on a host without Docker), pass
`--skip-sandbox` to step 3. This is fine for trying out the workflow,
but reference solutions won't be verified against CPython.

To skip the demo wire-up (e.g., on a fresh dev DB without
`scripts/seed_problem_demo.sql` applied), drop the `--wire-demo-class`
flag. The library + Bridge HQ course are still imported.

The Python 101 source-of-truth is `content/python-101/`. Edit the
YAML files, run the importer, and the changes propagate. See
`content/python-101/README.md` for the authoring guide.

## Google OAuth Setup

1. Go to [Google Cloud Console](https://console.cloud.google.com)
2. Create a new project (or select existing)
3. Navigate to **APIs & Services > Credentials**
4. Create an **OAuth 2.0 Client ID** (Web application)
5. Add authorized redirect URIs:
   - `http://localhost:3003/api/auth/callback/google` — Bridge's dev port (matches `PORT=3003 bun run dev` in `CLAUDE.md`).
   - Add `http://localhost:<PORT>/api/auth/callback/google` for any other port you actually run on. NextAuth derives the redirect URI from the request's host:port, so `:3000` doesn't work if your dev server is on `:3003`.

   The mismatch shows up as `Error 400: redirect_uri_mismatch` from Google during sign-in.
6. Copy the Client ID and Client Secret to your `.env` file

## Auth.js Secret

Generate a secret for Auth.js:

```bash
openssl rand -base64 32
```

Add it to `.env` as `NEXTAUTH_SECRET`.

## Environment Classification (plan 050)

Set `APP_ENV` to one of `development`, `staging`, `production`. The
Go API uses it to gate the `DEV_SKIP_AUTH` safety check:

- With `DEV_SKIP_AUTH` set AND `APP_ENV=production`, the server
  refuses to start. `DEV_SKIP_AUTH` bypasses authentication entirely
  (any request → fully-privileged dev user); a leak into prod would
  silently make every request admin.
- Absence of `APP_ENV` is treated as "not production" (safe default
  for dev). Set `APP_ENV=production` in production deployments.

## Host Exposure Declaration (plan 068)

The `APP_ENV=production` guard above only catches a leak into the
production environment classification. Browser review 010 §P0 #1 caught
a tunneled review server running with `DEV_SKIP_AUTH=admin` and
`APP_ENV=development` — the prod-only guard didn't fire because the
host wasn't classified as production, even though the server was
reachable from another machine and the dev-bypass identity was
leaking into authenticated requests from real users.

`BRIDGE_HOST_EXPOSURE` is the second layer of defense. Set it on every
Go API deployment:

- `localhost` (default) — server is bound to loopback / only reachable
  from the local machine. `DEV_SKIP_AUTH` is allowed.
- `exposed` — server is reachable from outside the local machine
  (tunneled via SSH, on a LAN, behind a public ingress, etc.). If
  `DEV_SKIP_AUTH` is also set, the server refuses to start.

The simplest test: if `curl http://<server-host>:8002/healthz` works
from any other machine, the host is `exposed`.

For the rare case you genuinely want `DEV_SKIP_AUTH` active on a
tunneled host (e.g., a private demo machine where you've fully
internalized the trade-off), set `ALLOW_DEV_AUTH_OVER_TUNNEL=true` to
open the escape hatch. The startup logs a loud warning when this is
engaged. Both env var values are case-insensitive and trimmed —
`localhost`, `Localhost`, and `LOCALHOST` all work the same way; same
for `exposed` / `Exposed` / `EXPOSED`. The escape hatch opens on
literal `true`/`TRUE` only; `1`, `yes`, or any other truthy-looking
value is treated as unset.

**Unrecognized exposure values fail loud** rather than silently
falling through. `BRIDGE_HOST_EXPOSURE=public` (or any other unknown
string) refuses startup with a clear "unrecognized" error so a typo
can't accidentally defang the guard.

## Hocuspocus Token Secret (plan 053 / plan 072)

The Go API mints short-lived HMAC-SHA256 JWTs that the browser presents to the
Hocuspocus WebSocket server. Both processes must share the same secret:

```bash
openssl rand -base64 32
```

Add the value to `.env` as `HOCUSPOCUS_TOKEN_SECRET`. Both `platform/` (Go API,
which signs and verifies tokens for the internal callback) and the Hocuspocus
Node process (`server/hocuspocus.ts`, which verifies on connect) read it from
the same env var.

**`HOCUSPOCUS_TOKEN_SECRET` is required.** As of plan 072 the Hocuspocus
server refuses to start if the secret is unset — no escape hatch. Phase 2
of the plan also deleted the legacy `userId:role` token path entirely; JWT
is the only runtime auth.

The sibling `/api/internal/realtime/auth` endpoint is server-to-server
only and is gated by the same secret as a bearer token — it must NOT be
exposed publicly. (It is registered OUTSIDE the user-auth middleware so
the bearer check runs first; mounting it under user-auth would 401 the
unauthenticated callback before the bearer could be validated.)

### Hocuspocus-side configuration (plan 072 — secure-by-default)

The Node process (`server/hocuspocus.ts`) reads four env vars. A boot-time
validation function (`validateRealtimeAuthEnv`) checks these before the server
starts and calls `process.exit(1)` on any misconfig — mirrors the Go API's
`validateDevAuthEnv` pattern.

- `HOCUSPOCUS_TOKEN_SECRET` — **required.** The shared HMAC secret signed by
  the Go API and verified by Hocuspocus on every WebSocket connect. Unset
  causes a boot failure with no escape hatch — plan 072 phase 2 made this
  unconditional. Generate with `openssl rand -hex 32` and set the same value
  on both the Go API and the Hocuspocus process.
- `BRIDGE_HOST_EXPOSURE` — same semantics as the Go API (see "Host Exposure
  Declaration" above). Allowed values: `""` / `"localhost"` (default) and
  `"exposed"`. Unrecognized values fail loud at boot.
- `GO_INTERNAL_API_URL` — base URL the Hocuspocus Node process uses to
  call the recheck endpoint (`POST /api/internal/realtime/auth`). Defaults to
  `http://localhost:8002` (Go's local port). A warning is logged at boot if
  this is still the default value under `BRIDGE_HOST_EXPOSURE=exposed` — the
  Go API will be unreachable in that configuration. The recheck path is
  internal-only — keep it off any internet-facing route.

## Trusted Reverse-Proxy Configuration

The Go backend chooses between the secure (`__Secure-authjs.session-token`)
and non-secure (`authjs.session-token`) Auth.js session cookies based on the
request scheme. For requests that arrive over HTTPS, that scheme is taken
from `r.TLS` (direct hits) or the `X-Forwarded-Proto` header (proxied hits).

To prevent an unauthenticated client from spoofing `X-Forwarded-Proto: https`
and steering us to read the wrong (potentially stale) cookie variant, the
header is ONLY honored when the request's immediate peer is in the
`TRUSTED_PROXY_CIDRS` allowlist.

- **Local dev**: leave `TRUSTED_PROXY_CIDRS` empty. Direct hits to Go on
  HTTP rely on `r.TLS == nil` and read the non-secure cookie name.
- **Production**: set `TRUSTED_PROXY_CIDRS` to the load balancer / ingress
  CIDR range (e.g. `10.0.0.0/8,fd00::/8`).

> **Operational requirement (do not skip):** the configured ingress proxy
> MUST strip any client-supplied `X-Forwarded-Proto` header before
> forwarding. Allowlist + stripping are required together — without
> stripping, an attacker behind the proxy can still inject scheme metadata.

## Running Tests

Tests use a separate `bridge_test` database that is cleaned between each test.

```bash
# Run all tests
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bun run test

# Run a specific test file
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bun run test tests/unit/classes.test.ts

# Watch mode
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bun run test:watch
```

> **Note:** Tests run sequentially (`fileParallelism: false`) to prevent database cleanup conflicts between test files.

## Running E2E Tests (Playwright)

Playwright tests hit a live stack: Next.js (3003) + Go platform (8002) + Hocuspocus (4000). Start all three, then:

```bash
bun run test:e2e              # headless
bun run test:e2e:ui           # interactive
```

### Required test accounts

E2E tests expect the following accounts to exist in the dev DB (passwords all `bridge123`):

| Role        | Email                |
|-------------|----------------------|
| teacher     | eve@demo.edu         |
| student     | alice@demo.edu       |
| student2    | bob@demo.edu         |
| org admin   | frank@demo.edu       |
| parent      | diana@demo.edu       |
| platform admin | admin@e2e.test    |

The `demo.edu` accounts come from the demo seed. The `admin@e2e.test` account must be created once with `is_platform_admin=true`:

```sql
-- Bcrypt hash for "bridge123" (same hash used by the demo accounts).
-- Run once in the dev DB:
INSERT INTO "user" (id, email, name, password_hash, is_platform_admin)
VALUES (
  gen_random_uuid(),
  'admin@e2e.test',
  'E2E Admin',
  '<bcrypt-of-bridge123>',
  true
);
```

To generate the bcrypt hash:

```bash
bun -e "import('bcryptjs').then(b => console.log(b.default.hashSync('bridge123', 10)))"
```

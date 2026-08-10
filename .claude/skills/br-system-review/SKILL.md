---
name: br-system-review
description: Use when the user asks for a Bridge health check / "what's the system state right now?" / "anything broken?" / "audit the running services". Scans service status (processes + ports + /healthz), Go build health (air tmp dir, go vet, go build), log surfaces (slog output, platform/tmp/), DB integrity (Postgres schema probe), disk usage, and cross-correlates the signals into a single report tagged [BLOCKER] / [WARNING] / [OK] / [INFO]. Distinct from `investigate-errors` (deep-dive on one error source) — this is a breadth-first checkup.
---

# br-system-review

Health-check the running Bridge system. Breadth-first counterpart to `investigate-errors`.

## When to invoke

- User says "what's broken?" / "is the system healthy?" / "audit services" / "system check" / "run a checkup".
- After a long absence (weekend, vacation) — quick situational awareness before resuming work.
- After a major merge that might have introduced regressions.
- When intermittent failures are happening and the user doesn't know where to start — this skill produces the high-level map; `investigate-errors` drills into specific error patterns.

## When NOT to invoke

- Single-error deep dive → use `investigate-errors`.
- Session/transcript auditing → out of scope for Bridge (no session-audit skill).
- CI / PR / dependency-version status → out of scope (different skill if needed).

## Inputs

All optional. Args parsed `key=value` from the slash-command tail.

| Arg | Default | Meaning |
|---|---|---|
| `services` | `all` | Comma-sep filter — `frontend,platform,hocuspocus`. Default checks all three. |
| `include_ok` | `true` | Show `[OK]` rows for healthy components. Set `false` for a problems-only report. |
| `skip` | `none` | Comma-sep list of sections to skip: `services` / `errors` / `logs` / `db` / `disk` / `git`. |
| `verbose` | `false` | Include raw command output (longer report). |

Note: the `since` and `top` error-stats args are dropped — Bridge has no error-stats CLI.
The `since` arg is repurposed only as a `--since` window for `git log` (see §6).

## What gets checked

### 1. Service status

| Component | Probe |
|---|---|
| Next.js frontend (`:3003`) | `curl -sf -m 2 -o /dev/null -w '%{http_code}' http://127.0.0.1:3003` + `pgrep -f "next-server"` |
| Go platform API (`:8002`) | `curl -fsS -m 2 http://127.0.0.1:8002/healthz` + `pgrep -f "go run.*cmd/api\|air.*cmd/api\|__debug_bin"` |
| Hocuspocus / Yjs (`:4000`) | `curl -sf -m 2 -o /dev/null -w '%{http_code}' http://127.0.0.1:4000` + `pgrep -f "hocuspocus"` |

Port-check probe (all three in one call):
```bash
ss -ltnp | grep -E ':3003|:8002|:4000'
```

Tag rules:
- `[OK]` — process running AND port responds.
- `[WARNING]` — process running but port not responding (startup hung / port wrong) OR port responds but no process match (proxied through tmux/screen).
- `[BLOCKER]` — process not running AND port not responding (service down).

Also flag:
- **Duplicate processes** — more than one `next-server` OR `air` instance → leftover from kill -9 / crashed shell → `[WARNING]`.
- **Stale air binary** — `platform/tmp/` binary mtime >1 hour old AND no live `air` process → `[WARNING]` "air died, binary stale".

### 2. Error surfaces (Go platform)

> **Bridge has no centralized error-stats CLI.** Error aggregation in
> Bridge is limited to: (a) the live `air`/`go run` process log, (b) the
> `platform/tmp/` build output, and (c) static build/vet checks. If you need
> a deep-dive into an error pattern, use the `investigate-errors` skill — but
> note that `investigate-errors` requires an error log source to be configured
> (e.g. a structured JSONL log file). As of now, Bridge's Go platform emits
> slog output to stdout only. This section surfaces what is available without
> a CLI tool.

#### 2a. Static build health

```bash
cd /home/chris/workshop/bridge/platform && go vet ./... 2>&1 | head -40
cd /home/chris/workshop/bridge/platform && go build ./... 2>&1 | head -40
```

- `go vet` output non-empty → `[WARNING]` "go vet found issues — list them".
- `go build` fails → `[BLOCKER]` "Go build failing — platform cannot start".

#### 2b. air build artifacts

```bash
ls -la /home/chris/workshop/bridge/platform/tmp/ 2>/dev/null
```

Look for:
- Build error output files with non-empty content → `[BLOCKER]` "Go build failing in air".
- `build-errors.log` or `errors.log` if present → read up to 40 lines.
- Stale binary mtime (>1 hour old AND no live `air` process) → `[WARNING]` "air died, build artifacts stale".

#### 2c. Live process log (opportunistic)

If the platform is running under a terminal multiplexer (tmux/screen), the live
slog output is not directly capturable by this skill. Instead:

- Note `[INFO]` "Go platform running — live slog output visible in terminal only. For structured error aggregation, configure a log file sink and use investigate-errors."
- If the platform is NOT running, note that error log review is moot until the service is restored.

### 3. Log surfaces (supplemental)

#### 3.1 Next.js build output

```bash
ls -la /home/chris/workshop/bridge/.next/ 2>/dev/null | head -10
```

- `.next/` directory missing → `[INFO]` "Next.js not built locally (dev mode auto-builds on first request; this is normal)".
- Any `BUILD_ERROR` markers in `.next/` → `[WARNING]`.

#### 3.2 Hocuspocus

Hocuspocus has no persistent log file by default (stdout only). Note `[INFO]`
"Hocuspocus log review requires the terminal session — no file log configured."

### 4. DB integrity (Postgres only)

Bridge uses Postgres for both dev (`bridge`) and test (`bridge_test`) databases.
There is no SQLite in production — do NOT report SQLite health as a DB signal.

Bridge does NOT use golang-migrate's `schema_migrations` version table. It uses
a **schema-probe** model: the platform boots and verifies that the latest
migration's complete output (tables + columns + named constraints + indexes +
ordered enum labels) is present in the live DB. Failure causes the server to
refuse to start.

```bash
# Connectivity probe
psql postgresql://work@127.0.0.1:5432/bridge -c 'select 1' 2>&1
psql postgresql://work@127.0.0.1:5432/bridge_test -c 'select 1' 2>&1

# Schema probe — current contract from drizzle/0028_session_canvases.sql:
# public.session_canvases; all nine named canvas columns; both named indexes
# on that table; public.sessions.canvas_floor; and ordered public.canvas_visibility.
psql postgresql://work@127.0.0.1:5432/bridge -tAc \
  "SELECT to_regclass('public.session_canvases') IS NOT NULL
     AND (SELECT count(DISTINCT column_name)
            FROM information_schema.columns
           WHERE table_schema='public'
             AND table_name='session_canvases'
             AND column_name = ANY (ARRAY['id','session_id','owner_id','title','visibility','yjs_state','plain_text','created_at','updated_at'])) = 9
     AND (SELECT count(DISTINCT indexname)
            FROM pg_indexes
           WHERE schemaname='public'
             AND tablename='session_canvases'
             AND indexname = ANY (ARRAY['session_canvases_session_idx','session_canvases_session_owner_idx'])) = 2
     AND EXISTS (SELECT 1
                   FROM information_schema.columns
                  WHERE table_schema='public'
                    AND table_name='sessions'
                    AND column_name='canvas_floor')
     AND ARRAY(SELECT e.enumlabel
                 FROM pg_type t
                 JOIN pg_namespace n ON n.oid=t.typnamespace
                 JOIN pg_enum e ON e.enumtypid=t.oid
                WHERE n.nspname='public'
                  AND t.typname='canvas_visibility'
                ORDER BY e.enumsortorder)
         = ARRAY['private','host','participants','session']::text[]" 2>&1

# Count of Drizzle migration files on disk (latest prefix)
ls /home/chris/workshop/bridge/drizzle/*.sql 2>/dev/null | \
  grep -oP '^\d+' | sort -n | tail -1
```

Tag rules:
- Postgres `bridge` unreachable → `[BLOCKER]` if platform is running (misconfigured or DB down), `[INFO]` if nothing is running.
- `session_canvases`, any of its nine named columns, either named canvas index, `sessions.canvas_floor`, or ordered `canvas_visibility` missing/mismatched in `bridge` DB → `[BLOCKER]` "Schema probe would fail — migration 0028 is partially or wholly unapplied. Do not blindly rerun it: inspect and reconcile `drizzle/0028_session_canvases.sql` through the approved database-change workflow."
- `bridge_test` unreachable → `[WARNING]` "Test DB unreachable — Go integration tests and Vitest API tests will fail."
- Both reachable, complete schema contract present → `[OK]`.

For any blocker, remind: "Bridge's platform refuses to start if the schema probe
fails at boot (`ExpectedSchemaProbe = 'session_canvases'`, `ExpectedSchemaSentinels` in
`platform/internal/db/migrations.go`)."

### 5. Disk usage

```bash
df -h /home/chris/workshop/bridge | tail -1
du -sh /home/chris/workshop/bridge/.next 2>/dev/null
du -sh /home/chris/workshop/bridge/platform/tmp 2>/dev/null
```

- Free space < 1 GB → `[BLOCKER]` "Disk full imminent".
- `.next/` or `platform/tmp/` > 2 GB → `[WARNING]` "Build cache large — consider cleaning".

### 6. Git working tree

```bash
git -C /home/chris/workshop/bridge status --porcelain | head -20
git -C /home/chris/workshop/bridge stash list | head -5
git -C /home/chris/workshop/bridge log --oneline @{u}..HEAD 2>/dev/null | head -5
git -C /home/chris/workshop/bridge log --oneline -10
```

Tag rules:
- Uncommitted changes > 10 files → `[WARNING]` "Significant uncommitted work — consider WIP commit (multi-agent coord per AGENTS.md)".
- Uncommitted changes on `main` branch → `[WARNING]` "Dirty main — AGENTS.md requires feature branches; stage or stash before resuming".
- Unpushed commits on current branch → `[INFO]` "N commits ahead of remote".
- Stash entries older than 7 days → `[INFO]` "Old stashes; consider git stash drop".

## Workflow

### Step 1 — Discover what's running

Run all probes in parallel where possible (chain with `&` / `wait`, or use separate
parallel Bash tool calls). Keep timeouts short (`-m 2` for curl) so the skill returns
in <15s even when some services are down.

### Step 2 — Compute findings

For each check above, derive 0+ findings. Each finding:

- **Tag** — `[BLOCKER]` / `[WARNING]` / `[OK]` / `[INFO]`
- **Section** — `services` / `errors` / `logs` / `db` / `disk` / `git`
- **Component** — short name (`go-platform`, `frontend-3003`, `hocuspocus-4000`, `db-bridge`, etc.)
- **Detail** — one-line description
- **Suggested action** — when non-`[OK]`, one sentence on what to do

### Step 3 — Cross-correlate

Look for patterns that aren't obvious from individual checks:

- Platform down + `go build` failing → `[BLOCKER]` "Platform cannot start due to build error — fix before anything else".
- Platform running + `go vet` warnings → `[WARNING]` "Vet issues present in running code; may indicate logic error".
- Hocuspocus down + frontend running → `[WARNING]` "Real-time collaboration (Yjs) unavailable; editor sync will fail".
- All three services down + DB reachable → `[INFO]` "Stack not started; DB is up. Run: `PORT=3003 bun run dev`, `cd platform && air`, `bun run hocuspocus`".
- DB unreachable + platform process running → `[BLOCKER]` "Platform is up but DB is down; all API calls will fail".
- Schema sentinel missing + platform running → `[BLOCKER]` "Platform will fail on next restart (schema probe fails at boot)".
- Build stale (`platform/tmp/` mtime old) + platform process responding → `[WARNING]` "Serving a stale binary; air can't rebuild. Check for compilation errors."

### Step 4 — Format report

Group by severity, then by section. Always emit BLOCKER + WARNING. Emit OK/INFO
unless `include_ok=false`. Structure:

```
## System review — <ISO timestamp>

### BLOCKERS (N)

[BLOCKER] services/go-platform — port 8002 not responding AND no air/go-run process
  → start with: cd platform && air
  → or: cd platform && go run ./cmd/api/

[BLOCKER] db/schema — session_canvases / canvas_floor / canvas_visibility sentinel missing from bridge DB
  → inspect drizzle/0028_session_canvases.sql and identify the missing DDL
  → do not blindly rerun the full migration; use the approved database-change workflow
  → note: platform will refuse to start until the complete schema probe passes

### WARNINGS (M)

[WARNING] errors/go-build — go vet found 2 issues in platform/internal/handlers/
  → fix before next deploy; run: cd platform && go vet ./...

[WARNING] services/frontend-3003 — 2 next-server processes running
  PIDs: 12345 (bridge-worktree-A), 67890 (bridge)
  → kill the stale one: kill 12345

### OK (K)  [hidden if include_ok=false]

[OK] services/hocuspocus-4000 — process running, port 4000 responds
[OK] db/bridge-test — reachable
[OK] disk — 180 GB free

### INFO (P)

[INFO] errors/live-log — Go platform slog goes to stdout only; no file sink configured.
  Use investigate-errors if a structured log source is set up.
[INFO] git — 2 commits ahead of origin/feat/090-something on current branch

### Cross-correlation

- Platform build is broken (go build fails) AND service port 8002 is dark: the platform
  is down because it can't be compiled. Fix the build error first; all other services
  (frontend, hocuspocus) can run independently.
```

### Step 5 — Offer next actions

If BLOCKERs exist, end the report by asking the user (one short question):
> "BLOCKERs found. Want me to: (1) attempt to fix the build error, (2) apply the missing migration, (3) drill into error patterns via investigate-errors, or (4) leave it for now?"

If only WARNINGs/INFO, end with one suggested next-step + leave decision to user.

## Constraints

- **Read-only by default.** No service restarts, no `pkill`, no schema changes without explicit user approval after the report.
- **Short timeouts.** Every probe must have a 2-5 second timeout. The skill should return in <15s even if half the system is down.
- **No log tailing.** Never `tail -f` — it'll never return. Bounded `head -40` / `tail -200` is the limit.
- **Don't conflate signals.** A build failure is NOT the same as a service being down (a stale binary might still be running). Report them separately and only cross-correlate in Step 3.
- **Defer deep dives.** This skill flags problems but doesn't diagnose them — that's `investigate-errors`. Always reference it for error-pattern BLOCKERs.
- **No invented tooling.** Bridge has no `bridge errors stats` CLI. Do not claim one exists. Surface what is genuinely available: build output, go vet, process logs accessible without tailing.

## Quick recipes

```
/br-system-review                           # default: all services, show OK
/br-system-review include_ok=false          # problems-only report
/br-system-review services=platform,db      # only Go API + DB sections
/br-system-review skip=git,disk             # skip git/disk sections
/br-system-review verbose=true              # include raw command output
```

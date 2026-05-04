# Plan 068 — Runtime guards + identity-drift detection

## Status

- **Date:** 2026-05-03
- **Origin:** Comprehensive browser review 010 (`docs/reviews/010-comprehensive-browser-review-2026-05-03.md`) §P0 #1 + the closing recommendations §6. The review caught a tunneled review environment running with `DEV_SKIP_AUTH=admin` while real Auth.js sessions were also present — Next saw the real user, Go saw `Dev User`. Identity diverged silently, and several P1 findings (admin pages 403, student no classes, direct-session URL works) cascaded from this single misconfiguration. The reviewer recommends loud, fail-fast guards rather than relying on humans to notice.
- **Scope:** Server-side startup checks + a small Next-side runtime banner. No new API endpoints; no schema changes. Operator-facing diagnostics, not user features.
- **Predecessor context:** Plan 050 added the existing `validateDevAuthEnv` guard that refuses to start when `DEV_SKIP_AUTH != "" && APP_ENV == "production"`. Plan 065 phase 4 made identity live-from-DB on every authenticated request. This plan layers more sensitive guards: catch the case the existing one missed (tunneled non-localhost env with `APP_ENV=development`), and detect post-boot drift in real time.

## Problem

The plan-050 guard only fires for `APP_ENV=production`. The review environment was a tunneled remote machine with `APP_ENV=development` (or unset, treated as non-production), and `DEV_SKIP_AUTH=admin` was active. Result:

- Next.js portal shell rendered the real signed-in user (Eve Teacher, Alice Student, etc.) because Auth.js / `/api/auth/session` work normally.
- Every Go-backed endpoint saw `Dev User` (`dev@localhost`, the synthetic claims from `DEV_SKIP_AUTH`).
- `/api/me/identity` returned the dev user — the mismatch with the Next-side session was visible but only to a reviewer who knew to look.

Cascading symptoms in the review report (all P0 #1 in disguise):
- Platform admin shell rendered, but `/api/admin/stats` returned 403 (Go's middleware sees Dev User which isn't admin in this DB).
- Student dashboard showed "No classes" (Go's `/api/classes/mine` returned the dev user's empty class list).
- `/student/sessions/{id}` direct URLs rendered (Go authorized as Dev User who happened to be a teacher in seed data).

The review report's recommendations:
1. **Refuse to start when `DEV_SKIP_AUTH` is set on a non-localhost listener** unless an explicit escape hatch is present (`APP_ENV=local` or `ALLOW_DEV_AUTH_OVER_TUNNEL=true`).
2. **Add a runtime warning banner when Next sees a real Auth.js session but `/api/me/identity` returns `dev@localhost`.** Visible to anyone signed in, not just operators.
3. **Schema-drift / migration health check** at startup — the same review hit `relation "parent_links" does not exist` because the remote DB was un-migrated.
4. **Realtime-config banner for live sessions** when `HOCUSPOCUS_TOKEN_SECRET` is unset, instead of letting the failure surface as a console error.

All four are operator UX improvements that turn silent misconfigurations into loud, actionable failures.

## Out of scope

- Authentication/authorization logic changes. Plan 065 owns that surface; this plan only adds diagnostics around it.
- A general-purpose `/api/health` endpoint with subsystem checks. Each guard here is targeted at a specific known foot-gun. A unified health surface is plan-068b material if/when it's needed.
- Removing `DEV_SKIP_AUTH` itself. It remains useful for fast local iteration on the auth-free code path. The plan adds *guards* against misuse, not deprecation.
- Hocuspocus-side configuration validation. Plan 053 phase 2 already errors loudly on the Hocuspocus side when the token secret is missing; this plan only adds the Next-side banner so end users see why their live session won't connect.
- The `/api/auth/debug` drift endpoint already exists for manual checks (per plan 065 §"Why dual JWE was bad enough to delete"); this plan turns the drift signal into an automatic banner.

## Approach

Four small, independent additions:

1. **Stronger `DEV_SKIP_AUTH` startup guard.** Codex pass-1 caught the original "use cfg.Server.Host" approach as unsound: the listener binds `":%d"` (all interfaces) regardless of `cfg.Server.Host`, and `validateDevAuthEnv` runs BEFORE config loading. Revised approach: introduce a NEW dedicated env var `BRIDGE_HOST_EXPOSURE` with values `localhost` (default — guard fires when DEV_SKIP_AUTH is set) or `exposed` (operator's explicit declaration that this server is on a tunnel/LAN/public address). The guard refuses to start when `DEV_SKIP_AUTH != ""` AND `BRIDGE_HOST_EXPOSURE == "exposed"` AND `ALLOW_DEV_AUTH_OVER_TUNNEL != "true"`. Operators set `BRIDGE_HOST_EXPOSURE=exposed` in any deployment that's reachable from outside the local machine; the default-localhost-bias keeps friction-free local development. The check stays in `validateDevAuthEnv(getEnv)` — no config-load reordering needed, just a new env var.

2. **Identity-drift warning banner.** Add a small client component that fires `/api/auth/debug` on every portal page mount and renders a red banner if `match === false` AND `goClaimsUserId === devUserPlaceholderId`. The endpoint already exists (`src/app/api/auth/debug/route.ts`) and returns 404 in production builds — banner only ever shows in dev/staging where the mismatch could happen. Visible to all users (so a teacher hitting weirdness can flag it), not just operators.

3. **Migration health check at startup.** Codex pass-2 caught a deeper issue with the original COUNT(*) approach: per `TODO.md:10`, Bridge applies migrations 0003+ via `psql -f` (not drizzle-kit migrate), so those migrations leave NO row in `drizzle.__drizzle_migrations`. Counting tracking-table rows would refuse a fully-migrated production DB. Revised approach (Codex's Option (a)): instead of counting, **probe for the existence of a known late-migration schema object**. The latest schema-affecting migration was `drizzle/0024_parent_links.sql` which creates the `parent_links` table; the boot check verifies `to_regclass('public.parent_links') IS NOT NULL`. On a freshly-migrated DB this passes; on a stale DB (missing the latest migration) it fails. The constant is `ExpectedSchemaProbe = "parent_links"` and is bumped manually in any PR that adds a new schema-affecting migration. A CI-side parity test (Phase 3) confirms the constant references a table that actually exists in the latest .sql files. This approach is robust to the hand-apply workflow because it checks the END STATE of the schema, not the audit-log entries.

4. **Realtime-config banner for live sessions.** When `/api/realtime/token` returns 503 ("Realtime tokens not configured"), the existing `useRealtimeToken` helper at `src/lib/realtime/get-token.ts:65-83` throws and the failure surfaces as a console error. Instead, render a small in-page banner on session/teacher-watch/parent-viewer pages: "Realtime is not configured for this environment. Live collaboration is unavailable. [retry]". The banner replaces the silent console drop and gives end users an actionable message.

## Decisions to lock in

1. **Escape hatch is `ALLOW_DEV_AUTH_OVER_TUNNEL=true`, not `APP_ENV=local`.** Reviewer suggested either; the explicit allowlist env is more searchable and less ambiguous (`APP_ENV` already has prod-vs-not-prod semantics from plan 050). Setting the escape hatch is a deliberate operator decision; defaulting to off catches the foot-gun.
2. **Exposure declaration via `BRIDGE_HOST_EXPOSURE` env var, not bind-host inference** (Codex pass-1 finding). The Go listener binds `":%d"` (all interfaces) regardless of `cfg.Server.Host`, so inferring "is this localhost-only?" from the host field is unsound. Operators set `BRIDGE_HOST_EXPOSURE=localhost` (default — local dev) or `BRIDGE_HOST_EXPOSURE=exposed` (tunneled / staging / production). The default-to-localhost choice means an operator who forgets the var on a tunneled server STILL hits the guard if `DEV_SKIP_AUTH` is set, because the guard fires conservatively when exposure is `localhost` — actually wait, that's backwards. Re-reading: the guard should fire when `exposed` AND `DEV_SKIP_AUTH != ""`. If exposure isn't set, default to `localhost` (no guard fire — local dev "just works"). The escape hatch `ALLOW_DEV_AUTH_OVER_TUNNEL=true` is the override. Operators on shared/tunneled infra MUST set `BRIDGE_HOST_EXPOSURE=exposed` for the guard to be useful — this is a deliberate ops-discipline requirement, documented in `docs/setup.md`.
3. **Identity-drift banner is dev/staging only.** It calls `/api/auth/debug` which 404s in production (`src/app/api/auth/debug/route.ts:24-26`). The banner component handles 404 by silently no-op'ing. Production users never see this banner.
4. **Schema-version constant lives in Go.** `platform/internal/db/version.go` exports `ExpectedSchemaVersion = "0024"` (matching the latest migration filename). Bumped by hand on each migration; a CI test compares against `drizzle/` directory contents.
5. **Realtime banner only when 503.** Other failure modes (network, 401, malformed response) keep the existing console-error path — those are bugs in the realtime mint flow, not config issues. The 503-specific banner avoids false positives.
6. **No retry button on the realtime banner v1.** The user has to refresh the page after the operator fixes the env. Adding a working retry requires re-running the auth check + remounting the WebSocket; defer to v2 if the friction matters.

## Files

### Phase 1 — `DEV_SKIP_AUTH` non-localhost guard

**Modify:**
- `platform/cmd/api/main.go` — extend `validateDevAuthEnv(getEnv)` to also check `BRIDGE_HOST_EXPOSURE` and `ALLOW_DEV_AUTH_OVER_TUNNEL`. New logic: if `DEV_SKIP_AUTH != ""` AND `BRIDGE_HOST_EXPOSURE == "exposed"` AND `ALLOW_DEV_AUTH_OVER_TUNNEL != "true"`, return an error. Existing prod-guard logic stays. **No config-load reordering needed** — both new vars are env vars read via `getEnv`, same path as `DEV_SKIP_AUTH` and `APP_ENV`. (Codex pass-1 caught the original "use cfg.Server.Host" approach that would have required reordering.)
- `platform/cmd/api/main_test.go` — extend `TestValidateDevAuthEnv` with table cases:
  - `DEV_SKIP_AUTH=admin`, `BRIDGE_HOST_EXPOSURE=` (empty/default = localhost) → allowed (local dev)
  - `DEV_SKIP_AUTH=admin`, `BRIDGE_HOST_EXPOSURE=localhost` → allowed
  - `DEV_SKIP_AUTH=admin`, `BRIDGE_HOST_EXPOSURE=exposed` → ERROR
  - `DEV_SKIP_AUTH=admin`, `BRIDGE_HOST_EXPOSURE=exposed`, `ALLOW_DEV_AUTH_OVER_TUNNEL=true` → allowed (escape hatch)
  - `DEV_SKIP_AUTH=`, `BRIDGE_HOST_EXPOSURE=exposed` → allowed (no dev bypass to guard)
- `.env.example` — add both vars with a comment explaining the contract: set `BRIDGE_HOST_EXPOSURE=exposed` on any deployment reachable from outside localhost; set `ALLOW_DEV_AUTH_OVER_TUNNEL=true` only as a deliberate, time-boxed escape hatch.
- `docs/setup.md` — document the guard alongside the existing `APP_ENV=production` guard description.

### Phase 2 — Identity-drift warning banner

**Add:**
- `src/components/portal/identity-drift-banner.tsx` — client component. On mount, fetches `/api/auth/debug` (silent 404 = production = no-op). Renders a red banner across the top of the viewport when:
  - `match === false`
  - `goClaimsUserId` matches the well-known dev-user placeholder (`00000000-0000-0000-0000-000000000001`, set by `platform/internal/auth/middleware.go:115-127`).

  Banner copy: "Auth identity mismatch detected. Next.js shows {nextUserEmail}; Go API shows {goUserEmail}. The Go server is likely running with `DEV_SKIP_AUTH` set on a non-localhost host. Operator action required."
- `tests/unit/identity-drift-banner.test.tsx` — happy-path no-banner, drift-banner-renders, production-404-no-banner.

**Modify:**
- `src/components/portal/portal-shell.tsx` — render `<IdentityDriftBanner />` at the top of the shell, inside the main content area. Stays out of the sidebar so it doesn't shift navigation.

### Phase 3 — Schema-probe startup check

**Add:**
- `platform/internal/db/migrations.go` — exports `ExpectedSchemaProbe = "parent_links"` (current latest schema-affecting migration). Comment documents the bump procedure: "when adding a NEW table or schema-affecting migration, update this constant to the new table name in the same PR. The constant must reference a table that exists in the schema after the latest .sql migration is applied."
- `platform/internal/db/schema_probe.go` — `func CheckSchemaProbe(ctx context.Context, db *sql.DB) error`. Queries `SELECT to_regclass($1) IS NOT NULL` with the probe table name. Returns:
  - `nil` when the table exists.
  - Descriptive error when missing: "expected table `{name}` is not present; the database is missing migration `{file}`. Run `psql ... -f drizzle/{file}` to apply it (see TODO.md for the manual-apply workflow)."
- `platform/internal/db/schema_probe_test.go` — happy-path (probe table exists), missing-table error, malformed input.

**Modify:**
- `platform/cmd/api/main.go` — call `db.CheckSchemaProbe` after `db.Open` succeeds. On error: `slog.Error` + `os.Exit(1)`. New behavior: refuses to start against a stale schema.

**CI parity test:**
- `platform/internal/db/schema_probe_parity_test.go` — at test time, scan `drizzle/*.sql` for the latest `CREATE TABLE` statement; assert that the table name appears in the `ExpectedSchemaProbe` const. Fails any PR that adds a new schema migration without bumping the constant, OR bumps the constant without adding a corresponding `CREATE TABLE` in the .sql files.

### Phase 4 — Realtime-config banner

Codex pass-1 confirmed `getRealtimeToken` already differentiates 503 (`src/lib/realtime/get-token.ts:79` throws `RealtimeMintError` with `status: 503`) but `useRealtimeToken` itself catches all failures identically (`src/lib/realtime/use-realtime-token.ts:38`). The banner needs to either pierce through `useRealtimeToken` to get the discrimination back OR `useRealtimeToken` itself surfaces the 503 specifically.

**Modify:**
- `src/lib/realtime/use-realtime-token.ts` — extend the hook's return shape to expose `unavailable: boolean` (true when the most recent fetch failed with 503). Keeps the rest of the hook's contract identical.
- `src/components/session/teacher/teacher-dashboard.tsx`, `src/components/session/student/student-session.tsx`, `src/components/parent/live-session-viewer.tsx`, `src/components/problem/teacher-watch-shell.tsx`, `src/components/problem/problem-shell.tsx`, `src/components/session/broadcast-controls.tsx`, `src/components/session/student-tile.tsx`, `src/lib/yjs/use-yjs-tiptap.ts` (consumed indirectly via `src/components/editor/tiptap/teaching-unit-editor.tsx`) — Codex pass-2 audit caught the missing `use-yjs-tiptap` chain; the actual surface is 8 direct/indirect consumers, not 7. All read `unavailable` from `useRealtimeToken` (or its wrapper for the Yjs case) and render the new banner when true.

**Add:**
- `src/components/realtime/realtime-config-banner.tsx` — pure presentation. Banner copy: "Live collaboration is unavailable in this environment. The realtime token service is not configured. Static viewing still works; reload the page after the operator sets `HOCUSPOCUS_TOKEN_SECRET`."

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| New startup guard breaks existing local dev workflows that bind to 0.0.0.0 | medium | Default behavior change is loud — operator gets a clear refusal-to-start message with the escape-hatch instruction. Document in `docs/setup.md`. |
| Identity-drift banner false-positives during impersonation | low | Skip banner when `impersonatedBy !== ""` (the live identity *should* differ from the JWT in that case). Codex pass should confirm. |
| Schema-version constant drifts from actual migrations | medium | Phase 3 includes a CI-side check that compares `ExpectedSchemaVersion` to the highest filename under `drizzle/`. Failing PRs forces operators to update the constant in the migration PR. |
| Realtime banner appears on pages that don't actually need realtime | low | Only wrap pages that consume realtime tokens. Phase 4 enumerates the four current consumers. |
| `/api/auth/debug` removed at some point (it's dev-only and could be deleted) | low | The banner already handles 404 silently. If the endpoint goes away in production builds, banner just doesn't render — no error. |
| Bind-host detection: `cfg.Server.Host` could be a hostname (e.g., `bridge.example.com`) | low | Resolve via `net.LookupHost`. If any resolved IP is non-loopback, treat as exposed. |

## Phases

### Phase 0 — Pre-impl Codex review

Per CLAUDE.md plan-review gate. Dispatch `codex:codex-rescue` to review against:
- `platform/cmd/api/main.go` (existing validateDevAuthEnv + new guard insertion point)
- `platform/internal/auth/middleware.go` (the dev-user injection logic the new guard protects against)
- `src/app/api/auth/debug/route.ts` (the endpoint the drift banner calls)
- `src/lib/realtime/get-token.ts` (the realtime-token consumer the new banner wraps)
- The four session-page components Phase 4 modifies

Specific questions:
1. Is `cfg.Server.Host` actually populated in current code paths? It defaults to `0.0.0.0` at `config.go:62` but is there an env override path that leaves it empty?
2. Drizzle's migration-tracking table name — what is it actually called in this codebase? Need the right SELECT for Phase 3.
3. The well-known dev-user placeholder (`00000000-0000-0000-0000-000000000001`) — is that ID also used elsewhere as a real user? If yes, the drift banner's signature check needs to be more specific.
4. Are there any existing identity-drift detection paths in the codebase (e.g., the api-client's stale-cookie warning at `src/lib/api-client.ts:43-45`) the new banner should reuse rather than duplicate?
5. Phase 3 schema-version: should the constant be in Go OR computed at build time from `ls drizzle/*.sql | sort | tail -1`? The latter avoids manual bumps but couples Go build to a glob.
6. Phase 4: does `useRealtimeToken` already differentiate 503 from other failures, or do we need to add the discrimination?

### Phase 1 — DEV_SKIP_AUTH non-localhost guard (PR 1)

- Implement guard + tests.
- Update `.env.example` and `docs/setup.md`.
- Smoke: try to start with `DEV_SKIP_AUTH=admin` and host `0.0.0.0` → should refuse. With `ALLOW_DEV_AUTH_OVER_TUNNEL=true` → should start with a warning.
- Codex post-impl review.
- PR + merge.

### Phase 2 — Identity-drift banner (PR 2)

- Implement banner component + tests.
- Mount in PortalShell.
- Smoke: simulate the drift in dev (set `DEV_SKIP_AUTH=admin` after signing in as a real user) → banner appears.
- Codex post-impl review.
- PR + merge.

### Phase 3 — Schema-version startup check (PR 3)

- Implement constant + check.
- Wire into startup.
- Add CI check for constant-vs-files parity.
- Codex post-impl review.
- PR + merge.

### Phase 4 — Realtime-config banner (PR 4)

- Implement banner component.
- Wrap the four session pages.
- Smoke: unset `HOCUSPOCUS_TOKEN_SECRET` and load a live session → banner appears in-page instead of console error.
- Codex post-impl review.
- PR + merge.

## Codex Review of This Plan

### Pass 2 — 2026-05-03: BLOCKED → 2 new blockers folded

Codex pass-2 confirmed the BRIDGE_HOST_EXPOSURE pivot is sound and the validateDevAuthEnv ordering issue is resolved. Two new blockers caught:

1. **COUNT(*) approach incompatible with hand-applied migration workflow** — TODO.md:10 confirms migrations 0003+ are applied via `psql -f`, not drizzle-kit migrate, so they leave NO row in `__drizzle_migrations`. Counting tracking-table rows would refuse a fully-migrated DB. Resolved: switched from COUNT(*) to a schema-probe approach — `to_regclass('public.parent_links') IS NOT NULL` checks the END STATE of the schema, robust to the hand-apply workflow. Constant is now `ExpectedSchemaProbe = "parent_links"` (the latest CREATE TABLE migration's target). CI parity test compares against the latest `CREATE TABLE` in .sql files.

2. **Realtime callsite list missed `use-yjs-tiptap`** — Codex pass-2 found that `src/lib/yjs/use-yjs-tiptap.ts` calls `useRealtimeToken` and is consumed by `teaching-unit-editor.tsx`. Phase 4 callsite list expanded from 7 to 8.

### Pass 1 — 2026-05-03: BLOCKED → 3 blockers + 2 important folded in

Codex pass-1 returned BLOCKED with three blockers, all addressed:

1. **Bind-host detection unsound** — `cfg.Server.Host` is populated but the listener binds `":%d"` (all interfaces) regardless. Inferring "is this localhost-only?" from the host field doesn't reflect actual network exposure. Resolved: replace host-inference with explicit `BRIDGE_HOST_EXPOSURE` env var (operator declaration). Approach §1, Decisions §2, Phase 1 Files all rewritten.
2. **`validateDevAuthEnv` runs before config load** — the original plan's approach would have required reordering. Resolved: with `BRIDGE_HOST_EXPOSURE` as an env var, the check stays in the existing `getEnv`-based validator. No reordering needed.
3. **Migration table name + columns wrong** — actual table is `drizzle.__drizzle_migrations` with `id, hash, created_at` (no `version`). `_journal.json` covers only 0000-0002 while codebase has 0000-0024 .sql files. Resolved: switch to `COUNT(*)`-vs-`ExpectedMigrationCount` comparison (count is robust to journal staleness). Phase 3 Files rewritten with correct query + count constant + CI parity test.

Important non-blocking, both folded:

4. **Realtime token consumer count was 4, actual is 7** — Codex audit found three more callsites (`problem-shell.tsx`, `broadcast-controls.tsx`, `student-tile.tsx`). Phase 4 Files updated with the full enumeration.
5. **`useRealtimeToken` catches all failures identically** — needs to surface the 503 discrimination to the new banner. Phase 4 now extends the hook's return shape with `unavailable: boolean`.

CONFIRMED by Codex (no changes needed):
- `/api/auth/debug` 404s in production (banner safe to mount unconditionally).
- The well-known dev-user UUID `00000000-0000-0000-0000-000000000001` is NEVER a real user.
- `getRealtimeToken` already throws `RealtimeMintError{status: 503}` for the 503 case.
- Existing identity-drift helpers (`src/lib/identity-assert.ts`, `src/app/api/auth/debug/route.ts`) are reusable.
- Plan-050 production guard's premise (only fires on `APP_ENV=production`) is correct.

Verdict: **BLOCKED → all blockers resolved → ready for Phase 1** pending pass-2 convergence check.

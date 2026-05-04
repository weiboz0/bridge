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

1. **Stronger `DEV_SKIP_AUTH` startup guard.** Extend `validateDevAuthEnv` (currently in `platform/cmd/api/main.go:372-386`) to also refuse start when `DEV_SKIP_AUTH != ""` AND the bind address resolves to a non-localhost interface AND the new escape-hatch env `ALLOW_DEV_AUTH_OVER_TUNNEL=true` is unset. Localhost-bound dev servers stay unaffected; tunneled / shared servers fail loud.

2. **Identity-drift warning banner.** Add a small client component that fires `/api/auth/debug` on every portal page mount and renders a red banner if `match === false` AND `goClaimsUserId === devUserPlaceholderId`. The endpoint already exists (`src/app/api/auth/debug/route.ts`) and returns 404 in production builds — banner only ever shows in dev/staging where the mismatch could happen. Visible to all users (so a teacher hitting weirdness can flag it), not just operators.

3. **Migration health check at startup.** Extend the Go startup sequence with a check that the latest expected migration has been applied. The simplest mechanism: at boot, query `SELECT MAX(version) FROM migrations_log` (or whatever Drizzle's schema-tracking table is) and compare against a baked-in `expectedSchemaVersion` constant. Mismatch → `slog.Error` + refuse to start. The constant is bumped manually each migration release; CI-side enforcement that the constant stays in sync with `drizzle/*.sql` is a Phase-3 follow-up.

4. **Realtime-config banner for live sessions.** When `/api/realtime/token` returns 503 ("Realtime tokens not configured"), the existing `useRealtimeToken` helper at `src/lib/realtime/get-token.ts:65-83` throws and the failure surfaces as a console error. Instead, render a small in-page banner on session/teacher-watch/parent-viewer pages: "Realtime is not configured for this environment. Live collaboration is unavailable. [retry]". The banner replaces the silent console drop and gives end users an actionable message.

## Decisions to lock in

1. **Escape hatch is `ALLOW_DEV_AUTH_OVER_TUNNEL=true`, not `APP_ENV=local`.** Reviewer suggested either; the explicit allowlist env is more searchable and less ambiguous (`APP_ENV` already has prod-vs-not-prod semantics from plan 050). Setting the escape hatch is a deliberate operator decision; defaulting to off catches the foot-gun.
2. **Bind-address detection: parse `cfg.Server.Host`.** If host is `127.0.0.1`, `::1`, or `localhost`, treat as localhost-only. If `0.0.0.0` or any explicit non-loopback address, treat as exposed. Unset host (which today defaults to `0.0.0.0` per `platform/internal/config/config.go:62`) counts as exposed — safer default.
3. **Identity-drift banner is dev/staging only.** It calls `/api/auth/debug` which 404s in production (`src/app/api/auth/debug/route.ts:24-26`). The banner component handles 404 by silently no-op'ing. Production users never see this banner.
4. **Schema-version constant lives in Go.** `platform/internal/db/version.go` exports `ExpectedSchemaVersion = "0024"` (matching the latest migration filename). Bumped by hand on each migration; a CI test compares against `drizzle/` directory contents.
5. **Realtime banner only when 503.** Other failure modes (network, 401, malformed response) keep the existing console-error path — those are bugs in the realtime mint flow, not config issues. The 503-specific banner avoids false positives.
6. **No retry button on the realtime banner v1.** The user has to refresh the page after the operator fixes the env. Adding a working retry requires re-running the auth check + remounting the WebSocket; defer to v2 if the friction matters.

## Files

### Phase 1 — `DEV_SKIP_AUTH` non-localhost guard

**Modify:**
- `platform/cmd/api/main.go` — extend `validateDevAuthEnv(getEnv)` to also take the bind host (or call into a separate validator). New logic: if `DEV_SKIP_AUTH != ""` AND host is non-localhost AND `ALLOW_DEV_AUTH_OVER_TUNNEL` is not `"true"`, return an error. Existing prod-guard logic stays. Wire the bind host through (cfg.Server.Host is loaded by line 36 of main.go).
- `platform/cmd/api/main_test.go` — extend `TestValidateDevAuthEnv` with table cases for the new combinations: localhost+DEV_SKIP_AUTH (allowed), 0.0.0.0+DEV_SKIP_AUTH (rejected), 0.0.0.0+DEV_SKIP_AUTH+escape hatch (allowed).
- `.env.example` — add `ALLOW_DEV_AUTH_OVER_TUNNEL=` with a comment explaining the escape hatch and warning against using it in shared/staging environments.
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

### Phase 3 — Schema-version startup check

**Add:**
- `platform/internal/db/version.go` — exports `ExpectedSchemaVersion` constant (current value: highest migration number under `drizzle/`). Includes a comment with bump procedure ("when adding a migration, update this constant in the same PR").
- `platform/internal/db/version_check.go` — `func CheckSchemaVersion(ctx context.Context, db *sql.DB) error`. Queries Drizzle's tracking table (verify name during impl — `__drizzle_migrations` or similar), reads the latest applied version, compares against `ExpectedSchemaVersion`. Returns descriptive error on mismatch.
- `platform/internal/db/version_check_test.go` — happy-path matches, mismatch returns clear error, missing tracking table returns "DB never initialized" error.

**Modify:**
- `platform/cmd/api/main.go` — call `db.CheckSchemaVersion` after `db.Open` succeeds. On error: `slog.Error` + `os.Exit(1)`. New behavior: refuses to start against a stale DB.

### Phase 4 — Realtime-config banner

**Add:**
- `src/components/realtime/realtime-config-banner.tsx` — client component. Wrapped by any session/teacher-watch/parent-viewer page that today uses `useRealtimeToken`. Catches the `RealtimeMintError` with `status === 503` and renders an in-page banner instead of letting the error bubble to the console.

  Banner copy: "Live collaboration is unavailable in this environment. The realtime token service is not configured. Static viewing still works; reload the page after the operator sets `HOCUSPOCUS_TOKEN_SECRET`."

**Modify:**
- `src/components/session/teacher/teacher-dashboard.tsx`, `src/components/session/student/student-session.tsx`, `src/components/parent/live-session-viewer.tsx`, `src/components/problem/teacher-watch-shell.tsx` — wrap the realtime-token consumer with `<RealtimeConfigBanner>`. The banner falls through to the children when the token mints OK; renders the banner instead when 503.

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

_(To be populated by Codex pass — see Phase 0.)_

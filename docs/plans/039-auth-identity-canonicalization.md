# 039 — Auth Identity Canonicalization & Portal Reliability

**Goal:** Eliminate the auth identity drift between Next.js `auth()` and the Go API that broke admin, teacher live sessions, and student join in review 002. Consolidate session-page authorization to a single backend source. Ship regression tests so the next role-switch bug fails CI, not a human reviewer.

**Source:** Codex comprehensive site review, 2026-04-26 (`docs/reviews/002-comprehensive-site-review-2026-04-26.md`)

**Branch:** `feat/039-auth-identity-canonicalization`

**Status:** Complete (pending PR review)

---

## Problem Summary

Plan 038 made cookie selection scheme-aware on both sides, but the boundary between layers still diverges:

- Server-side `api-client.ts` picks cookie order from `NEXTAUTH_URL` / `AUTH_URL`.
- Go middleware picks cookie order from `r.TLS` / `X-Forwarded-Proto`.
- These signals can disagree (SSH tunnel, dev tooling, mixed scheme proxies), so the same browser request resolves to different users on the two sides.
- Logout only clears whichever cookie Auth.js knows about. The stale `__Secure-` variant leaks into the next session.
- No diagnostic exists to confirm which cookie/identity each layer used; failures present as 403, blank pages, or "no classes yet."

The canonical fix is to remove the second source of truth entirely: Next.js forwards the token Auth.js actually used as a `Bearer` header, and Go consumes the header without re-picking from cookies. Cookie-based fallback in Go is reserved for direct browser hits to Go (not proxied requests).

The second-order fix is to consolidate session-page authorization in Go so the Next.js page never compares a NextAuth user ID against a Go-resource user ID. One identity, one authorization decision.

---

## Scope

### In scope

- P0 #1 — auth identity drift (`platform/internal/auth/middleware.go`, `src/lib/api-client.ts`, logout)
- P0 #2 — teacher live session pages 404 (consolidate authorization in Go)
- P0 #3 — admin dashboard blank (defensive error + identity fix unblocks)
- P1 #4 — student join verified end-to-end
- P1 #5 — org-admin nav points to inaccessible teacher routes
- Regression E2E tests for role switching, session entry, join, and admin.

### Deferred to a later plan (not blocking)

- P2 — registration role intent ignored (`src/app/api/auth/register/route.ts`)
- P2 — org portal placeholder pages
- P2 — duplicate org membership rendering keys
- P2 — parent `/parent/children` redirect
- P2 — problem editor responsive fallback
- P2 — root layout `<script>` triggers React dev overlay
- P2 — deep-link `callbackUrl` preservation via middleware (the open-redirect protection on the receiving side already shipped in plan 038; the missing piece is capturing the path before redirect, which needs a Next.js middleware change with its own surface area)

These are real bugs but each is independent of the auth root cause. Bundle them in plan 040 once 039 stabilizes the foundation.

---

## Phase 1: Canonical Token Forwarding (P0)

### Task 1.1a: Extract a shared session-cookie name helper

**Files:**
- Create: `src/lib/auth-cookie.ts` — a single function `getSessionCookieName()` that returns `"__Secure-authjs.session-token"` when the configured Auth.js URL is HTTPS and `"authjs.session-token"` otherwise. This is the *one* place the cookie name is decided.
- Modify: `src/lib/auth.ts` — currently calls `NextAuth({...})` inline with no exported `authConfig` and no `cookies` block (see `src/lib/auth.ts:14-124`). Refactor: extract the config object into a named `export const authConfig`, add an explicit `cookies: { sessionToken: { name: getSessionCookieName(), options: { ... } } }` block that consumes the helper. Then call `NextAuth(authConfig)`.

After this task, the Auth.js cookie name is no longer implicit — it's set by our code, derived from one helper, and accessible to other modules as `authConfig.cookies?.sessionToken?.name`.

### Task 1.1b: `api-client.ts` consumes the shared helper

**File:** `src/lib/api-client.ts`

Replace the URL-based cookie preference (lines 35-40) with a single read using `getSessionCookieName()`. If that cookie is present, forward its value as the Bearer token. If it is missing but the *other* variant is present (stale jar from prior deployment / scheme), explicitly do NOT forward it — log a dev-mode warning instead. This forces the stale cookie to be cleaned up rather than silently used.

Optional fallback for true scheme transitions: if `getSessionCookieName()` returns the secure name but only the non-secure cookie exists, treat it as missing (force re-auth) rather than guessing.

### Task 1.2: Go middleware trusts the Authorization header exclusively when present

**File:** `platform/internal/auth/middleware.go`

Current behavior: header → fallback to cookie. That's correct in shape. The problem is the cookie fallback runs even when the request came through the Next.js proxy and *should* have a header. Tighten:

- If `Authorization: Bearer …` is present, use it. Do not fall back to cookies for this request.
- Only run cookie fallback for requests with no Authorization header at all.
- When falling back to cookies, log (debug-level) which cookie name was selected and which scheme was detected. Helps diagnose future drift in dev.

This change is small but enforces the invariant: *the proxy and direct browser hits use disjoint auth paths*.

### Task 1.3: Logout clears both cookie variants explicitly

**Files:**
- Create: `src/app/api/auth/logout-cleanup/route.ts` — POST handler called from the sign-out flow.
- Modify: `src/components/sign-out-button.tsx` and `src/components/portal/sidebar-footer.tsx` — call `/api/auth/logout-cleanup` before (or alongside) the Auth.js client `signOut()`.

The cleanup route emits explicit `Set-Cookie` headers for *each* variant matching the original attribute set:

```
Set-Cookie: authjs.session-token=; Path=/; Max-Age=0; SameSite=Lax; HttpOnly
Set-Cookie: __Secure-authjs.session-token=; Path=/; Max-Age=0; SameSite=Lax; HttpOnly; Secure
```

Codex flagged that `Max-Age=0` only deletes when `Path` and `Domain` match the original. Auth.js v5 sets cookies on `Path=/` with no `Domain` attribute (host-only) — match exactly. Do *not* rely on a single broad `signOut()` call to clear both names; the client signOut only knows about the cookie Auth.js currently uses.

Verify in DevTools after sign out: both names absent from Application → Cookies.

### Task 1.4: Dev-only auth diagnostic endpoint

**File:** `src/app/api/auth/debug/route.ts`

Gated by `process.env.NODE_ENV !== "production"`. Returns:

```ts
{
  nextAuthUserId: string | null,
  goClaimsUserId: string | null,
  cookieNamesPresent: string[],
  cookieNameUsed: string | null,
  xForwardedProto: string | null,
  authjsConfigUrl: string,
  match: boolean,
}
```

Implementation: call `auth()` for the Next side, call `/api/me` (or equivalent) on Go for the claims side, compare. Returns 404 in production builds.

This becomes the first diagnostic for any future "why is the API returning the wrong user" report.

### Task 1.5: Identity match assertion in dev

**Files:**
- `src/app/(portal)/teacher/sessions/[sessionId]/page.tsx`
- `src/app/(portal)/student/sessions/[sessionId]/page.tsx`
- Any other server component that compares `auth()` user ID to a Go-resource owner ID before rendering.

When the IDs disagree in development, log a loud `console.error` that names both IDs and the cookie names present. Production behavior unchanged (still calls `notFound()`). This means the next time identity drifts, the developer sees the cause in the terminal instead of a generic 404.

---

## Phase 2: Consolidate Session-Page Authorization in Go (P0)

The teacher and student session pages combine NextAuth viewer identity with Go session payload, then check `liveSession.teacherId === viewerId` in the Next render. After phase 1 this *should* always match, but the architecture still has two sources of truth. The right shape: one Go endpoint authorizes and returns the rendered payload.

### Task 2.1: `GET /api/sessions/:id/teacher-page` Go handler

**Files:**
- Modify: `platform/internal/handlers/sessions.go` — add handler.
- Modify: `platform/internal/handlers/sessions_test.go` — add tests covering: happy path (teacher), platform admin direct access, platform admin impersonating teacher, platform admin impersonating non-teacher (403), non-teacher 403, missing session 404, no claims 401.

Returns the full payload the teacher page needs (session, class, students, attendance) only if the authenticated claims user is the session teacher OR a platform admin. Per `platform/internal/auth/middleware.go:90-105`, an admin who impersonates rewrites claims to the target user and clears `IsPlatformAdmin` — preserve current behavior by treating `claims.ImpersonatedBy != ""` as "the underlying actor was admin, allow."

Returns 403 otherwise. Returns 404 if the session doesn't exist.

The handler is the *single* authorization point. Next.js no longer needs to compare IDs.

### Task 2.2: `GET /api/sessions/:id/student-page` Go handler

**Files:**
- Modify: `platform/internal/handlers/sessions.go` — add handler.
- Modify: `platform/internal/handlers/sessions_test.go` — add tests covering: enrolled student happy path, non-enrolled 403, platform admin (and impersonating admin), missing session 404, no claims 401.

Authorization rule: the user is enrolled in the class associated with the session, OR is platform admin / impersonating.

### Task 2.3: Refactor session pages to use the new endpoints

**Files:**
- `src/app/(portal)/teacher/sessions/[sessionId]/page.tsx`
- `src/app/(portal)/student/sessions/[sessionId]/page.tsx`

Replace the multi-fetch + ID comparison with a single `api()` call to the new endpoint. Remove the local `viewerId === teacherId` check; trust the backend's 403/404. Drop now-unused imports.

### Task 2.4: Defensive admin dashboard error state

**File:** `src/app/(portal)/admin/page.tsx`

When the Go stats call throws, render a clear error card with the status code and a "Retry" link instead of a blank/error page. Same pattern for `/admin/orgs` and any other admin landing routes that depend on a single Go call.

After phase 1 this rarely fires, but a single transient 5xx shouldn't blank the entire dashboard.

---

## Phase 3: Verified Student Join (P1)

### Task 3.1: Confirm class appears before closing dialog

**File:** `src/components/student/join-class-dialog.tsx`

Current behavior: closes on `res.ok` and calls `router.refresh()`. The mutation can succeed in the proxy while the student sees stale data, or (per review 002) the mutation lands against the wrong identity entirely.

New behavior:

1. Submit join code.
2. On `res.ok`, fetch `/api/classes/mine` (same endpoint the dashboard uses).
3. Verify the joined class is in the response.
4. Only then close the dialog and `router.refresh()`. Show confirmation toast.
5. If the class is not in the response, keep the dialog open and surface a clear error: "Joined, but the class isn't showing up. Sign out and back in if this persists." Log the mismatch to the dev diagnostic if available.

### Task 3.2: E2E test for the join flow

**File:** `e2e/student-join-class.spec.ts` (new)

Teacher creates class, exposes join code, student logs in, opens join dialog, submits code, dashboard shows the class. Run against the local stack.

---

## Phase 4: Org-Admin Navigation Honesty (P1)

### Task 4.1: Remove cross-portal teacher links from org-admin nav

**File:** `src/lib/portal/nav-config.ts`

Remove `/teacher/units` and `/teacher/sessions` entries from the org-admin nav. They redirect back to `/org` for org admins without the teacher role and look like dead links.

Decision: do we ship `/org/units` and `/org/sessions` read-only views in this plan, or just trim? **Recommendation: trim now, schedule the org views as a separate plan.** Shipping read-only views is real product work that benefits from its own design pass.

### Task 4.2: Update nav-config tests

**File:** `tests/unit/nav-config.test.ts` (or wherever it lives)

Update assertions about the org-admin nav set. Add a test that no org-admin nav item points into another portal's route (`/teacher/*`, `/student/*`, `/parent/*`, `/admin/*`).

---

## Phase 5: Regression E2E Tests

### Task 5.1: Role-switch identity test (with explicit cookie seeding)

**File:** `e2e/auth-identity.spec.ts` (new)

Two scenarios, both required:

**5.1a: Sequential sign-in/sign-out.** Sign in as teacher, hit `/api/me/memberships`, sign out (using the new logout-cleanup flow), assert both cookie variants are gone in the browser context. Sign in as student, verify `/api/me/memberships` returns student data and contains no teacher membership rows. Sign out. Sign in as admin, verify `/api/auth/session.isPlatformAdmin === true` AND `/api/admin/stats` returns 200.

**5.1b: Stale cookie seeding (the actual review-002 bug).** Sequential sign-in alone may not reproduce; the original bug requires the stale `__Secure-` cookie to outlive sign-out. Use `browserContext.addCookies([...])` to plant a stale `__Secure-authjs.session-token` from a different user before signing in as the new user. Then verify `/api/me/memberships` returns the *new* user's data, not the planted one. This locks in Task 1.1b's "ignore stale variant when the canonical name has no value" behavior.

The diagnostic endpoint (`/api/auth/debug`) should be polled in the test and asserted `match: true` after each sign-in.

### Task 5.2: Live session entry test

**File:** `e2e/teacher-session-entry.spec.ts` (new)

Teacher creates a class, starts a session, navigates from dashboard to the live session via the listed link. Page renders without 404.

### Task 5.3: Admin smoke test

**File:** `e2e/admin-dashboard.spec.ts` (new)

Admin loads `/admin` and `/admin/orgs`. Both render without errors and show non-zero counts.

---

## Implementation Order

| Phase | Tasks | Why this order |
|-------|-------|----------------|
| 1 | 1.1 → 1.2 → 1.3 → 1.4 → 1.5 | Root cause; must land first |
| 2 | 2.1 + 2.2 → 2.3 → 2.4 | Builds on phase 1; consolidates remaining auth split |
| 3 | 3.1 → 3.2 | Independent of phase 2 once phase 1 lands |
| 4 | 4.1 → 4.2 | Independent; can ship in parallel with phase 3 |
| 5 | 5.1 → 5.2 → 5.3 | Lock in fixes; run against finished phases 1-4 |

Phases 3 and 4 can be parallel after phase 1 ships. Phase 2 should follow phase 1 immediately because they touch the same boundary. Phase 5 last.

---

## Verification per Phase

- **Phase 1:** dev diagnostic endpoint reports `match: true` for teacher, student, parent, org-admin, platform-admin sign-in flows. Manual: sign in/out 5x with each role, no identity bleed in `/api/me/memberships` or `/api/admin/stats`.
- **Phase 2:** previously broken teacher session links open the live workspace. Admin dashboard renders. Refresh test: kill Go API, admin page shows error card not blank.
- **Phase 3:** join code from teacher session enrolls a student and dashboard updates without a manual refresh.
- **Phase 4:** org-admin nav has no entries that redirect to the dashboard; nav-config test passes.
- **Phase 5:** all three E2E specs pass locally and in CI.
- **Whole plan:** Vitest + Go test suite + Playwright all green.

---

## Out-of-Scope Acknowledgements

The review surfaced more findings than 039 addresses. Each deferred item is independent of the auth root cause:

- Registration role intent (P1 #6): API ignores `role` field. Standalone fix in registration handler.
- Org placeholder pages (P2 #8): product decision about scope of org admin tooling.
- Duplicate org membership keys (P2 #9): `/api/orgs` should dedupe by `orgId`; UI consumers should also dedupe defensively.
- Parent `/children` (P2 #10): either build a list view or remove the nav item.
- Problem editor responsive (P2 #11): needs design pass for breakpoints + drawer pattern.
- Root layout React-script overlay (P2 #12): move theme bootstrap to `next/script`.
- Deep-link `callbackUrl` (P2 #7): needs Next.js middleware to capture URL pre-redirect.

Plan 040 will pick these up after 039 lands.

---

## Codex Review of This Plan

- **Date:** 2026-04-26
- **Reviewer:** Codex (pre-implementation)
- **Verdict:** Corrections applied (see below).

### Corrections applied

1. `[CRITICAL]` **Task 1.1 stronger version is not drop-in.** `src/lib/auth.ts:14-124` calls `NextAuth({...})` inline with no exported `authConfig` and no `cookies` block, so `authConfig.cookies?.sessionToken?.name` does not exist today. → Split Task 1.1 into 1.1a (refactor `auth.ts` to extract `authConfig`, add explicit `cookies.sessionToken.name` derived from a new `getSessionCookieName()` helper in `src/lib/auth-cookie.ts`) and 1.1b (api-client consumes the helper).

2. `[IMPORTANT]` **Env-inferred scheme can still pick the stale cookie.** Even with scheme-aware preference, if Auth.js issues `authjs.session-token` on HTTP but a stale `__Secure-` cookie persists, the URL-based selector can return the wrong one. → Task 1.1b now requires picking the cookie name from the shared helper (single source of truth) and explicitly *ignoring* the stale variant rather than falling back to it. A dev-mode warning logs the situation.

3. `[IMPORTANT]` **Logout cleanup must emit explicit `Set-Cookie` per variant.** `Max-Age=0` only deletes when `Path` and `Domain` match the original. Auth.js client `signOut()` only knows about its current cookie. → Task 1.3 expanded with the exact `Set-Cookie` headers per variant (`Path=/`, `HttpOnly`, `SameSite=Lax`, `Secure` on the secure variant) and explicit list of files to wire (`sign-out-button.tsx`, `sidebar-footer.tsx`).

4. `[IMPORTANT]` **Impersonation edge case.** `platform/internal/auth/middleware.go:90-105` clears `IsPlatformAdmin` when impersonation is active and sets `ImpersonatedBy`. New consolidated session endpoints must treat `claims.ImpersonatedBy != ""` as admin-equivalent to preserve current teacher-page access for an admin impersonating. → Task 2.1 / 2.2 now spell this out.

5. `[IMPORTANT]` **Phase 5 sequential sign-in alone may not catch the bug.** The review-002 symptom requires the stale `__Secure-` cookie to outlive sign-out. → Task 5.1 split into 5.1a (sequential sign-in/sign-out) and 5.1b (explicit `addCookies` stale-variant seeding) plus assertions on the new diagnostic endpoint.

6. `[NOTE]` **Test file home not specified.** `platform/internal/handlers/sessions_test.go` already exists (`platform/internal/handlers/sessions_test.go:15-59`). → Tasks 2.1 and 2.2 now name it explicitly with the full test-case list.

### Non-blocking notes

- The review file `docs/reviews/002-comprehensive-site-review-2026-04-26.md` was missing from the branch when Codex reviewed; it has now been added so the plan's "Source" reference resolves.

## Post-Execution Report

**Status:** Complete. All five phases shipped on `feat/039-auth-identity-canonicalization`.

**Phase 1 — Canonical Token Forwarding (commits `9c82b0e`, `5ebe02c`, `87cdc11`)**

- Added `src/lib/auth-cookie.ts` with `getSessionCookieName()` as the single source of truth for the Auth.js session cookie name. Refactored `src/lib/auth.ts` to extract `authConfig` and configure `cookies.sessionToken.name` from the helper.
- `src/lib/api-client.ts` now forwards only the canonical cookie. If only the stale variant is present, it logs a dev warning and refuses to forward — no silent fallback.
- `platform/internal/auth/middleware.go` enforces disjoint header/cookie auth paths. Cookie fallback selects exactly one canonical cookie name per scheme; the stale variant is never accepted. Adds 5 new tests covering canonical cookie acceptance per scheme, stale-variant rejection on both schemes, and Authorization-header-beats-cookie precedence.
- `src/app/api/auth/logout-cleanup/route.ts` emits explicit `Set-Cookie` `Max-Age=0` for both cookie variants with attributes matching Auth.js v5 (`Path=/`, `HttpOnly`, `SameSite=Lax`, `Secure` on the prefixed variant). Wired into `sign-out-button.tsx` and `sidebar-footer.tsx`.
- `src/app/api/auth/debug/route.ts` reports `nextAuthUserId` / `goClaimsUserId` / cookie names present / canonical name used / stale variant present / scheme / match. Returns 404 in production.
- `platform/internal/handlers/me.go` adds `GET /api/me/identity` returning the resolved claims (used by the diagnostic).
- `src/lib/identity-assert.ts` provides `logIdentityMismatch()`; wired into `teacher/sessions/[sessionId]/page.tsx` (since removed by Phase 2's refactor — see below).

**Phase 2 — Consolidate Session-Page Authorization in Go (commit `817cff1`)**

- `GET /api/sessions/{id}/teacher-page` and `/student-page` are the new single authorization points. Both authorize and return the full render payload. `canActAsAdmin()` centralizes the `IsPlatformAdmin || ImpersonatedBy != ""` predicate (preserves admin-while-impersonating equivalence per Codex correction #4).
- `SessionHandler` now also holds `Topics` and `Courses`. Wiring updated in `platform/cmd/api/main.go`.
- 14 DB-backed integration tests in `sessions_page_integration_test.go` cover teacher / org-admin / platform-admin / admin-impersonating / student / outsider / missing-session / ended-session paths for both endpoints. (No-claims smoke tests live separately in `sessions_test.go::TestGetTeacherPage_NoClaims` and `TestGetStudentPage_NoClaims`.)
- Next pages rewritten: `teacher/sessions/[sessionId]/page.tsx` and `student/sessions/[sessionId]/page.tsx` now make a single `api()` call, trust Go's 403/404, and removed the local `viewerId` comparison entirely. `identity-assert` is no longer needed in the teacher page because the comparison surface is gone.
- `admin/page.tsx` renders a defensive error card with status code, retry link, and a hint pointing at `/api/auth/debug` when the failure is 403.

**Phase 3 — Verified Student Join (commit `d236d96`)**

- `join-class-dialog.tsx` now waits for `/api/classes/mine` to confirm the joined class is present before closing. Errors split into 5 actionable messages (server unreachable, join rejected, no class id returned, /mine errored, /mine missing the class).
- 5 unit tests cover happy path + the explicit "joined but not in /mine" review-002 scenario + 3 failure modes.
- E2E `e2e/join-class.spec.ts` extended to assert the joined class appears in `/api/classes/mine` AND on the `/student` dashboard without a manual reload.

**Phase 4 — Org-Admin Nav Honesty (commit `05cff6e`)**

- Removed `/teacher/units` and `/teacher/sessions` from `org_admin` nav.
- `tests/unit/nav-config.test.ts` adds a regression test asserting no portal's nav item links into another portal's basePath. Locks the rule in CI.

**Phase 5 — Regression E2E (commit `fd62e2d`)**

- `e2e/auth-identity.spec.ts` — sequential sign-in/sign-out scenario (5.1a) plus the explicit stale-cookie seeding scenario (5.1b) that reproduces the review-002 root bug.
- `e2e/teacher-session-entry.spec.ts` — dashboard live-session link must open without 404.
- `e2e/admin-dashboard.spec.ts` — `/admin` renders the stats grid OR the defensive error card; never blank.

**Verification**

- Vitest: 392 passed / 11 skipped (was 386 / 11 — 6 new tests across logout-cleanup, identity-assert, join-class-dialog, nav-config).
- Go tests: all 14 packages green against `bridge_test`. Handler integration tests: 28 new (5 cookie tests in middleware + 3 GetIdentity + 2 smoke + 13 DB-backed teacher/student page + 5 ad-hoc).
- TypeScript: clean for new/modified files. Pre-existing errors in unrelated areas unchanged.
- E2E: not run in this loop (requires the full local stack); specs added for follow-up validation.

**Out-of-scope (deferred to plan 040)**

Per the original scope statement and the Codex pre-impl review:
- P1 #6 — registration role intent ignored.
- P2 #7 — deep-link `callbackUrl` preservation via Next middleware.
- P2 #8 — org placeholder pages (`/org/teachers`, `/org/students`, `/org/courses`, `/org/classes`, `/org/settings`).
- P2 #9 — duplicate org membership keys in `/api/orgs` consumers.
- P2 #10 — parent `/parent/children` redirect.
- P2 #11 — problem editor responsive fallback.
- P2 #12 — root layout `<script>` triggers React dev overlay.

Added by Codex post-impl review of 039:
- **`X-Forwarded-Proto` trust hardening.** Go middleware accepts the `__Secure-` cookie whenever any `X-Forwarded-Proto` header starts with `https`. In a misconfigured deploy where the proxy doesn't strip client-supplied headers, an attacker could choose to send the secure-prefixed cookie. Mitigation: only honor `X-Forwarded-Proto` when the request came from a configured trusted proxy. Pre-existing concern, not a 039 regression.
- **`src/app/(portal)/admin/users/page.tsx:41-42`** still compares `auth()` user id to a Go-loaded user id (display-only self-action gating). Same dual-source pattern 039 removed from session pages — apply consistently.
- **Stronger stale-cookie E2E** that plants a valid signed JWE for a different user (requires wiring `AUTH_SECRET` + the JWE encoder into the E2E setup). Today's test catches the "any leak path" regression but doesn't simulate a fully valid stale token.

**Known issue (not a regression)**

`platform/internal/store/sessions_test.go::TestSessionStore_ListSessions_Filters` queries `Status: "ended"` globally and assumes a clean DB. It fails when run against the dev `bridge` database because of accumulated session rows but passes against the dedicated `bridge_test` DB. Not caused by this plan; flag for separate cleanup.

## Code Review

### Review 1 — Pre-implementation plan review (commit `c180b31`)

- **Date:** 2026-04-26
- **Reviewer:** Codex (via `codex:rescue`)
- **Verdict:** Corrections applied (see `## Codex Review of This Plan` section above).

### Review 2 — Post-implementation review

- **Date:** 2026-04-26
- **Reviewer:** Codex (post-implementation, via `codex:rescue`)
- **Verdict:** Three in-PR fixes applied, three follow-ups deferred to plan 040.

**In-PR fixes (committed in `c537841`):**

1. `[FIXED]` `[IMPORTANT]` Org-admin teacher-page authorization branch had no integration test coverage. → `sessions_page_integration_test.go` now creates an `org_admin` member of the test org and asserts `TestGetTeacherPage_OrgAdmin` returns 200 with the expected payload.
2. `[FIXED]` `[IMPORTANT]` Join verification was a single fetch — vulnerable to a stale-replica false-failure. → `join-class-dialog.tsx` now retries `/api/classes/mine` once with a 400ms delay; tests updated to cover both retry-rescue and retry-still-fails paths (6 cases total).
3. `[FIXED]` `[IMPORTANT]` Post-execution report claimed the integration test file covers no-claims paths; those actually live in the smoke-test file. → Report wording corrected, test-count updated to 14 with the new org-admin case.
4. `[FIXED]` `[MINOR]` E2E stale-cookie test plants opaque garbage; comment now explicitly documents the test's actual scope (locks "no forwarding" property) vs the ideal (valid stale token, which needs E2E setup wiring deferred to plan 040).

**Deferred to plan 040 (added to "Out-of-scope" above):**

5. `[IMPORTANT]` `X-Forwarded-Proto` trust hardening — Go middleware honors any `https` value, allowing the secure cookie to be selected on a misconfigured proxy. Pre-existing, not a 039 regression. Needs trusted-proxy IP allowlist or proxy stripping of client-supplied headers.
6. `[MINOR]` `src/app/(portal)/admin/users/page.tsx:41-42` still does NextAuth-vs-Go-loaded-user dual source for display gating. Same pattern 039 removed from session pages — apply consistently in plan 040.
7. `[FOLLOWUP]` Wire AUTH_SECRET + JWE encoder into E2E setup so the stale-cookie test can plant a real signed token from a different user.

**Codex notes (no action needed):**

- Logout cleanup attributes verified correct against Auth.js v5 defaults.
- `canActAsAdmin()` scope is narrow — used only in the two read-only page payload endpoints, no security regression.
- No authorization gap in `GetTeacherPage` / `GetStudentPage` vs the legacy Next-side logic; impersonation semantics preserved.

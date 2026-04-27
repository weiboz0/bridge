# 039 — Auth Identity Canonicalization & Portal Reliability

**Goal:** Eliminate the auth identity drift between Next.js `auth()` and the Go API that broke admin, teacher live sessions, and student join in review 002. Consolidate session-page authorization to a single backend source. Ship regression tests so the next role-switch bug fails CI, not a human reviewer.

**Source:** Codex comprehensive site review, 2026-04-26 (`docs/reviews/002-comprehensive-site-review-2026-04-26.md`)

**Branch:** `feat/039-auth-identity-canonicalization`

**Status:** Draft — awaiting approval

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

### Task 1.1: `api-client.ts` forwards the token Auth.js actually used

**File:** `src/lib/api-client.ts`

Replace the URL-based cookie preference with a content-aware rule: read both cookie names, pick whichever Auth.js wrote. If both are present, prefer the one whose value decrypts under the current secret (Auth.js only ever writes one). If only one is present, use it.

The simplest deterministic rule that works without re-running JWT decrypt: prefer the cookie name that matches the Auth.js URL config, but fall back to the *only* cookie present when the preferred one is missing. This is the existing scheme rule plus an explicit fallback when the preferred cookie isn't there at all.

**Stronger version (preferred):** consult the Auth.js cookie name from its config (`authConfig.cookies?.sessionToken?.name`) so the source of truth is one place, not two.

### Task 1.2: Go middleware trusts the Authorization header exclusively when present

**File:** `platform/internal/auth/middleware.go`

Current behavior: header → fallback to cookie. That's correct in shape. The problem is the cookie fallback runs even when the request came through the Next.js proxy and *should* have a header. Tighten:

- If `Authorization: Bearer …` is present, use it. Do not fall back to cookies for this request.
- Only run cookie fallback for requests with no Authorization header at all.
- When falling back to cookies, log (debug-level) which cookie name was selected and which scheme was detected. Helps diagnose future drift in dev.

This change is small but enforces the invariant: *the proxy and direct browser hits use disjoint auth paths*.

### Task 1.3: Logout clears both cookie variants

**Files:**
- `src/app/api/auth/[...nextauth]/route.ts` (or wherever Auth.js handlers are wired)
- New: `src/app/api/auth/logout-cleanup/route.ts` (called from logout flow)

After Auth.js completes signOut, expire both `authjs.session-token` and `__Secure-authjs.session-token` with `Max-Age=0`, `Path=/`, and the `Secure` attribute on the secure variant. Without this, a cookie left over from a previous deployment or scheme persists across sessions and re-injects a stale identity on the next login.

Verify by inspecting `document.cookie` and the network tab after sign out — both names should be gone.

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

**File:** `platform/internal/handlers/sessions.go`

Returns the full payload the teacher page needs (session, class, students, attendance) only if the authenticated claims user is the session teacher (or a platform admin / impersonating). Returns 403 otherwise. Returns 404 if the session doesn't exist.

The handler is the *single* authorization point. Next.js no longer needs to compare IDs.

### Task 2.2: `GET /api/sessions/:id/student-page` Go handler

**File:** `platform/internal/handlers/sessions.go`

Same shape, authorization rule: the user is enrolled in the class associated with the session.

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

### Task 5.1: Role-switch identity test

**File:** `e2e/auth-identity.spec.ts` (new)

Sign in as teacher, verify `/api/me/memberships` returns teacher data. Sign out. Sign in as student, verify `/api/me/memberships` returns student data and contains *no* teacher fields. Sign out. Sign in as admin, verify `/api/auth/session.isPlatformAdmin === true` AND `/api/admin/stats` returns 200.

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

_To be added after the plan is dispatched to Codex via `/codex:rescue`._

# 040 — Auth Hardening & Workflow Gaps

**Goal:** Pick up the items deferred from plan 039 — security hardening on the cookie auth boundary, fix-once workflow gaps surfaced in Codex review 002 (registration intent, deep-link preservation, duplicate org rows, parent children, theme script overlay), and tighten the admin/users dual-source pattern. No new product surface area; defer org placeholder pages and editor responsive layout to their own plans (041, 042).

**Source:**
- Codex comprehensive site review, 2026-04-26 (`docs/reviews/002-comprehensive-site-review-2026-04-26.md`)
- Codex post-implementation review of plan 039 — three deferrals documented in `docs/plans/039-auth-identity-canonicalization.md` ("Out-of-scope" + Review 2 / Review 3 sections).

**Branch:** `feat/040-auth-hardening-workflow-gaps`

**Status:** Draft — awaiting approval

---

## Problem Summary

Plan 039 closed the auth identity drift root cause but left three follow-ups in the auth space and six independent UX gaps. Each is small on its own and they don't share architecture; they're bundled here because a single plan keeps the tracking simple and lets us ship one PR rather than seven. If any phase grows during implementation, it gets pulled into its own plan rather than ballooning this one.

---

## Scope

### In scope

**Auth hardening (deferred from 039):**
1. `X-Forwarded-Proto` trust hardening — only honor the header from configured trusted proxies.
2. `admin/users` page dual-source cleanup — stop comparing `auth()` user to Go-loaded user IDs.
3. Valid-signed-stale-token E2E — wire `AUTH_SECRET` into the E2E setup so the stale-cookie test plants a real signed token from a different user, not opaque garbage.

**Workflow gaps (review 002 deferrals):**
4. P1 #6 — registration role intent currently ignored by `/api/auth/register`. Either persist it and seed onboarding, or remove the selector.
5. P2 #7 — deep-link `callbackUrl` preservation. The redirect-to-login flow can't read the original path because `x-invoke-path` / `x-url` headers aren't always present; needs a Next.js `middleware.ts`.
6. P2 #9 — `/api/orgs` returns one row per (org × role); UI consumers render `key={orgId}` and React errors. Dedupe at API + add defensive dedupe at consumers.
7. P2 #10 — parent `/parent/children` redirects to `/parent`. Either build a real list view or remove the nav entry.
8. P2 #12 — root layout `<script dangerouslySetInnerHTML>` triggers a React dev-overlay error on every route. Move theme bootstrap to `next/script` with the right strategy.

### Deferred

- **Plan 041 — Org portal pages.** P2 #8 — `/org/teachers`, `/org/students`, `/org/courses`, `/org/classes`, `/org/settings` all render placeholder text. Real product work that needs its own design pass for read-only list views (or scope them out of nav until reduced to dashboard-only).
- **Plan 042 — Problem editor responsive layout.** P2 #11 — fixed three-pane layout with `min-w-[360px]` left + `min-w-[320px]` right inside `overflow-hidden`. Needs a design pass for breakpoint-driven drawer pattern.

---

## Phase 1: Trusted-proxy `X-Forwarded-Proto` (Auth Hardening)

### Task 1.1: Add `TRUSTED_PROXY_CIDRS` env var + helper

**Files:**
- Create: `platform/internal/auth/proxy.go` — `IsTrustedProxy(remoteAddr string) bool` reads a comma-separated list of CIDR ranges from `TRUSTED_PROXY_CIDRS` (defaults to empty). Returns true when the remote IP falls in any range. Uses `net.ParseCIDR`.
- Modify: `platform/internal/auth/middleware.go` (`canonicalCookieName()` at lines 14-24) — only treat `X-Forwarded-Proto` as authoritative when `IsTrustedProxy(r.RemoteAddr)` returns true. Otherwise rely on `r.TLS != nil` only.
- New tests in `platform/internal/auth/middleware_test.go`: spoofed `X-Forwarded-Proto: https` from an untrusted source must NOT pick the secure cookie name; the same header from a trusted proxy must.

For local dev: leave `TRUSTED_PROXY_CIDRS` empty so direct `r.TLS == nil` requests default to the HTTP cookie name. For prod: set the load balancer's CIDR.

### Task 1.2: Document the env var

**Files:**
- Modify: `docs/setup.md` — add `TRUSTED_PROXY_CIDRS` to the env list with the deployment guidance.
- Modify: `.env.example` (if present) — add the var with empty default and a comment.

---

## Phase 2: Admin Users Page Dual-Source Cleanup

### Task 2.1: Drop `auth()` from `/admin/users`

**File:** `src/app/(portal)/admin/users/page.tsx`

Replace `const session = await auth()` (line 14) with a single Go call. The page already fetches `/api/admin/users` (line 15) — extend that endpoint, or rely on the existing `/api/me/identity` from plan 039, to determine the current admin's user id from the same source.

Implementation: prepend `/api/me/identity` fetch. Compare `userList.id !== identity.userId` instead of `session?.user?.id` (line 41). One identity source — same pattern as the 039 session pages.

### Task 2.2: Update tests

**File:** `tests/integration/admin-users-page.test.ts` (new, if not already present) — assert the page hides actions for the current admin's row and shows them for everyone else, using a mocked `/api/me/identity` response.

---

## Phase 3: Valid-Signed-Stale-Token E2E

### Task 3.1: Expose `AUTH_SECRET` to E2E setup

**Files:**
- Modify: `e2e/playwright.config.ts` — pass `AUTH_SECRET` and `AUTH_URL` from the host environment into the test fixtures.
- Create: `e2e/helpers/jwe.ts` — small helper that wraps the same `jose` JWE encoder Auth.js uses and produces a valid `__Secure-authjs.session-token` value for a given `{ sub, email, name }`. The salt is the cookie name; HKDF derives the key from `AUTH_SECRET`. Mirrors the algorithm in `platform/internal/auth/jwt.go::DecryptAuthJSToken` so a token minted by this helper is decryptable by the Go middleware.

### Task 3.2: Strengthen `e2e/auth-identity.spec.ts` (5.1b)

Replace the opaque-garbage stale cookie value with a valid signed token for a non-existent / different user. Assert that after signing in as the live student, `/api/me/identity` returns the live student — not the planted token's `sub`. This proves Go's canonical-cookie selection ignores a fully valid stale variant.

Keep the original assertion (no leak path) intact.

---

## Phase 4: Registration Role Intent

Two approaches considered. **Recommendation: persist the role** because the UI already asks the question and discarding it has no upside. The alternative — remove the selector and route everyone through `/onboarding` — gives up information that the user typed.

### Task 4.1: Schema — add `intended_role` to users

**Files:**
- Create: `drizzle/00XX_user_intended_role.sql` — `ALTER TABLE users ADD COLUMN intended_role TEXT NULL CHECK (intended_role IN ('teacher','student'))`
- Modify: `src/lib/db/schema.ts` — add the column.
- Run: `bun run db:generate` and apply the migration to `bridge` + `bridge_test`.

### Task 4.2: Persist on register

**File:** `src/app/api/auth/register/route.ts`

Extend the zod schema to accept `role: z.enum(["teacher", "student"]).optional()`. Persist it on `users.intendedRole`. The schema is permissive on absent role to avoid breaking other callers.

### Task 4.3: Read in onboarding

**Files:**
- Modify: `src/app/(portal)/onboarding/page.tsx` (locate it first; `find src/app -name "onboarding"` if needed) — read `users.intendedRole` for the current user. If `teacher`, route to `/register-org`. If `student`, show the "your teacher will add you" copy. Removes the fork from "I always assume student first" → "use the explicit signal."

### Task 4.4: Tests

- Vitest integration in `tests/integration/auth-register.test.ts` — `role: "teacher"` and `role: "student"` are persisted; absent role still works (BC).
- Unit test for the onboarding page if one exists; otherwise add a smoke test.

---

## Phase 5: Deep-Link `callbackUrl` Preservation

### Task 5.1: Add Next.js root middleware

**File:** Create `src/middleware.ts`

Captures the request URL pre-redirect by setting `x-invoke-path` (the header `portal-shell.tsx:22` already reads). Use `NextResponse.next()` and pass headers through.

```ts
import { NextRequest, NextResponse } from "next/server";

export function middleware(req: NextRequest) {
  const headers = new Headers(req.headers);
  headers.set("x-invoke-path", req.nextUrl.pathname + req.nextUrl.search);
  return NextResponse.next({ request: { headers } });
}

export const config = {
  matcher: ["/((?!_next/|api/|favicon|.*\\.).+)"],
};
```

### Task 5.2: Verify portal-shell consumes the header

**File:** `src/components/portal/portal-shell.tsx:22`

Existing code already reads `x-invoke-path`; with the middleware in place the deep-link case stops being lossy. Open redirect protection (added in plan 038's review) still applies on the receiving side.

### Task 5.3: E2E for deep-link

**File:** `e2e/deep-link.spec.ts` (new)

Open `/teacher/classes/<some-uuid>` while signed out. Assert redirect to `/login?callbackUrl=%2Fteacher%2Fclasses%2F<uuid>`. Sign in. Assert landing on the original deep link.

---

## Phase 6: Org Membership Dedup

### Task 6.1: Backend dedup on the API edge

**File:** `platform/internal/handlers/orgs.go`

Find the handler that backs `/api/orgs` (likely `ListMyOrgs` or similar). Change the response shape from `[membership_row]` to `[{ orgId, name, slug, status, roles: [...] }]` — one entry per distinct org with roles attached. Or add a `/api/orgs/distinct` variant if a wholesale shape change is too invasive. Decision in the plan implementation: pick the lower-blast-radius option after grepping consumers.

### Task 6.2: Frontend defensive dedup

**Files:**
- Modify: `src/app/(portal)/teacher/units/new/page.tsx:40-69` — dedupe `data` by `orgId` before `setOrgs`.
- Modify: `src/app/(portal)/teacher/units/page.tsx` (the same `loadOrgs` pattern around line 135).
- Any other consumer surfaced by `grep -rn "/api/orgs" src/`.

Defensive even after the backend fix because rolling deploys can serve old data.

### Task 6.3: Test

- Vitest: render the units/new page with a mock that returns duplicate rows; assert no duplicate option keys (no React warning).

---

## Phase 7: Parent `/parent/children`

### Task 7.1: Decision — list view or trim

**Recommendation: trim now, defer the list view to the parent dashboard plan when one exists.** A real children list needs to know what data we want to show (latest activity? grade summary? linked classes?), and that's a product question beyond review-002.

### Task 7.2: Implementation

**Files:**
- Delete: `src/app/(portal)/parent/children/page.tsx` (the redirect-only file).
- Modify: `src/lib/portal/nav-config.ts` (`parent` block) — remove the `My Children` entry.
- Modify: `tests/unit/nav-config.test.ts` — assert `parent.navItems.length === 1` (Dashboard only).

---

## Phase 8: Root Layout Theme Script

### Task 8.1: Move the inline `<script>` to `next/script`

**File:** `src/app/layout.tsx:23-46`

Replace the raw `<script dangerouslySetInnerHTML>` with `next/script` using `strategy="beforeInteractive"`. The strategy guarantees the script runs before any client React tree renders, preserving the no-FOUC behavior.

```tsx
import Script from "next/script";

const themeScript = `(function(){var t=localStorage.getItem('bridge-theme')||'light';if(t==='dark')document.documentElement.classList.add('dark');else document.documentElement.classList.remove('dark');})()`;

// In the body, before SessionProvider:
<Script id="bridge-theme-bootstrap" strategy="beforeInteractive">
  {themeScript}
</Script>
```

The dev-overlay error ("Encountered a script tag while rendering React component…") goes away because `next/script` is the supported pattern.

### Task 8.2: Manual verification + smoke E2E

**File:** `e2e/theme-bootstrap.spec.ts` (new)

Open `/` with `localStorage.setItem('bridge-theme','dark')` pre-seeded; assert `<html class>` includes `dark` on initial paint without a flash. (Hard to assert "no flash" reliably in headless; minimum assertion: dark class present after load.)

---

## Implementation Order

| Phase | Tasks | Why |
|-------|-------|-----|
| 1 | Trusted proxy XFP | Smallest auth hardening; pure backend, isolated |
| 2 | Admin users dual-source | Tiny one-page change; uses /api/me/identity from 039 |
| 3 | Stronger E2E | Builds on phases 1-2; finishes the auth hardening track |
| 4 | Registration intent | Schema + API + onboarding; standalone |
| 5 | Deep-link middleware | One new file (`middleware.ts`); standalone |
| 6 | Org membership dedup | Touches multiple consumers; do after the simpler items |
| 7 | Parent /children trim | Trivial deletion + nav update |
| 8 | Theme script | Layout-touching but isolated |

Phases are independent; could be split into multiple PRs if any individual phase grows. Default plan: one PR, eight commits.

---

## Verification per Phase

- **Phase 1:** `TRUSTED_PROXY_CIDRS=""` (default) → spoofed `X-Forwarded-Proto: https` ignored. `TRUSTED_PROXY_CIDRS="127.0.0.1/32"` → header honored. New middleware tests cover both.
- **Phase 2:** `/admin/users` renders with self-action hidden using `/api/me/identity` data; no `auth()` import remains in the page.
- **Phase 3:** `e2e/auth-identity.spec.ts` 5.1b passes with a real signed JWE planted; `/api/me/identity` returns the live student.
- **Phase 4:** Sign up as teacher → onboarding routes to `/register-org`. Sign up as student → onboarding shows "your teacher will add you." Vitest happy paths green.
- **Phase 5:** Deep-link spec passes. Manual: sign out, hit `/teacher/classes/<uuid>`, redirected to `/login?callbackUrl=…`, sign in, land on original URL.
- **Phase 6:** Units/new page renders no duplicate-key React warnings; one option per distinct org.
- **Phase 7:** Parent nav has Dashboard only; nav-config test asserts.
- **Phase 8:** Dev console clean of "Encountered a script tag" errors on every route load.
- **Whole plan:** Vitest + Go + new E2E specs all green.

---

## Codex Review of This Plan

_To be added after the plan is dispatched to Codex via `/codex:rescue`._

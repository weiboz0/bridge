# 040 — Auth Hardening & Workflow Gaps

**Goal:** Pick up the items deferred from plan 039 — security hardening on the cookie auth boundary, fix-once workflow gaps surfaced in Codex review 002 (registration intent, deep-link preservation, duplicate org rows, parent children, theme script overlay), and tighten the admin/users dual-source pattern. No new product surface area; defer org placeholder pages and editor responsive layout to their own plans (041, 042).

**Source:**
- Codex comprehensive site review, 2026-04-26 (`docs/reviews/002-comprehensive-site-review-2026-04-26.md`)
- Codex post-implementation review of plan 039 — three deferrals documented in `docs/plans/039-auth-identity-canonicalization.md` ("Out-of-scope" + Review 2 / Review 3 sections).

**Branch:** `feat/040-auth-hardening-workflow-gaps`

**Status:** Complete (pending PR review)

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

`r.RemoteAddr` is the right address to trust-check because it's the immediate peer (the proxy IP for proxied requests, the client IP for direct hits). We're deciding whether the *immediate sender* is allowed to inject scheme metadata — exactly what `r.RemoteAddr` identifies.

### Task 1.2: Document the env var + ingress requirement

**Files:**
- Modify: `docs/setup.md` — add `TRUSTED_PROXY_CIDRS` to the env list with deployment guidance: **the ingress proxy MUST strip client-supplied `X-Forwarded-Proto` headers before forwarding**, otherwise an attacker behind the proxy can still spoof scheme. Allowlist + stripping are required together; allowlist alone is not sufficient.
- Modify: `.env.example` — add the var with empty default and a comment pointing at the ingress requirement.

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

### Task 3.1: Expose `NEXTAUTH_SECRET` to E2E setup

**Files:**
- Modify: `e2e/playwright.config.ts` — pass `NEXTAUTH_SECRET` and `NEXTAUTH_URL` from the host environment into the test fixtures.
- Create: `e2e/helpers/jwe.ts` — small helper that wraps the same `jose` JWE encoder Auth.js uses and produces a valid `__Secure-authjs.session-token` value for a given `{ sub, email, name }`. The salt is the cookie name; HKDF derives the key from `NEXTAUTH_SECRET` with the info string `Auth.js Generated Encryption Key (${salt})`. Mirrors the algorithm in `platform/internal/auth/jwt.go::DecryptAuthJSToken` so a token minted by this helper is decryptable by the Go middleware.

**Env var note:** the Go backend reads `NEXTAUTH_SECRET` (`platform/internal/config/config.go:68`); the project's `.env.example` and `docs/setup.md` use that name. Auth.js v5 alias-accepts both `AUTH_SECRET` and `NEXTAUTH_SECRET` on the Next side, but Go does not. Use `NEXTAUTH_SECRET` everywhere in the E2E plumbing for consistency.

### Task 3.2: Strengthen `e2e/auth-identity.spec.ts` (5.1b)

Replace the opaque-garbage stale cookie value with a valid signed token for a non-existent / different user. Assert that after signing in as the live student, `/api/me/identity` returns the live student — not the planted token's `sub`. This proves Go's canonical-cookie selection ignores a fully valid stale variant.

Keep the original assertion (no leak path) intact.

---

## Phase 4: Registration Role Intent

Two approaches considered. **Recommendation: persist the role** because the UI already asks the question and discarding it has no upside. The alternative — remove the selector and route everyone through `/onboarding` — gives up information that the user typed.

### Task 4.1: Schema — add `intended_role` enum column to users

**Files:**
- Modify: `src/lib/db/schema.ts` — add a new `signupIntentEnum = pgEnum("signup_intent", ["teacher", "student"])` (consistent with the existing enum-per-domain pattern at `src/lib/db/schema.ts:19-93`; do NOT use raw `TEXT` with a CHECK constraint, which would be inconsistent with the rest of the schema). Add `intendedRole: signupIntentEnum("intended_role")` (nullable) on `users`.
- Run: `bun run db:generate` to produce `drizzle/00XX_user_intended_role.sql`. Apply to `bridge` and `bridge_test`.

**Note re-using `userRoleEnum`:** that enum is explicitly tagged as kept-only-for-migration-compatibility (`src/lib/db/schema.ts:19-24`). Do not extend it — make a fresh enum scoped to signup intent.

### Task 4.2: Persist on register

**File:** `src/app/api/auth/register/route.ts`

Extend the zod schema to accept `role: z.enum(["teacher", "student"]).optional()`. Persist it on `users.intendedRole`. The schema is permissive on absent role to avoid breaking other callers.

### Task 4.3: Read in onboarding

**Files:**
- Modify: `src/app/(portal)/onboarding/page.tsx` (locate it first; `find src/app -name "onboarding"` if needed) — read `users.intendedRole` for the current user. If `teacher`, route to `/register-org`. If `student`, show the "your teacher will add you" copy. Removes the fork from "I always assume student first" → "use the explicit signal."

### Task 4.4: Tests

- Vitest integration in `tests/integration/auth-register.test.ts` — `role: "teacher"` and `role: "student"` are persisted; absent role still works (BC).
- Unit test for the onboarding page if one exists; otherwise add a smoke test.

### Task 4.5 (scope add per Codex): student signup from class invite carries intent

Review 002 (line 179) calls out that a student signing up from a class invite/join context should carry that intent through registration.

**Files:**
- Modify: `src/app/(auth)/register/page.tsx` — read `?invite=<token>` from the URL. When present, default `role` to `student` and persist the invite token through registration (currently lost across the signup → login redirect chain).
- Modify: `src/app/api/auth/register/route.ts` — accept an optional `inviteToken` and, on successful signup, immediately attempt to honor it (call the existing class-join code path). Failure to honor is non-fatal — the user lands on the dashboard either way.
- E2E follow-up: existing `e2e/join-class.spec.ts` covers logged-in join; add an unauthenticated join-via-invite-link variant.

---

## Phase 5: Deep-Link `callbackUrl` Preservation

**Important correction from Codex:** `src/middleware.ts` already exists and binds `auth as middleware` to `/api/orgs/*` and `/api/admin/*`. Naively replacing it would drop those API auth guards — a security regression. Also, `x-invoke-path` is internal Next.js request metadata, not a public header we can rely on injecting. Both reasons force a different shape for this phase.

### Task 5.1: Extend the existing middleware to handle portal redirects

**File:** Modify `src/middleware.ts`

Replace the one-line re-export with a composed middleware that:

1. For paths matching `/api/orgs/*` or `/api/admin/*`: defer to `auth as middleware` (preserves the existing auth guard exactly).
2. For paths matching the portal trees (`/teacher/*`, `/student/*`, `/parent/*`, `/org/*`, `/admin/*` — but NOT `/api/admin/*`): call `auth()` to check the session. If unauthenticated, redirect with the original path baked into `callbackUrl`. If authenticated, fall through.

This pattern lets the middleware see the URL natively (no header injection needed) and pass `callbackUrl=<originalPath>` directly to `/login`.

The matcher must explicitly exclude `_next/`, `favicon`, anything with a file extension, and the API auth paths handled by branch (1) so they aren't double-processed.

```ts
// Sketch — exact form decided at implementation time after re-reading
// auth.config and confirming Auth.js v5 export shape.
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";

const PORTAL_TREES = ["/teacher", "/student", "/parent", "/org", "/admin"];

export default auth((req) => {
  const { pathname } = req.nextUrl;

  // Existing API auth-guard contract: continue to enforce session presence
  // for /api/orgs/* and /api/admin/*. The auth() wrapper already does this.

  // Deep-link preservation: portal trees redirect-to-login with callbackUrl
  // when unauthenticated. Auth.js's auth() callback runs with req.auth set.
  const isPortal = PORTAL_TREES.some(
    (p) => pathname === p || pathname.startsWith(p + "/")
  );
  if (isPortal && !req.auth) {
    const callback = encodeURIComponent(pathname + req.nextUrl.search);
    const url = new URL(`/login?callbackUrl=${callback}`, req.nextUrl.origin);
    return NextResponse.redirect(url);
  }
  return NextResponse.next();
});

export const config = {
  matcher: [
    "/api/orgs/:path*",
    "/api/admin/:path*",
    "/teacher/:path*",
    "/student/:path*",
    "/parent/:path*",
    "/org/:path*",
    "/admin/:path*",
  ],
};
```

### Task 5.2: Simplify `portal-shell.tsx`

**File:** `src/components/portal/portal-shell.tsx`

After Task 5.1, the unauth redirect is handled at the middleware layer with the correct `callbackUrl`. The page-level `loginRedirect()` (lines 22-29) becomes a defense-in-depth fallback for the unlikely case middleware didn't run. Keep it but drop the `x-invoke-path` / `x-url` lookup — middleware ensures the user never hits portal-shell unauthenticated.

### Task 5.3: E2E for deep-link

**File:** `e2e/deep-link.spec.ts` (new)

Open `/teacher/classes/<some-uuid>` while signed out. Assert redirect to `/login?callbackUrl=%2Fteacher%2Fclasses%2F<uuid>`. Sign in via the form. Assert landing on the original deep link.

### Task 5.4: Regression test for the existing API auth guard

**File:** `tests/unit/middleware-config.test.ts` (new)

Assert that the matcher list includes `/api/orgs/:path*` and `/api/admin/:path*` — locks in the contract that the existing auth guard hasn't been dropped by the rewrite.

---

## Phase 6: Org Membership Dedup

**Locked decision (per Codex pre-impl review):** dedupe in the **store query**, not at the handler shape and not via a new endpoint. The existing response contract — array of `{orgId, role, status, orgStatus, ...}` rows — has multiple consumers (`teacher/units/new`, `teacher/units`) that filter by role; changing the shape forces parallel rewrites and is a cross-cutting contract break. Dedup in `GetUserMemberships` returns one row per `(orgId, role)` pair from the query layer, plus a frontend defensive dedup so rolling deploys don't regress the UI.

### Task 6.1: Dedup at the store query

**File:** `platform/internal/store/orgs.go::GetUserMemberships` (lines 332-359)

Add `DISTINCT ON (om.org_id, om.role)` to the SELECT to collapse duplicate `(orgId, role)` pairs from the same org. Order by `om.created_at` so DISTINCT ON keeps the oldest row deterministically.

```sql
SELECT DISTINCT ON (om.org_id, om.role)
  om.id, om.org_id, om.user_id, om.role, om.status, om.created_at,
  o.name, o.slug, o.status
FROM org_memberships om
INNER JOIN organizations o ON om.org_id = o.id
WHERE om.user_id = $1
ORDER BY om.org_id, om.role, om.created_at
```

The response shape stays identical — consumers see one row per `(orgId, role)` instead of one per (orgId, role, duplicate insertion).

### Task 6.2: Frontend defensive dedup by `orgId`

For UI selectors that show one option per org regardless of role, the React `key={org.orgId}` warning still fires if the user has multiple roles in the same org (`teacher` + `org_admin`). Dedup at the consumer:

**Files:**
- Modify: `src/app/(portal)/teacher/units/new/page.tsx:40-69` — `Array.from(new Map(filtered.map(o => [o.orgId, o])).values())` before `setOrgs`.
- Modify: `src/app/(portal)/teacher/units/page.tsx` (the same `loadOrgs` pattern around line 135).
- Any other consumer surfaced by `grep -rn "/api/orgs" src/`.

### Task 6.3: Tests

- Go: integration test in `platform/internal/store/orgs_test.go` that inserts a user with two memberships in the same org with different roles and asserts `GetUserMemberships` still returns each `(orgId, role)` pair once.
- Vitest: render `units/new` with a mock returning two duplicate `(orgId, teacher)` rows; assert the rendered `<option>` count is 1 per org.

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

Replace the raw `<script dangerouslySetInnerHTML>` (which triggers the dev-overlay "Encountered a script tag while rendering React component" error on every route) with `next/script` using `strategy="beforeInteractive"`. App Router supports this strategy in the root layout since Next 13; project is on Next 16.

```tsx
import Script from "next/script";

const themeScript = `(function(){var t=localStorage.getItem('bridge-theme')||'light';if(t==='dark')document.documentElement.classList.add('dark');else document.documentElement.classList.remove('dark');})()`;

// Inside <html>, before <body>:
<Script id="bridge-theme-bootstrap" strategy="beforeInteractive">
  {themeScript}
</Script>
```

**Validation gate before merging:** confirm visually in dev that (a) the dev-overlay error is gone and (b) there is no theme FOUC on initial page load (preserve the existing behavior). If `beforeInteractive` doesn't actually run early enough in App Router for our case, fall back to keeping the inline `<script>` but suppress the warning via the documented `<head>` placement pattern. The plan's expected outcome is "no dev-overlay error AND no FOUC"; the implementation chooses whichever pattern delivers both.

### Task 8.2: Manual verification + smoke E2E

**File:** `e2e/theme-bootstrap.spec.ts` (new)

Open `/` with `localStorage.setItem('bridge-theme','dark')` pre-seeded; assert `<html class>` includes `dark` on initial paint without a flash. (Hard to assert "no flash" reliably in headless; minimum assertion: dark class present after load.) Add a console-error sentinel that fails the test if any "Encountered a script tag while rendering React component" message appears.

---

## Implementation Order

| Phase | Tasks | Why |
|-------|-------|-----|
| 1 | Trusted proxy XFP | Smallest auth hardening; pure backend, isolated |
| 2 | Admin users dual-source | Tiny one-page change; uses /api/me/identity from 039 |
| 3 | Stronger E2E | Builds on phases 1-2; finishes the auth hardening track |
| 4 | Registration intent | Schema + API + onboarding + invite-token carry-through; **largest phase, watch for scope creep** |
| 5 | Deep-link middleware | Edits existing `src/middleware.ts` — composes with the existing API auth guard; do after Phase 4 to confirm onboarding flow still works through the new middleware |
| 6 | Org membership dedup | Touches DB query + multiple consumers; do after the simpler items |
| 7 | Parent /children trim | Trivial deletion + nav update |
| 8 | Theme script | Layout-touching but isolated |

Phases are independent; **Phase 4 is the most likely to need its own PR** (schema migration + onboarding behavior change has product blast radius). If Phase 4 grows during implementation — new tests fail, schema migration is non-trivial, onboarding logic ramifies — extract it to plan 040b and ship the rest of 040 first.

Default plan: one PR, eight commits, with an explicit gate after Phase 4 to confirm scope hasn't blown up.

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

- **Date:** 2026-04-26
- **Reviewer:** Codex (pre-implementation, via `codex:rescue`)
- **Verdict:** Corrections applied (see below). Plan is now ready for implementation.

### Corrections applied

1. `[CRITICAL]` **Phase 5 middleware collision.** `src/middleware.ts` already exists and binds `auth as middleware` to `/api/orgs/*` and `/api/admin/*`. The original draft would have replaced it and dropped the existing auth guard — a security regression. → Phase 5 rewritten to **extend** the existing middleware with both branches: preserve API auth-guard semantics for `/api/orgs/*` and `/api/admin/*`, and add portal-tree redirect-with-callbackUrl for unauthenticated users hitting `/teacher/*`, `/student/*`, etc. New regression test (Task 5.4) asserts the API matcher contract isn't dropped. Also dropped the `x-invoke-path` injection approach because that header is internal Next.js metadata, not a public surface — middleware sees the URL natively.

2. `[IMPORTANT]` **Phase 1 deployment requirement.** Allowlist alone is insufficient if the proxy passes through client-supplied `X-Forwarded-Proto`. → Task 1.2 now explicitly requires the ingress proxy to strip client-supplied XFP headers before forwarding; allowlist + stripping are required together.

3. `[IMPORTANT]` **Phase 3 env var name.** Plan said `AUTH_SECRET` but Go reads `NEXTAUTH_SECRET` (`platform/internal/config/config.go:68`); project's `.env.example` and `docs/setup.md` use that name. → Task 3.1 now uses `NEXTAUTH_SECRET` consistently with a note about the Auth.js v5 alias situation. JWE algorithm confirmed (`alg=dir`, `enc=A256CBC-HS512`, HKDF SHA-256 with info `Auth.js Generated Encryption Key (${salt})`) so a token minted by the helper IS decryptable by Go middleware.

4. `[IMPORTANT]` **Phase 4 schema choice.** Original plan proposed `intended_role TEXT NULL CHECK (...)`. Schema precedent is enums-per-domain (`src/lib/db/schema.ts:19-93`); raw TEXT+CHECK is inconsistent. → Task 4.1 now creates a fresh `signupIntentEnum` rather than extending the legacy `userRoleEnum` (which is explicitly tagged for migration compatibility only).

5. `[IMPORTANT]` **Phase 6 API shape decision deferred to impl time.** Codex flagged that leaving the choice open invites a contract break. → Phase 6 now locks the decision: dedup at the **store query** with `DISTINCT ON (org_id, role)`. Response shape unchanged; existing consumers keep working. Frontend defensive dedup added by `orgId` for selectors that show one option per org regardless of role.

6. `[IMPORTANT]` **Phase 4 size flag.** Codex noted Phase 4 has product/data blast radius beyond the other phases. → Implementation Order updated with a note that Phase 4 is the most likely to need its own PR; if it grows, extract to 040b and ship the rest of 040 first.

7. `[MINOR]` **Phase 4 invite-context scope gap.** Review 002 (line 179) mentioned that students signing up from a class invite should carry that intent. → Task 4.5 added covering invite-token plumbing through registration.

### Codex notes (no plan change required)

- `r.RemoteAddr` is the right address class to trust-check for `X-Forwarded-Proto` — it's the immediate peer (proxy IP for proxied requests, client IP for direct hits).
- No other read-sites for `X-Forwarded-Proto` in production code beyond `platform/internal/auth/middleware.go:20`.
- Phase 8 `next/script beforeInteractive` likely works in App Router root layout for Next 16, but still needs visual validation in dev. Plan now has an explicit validation gate before the implementation commits to that pattern.

## Post-Execution Report

**Status:** Complete. All 8 phases shipped on `feat/040-auth-hardening-workflow-gaps` across 8 commits.

**Phase 1 — Trusted-proxy XFP** (commit `34bdbee`)
- `platform/internal/auth/proxy.go` adds `IsTrustedProxy()` + `TRUSTED_PROXY_CIDRS` env var with cached parse.
- `platform/internal/auth/middleware.go::canonicalCookieName` only honors `X-Forwarded-Proto` when the immediate peer is in the allowlist; `r.TLS != nil` is always trusted.
- 7 tests in `proxy_test.go` (empty allowlist, single CIDR, multiple CIDRs, IPv6, bad CIDR entries, bad RemoteAddr, host-only addresses) + 2 new cases in `middleware_test.go` covering spoofed-XFP rejection and trusted-XFP acceptance.
- `.env.example` and `docs/setup.md` document the env var + the explicit "ingress proxy must strip client-supplied XFP" deployment requirement.

**Phase 2 — admin/users single identity source** (commit `dd0ff4c`)
- `src/app/(portal)/admin/users/page.tsx` drops `auth()` import; reads `/api/me/identity` (added in 039) in parallel with `/api/admin/users` and compares against `identity.userId`.

**Phase 3 — Valid-signed-stale-token E2E** (commit `77248cc`)
- `e2e/helpers/jwe.ts` wraps `@auth/core/jwt::encode` to mint a token decryptable by Go middleware (same HKDF + JWE algorithm).
- `e2e/auth-identity.spec.ts` 5.1b now plants a real signed token for a fake attacker user (`stale-attacker-user-id`) and asserts the live student's identity is what `/api/me/identity` returns. Skips with a clear message when `NEXTAUTH_SECRET` isn't in the env.

**Phase 4 — Registration role intent + invite carry-through** (commit `792eb73`)
- New `signup_intent` enum + `users.intended_role` nullable column (migration `0021_user_intended_role.sql`).
- `/api/auth/register` accepts optional `role` and persists it; returns it in the response.
- `/onboarding` reads intent: teacher → straight to `/register-org`; student → leads with student card.
- `/register` reads `?invite=<code>` from URL, defaults to student, and after sign-in redirects to `/student?invite=<code>`.
- `JoinClassDialog` accepts new `initialInviteCode` prop that auto-opens with the code prefilled (uppercased).
- 4 new register tests (persist teacher / student / null / reject unknown) + 2 new dialog tests (auto-open with prefill, closed without prop).

**Phase 5 — Deep-link `callbackUrl` middleware** (commit `5378984`)
- `src/lib/auth.ts` gains an `authorized` callback that branches on path: `/api/orgs/*`, `/api/admin/*` → return `isAuthed` (preserves the legacy 401 contract); portal trees → return `Response.redirect` to `/login?callbackUrl=<encoded>`.
- `src/middleware.ts` matcher extended to all 5 portal trees + the original 2 API trees. Matcher list extracted to `src/lib/portal/middleware-matcher.ts` so unit tests can assert the contract without pulling Auth.js.
- `src/components/portal/portal-shell.tsx` simplified — removed the `x-invoke-path`/`x-url` header lookup (those headers aren't actually populated by Next.js); middleware handles the redirect with `callbackUrl` baked in.
- Regression test at `tests/unit/middleware-config.test.ts` locks both API + portal entries; new E2E `e2e/deep-link.spec.ts` covers redirect-with-callbackUrl + post-login landing.

**Phase 6 — Org membership dedup** (commit `012d6cd`)
- `platform/internal/store/orgs.go::GetUserMemberships` now uses `DISTINCT ON (om.org_id, om.role)` ordered by `created_at`. Same-pair duplicates collapse; distinct roles preserved.
- New Go test `TestOrgStore_GetUserMemberships_DistinctRolesPreserved` covers both behaviors.
- Frontend defensive dedup by `orgId` in `teacher/units/page.tsx` and `teacher/units/new/page.tsx` for selectors that show one option per org regardless of role.

**Phase 7 — Parent /children trim** (commit `3eaec34`)
- Deleted `src/app/(portal)/parent/children/page.tsx` (redirect-only).
- `nav-config.ts` parent block now Dashboard only; new test asserts `parent.navItems.length === 1`.

**Phase 8 — Theme script via next/script** (commit `6cc1f20`)
- `src/app/layout.tsx` replaces `<script dangerouslySetInnerHTML>` with `<Script id="bridge-theme-bootstrap" strategy="beforeInteractive">`.
- `e2e/theme-bootstrap.spec.ts` asserts `<html>` has the `dark` class with `bridge-theme=dark` pre-seeded AND no "Encountered a script tag" message in console.

**Verification**
- Vitest: 403 passed / 11 skipped (was 394 — 9 new tests across proxy, register intent, dialog prefill, middleware matcher, nav-config trim).
- Go tests: all 14 packages green against `bridge_test` (proxy_test 7 cases, middleware_test 2 new cases, orgs_test 1 new case).
- TypeScript: clean for new/modified files.
- E2E: 4 new specs added (auth-identity strengthened, deep-link, theme-bootstrap, plus the existing identity test). Not run in this loop — require local stack.

**Plan compliance**
- `[CRITICAL]` Codex pre-impl finding (Phase 5 middleware collision) addressed: extended existing middleware via `authorized` callback rather than replacing it. Regression test locks the matcher contract.
- `[IMPORTANT]` Phase 1 deployment requirement (proxy must strip client-supplied XFP) documented in `.env.example` and `docs/setup.md`.
- `[IMPORTANT]` Phase 3 env var name corrected to `NEXTAUTH_SECRET`.
- `[IMPORTANT]` Phase 4 schema uses fresh `signupIntentEnum` (not raw TEXT+CHECK).
- `[IMPORTANT]` Phase 6 locked decision: dedup at the store query (DISTINCT ON), not at the API shape.
- `[MINOR]` Phase 4.5 (invite-context carry-through) shipped via redirect with query param rather than coupling register API to class-join logic.

**Pending manual validation**
- Phase 8: confirm in dev that the `<script>` dev-overlay error is gone AND no theme FOUC on initial load. If `beforeInteractive` doesn't fire early enough, fall back to inline `<script>` placement.
- Full E2E suite run (requires local stack; user-driven).

## Code Review

### Review 1 — Pre-implementation plan review (commit `b85f6f4`)

- **Date:** 2026-04-26
- **Reviewer:** Codex (via `codex:rescue`)
- **Verdict:** Corrections applied (see `## Codex Review of This Plan` section above).

### Review 2 — Post-implementation review

- **Date:** 2026-04-26
- **Reviewer:** Codex (post-implementation, via `codex:rescue`)
- **Verdict:** Three `[IMPORTANT]` + one `[NOTE]` fixed in-PR; one `[MINOR]` accepted with explanation; the remaining notes were observations not requiring action.

**Fixed in commit applied after Review 2:**

1. `[IMPORTANT]` **Phase 5 — API auth-guard contract change.** The `authorized` callback returned `isAuthed` for `/api/orgs/*` and `/api/admin/*`. Previously the no-callback default was `true` (pass-through, handler enforces auth). The new strict 401 behavior breaks `/api/admin/impersonate/status` which is designed to return `{ impersonating: null }` for unauthenticated callers (called from the always-rendered ImpersonateBanner). → Reverted API path to pass-through (`return true`) so handlers continue to enforce their own auth contracts. Portal redirect for unauth users still works.
2. `[IMPORTANT]` **Phase 4 — Login → Register link drops the invite code.** An unregistered visitor on `/student?invite=ABCD` gets redirected to `/login?callbackUrl=/student?invite=ABCD`, but the login page's "Sign up" link pointed at plain `/register`, losing the code. → Login page now extracts the invite from `callbackUrl` (or top-level `?invite=`) and pre-populates the register link's URL. Carry-through works for both flows: existing-account sign-in (`callbackUrl` redirect) and new-account sign-up.
3. `[IMPORTANT]` **Phase 7 — Stale E2E expectations.** `e2e/parent.spec.ts` still asserted that `/parent/children` redirects to `/parent`; `e2e/help-queue.spec.ts` clicked a "My Children" nav link that no longer exists. → Updated `parent.spec.ts` to assert 404 (the locked-in new behavior); removed the children-list nav-click test from `help-queue.spec.ts`.
4. `[NOTE]` **Phase 1 — `cidrCacheOnce` is unused ceremony.** Dropped the `sync.Once` from `proxy.go`; the `RWMutex` is sufficient for cache safety.
5. `[MINOR]` **Cross-cutting — unused imports.** Removed `beforeEach` + `testDb` from `auth-register.test.ts`; removed `ACCOUNTS` import from `help-queue.spec.ts` (only the import; the Parent Portal block survives without it).

**Accepted without action:**

- `[MINOR]` Phase 8 timing — `theme-bootstrap.spec.ts` checks the `dark` class after `page.goto`, not strictly before first paint. True FOUC verification is hard in headless Playwright; the spec's purpose is to lock the dev-overlay sentinel and confirm the bootstrap runs at all. Manual visual validation gate remains in the post-execution report.
- `[IMPORTANT]` Phase 3 skip when `NEXTAUTH_SECRET` is absent — the test skips with a clear actionable message. The secret is shell-environment provided (not embedded). Acceptable because (a) the alternative is failing on every default-clean E2E run, and (b) the skip message explicitly tells the operator how to enable it.

**Observations (no action):**

- Phase 3 JWE encoder/decoder algorithm match confirmed by Codex (no salt/info string mismatch between `e2e/helpers/jwe.ts` and `platform/internal/auth/jwt.go`).
- Phase 4 schema migration `0021_user_intended_role.sql` matches `src/lib/db/schema.ts`.
- Phase 5 portal redirect preserves query strings via `pathname + (search || "")`.
- Deferral integrity verified: no diff content pulls forward org placeholder pages or editor responsive — those remain plan 041 / 042 scope.


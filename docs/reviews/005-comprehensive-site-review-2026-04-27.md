# Comprehensive Site Review

Date: 2026-04-27 America/Los_Angeles
Target: `main` codebase at `/home/chris/workshop/Bridge`
Reviewer: Codex

## Scope

Access mode: **code-only review**. `localhost:3003` and `localhost:8002` were both checked and neither service responded, so no browser-driven or API-runtime verification was possible in this pass.

This review cross-checked review 002's findings against plans 039-042 and the current code. It covered public/auth surfaces, portal routing, teacher workflows, student join, parent navigation, organization admin list views, platform admin, teaching units, live sessions, problem editor responsive layout, invalid-route guards, deep-link handling, registration with `?invite=`, and role-driven onboarding.

Because this was code-only, every behavioral claim below traces to route/component/handler code rather than observed browser state.

## Accounts Used

No accounts were used because the local stack was not running.

Reference accounts reserved for browser-driven follow-up:

- Teacher: `eve@demo.edu`
- Student: `alice@demo.edu`
- Parent: `diana@demo.edu`
- Org admin: `frank@demo.edu`
- Platform admin: `admin@e2e.test`

## Test Data Created

None.

## Executive Summary

Plans 039-042 substantially improved the site from review 002. The auth identity boundary is now much better structured: Next forwards only the canonical Auth.js cookie as a bearer token, Go treats bearer-header auth as the exclusive proxy path, logout explicitly expires both cookie variants, and a dev diagnostic endpoint exists. The teacher/student session pages now delegate authorization to Go page-payload endpoints instead of comparing NextAuth and Go identities locally. Org-admin list pages are real read-only pages, parent duplicate navigation is trimmed, registration intent is persisted for credentials signup, deep links are captured in middleware, and the root theme script no longer uses a raw React-rendered `<script>`.

No P0 code issue was found in this code-only pass. The remaining risks are mostly workflow and responsive-layout gaps:

1. Org-admin list views have no multi-org context selector, so a multi-org admin sees only the first active org unless callers manually append `orgId`.
2. The problem editor still uses a viewport breakpoint while the desktop portal sidebar consumes 56 or 224 px; at a 1024 px desktop viewport the editor remains in three-pane mode inside roughly 800 px of content width.
3. Credentials registration carries role/invite intent, but Google signup from `/register?invite=...` still drops both the invite and the selected role.

Recommended next-cycle priority:

1. Add org context selection/persistence for org-admin pages and role switching.
2. Move the problem editor's responsive decision to a container-aware breakpoint or raise the viewport breakpoint to account for the portal sidebar.
3. Carry invite/role intent through Google OAuth registration.
4. Replace or remove the platform-admin Settings placeholder.
5. Make the responsive editor E2E create deterministic problem data and assert editor pane width, not only page overflow.

## Findings

### P1: Org-admin list views silently collapse multi-org admins to the first org

Files:

- `platform/internal/handlers/org_dashboard.go:46-58`
- `src/app/(portal)/org/teachers/page.tsx:5-10`
- `src/components/portal/role-switcher.tsx:26-29`
- `src/components/portal/role-switcher.tsx:56-59`

Repro:

Code-only path:

- The org dashboard/list authorization helper accepts `?orgId=...`, but when it is absent it iterates the caller's memberships and picks the first active `org_admin` membership, then breaks.
- The shipped org pages call endpoints such as `/api/org/teachers` without an `orgId`, so they always depend on that first-membership fallback.
- The role switcher maps only by portal role and pushes `/org`; it does not include `orgId` or display `orgName`. It also keys buttons by `r.role`, so two `org_admin` roles for different orgs produce duplicate React keys.

Impact:

Now that `/org/teachers`, `/org/students`, `/org/courses`, `/org/classes`, and `/org/settings` are real pages, a user who administers multiple organizations has no visible way to select which org they are inspecting. They can only see whichever membership the backend happens to return first.

Recommendation:

Add an org context selector for org-admin pages, persist the selected `orgId` in the URL, and include `orgId` in role-switcher navigation/keys. Org pages should call `/api/org/...?...orgId=<selected>` once a user has more than one active org-admin membership.

### P1: Problem editor still uses the wide three-pane layout at a squeezed 1024 px desktop viewport

Files:

- `src/components/portal/sidebar.tsx:25-28`
- `src/components/portal/sidebar.tsx:50-51`
- `src/components/problem/problem-shell.tsx:192-196`
- `src/components/problem/problem-shell.tsx:204-207`
- `src/components/problem/problem-shell.tsx:223-227`
- `src/components/problem/problem-shell.tsx:289-296`

Repro:

Code-only path:

- The desktop sidebar is `w-56` expanded or `w-14` collapsed at `md+`.
- The problem shell switches to wide mode at the viewport `lg` breakpoint via `lg:flex-row`.
- In wide mode, the left pane has `lg:min-w-[360px]`, the right pane has `lg:min-w-[320px]`, and the center editor is only `min-w-0 flex-1`.
- At a 1024 px desktop viewport with the expanded sidebar, the main content area is about 800 px. The two side pane floors consume 680 px before borders/gaps, leaving the editor around 120 px wide while the layout still counts as `lg`.

Impact:

Plan 042 fixes narrow tablet portrait, but the original small-laptop/sidebar failure still leaks at the exact breakpoint where QA is likely to test. The page may avoid document-level horizontal overflow while the load-bearing code editor is too narrow to use.

Recommendation:

Make the breakpoint container-aware, or raise the wide-layout breakpoint to account for the portal sidebar. Add an assertion that the code pane's bounding box exceeds a usable minimum width at 1024 px with the desktop sidebar visible.

### P1: Google registration from an invite link drops invite and role intent

Files:

- `src/app/(auth)/register/page.tsx:25-31`
- `src/app/(auth)/register/page.tsx:40-43`
- `src/app/(auth)/register/page.tsx:63-68`
- `src/app/(auth)/register/page.tsx:83-87`
- `src/lib/auth.ts:90-109`

Repro:

Code-only path:

- `/register?invite=<code>` correctly defaults credentials signup to `student`.
- The credentials submit sends `{ name, email, password, role }` and then redirects to `/student?invite=<code>`.
- The Google signup button always calls `signIn("google", { callbackUrl: "/" })`.
- The Google `signIn` callback creates a new user with `name`, `email`, and `avatarUrl`, but no `intendedRole` and no invite context.

Impact:

Review 002 specifically called out registration with `?invite=` and role-driven onboarding. Credentials signup now handles that path, but the visible "Sign up with Google" path still loses the invite, creates an OAuth user with no intended role, and lands them at `/` instead of the join flow.

Recommendation:

For `/register?invite=<code>`, call Google with `callbackUrl=/student?invite=<code>` and persist student intent for new OAuth users. If Auth.js cannot receive the selected role directly, store a short-lived signed intent cookie before OAuth and consume it in the `signIn` callback.

### P2: Platform-admin Settings remains advertised as primary nav but is only a placeholder

Files:

- `src/lib/portal/nav-config.ts:8-11`
- `src/app/(portal)/admin/settings/page.tsx:1-6`

Repro:

Code-only path:

- The platform-admin primary nav includes `Settings -> /admin/settings`.
- The page renders only "Platform configuration coming soon."

Impact:

The org-admin portal placeholder issue from review 002 was fixed, but the platform-admin portal still advertises an operational route that does not do anything. This is lower risk than the previous org issue because admin Organizations and Users are useful, but the primary nav still over-promises.

Recommendation:

Either remove Settings from admin nav until it has an actionable read view, or ship a read-only settings/status page that exposes current platform configuration safely.

### P2: Problem editor responsive E2E can skip all coverage and does not catch the squeezed-editor case

Files:

- `e2e/problem-editor-responsive.spec.ts:20-36`
- `e2e/problem-editor-responsive.spec.ts:42-44`
- `e2e/problem-editor-responsive.spec.ts:56-71`

Repro:

Code-only path:

- `findProblemUrl()` returns `null` if the authenticated student has no class or no problem.
- Each responsive test calls `test.skip(!url, "no problem available to test against")`.
- The 1024 px boundary test asserts all panes are visible and `documentElement.scrollWidth <= window.innerWidth`, but it does not assert that the code pane has a usable width.

Impact:

The test can pass by skipping in an under-seeded local/CI environment. Even when it runs, it can miss the current sidebar-squeezed editor because the page can have no document-level horizontal overflow while the center pane is nearly unusable.

Recommendation:

Seed or create a deterministic class/problem in the spec. At 1024 px, assert the code pane bounding box is above a practical minimum width with the portal sidebar present.

## Confirmed Improvements Since Review 002

- Auth identity drift root cause is closed in code. `src/lib/auth-cookie.ts:25-30` centralizes the Auth.js cookie name, `src/lib/api-client.ts:33-58` forwards only the canonical cookie as a bearer token, and `platform/internal/auth/middleware.go:106-115` uses Authorization-header auth without falling back to cookies. Logout cleanup explicitly expires both session-cookie names in `src/app/api/auth/logout-cleanup/route.ts:18-31`, and the dev diagnostic response compares Next and Go identities in `src/app/api/auth/debug/route.ts:57-73`.
- Teacher live-session page authorization no longer compares NextAuth and Go identities locally. `src/app/(portal)/teacher/sessions/[sessionId]/page.tsx:33-41` makes one `/teacher-page` call and trusts Go 403/404. Go authorizes in `platform/internal/handlers/sessions.go:994-1043`.
- Platform admin dashboard no longer has to blank on stats failure. `src/app/(portal)/admin/page.tsx:16-78` catches `ApiError` and renders an error card with status, retry, and debug guidance.
- Student join is no longer optimistic in the client. `src/components/student/join-class-dialog.tsx:41-108` waits for join success, verifies the joined class appears in `/api/classes/mine`, retries once, and only then closes/refreshes.
- Org-admin nav no longer points into teacher routes. `src/lib/portal/nav-config.ts:18-27` lists only `/org` routes for org admins.
- Credentials registration role intent is persisted and read by onboarding. `src/app/api/auth/register/route.ts:8-15` accepts `teacher`/`student`, `src/app/api/auth/register/route.ts:44-57` stores and returns `intendedRole`, and `src/app/onboarding/page.tsx:35-42` routes teacher intent to `/register-org` while leading student intent with the student card.
- Deep-link preservation moved to middleware. `src/lib/portal/middleware-matcher.ts:16-24` matches portal trees, and `src/lib/auth.ts:70-88` redirects unauthenticated portal requests to `/login?callbackUrl=<original>`. `src/components/portal/portal-shell.tsx:18-25` now treats its own login redirect as a fallback.
- Org portal placeholders were replaced by real read-only pages. The pages fetch live data in `src/app/(portal)/org/teachers/page.tsx:5-10`, `src/app/(portal)/org/students/page.tsx:6-10`, `src/app/(portal)/org/courses/page.tsx:5-10`, `src/app/(portal)/org/classes/page.tsx:5-10`, and `src/app/(portal)/org/settings/page.tsx:13-18`. Go endpoints are registered in `platform/internal/handlers/org_dashboard.go:20-26`.
- Duplicate org membership rendering in unit org selectors is fixed defensively. Go dedupes memberships in `platform/internal/store/orgs.go:353-361`, and the teacher unit pages dedupe selector rows by `orgId` in `src/app/(portal)/teacher/units/page.tsx:144-153` and `src/app/(portal)/teacher/units/new/page.tsx:52-60`.
- Parent duplicate navigation is trimmed. `src/lib/portal/nav-config.ts:53-62` now exposes only the parent Dashboard item, and the redirect-only `/parent/children` page is no longer present.
- Root layout no longer renders a raw `<script>` as a React child. `src/app/layout.tsx:23-28` defines the bootstrap payload and `src/app/layout.tsx:41-47` renders it through `next/script` with `strategy="beforeInteractive"`.
- Problem editor has a real narrow fallback. `src/components/problem/problem-shell.tsx:177-197` tracks a narrow active tab and renders responsive tabs, `src/components/problem/problem-shell.tsx:192-196` switches one-pane/tabs vs wide row, and `src/components/editor/code-editor.tsx:139-162` uses `ResizeObserver` to relayout Monaco after a hidden pane becomes visible. The remaining leak is the sidebar-aware breakpoint finding above.

## Verification Notes

Verified by code inspection:

- Plans 039-042 and their post-execution reports.
- Review 002 P0/P1/P2/P3 findings against current implementation.
- Auth cookie canonicalization, logout cleanup, diagnostic endpoint, middleware deep-link behavior, and Go auth middleware.
- Teacher/student session page authorization endpoints and server-component callers.
- Student join client verification path.
- Org-admin nav and read-only list page/API implementation.
- Parent nav trimming and remaining parent detail/live routes.
- Registration intent for credentials signup and the remaining Google/invite gap.
- Platform admin dashboard/users/settings routes.
- Problem editor responsive layout, Monaco relayout hook, responsive tabs, and E2E coverage.
- Invalid UUID guards on representative deep routes (`teacher/sessions/[sessionId]`, `student/sessions/[sessionId]`, teacher/student class/problem routes).

Not verified:

- Browser behavior, screenshots, console output, or API responses, because `localhost:3003` and `localhost:8002` were unreachable.
- Actual login/role switching with the demo accounts.
- Hocuspocus realtime behavior and Pyodide execution.
- Full Playwright suite or Go/Vitest test execution in this pass.

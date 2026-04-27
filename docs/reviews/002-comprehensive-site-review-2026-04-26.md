# Comprehensive Site Review

Date: 2026-04-26 America/Los_Angeles
Target: `http://localhost:3003`
Reviewer: Codex

## Scope

This is a fresh full-site review after plan 038 landed on `main`. It is not limited to regressions or changes since the previous review. The review covered public/auth surfaces, portal routing, teacher workflows, student join behavior, parent navigation, organization admin navigation, platform admin access, teaching units, live sessions, invalid routes, responsive editor layout, and architectural route/auth boundaries.

The site was accessed through the Codex in-app browser against `localhost:3003`, which is an SSH tunnel to a server running elsewhere. Test/demo accounts and review-created data were used.

## Accounts Used

- Teacher: `eve@demo.edu`
- Student: `alice@demo.edu`
- Parent: `diana@demo.edu`
- Org admin: `frank@demo.edu`
- Platform admin: `admin@e2e.test`

## Test Data Created

- Personal unit: `Fresh Review Personal Unit 1777254411213`
- Unit ID: `647b093f-37ff-4d64-8c03-10f401bfc9cb`

Existing review data reused:

- Course: `Codex Review Course 20260426193844`
- Course ID: `acb3a714-dd04-473e-97aa-57e537e74418`
- Class: `Codex Review Class acb3a714`
- Class ID: `44eec29a-96e8-4e4c-841d-6040a467f2ca`
- Join code: `KWMLCWG5`
- Unit: `Codex Review Unit acb3a714`
- Unit ID: `223f1e61-25d6-4a67-83bb-bfd498b62f00`

## Executive Summary

Plan 038 improved several areas: teacher schedule/reports and parent reports are no longer in primary nav, parent dashboard nested links are fixed, invalid course IDs now cleanly 404, session status checks have moved to `live`, personal unit creation is now accepted, and duplicate Tiptap extension warnings were not observed in the editor.

However, the site is still not ready for a reliable end-to-end QA pass. The top risk remains auth identity consistency, but it now shows up more broadly: after signing in as one user, direct Go-proxied browser requests can still resolve as a different prior user. This breaks admin access, live-session entry, and student join behavior. It also means client-side API calls and server-rendered pages can disagree about the current user.

The second major risk is navigation promise mismatch. The organization portal advertises multiple primary routes that are placeholders, and it advertises Units/Sessions links that point into the teacher portal and redirect back for an org admin who lacks the teacher role.

Recommended next-cycle priority:

1. Finish auth identity hardening across browser-proxied Go requests, server-side API calls, and logout cleanup.
2. Rework session page authorization to use one canonical backend authorization result.
3. Fix student join flow end-to-end and verify the joined class appears for the student.
4. Make org-admin navigation match real available workflows.
5. Fix deep-link preservation and registration onboarding.
6. Keep editor/layout polish behind those reliability issues.

## Findings

### P0: Browser-proxied Go APIs still resolve stale or different identities after role switches

Files:

- `platform/internal/auth/middleware.go:67-76`
- `platform/internal/auth/middleware.go:121-130`
- `src/lib/api-client.ts:35-40`

After signing out and signing in again, the in-app browser showed different identities depending on which runtime path handled the request.

Teacher repro:

- `/api/auth/session` returned Eve Teacher as `d0d3b031-a483-4214-97fb-48c9584f4dcb`
- `/api/me/memberships`, a Go-backed proxied endpoint, returned memberships for `da5cef74-66e5-4946-bf56-409b23f34503`

Student repro:

- `/api/auth/session` returned Alice Student as `242fea26-1527-4a10-b208-af4cad1e1102`
- `/api/me/memberships` and `/api/classes/mine` still returned Eve's teacher/org-admin memberships and instructor classes

Admin repro:

- `/api/auth/session` returned `admin@e2e.test` with `isPlatformAdmin: true`
- `/api/admin/stats` returned `{"error":"Platform admin required"}`
- `/admin` rendered a blank/error state because the server component's Go API call got 403

The plan 038 code changed cookie precedence, but the current behavior shows the boundary is still not canonical. One likely remaining trap is that Go middleware switches to secure-cookie priority whenever `X-Forwarded-Proto` starts with `https`, while server-side `api-client.ts` switches based on `NEXTAUTH_URL`/`AUTH_URL`. Logout also did not clear the stale secure cookie in this browser context, so the Go proxy path continued seeing a prior identity.

Recommendation:

- Add a dev-only auth diagnostic endpoint that reports non-secret `nextAuthUserId`, `goClaimsUserId`, `cookieNamesPresent`, `cookieNameUsed`, `xForwardedProto`, and `match`.
- On logout, explicitly expire both `authjs.session-token` and `__Secure-authjs.session-token` for localhost/dev.
- Make browser-proxied Go requests and server-side Go requests use the same canonical token-selection policy.
- Add regression tests for role switching: teacher -> student -> admin must not leak the previous role through `/api/me/*`, `/api/classes/mine`, or `/api/admin/stats`.

### P0: Core live-session links still 404 for the teacher

Files:

- `src/app/(portal)/teacher/sessions/[sessionId]/page.tsx:30-37`
- `src/app/(portal)/teacher/sessions/[sessionId]/page.tsx:54-61`

As Eve Teacher, the dashboard and Sessions page listed live sessions, including:

- `/teacher/sessions/3dfea9ea-98d7-4e44-8b02-b9c8cea07d31`
- `/teacher/sessions/312c0024-383f-4df4-ad18-4ac20a932206`

Opening the office-hours session still produced a Next 404. The page still fetches session data through Go and then compares Go-owned session fields to `auth()`/local DB-derived viewer data. When the identity boundary drifts, valid listed sessions become inaccessible.

Recommendation:

- Move teacher session-page authorization into a single Go endpoint that returns the page payload for the authenticated claims user.
- If the server component continues to combine NextAuth and Go data, fail loudly in development on identity mismatch rather than returning a generic 404.
- Add an E2E test for "dashboard live session link opens the live workspace."

### P0: Platform admin dashboard is blocked by Go auth mismatch

File: `src/app/(portal)/admin/page.tsx:13`

Signing in as `admin@e2e.test` succeeded and `/api/auth/session` returned `isPlatformAdmin: true`, but `/admin` rendered an empty/error page. Console logs showed:

```text
ApiError: API 403: /api/admin/stats
```

Directly opening `/api/admin/stats` returned:

```json
{"error":"Platform admin required"}
```

This is another high-impact symptom of the same auth split: the NextAuth session knows the user is an admin, but the Go admin endpoint authorizes a different or stale token.

Recommendation:

- Fix the canonical auth issue first.
- Add a defensive admin dashboard error state so a single stats failure does not produce a blank page.
- Add an admin E2E smoke test that loads `/admin` and `/admin/orgs`.

### P1: Student join-class flow still appears successful but does not enroll the student

File: `src/components/student/join-class-dialog.tsx:23-36`

As Alice Student, submitting the visible class join code `KWMLCWG5` closed the form and returned to the empty dashboard. `/student` still showed "No classes yet." Direct `/api/classes/mine` returned Eve's instructor classes because of the auth split, so the mutation is not trustworthy from the student point of view.

The client implementation also remains too optimistic: it closes the dialog immediately on `res.ok`, clears the code, and only calls `router.refresh()`. It does not check that the returned class is present in the student's own class list, nor does it show a post-join confirmation.

Recommendation:

- After join, fetch a canonical student class list with the same identity source used by the dashboard.
- Keep the dialog open and show an error if the class is not visible to the student after the mutation.
- Add an E2E test: teacher exposes join code, student joins, class appears on dashboard and `/student/classes`.

### P1: Org-admin primary navigation points to inaccessible teacher routes

File: `src/lib/portal/nav-config.ts:15-16`

The org-admin sidebar includes:

- `Units` -> `/teacher/units`
- `Sessions` -> `/teacher/sessions`

As Frank OrgAdmin, clicking/opening `/teacher/units` redirected back to `/org`. The route requires the teacher portal role, so org admins without the teacher role get dead-end primary navigation.

Recommendation:

- Provide org-scoped `/org/units` and `/org/sessions` routes, or remove those entries from the org-admin nav.
- If org admins should access teacher unit/session tooling, make the portal role/access model support that explicitly instead of linking across portal boundaries.

### P1: Registration role selector is ignored, creating accounts with no selected role

Files:

- `src/app/(auth)/register/page.tsx:24-37`
- `src/app/api/auth/register/route.ts:8-12`
- `src/app/api/auth/register/route.ts:41-58`

The registration UI asks "I am a..." and lets the user choose Teacher or Student. The client sends `role`, but the API schema ignores it and only creates a user and auth provider. It does not create a student role, teacher role, org membership, or any onboarding record based on the selected role.

New users then land in `/onboarding`, whose student path says the teacher will add them later and whose teacher path points at `/register-org`. The role selector sets an expectation that signup will personalize onboarding, but it currently has no backend effect.

Recommendation:

- Either remove the role selector and make onboarding explicit, or persist the selected intent and use it to tailor the next step.
- If a student signs up from a class invite/join context, carry that intent through registration.
- Add tests that assert selected registration intent changes the resulting onboarding state.

### P2: Deep links are still lost when unauthenticated users hit portal URLs

File: `src/components/portal/portal-shell.tsx:20-28`

Opening `/teacher/classes/3796febc-8d7b-477a-9ecf-75be3143471d` while signed out redirected to:

```text
/login
```

There was no `callbackUrl`. The current implementation attempts to read `x-invoke-path` or `x-url`, but those headers were not present in this runtime.

Recommendation:

- Use middleware to capture the request URL before the server component render, or pass the current path through a supported request header.
- Add a browser test for a protected deep link: signed-out request -> login with callback -> original route.

### P2: Organization portal is mostly placeholder pages despite full primary navigation

Files:

- `src/app/(portal)/org/teachers/page.tsx:1-8`
- `src/app/(portal)/org/students/page.tsx:1-8`
- `src/app/(portal)/org/courses/page.tsx:1-8`
- `src/app/(portal)/org/classes/page.tsx:1-8`
- `src/app/(portal)/org/settings/page.tsx:1-8`

As Frank OrgAdmin, the dashboard shows real counts, but the primary nav routes for Teachers, Students, Courses, Classes, and Settings all render "coming soon" pages. This makes the organization dashboard operationally shallow: it exposes counts but no way to inspect or act on the underlying entities.

Recommendation:

- Ship read-only list views for each count before exposing the nav items.
- Or reduce org nav to the dashboard until those pages are actionable.

### P2: Duplicate org memberships cause repeated React key errors in unit pages

Files:

- `src/app/(portal)/teacher/units/page.tsx:137-146`
- `src/app/(portal)/teacher/units/page.tsx:300-303`
- `src/app/(portal)/teacher/units/new/page.tsx:45-56`
- `src/app/(portal)/teacher/units/new/page.tsx:164-168`

`/api/orgs` returned two memberships for the same org because Eve is both `teacher` and `org_admin` in Bridge Demo School. The unit pages filter memberships but do not dedupe by `orgId`, then render `<option key={org.orgId}>`. React logged repeated duplicate-key errors for `d386983b-6da4-4cb8-8057-f2aa70d27c07`.

Recommendation:

- Dedupe memberships by `orgId` before storing/rendering org options.
- Prefer a backend response shape that returns distinct organizations with roles attached, rather than one row per role for UI selectors.

### P2: Parent "My Children" remains duplicate navigation

File: `src/app/(portal)/parent/children/page.tsx:1-5`

The parent sidebar still advertises `My Children`, but `/parent/children` immediately redirects to `/parent`. The nested-link issue from the previous review is fixed, but this nav item still has no distinct destination or active route.

Recommendation:

- Implement a real children list view, even if it is a simple read-only list.
- Or remove the nav item until it is distinct from the dashboard.

### P2: Problem editor still has no responsive fallback

File: `src/components/problem/problem-shell.tsx:177-255`

The problem workspace still uses a fixed three-pane layout with `min-w-[360px]` on the left and `min-w-[320px]` on the right inside an `overflow-hidden` viewport. With a portal sidebar, this remains risky on smaller laptop widths and has no mobile/tablet fallback.

Recommendation:

- Collapse side panels into tabs/drawers below a breakpoint.
- Let the editor claim full width on narrow viewports.
- Add visual regression checks around common laptop and mobile widths.

### P2: Root layout script triggers a React dev overlay on every route

File: `src/app/layout.tsx:45-47`

Every route produced a dev console error:

```text
Encountered a script tag while rendering React component. Scripts inside React components are never executed when rendering on the client.
```

The Next.js issue overlay appeared during review and sometimes obscured the page. This is especially disruptive for local QA because the page itself may be functionally usable while the overlay dominates the viewport.

Recommendation:

- Move the theme bootstrap script to `next/script` with an appropriate strategy, or use an inline script pattern compatible with the App Router.
- Add a smoke test or lint guard that fails on React/Next runtime errors in dev.

### P3: Plan 038 status and post-execution report were not updated

File: `docs/plans/038-review-driven-fixes.md:7-9`

The plan was reportedly completed and code has landed, but the plan still says:

```text
Status: Not started
```

The post-execution report section is still empty. This makes it hard for future agents to distinguish completed work, deferred work, and known leftovers.

Recommendation:

- Update the plan status and post-execution report.
- Explicitly list which previous findings were fixed, partially fixed, or deferred.

## Confirmed Improvements Since Review 001

- Teacher Schedule and Reports are no longer in teacher primary nav.
- Parent Reports is no longer in parent primary nav.
- Parent dashboard no longer nests a `Link` inside another `Link`.
- Invalid course IDs now render a clean 404.
- Class detail now checks session status `live` instead of `active`.
- Personal unit creation succeeded in browser.
- Unit editor did not show duplicate Tiptap extension warnings in this pass.

## Verification Notes

Verified in browser:

- Teacher, student, parent, org admin, and platform admin login/session state.
- Auth identity drift across `/api/auth/session`, `/api/me/memberships`, `/api/classes/mine`, and `/api/admin/stats`.
- Teacher dashboard and sessions list.
- Teacher live session link still 404s.
- Student join-class dialog still closes without class appearing.
- Org admin dashboard and nav destinations.
- Parent dashboard and `/parent/children` redirect.
- Teaching unit creation and editor entry.
- Invalid route handling for malformed IDs.

Not fully verified:

- Existing regular Chrome session behavior.
- Mobile screenshots across all portals.
- Full problem editor execution flow, Hocuspocus realtime, and Pyodide execution.
- Server logs on the remote machine behind the SSH tunnel.


# Localhost 3003 Portal Review

Date: 2026-04-26
Target: `http://localhost:3003`
Reviewer: Codex

## Scope

This review covered the public landing/register/login surfaces, role portals, teacher course/class/session authoring flows, student class join behavior, parent navigation, teaching unit creation/editor entry, invalid detail routes, and responsive behavior of the problem editor.

The site was accessed through the Codex in-app browser against `localhost:3003`. The server was later clarified to be running on another machine and exposed locally through an SSH tunnel. Existing demo users were used, and review-only entities were created to exercise teacher workflows.

## Test Data Created

- Course: `Codex Review Course 20260426193844`
- Course ID: `acb3a714-dd04-473e-97aa-57e537e74418`
- Class: `Codex Review Class acb3a714`
- Class ID: `44eec29a-96e8-4e4c-841d-6040a467f2ca`
- Join code: `KWMLCWG5`
- Unit: `Codex Review Unit acb3a714`
- Unit ID: `223f1e61-25d6-4a67-83bb-bfd498b62f00`

## Executive Summary

The highest-priority issue is not a single broken page. It is runtime consistency across Next Auth, server components, client-side fetches, and the Go API. The app can create and list live sessions, but the session page can deny access because Next and Go resolve the same browser request to different user IDs. Until that identity boundary is made canonical, QA results for teacher, student, and parent workflows can be misleading.

The second major theme is workflow promise mismatch. Several primary navigation entries either point to placeholders or duplicate another page. Some mutations appear successful but are not reflected deterministically. These behaviors make the product feel less reliable than the underlying feature set actually is.

Recommended next-cycle priority:

1. Fix auth identity consistency between Next and Go.
2. Normalize session lifecycle semantics across `active` and `live`.
3. Remove or explicitly fence legacy Next API shadows for migrated Go routes.
4. Harden teacher authoring and live-session workflows.
5. Tighten navigation promises and responsive classroom workspaces.

## Findings

### P0: Next and Go auth can resolve different users in one request

Files:

- `src/lib/api-client.ts:34-38`
- `platform/internal/auth/middleware.go:41-45`
- `src/app/(portal)/teacher/sessions/[sessionId]/page.tsx:28-36`

In the in-app browser, `/api/auth/session` identified Eve Teacher as:

```text
d0d3b031-a483-4214-97fb-48c9584f4dcb
```

Go-backed endpoints in the same browser context operated as:

```text
da5cef74-66e5-4946-bf56-409b23f34503
```

Evidence:

- `/api/auth/session` returned `session.user.id = d0d3b031-a483-4214-97fb-48c9584f4dcb`
- `/api/me/memberships` returned memberships for `userId = da5cef74-66e5-4946-bf56-409b23f34503`
- `/api/sessions/3dfea9ea-98d7-4e44-8b02-b9c8cea07d31` returned a live session with `teacherId = da5cef74-66e5-4946-bf56-409b23f34503`

The teacher session page combines both identity sources. It calls `auth()` for `viewerId`, then calls the Go API through `api()`, then compares Go's `liveSession.teacherId` to NextAuth's `session.user.id`. If those IDs differ, the page returns `notFound()` even when the Go API and session data are valid.

The likely mechanism is cookie precedence drift. `api-client.ts` manually prefers `__Secure-authjs.session-token` before `authjs.session-token`, and Go middleware also prefers the secure cookie first. On plain `http://localhost`, Auth.js commonly uses `authjs.session-token`. If both cookies exist in the browser jar, NextAuth and the Go bridge can consume different tokens.

This also explains why the live session could open in regular Chrome while 404ing in the in-app browser: the two browsers had different localhost cookie jars. This is not an SSH tunnel network failure; the in-app browser successfully fetched the Go session JSON through the tunnel.

Recommendation:

- Add a dev-only auth diagnostic that reports non-secret `nextAuthUserId`, `goClaimsUserId`, and `cookieNameUsed`.
- Make the server API client forward the same token selected by Auth.js, or centralize token selection in one shared helper.
- Treat a Next/Go identity mismatch as a diagnosable auth error in development instead of a silent `notFound()`.
- Add integration coverage for server components that fetch Go data and then authorize against the current user.

### P0: Live-session route authorization mixes two sources of truth

File: `src/app/(portal)/teacher/sessions/[sessionId]/page.tsx:28-36`

`loadTeacherSessionPageData` authorizes orphan/office-hours sessions by comparing `liveSession.teacherId` from Go to `viewerId` from NextAuth. For class sessions, it fetches class membership through local Next-side DB helpers and compares those records to the same NextAuth viewer ID.

That means the route is vulnerable to any drift between:

- Auth.js token identity
- Go claims identity
- local DB helper identity
- remote server DB identity behind the SSH tunnel

Recommendation:

- Use one canonical authorization path for session pages.
- Prefer asking Go for a session page authorization payload for the authenticated claims user, rather than reassembling authorization from mixed Next and Go data sources.
- If local DB helpers must remain, assert that the local viewer ID and Go claims identity match before evaluating access.

### P1: Session status semantics are split between `active` and `live`

File: `src/app/(portal)/teacher/classes/[id]/page.tsx:55-56`

The class detail page still searches for `status === "active"`, while observed Go-created sessions use `status: "live"`. This caused the teacher dashboard and class detail page to disagree: the dashboard could show a class as live while the class detail page still offered `Start Live Session`.

Recommendation:

- Consolidate session lifecycle statuses into one shared enum.
- Update frontend checks, Go store/handlers, schema comments, seed data, and tests together.
- Add regression coverage for "start session, return to class detail, see active/live session affordance".

### P1: Migrated API routes still have local Next shadows

File: `next.config.ts:7-31`

`next.config.ts` proxies many `/api/*` routes to Go, including courses, classes, sessions, units, topics, problems, and `/api/me`. Legacy Next handlers still exist for some of the same surfaces. This makes it difficult to know whether browser behavior, server-component behavior, and tests are exercising the same implementation.

Recommendation:

- Remove migrated Next handlers where possible.
- Where temporary shadows must remain, document ownership and add contract tests proving the route is resolved to the intended Go handler.
- Add logging or test assertions around route ownership during migration.

### P1: Student join-class flow appears successful but does not enroll

File: `src/components/student/join-class-dialog.tsx:23-32`

As Alice Student, submitting visible teacher join codes closed the dialog as if successful, but `/student` and `/student/classes` continued to show no classes. The client treats any 2xx response as success and refreshes, but does not validate that the class appears in the student's class list.

This may be affected by the broader auth identity split, so the backend enrollment result should be rechecked after auth is fixed.

Recommendation:

- Return and display the joined class from the join endpoint.
- After mutation, verify that `/api/classes/mine` includes the joined class or surface a clear error.
- Add an E2E test that logs in as a student, joins a teacher-created class, and verifies the class appears in both dashboard and class list.

### P1: Course topic creation is not reflected deterministically

File: `src/app/(portal)/teacher/courses/[id]/page.tsx:47-52`

After adding a topic to a newly-created course, the page initially still showed `Topics (0)` until navigation or refresh settled. Teachers need deterministic feedback for authoring actions.

Recommendation:

- Use deterministic server action revalidation, explicit client refresh, or optimistic state.
- Show pending/error states for topic mutation.
- Add coverage that a created topic appears immediately after the add action completes.

### P1: Problem editor becomes unusable on narrower viewports

File: `src/components/problem/problem-shell.tsx:177-255`

The student editor uses a fixed three-pane flex layout with hard minimum widths, including side panes around 360px and 320px, inside an overflow-hidden shell. With a portal sidebar open, common laptop widths can squeeze or clip the editor. Mobile has no clear fallback layout.

Recommendation:

- Add responsive breakpoints for collapsing side panels.
- Let the main editor claim usable width before secondary panels.
- Consider tabs, drawers, or resizable panes for narrow viewports.
- Add Playwright visual checks at laptop and mobile widths.

### P2: Deep links are lost after login

File: `src/components/portal/portal-shell.tsx:23-30`

Unauthenticated portal access redirects to `/login` without preserving the original destination. The login flow then pushes to `/`, forcing users who opened shared class, session, or problem links to navigate manually after signing in.

Recommendation:

- Include a callback URL when redirecting unauthenticated users.
- Validate callback targets to same-origin app paths.
- After login, route users back to the originally requested page when allowed.

### P2: Parent dashboard nests links

File: `src/app/(portal)/parent/page.tsx:38-57`

Each child card is a `Link`, and the `Watch Live` action inside it is another `Link`. Nested anchors are invalid HTML and can produce confusing click behavior or hydration issues. `stopPropagation` does not make the markup valid.

Recommendation:

- Make the card a non-anchor container with explicit child actions.
- Or make only one part of the card the link and render secondary actions as sibling links/buttons.

### P2: Primary navigation exposes placeholder destinations

Files:

- `src/lib/portal/nav-config.ts:35-36`
- `src/app/(portal)/teacher/schedule/page.tsx:1`

Teacher Schedule, Teacher Reports, and Parent Reports appear as top-level navigation items but resolve to placeholder/coming-soon experiences. For a workflow app, primary navigation should lead to actionable surfaces.

Recommendation:

- Hide placeholder entries until there is a minimally useful view.
- Or ship lightweight read-only views that support planning and review.

### P2: Parent My Children route duplicates dashboard

File: `src/app/(portal)/parent/children/page.tsx:3-4`

The parent sidebar advertises `My Children` as a separate route, but `/parent/children` immediately redirects to `/parent`. Users get no unique list view or active navigation state.

Recommendation:

- Implement a dedicated children list/detail surface.
- Or remove the nav item until it provides distinct behavior.

### P2: Invalid course IDs do not fail cleanly

File: `src/app/(portal)/teacher/courses/[id]/page.tsx:40-45`

Opening `/teacher/courses/does-not-exist` produced an API 400 and no meaningful route content. Invalid UUIDs are normal URL edge cases and should render a stable not-found or error boundary state.

Recommendation:

- Validate route params before API calls.
- Map backend 400 validation failures for detail IDs to `notFound()` where appropriate.
- Add route tests for malformed and unknown IDs.

### P2: Personal unit creation defaults to an unauthorized path

File: `src/app/(portal)/teacher/units/new/page.tsx:94-107`

The Create Unit form defaults to `Personal` and sends `scopeId = session.user.id`, but Eve Teacher received `not authorized for scope`. Org-scoped unit creation worked.

Recommendation:

- Decide whether personal-scoped units are supported for teachers.
- If supported, fix authorization.
- If not supported, do not default the UI to an unavailable scope.

### P2: Unit editor initializes with runtime stability warnings

File: `src/components/editor/tiptap/teaching-unit-editor.tsx:283-289`

Opening a newly-created org-scoped unit editor produced duplicate React key warnings and duplicate Tiptap extension warnings for link/underline. These warnings point to unstable editor composition and can become real editing bugs once content grows.

Recommendation:

- Audit extension registration and ensure each Tiptap extension is included once.
- Fix duplicate React keys in rendered editor lists/toolbars.
- Add a smoke test that opens the editor and fails on console errors/warnings for duplicate keys/extensions.

## Architectural Themes

### Auth Boundary

The app currently depends on Auth.js, manual cookie forwarding, Go-side token parsing, Next server components, and local DB helpers all agreeing on identity. That agreement is implicit rather than enforced. The live-session 404 demonstrated that the agreement can fail silently.

The next cycle should make identity resolution observable and canonical.

### Migration Boundary

The Go migration is far enough along that route ownership matters more than raw endpoint coverage. Shadow Next handlers and mixed local/Go access checks make debugging slower and QA less reliable.

The next cycle should make route ownership explicit and testable.

### Product Navigation

The portals are close to useful, but several nav entries overpromise. Placeholder routes and duplicate routes cost user trust because they make working features feel unfinished.

The next cycle should align navigation with available workflows.

### Mutation Feedback

Several authoring flows need stronger post-mutation confirmation. Teachers and students should not have to infer whether creation, join, or start-session actions succeeded by navigating around the app.

The next cycle should standardize mutation feedback and verification.

## Suggested Next Dev Cycle

1. Auth consistency hardening
   - Add a dev-only identity diagnostic.
   - Normalize token/cookie selection.
   - Add tests for Next server component to Go API identity consistency.

2. Session model cleanup
   - Consolidate `active` and `live`.
   - Re-test teacher dashboard, class detail, start-session, and session entry.

3. Route ownership cleanup
   - Remove or fence migrated Next API handlers.
   - Add route ownership checks for migrated endpoints.

4. Workflow reliability
   - Fix student join verification.
   - Fix topic mutation refresh.
   - Fix invalid detail route handling.
   - Fix unit scope default/authorization.

5. Navigation and layout polish
   - Remove or implement placeholder nav.
   - Implement or hide parent children route.
   - Add responsive behavior to problem editor.

## Verification Notes

Verified in browser:

- Public landing, login, register, and design routes load.
- Teacher login works for `eve@demo.edu`.
- Teacher can create course, class, live sessions, and org-scoped teaching unit.
- Go API can return live session JSON through tunneled `localhost:3003`.
- In-app browser auth identity split is reproducible between `/api/auth/session` and Go-backed endpoints.
- Parent reports and teacher schedule/report surfaces are placeholder-level.
- Invalid course ID route fails poorly.

Not fully verified:

- Regular Chrome behavior, because Codex cannot manipulate the user's existing Chrome session through the in-app browser tools.
- Whether student join failure persists after auth identity consistency is fixed.
- Whether duplicate user rows exist in the remote database.
- Full mobile/responsive matrix for problem editor beyond layout inspection and observed pressure.


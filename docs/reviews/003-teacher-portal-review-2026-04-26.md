# Teacher Portal Review - localhost:3003

Date: 2026-04-26
Scope: Teacher portal only: dashboard, courses, classes, sessions, units, live-session entry points, and teacher authoring surfaces.

## Summary

The teacher portal has improved since the earlier broad review: the teacher primary navigation is now narrowed to actionable teacher surfaces, and the class detail page now recognizes `live` sessions. However, the portal is not ready for the next teacher workflow milestone until auth identity is made canonical across Next/Auth.js and the Go API. In the tested browser session, the shell showed Eve Teacher while Go-backed teacher endpoints authorized as a different user ID, which caused live sessions to be listed but not openable and caused class details to display another instructor identity.

The next development cycle should prioritize identity consistency first, then stabilize the live-session workspace semantics, then clean up unit-library org handling and detail-route failure states.

## Environment

- App URL: `http://localhost:3003`
- Browser context: Codex in-app browser
- Tested account: `eve@demo.edu`
- Current URL at end of pass: `/teacher/sessions`
- Repository branch during review: `main`

## Findings

### [P0] Teacher pages can render another user's backend data under the current teacher header

File: `platform/internal/auth/middleware.go:67-76`

In browser, `/api/auth/session` identified the signed-in teacher as Eve (`d0d3...`), while Go-backed teacher endpoints such as `/api/me/memberships` and `/api/sessions/{id}` authenticated as `da5...`. The teacher class detail then showed the portal shell as Eve Teacher while the class instructor content came from the other backend identity. The cookie selection path still allows the Go runtime and NextAuth runtime to resolve different users, which is both a privacy issue and the root cause of several teacher workflow failures.

Evidence:

- `/api/auth/session` returned Eve with user ID `d0d3b031-a483-4214-97fb-48c9584f4dcb`.
- `/api/me/memberships` returned memberships for user ID `da5cef74-66e5-4946-bf56-409b23f34503`.
- `/api/sessions/312c0024-383f-4df4-ad18-4ac20a932206` returned `teacherId: da5cef74-66e5-4946-bf56-409b23f34503`.
- `/teacher/classes/44eec29a-96e8-4e4c-841d-6040a467f2ca` rendered the shell as Eve Teacher but listed `Yunzhi Zhou (Chris)` as instructor.

Recommendation:

Make the Go API and Next server components consume one canonical identity source per request. Either forward the exact token Auth.js selected to Go for all server and client paths, or expose a Go-authenticated `/api/me` source and stop mixing it with `auth()` for authorization decisions.

### [P0] Live session page authorizes with mixed identity sources

File: `src/app/(portal)/teacher/sessions/[sessionId]/page.tsx:30-61`

The page fetches the session through Go, then compares the Go-returned `teacherId` and local class membership against `viewerId` from `auth()`. With the current identity split, sessions listed on the teacher dashboard and class detail open to a blank/not-found page even though `/api/sessions/{id}` returns a valid live session. This blocks the core live teaching workflow.

Evidence:

- Teacher dashboard listed live session `312c0024-383f-4df4-ad18-4ac20a932206`.
- Opening `/teacher/sessions/312c0024-383f-4df4-ad18-4ac20a932206` rendered only the dev-tools button / blank route shell.
- The direct Go-backed API endpoint returned the live session successfully.

Recommendation:

Use one authorization path for this route. Do not fetch session ownership from Go and compare it to a different NextAuth-derived identity. If the route remains a server component, prefer a server API call that returns both session data and the backend-authorized viewer relationship.

### [P1] Ended sessions route into the live-session workspace

File: `src/app/(portal)/teacher/sessions/page.tsx:82-86`

The sessions list links every session, including `ended`, to `/teacher/sessions/{id}`. That target renders the same `TeacherDashboard` with a `Live Session` header, timer, invite controls, student controls, and End Session action. Teachers need a distinct ended-session review surface or ended sessions should not link to the live control room.

Evidence:

- `/teacher/sessions` shows ended sessions with links to `/teacher/sessions/{id}`.
- The target component does not branch on `status` before rendering the live dashboard.

Recommendation:

Introduce a read-only session review route/state for ended sessions, or disable ended-session links until review functionality exists.

### [P2] Unit organization pickers duplicate the same school

File: `src/app/(portal)/teacher/units/page.tsx:137-147`

`/api/orgs` returns one row per role, so Eve receives both teacher and org_admin rows for the same org. The unit library filters roles but does not dedupe by `orgId`, then renders `<option key={org.orgId}>`, producing duplicate `Bridge Demo School` choices and React duplicate-key errors in browser.

Evidence:

- `/api/orgs` returned two active rows for `d386983b-6da4-4cb8-8057-f2aa70d27c07`.
- The Org Library picker showed two `Bridge Demo School` options.
- Console logged duplicate React key errors for that org ID.

Recommendation:

Dedupe memberships by `orgId` in the API or shared client helper before rendering org pickers. This should be handled once and reused by teacher units, unit creation, org-admin nav, and any future org-scoped authoring forms.

### [P2] Create Unit briefly defaults to the wrong scope

File: `src/app/(portal)/teacher/units/new/page.tsx:27-56`

The form starts with `scope='personal'` and only switches to `org` after `/api/orgs` returns. In browser, the first rendered state showed Personal with no org picker before flipping to Organization. A fast submit can create a unit in the wrong scope, and the UI feels unstable as the default changes after first paint.

Evidence:

- Initial `/teacher/units/new` snapshot showed Scope = Personal and no org picker.
- After org loading completed, Scope changed to Organization with duplicate org options.

Recommendation:

Keep the form in a loading/disabled state until scope resolution completes, or move org lookup to a server component so the initial render already knows the correct default.

### [P2] Topic editor has no failure state for missing or unauthorized topics

File: `src/app/(portal)/teacher/courses/[id]/topics/[topicId]/page.tsx:30-67`

The topic editor only calls `setTopic` on `res.ok`; any 400, 403, 404, or network failure leaves the teacher on an infinite `Loading...` state. I reproduced this with an invalid topic route. The same pattern would hide auth or backend failures during authoring.

Evidence:

- Valid topic route eventually rendered the editor.
- Invalid topic route remained on `Loading...`.

Recommendation:

Track loading, error, and not-found states separately. Invalid UUIDs should resolve to a stable not-found state, unauthorized responses should show an access/error state, and save failures should be visible to the author.

### [P2] Create-class deep links do not validate course IDs before hitting the API

File: `src/app/(portal)/teacher/courses/[id]/create-class/page.tsx:17-24`

`/teacher/courses/does-not-exist/create-class` produced a blank page and an API 400 console error. The parent course detail route validates UUIDs before fetching, but this sibling route does not, so malformed shared or bookmarked URLs fail as runtime errors instead of a stable not-found state.

Evidence:

- `/teacher/courses/does-not-exist/create-class` rendered an empty page shell.
- Console contained `ApiError: API 400: /api/courses/does-not-exist`.

Recommendation:

Mirror the UUID validation and bad-request-to-`notFound()` handling used by `src/app/(portal)/teacher/courses/[id]/page.tsx`.

## Positive Notes

- Teacher primary navigation no longer exposes Schedule and Reports as first-class placeholder destinations.
- Class detail now searches for `status === "live"` when deciding whether to show Resume Session.
- Course list, course detail, class list, class detail, sessions list, and unit library all render enough structure to continue workflow testing once auth is corrected.

## Suggested Next Cycle Order

1. Fix canonical identity across Auth.js, server API client, browser-proxied Go routes, and Go middleware.
2. Re-test teacher dashboard to live-session entry from dashboard, class detail, and sessions list.
3. Split live and ended session UX so historical sessions cannot render active controls.
4. Dedupe org memberships in one shared layer and apply it to unit library and create-unit.
5. Add route-level error/not-found handling for topic editor and create-class.

## Verification Notes

This was a review-only pass. I did not modify application code or run automated tests. Browser verification was performed manually through the in-app browser against `localhost:3003`.

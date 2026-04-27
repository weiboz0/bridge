# Comprehensive Browser Review - localhost:3003

Date: 2026-04-27
Scope: Whole website, using the Codex in-app browser first: public entry, auth, role portals, teacher/student/org/admin/parent routes, live-session APIs, and source-level follow-up for routes blocked by the browser-visible build error.

## Summary

The current site is not usable through the browser because every Next-rendered page I tried hits a Turbopack build error in `src/middleware.ts`. This blocks `/`, `/login`, `/register`, `/design`, `/teacher`, `/student`, and `/api/auth/session`. Proxied Go API endpoints such as `/api/me/memberships`, `/api/classes/mine`, `/api/org/dashboard`, and `/api/sessions` still respond, which is useful diagnostically but also shows that browser behavior is now split between a broken Next runtime and a still-running Go API.

Several prior fixes are visible in source: join-class now verifies enrollment before closing, teacher live-session page authorization has moved to a Go `teacher-page` payload, teacher nav no longer exposes Schedule/Reports, org-admin nav no longer links into inaccessible teacher routes, and teacher unit org pickers now dedupe memberships. The next development cycle should first restore Next page rendering, then close the remaining backend authorization gaps and finish the Units-only teaching-material migration.

## Browser Evidence

- `/login`, `/`, `/design`, `/teacher`, `/student`, and `/api/auth/session` displayed the Next.js build overlay.
- The overlay error was: `Next.js can't recognize the exported config field in route. matcher needs to be a static string or array of static strings or array of static objects.`
- `/api/me/memberships` returned active org-admin and teacher memberships for `da5cef74-66e5-4946-bf56-409b23f34503`.
- `/api/classes/mine` returned instructor-role classes including join codes `KWMLCWG5`, `HM8D3WHB`, and `7Y3P4J3C`.
- `/api/sessions?status=ended&limit=5` returned ended sessions.
- `/api/sessions/21c55d7d-1a9d-47ba-b1ec-a4df3876e15e/teacher-page` returned a full teacher dashboard payload even though that session's status was `ended`.

## Findings

### [P0] Next middleware config blocks every Next-rendered page

File: `src/middleware.ts:16-20`

`src/middleware.ts` imports `middlewareMatcher` from another module and assigns it to `config.matcher`. Next/Turbopack requires `config.matcher` to be statically analyzable in the same file, so the dev server renders a build overlay instead of the app. This is currently the highest-priority issue because auth, public pages, portal shells, and debug pages cannot be tested or used in-browser.

Evidence:

- Browser `/login` showed the Turbopack build error for `src/middleware.ts:18:14`.
- Browser `/`, `/design`, `/teacher`, `/student`, and `/api/auth/session` all showed the same route-level build overlay or blank overlay state.
- The code comment says the matcher was moved for unit testing, but that testability tradeoff breaks the runtime.

Recommendation:

Inline the literal matcher array directly in `src/middleware.ts` so Next can statically parse it. If the matcher contract needs tests, export a duplicate test-only constant from a separate module or assert against the literal through a small static source test, but do not feed an imported value into `config.matcher`.

### [P1] Runtime registration ignores role intent and teacher onboarding points to a missing route

Files: `next.config.ts:14`, `platform/internal/handlers/auth.go:23-59`, `src/app/onboarding/page.tsx:35-36`

The registration page still sends `role`, and the shadow Next route stores `intendedRole`, but `next.config.ts` proxies `/api/auth/register` to Go. The Go handler only decodes `name`, `email`, and `password`, so role intent is discarded at runtime. The onboarding page then tries to send teacher-intent users to `/register-org`, but there is no `src/app/register-org` route in the app.

Evidence:

- `GO_PROXY_ROUTES` includes `/api/auth/register`, so the Go handler wins over the local Next handler.
- Go `Register` has no `role` field in its request body and `RegisterInput` has no intended-role field.
- `src/app/onboarding/page.tsx` redirects teacher intent to `/register-org`; no matching route exists.

Recommendation:

Move intended-role persistence into the Go registration path, update `/api/me/roles` or onboarding to use that canonical value, and either implement `/register-org` or point teachers at an existing org-request flow. Add an E2E case for teacher signup and student signup with invite code once the middleware build error is fixed.

### [P0] Class detail API still exposes class metadata to any authenticated user

File: `platform/internal/handlers/classes.go:157-174`

`GetClass` checks only that the caller is authenticated, then returns the class by UUID. It does not require class membership, class instructor/TA status, org-admin access, or platform-admin access. Student pages call this endpoint before additional class-specific checks, so class IDs remain a metadata exposure boundary.

Evidence:

- The handler reads claims, fetches `h.Classes.GetClass(...)`, and writes the class JSON without any authorization check against membership or org role.

Recommendation:

Gate `GetClass` by class membership for students, instructor/TA membership for staff, org-admin membership for the class org, or platform admin. Return 404 for unauthorized callers when class existence should not be leaked.

### [P0] Live-session join still accepts a session ID without class membership or invite proof

File: `platform/internal/handlers/sessions.go:300-335`

`JoinSession` verifies that the session exists and is `live`, then creates a participant row for the caller. It does not require class membership, an explicit invitation, or an invite-token path. Any authenticated user who obtains a live session ID can attempt to join the live room directly.

Evidence:

- The join handler does not call class membership checks, invite-token validation, or the stricter `student-page` authorization logic.
- Browser `/api/sessions?status=live&limit=5` exposed live session IDs to the current teacher identity; the endpoint shape makes direct joins by ID the security boundary.

Recommendation:

Make direct `/api/sessions/{id}/join` require either class membership or an existing participant/invitation. Keep invite-token joins on `/s/{token}` or an explicit token-backed endpoint so possession of a raw session UUID is not enough.

### [P1] Ended sessions still render the live teacher workspace payload

Files: `platform/internal/handlers/sessions.go:994-1091`, `src/components/session/teacher/teacher-header.tsx:92-139`, `src/components/session/teacher/teacher-dashboard.tsx:360-372`

The student page endpoint rejects ended sessions, but `GetTeacherPage` does not. Browser API testing showed an ended session returning the same teacher-page payload used by `/teacher/sessions/{id}`. The React workspace always renders a `Live Session` header, timer, invite controls, and `End Session` action, so ended sessions can route teachers back into active-control UI.

Evidence:

- Browser `/api/sessions?status=ended&limit=5` returned `21c55d7d-1a9d-47ba-b1ec-a4df3876e15e` with `status: "ended"`.
- Browser `/api/sessions/21c55d7d-1a9d-47ba-b1ec-a4df3876e15e/teacher-page` returned a normal dashboard payload with the ended session.
- `TeacherHeader` hardcodes `Live Session` and always renders `End Session`.

Recommendation:

Branch `teacher-page` by status. Live sessions should render the live control room; ended sessions should return a read-only review payload or redirect to a dedicated review route. At minimum, `/teacher/sessions/{id}` should not mount active invite/end controls for ended sessions.

### [P1] Session teaching material is still sourced from Topic lesson content instead of Units

Files: `platform/internal/handlers/sessions.go:970-1086`, `src/components/session/teacher/teacher-dashboard.tsx:273-289`, `src/components/session/student/student-session.tsx:61-94`

The current taxonomy decision is that Unit is the canonical teaching-material object, while Topic/Syllabus Area should organize focus and sequencing. Session payloads still return `courseTopics` with `lessonContent`, and both teacher and student session components render `LessonRenderer` from topic-owned content. That preserves a second lesson-authoring surface and will keep content delivery split across models.

Evidence:

- Browser `teacher-page` payload included `courseTopics` with `lessonContent`.
- `GetTeacherPage` builds `teacherPageTopicRef` from `t.LessonContent`.
- Student session still fetches `/api/sessions/{sessionId}/topics` and renders each topic's `lessonContent`.

Recommendation:

Change session focus areas to reference Units. Use Topic/Syllabus Area for title, order, outcomes, and Unit references only. Migrate any topic `lessonContent` into Units or provide a one-time conversion path, then stop rendering topic-owned teaching material in live sessions.

### [P2] Student unit route does not validate malformed IDs before fetching

File: `src/app/(portal)/student/units/[id]/page.tsx:60-93`

Teacher unit routes validate UUIDs before fetching, but the student unit route sends `id` directly into `/api/units/{id}` and `/api/units/{id}/projected`. A malformed URL therefore becomes a client-side API error/unavailable state instead of a stable route-level not-found. This matters because units will become the canonical student material surface.

Evidence:

- The browser started on `/student/units/does-not-exist`; the middleware build error prevented visual validation, but source shows there is no `isValidUUID` check in this route.
- `fetchUnit` throws on non-403/404 non-OK responses, so backend 400s become the generic error state.

Recommendation:

Mirror the teacher unit route behavior: validate `id` with `isValidUUID` before fetching and call `notFound()` for malformed IDs.

### [P2] Platform Admin primary navigation still includes a placeholder destination

Files: `src/lib/portal/nav-config.ts:8-12`, `src/app/(portal)/admin/settings/page.tsx:1-6`

The admin sidebar advertises `Settings` as a primary destination, but the page only says "Platform configuration coming soon." For an operational/admin portal, first-class navigation should lead to actionable views. This is lower risk than the teacher/student workflow blockers, but it still makes the admin IA feel unfinished.

Evidence:

- `nav-config.ts` includes `Settings` in admin `navItems`.
- `admin/settings/page.tsx` is a one-screen placeholder.

Recommendation:

Hide Settings until there is a minimally useful read-only configuration view, or ship the first useful controls/status fields with clear authorization.

### [P1] Shadow Next API handlers still make runtime ownership hard to reason about

Files: `next.config.ts:3-36`, `src/app/api/*`

The app proxies many `/api/*` surfaces to Go, but the legacy Next route files remain. The browser pass exposed a sharp version of this split: proxied Go endpoints worked while Next-owned endpoints and pages failed on middleware compilation. This makes review and testing confusing because two route stacks can be healthy or broken independently.

Evidence:

- Browser `/api/me/memberships`, `/api/classes/mine`, `/api/org/dashboard`, and `/api/sessions?...` returned Go JSON.
- Browser `/api/auth/session` hit the Next build overlay.
- `next.config.ts` still carries a TODO noting dozens of overlapping Next route files.

Recommendation:

Finish route ownership cleanup. For each migrated API, delete the shadow Next handler or add explicit contract tests proving every runtime path hits the intended implementation. Keep a short route ownership map in docs until the migration is complete.

## Positive Notes

- Join-class client now verifies that the class appears in `/api/classes/mine` before closing.
- Teacher session page now uses one Go `teacher-page` authorization payload instead of comparing Go session ownership against a separate NextAuth identity.
- Teacher class detail recognizes `status === "live"`.
- Teacher primary navigation is now focused on actionable surfaces.
- Org-admin navigation no longer sends org admins into teacher-only routes.
- Teacher unit org pickers dedupe org memberships in source.

## Suggested Next Cycle Order

1. Fix `src/middleware.ts` so the website renders again in the browser.
2. Add a smoke/E2E test that opens `/login`, `/`, one portal route, and `/api/auth/session` under the same dev server configuration.
3. Close backend authorization gaps for class detail and direct session join.
4. Split live-session and ended-session teacher UX.
5. Complete the Unit-centered teaching-material migration and demote Topic to Syllabus Area / Focus Area.
6. Repair registration/onboarding around the Go runtime path and the missing teacher org-registration destination.
7. Finish Go-vs-Next API route ownership cleanup.

## Verification Notes

Browser verification was performed with the Codex in-app browser against `http://localhost:3003`. Visual page review was blocked by the middleware build error, so the report combines browser-visible failures, browser API checks, and source review.

Automated verification was limited:

- `bun run lint` and `bunx tsc --noEmit` could not run because `bun`/`bunx` are not installed in this shell.
- `npm run lint` could not run because local dependencies are not installed (`eslint: command not found`).
- `npx tsc --noEmit` attempted to reach npm and failed due restricted DNS/network.

No application code was changed by this review.

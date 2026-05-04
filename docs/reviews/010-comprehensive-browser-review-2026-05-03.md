# Comprehensive Browser Review 010

Date: 2026-05-03
Target: `http://localhost:3003` through the in-app browser, backed by the remote machine exposed through the SSH tunnel.
Reviewer stance: system/platform admin, organization admin, teacher, student, parent, and unauthenticated invite/onboarding user.

## Executive Summary

The teacher and org-admin portals are materially more complete than the previous review cycle: teacher dashboard, units, courses, classes, focus-area terminology, ended-session handling, and org-admin read-only list pages all now render useful surfaces. The main navigation is also much cleaner.

The site is still not ready for a full end-to-end acceptance pass because three environment/runtime blockers distort core workflows:

1. The Go API is still resolving every authenticated request as `Dev User` via `DEV_SKIP_AUTH`, while the Next/Auth.js shell resolves the real signed-in browser user.
2. Live collaboration cannot mint realtime tokens because `HOCUSPOCUS_TOKEN_SECRET` is unset.
3. The parent portal crashes because the running database is missing the newly-required `parent_links` table.

Those three items should be treated as release/setup blockers before another product QA pass. After that, the next product iteration should focus on admin unit editing, student enrollment/session consistency, problem-bank workflow affordances, and admin-managed parent-child linking.

## Review Scope

I walked these surfaces in-browser:

- Platform admin: `/admin`, `/admin/orgs`, `/admin/users`, `/admin/units`
- Organization admin: `/org`, `/org/teachers`, `/org/students`, `/org/courses`, `/org/classes`, `/org/units`, `/org/settings`
- Teacher: `/teacher`, `/teacher/units`, `/teacher/problems`, `/teacher/sessions`, `/teacher/courses`, `/teacher/classes`, representative course/class/session detail pages
- Student: `/student`, `/student/classes`, `/student/code`, `/student/help`, direct `/student/sessions/{id}`
- Parent: `/parent`, `/parent/reports`
- Public/auth: `/login`, `/register`, `/onboarding`, invalid `/s/{token}`

Known test accounts used:

- `admin@e2e.test`
- `frank@demo.edu`
- `eve@demo.edu`
- `alice@demo.edu`
- `diana@demo.edu`

## Findings

### [P0] Go auth bypass is active in the review environment, splitting UI identity from API identity

Evidence:

- Signed in as Eve Teacher, `/api/auth/session` reported `eve@demo.edu` with user id `d0d3b031-a483-4214-97fb-48c9584f4dcb`.
- The Go-backed `/api/me/identity` endpoint reported `dev@localhost`, `Dev User`, user id `da5cef74-66e5-4946-bf56-409b23f34503`.
- Signed in as Alice Student, the shell showed Alice, but Go-backed endpoints still resolved the same `Dev User`.
- Signed in as the E2E platform admin, the Next shell showed `E2E Admin`, but Go admin stats returned HTTP 403.

Relevant code:

- `platform/internal/auth/middleware.go:83-110` injects the dev user whenever `DEV_SKIP_AUTH` is set.
- `src/app/(portal)/admin/page.tsx:16-24` and `:61-66` already surface a helpful diagnostic when the admin shell and Go API disagree.

Impact:

- Platform admin cannot use admin stats/org/user pages reliably.
- Student class membership appears empty even when the org data shows students enrolled.
- Direct student session links can render because Go thinks the caller is the dev teacher, while the shell shows Alice.
- The review environment can display one user's backend data under another user's UI header. This is a privacy and QA-validity blocker if this environment is used beyond isolated local development.

Recommendation:

- Disable `DEV_SKIP_AUTH` for any browser QA, staging, tunneled demo, or shared environment.
- Add a startup/runtime warning banner when Next sees a real Auth.js session but `/api/me/identity` returns `dev@localhost`.
- Consider failing non-localhost startup when `DEV_SKIP_AUTH` is set unless `APP_ENV=local` or an explicit `ALLOW_DEV_AUTH_OVER_TUNNEL=true` escape hatch is present.

### [P0] Parent portal crashes because the running database is missing `parent_links`

Evidence:

- Logging in as Diana Parent routed to `/parent`, which opened the Next runtime error overlay.
- The server error was `relation "parent_links" does not exist`.
- The failing query originates from `src/lib/parent-links.ts:17`, called by `src/app/(portal)/parent/page.tsx:12`.

Relevant code:

- `src/lib/parent-links.ts:15-25` now queries the new `parent_links` table directly.
- `drizzle/0024_parent_links.sql` contains the required table migration.

Impact:

- Parent dashboard and any route that redirects home to `/parent` are unusable.
- `/` also crashed while Diana was signed in because it redirected to the parent dashboard.
- This blocks parent review entirely and makes the app feel broken immediately after parent login.

Recommendation:

- Apply `drizzle/0024_parent_links.sql` to the remote database used by the tunneled server.
- Add a migration health check to startup or admin diagnostics so schema drift is caught before browser QA.
- Add a defensive route-level error state for parent dashboard data loading so a schema/config problem produces an operator-facing message instead of a generic runtime overlay.

### [P0] Live classroom collaboration cannot start because realtime token minting is not configured

Evidence:

- Opening the live teacher session and live student session produced browser console errors:
  `Realtime tokens not configured (HOCUSPOCUS_TOKEN_SECRET unset)`.
- The student live session rendered the editor shell, but the issues overlay appeared and realtime token minting failed.

Relevant code:

- `src/lib/realtime/get-token.ts:65-83` treats `/api/realtime/token` HTTP 503 as an unset `HOCUSPOCUS_TOKEN_SECRET`.
- `platform/internal/handlers/realtime_token.go` and `server/hocuspocus.ts` both require the same secret.
- `docs/setup.md` documents `HOCUSPOCUS_TOKEN_SECRET`.

Impact:

- Teacher watch mode, student live collaboration, unit editor realtime, and any Hocuspocus-backed classroom document cannot be accepted in this environment.
- The UI currently degrades into a developer console/overlay problem rather than a clear teacher-facing setup error.

Recommendation:

- Set the same `HOCUSPOCUS_TOKEN_SECRET` for the Go API and Hocuspocus server in the remote environment.
- Add a small in-app fallback banner for live sessions: "Realtime is not configured for this environment" with a retry/status link, instead of relying on console errors.
- Include realtime token health in `/api/auth/debug` or a broader `/api/health` diagnostics page.

### [P1] Platform admin Units links route into teacher-only editor pages

Evidence:

- `/admin/units` renders unit titles as links to `/teacher/units/{id}/edit`.
- As the E2E platform admin, opening one of those links redirected back to `/admin`, which then showed the admin stats HTTP 403 error caused by the identity split.

Relevant code:

- `src/app/(portal)/admin/units/page.tsx:154-159` builds the editor link with the teacher portal path.

Impact:

- A first-class platform admin nav item shows editable-looking links, but the links are not admin routes.
- Platform admins cannot inspect/edit platform library units from their own portal.

Recommendation:

- Add a platform-admin unit detail/editor route, e.g. `/admin/units/{id}` or `/admin/units/{id}/edit`.
- If editing is intentionally out of scope, keep the rows static or route to a read-only admin detail page.
- Avoid cross-portal links unless the target role is explicitly authorized and the shell/navigation stays coherent.

### [P1] Student portal shows no classes while direct live-session URLs still render

Evidence:

- Signed in as Alice Student, `/student` and `/student/classes` showed "No classes yet."
- Org admin `/org/classes` showed active classes with student counts, including a Python 101 class with three students.
- A direct `/student/sessions/312c0024-383f-4df4-ad18-4ac20a932206` URL rendered the live student workspace anyway.

Relevant code:

- `src/app/(portal)/student/classes/page.tsx:13-15` depends on `/api/classes/mine` and filters for `memberRole === "student"`.
- `src/app/(portal)/student/sessions/[sessionId]/page.tsx:23-41` authorizes through Go's `/student-page` endpoint and then POSTs `/join`.

Impact:

- The student cannot discover their classes through normal navigation.
- The same student can still open a live workspace by URL because Go resolves the request as a different user in this environment.
- This makes student QA and classroom security testing unreliable until the auth bypass is removed.

Recommendation:

- Fix the P0 auth/runtime split first.
- After that, retest `/api/classes/mine`, student dashboard, class detail, join-class, direct live-session URLs, and invite-token flow in the same clean browser context.
- Add an end-to-end assertion that a student who can open `/student/sessions/{id}` also sees the owning class in `/student/classes`.

### [P1] Platform admin pages are partially unusable because Go rejects admin API calls

Evidence:

- As `admin@e2e.test`, the shell rendered the platform admin sidebar and name.
- `/admin` showed "Couldn’t load platform stats (HTTP 403)" for `/api/admin/stats`.
- `/admin/orgs` and `/admin/users` showed "Platform admin access required."
- `/admin/units` loaded data, but links were unusable as described above.

Relevant code:

- `src/app/(portal)/admin/page.tsx:16-24` calls `/api/admin/stats`.
- The underlying trigger is the P0 `DEV_SKIP_AUTH` behavior in `platform/internal/auth/middleware.go:96-110`.

Impact:

- System admin cannot perform the core platform tasks: inspect organizations, review users, or trust platform metrics.

Recommendation:

- Treat this as a direct symptom of the auth-bypass setup blocker.
- Once fixed, rerun the admin route sweep and verify all admin nav items load with platform-admin claims from the Go layer.

### [P2] Teacher Problem Bank is a browse-only dead end

Evidence:

- `/teacher/problems` is a primary teacher sidebar destination.
- It displays a searchable/filterable table of problems.
- Rows are static text; there are no create, open, preview, edit, attach-to-focus-area, or import actions.

Relevant code:

- `src/app/(portal)/teacher/problems/page.tsx:89-170` renders filters and a plain table without row links or actions.
- The page copy still says "attach to topics"; current product taxonomy says Topics are focus/syllabus areas and Units are teaching material.

Impact:

- Teachers can see a bank but cannot do the natural next action from that screen.
- This reduces the usefulness of a first-class nav item and leaves the problem-to-unit/focus-area workflow unclear.

Recommendation:

- Decide the problem taxonomy explicitly:
  - `Course` contains ordered `Focus Areas`.
  - `Focus Area` is syllabus/session framing, not material.
  - `Unit` is the canonical teaching material.
  - `Problem` is practice/assessment content attachable to either a Unit section or a Focus Area agenda slot.
- Add at least a read-only problem detail/preview page.
- Add teacher actions based on intended workflow: attach to Unit, attach to Focus Area, clone to personal/org scope, or create problem.
- Update copy from "topics" to "focus areas" everywhere in teacher-facing UI.

### [P2] Org-admin list pages are useful but remain read-only with no management affordances

Evidence:

- `/org/teachers`, `/org/students`, `/org/courses`, `/org/classes`, `/org/units`, and `/org/settings` now render real data.
- None of the pages exposes common org-admin operations such as invite teacher, invite student, create class, inspect a class, or manage parent-child links.
- `/org/settings` explicitly says editing is coming later.

Impact:

- This is no longer a dead-end navigation bug, but it is still operationally shallow for a school administrator.
- The parent-linking feature now exists at the schema/API level, but there is no admin UI to create or repair links.

Recommendation:

- Next org-admin cycle should add:
  - Invite teacher/student/member by email.
  - Class detail read-only drill-down.
  - Parent-child link management.
  - Settings edit flow for contact/domain fields, with audit trail.

### [P2] Parent onboarding copy promises teacher-driven linking, but only admin/API machinery currently exists

Evidence:

- `/onboarding` tells a parent: "Your child's teacher will link your account."
- Plan 064 and the current code model use `parent_links`, but the discovered UI has no teacher or org-admin path to create those links.
- Parent dashboard currently hard-crashes if the migration is absent; after migration it will show "No children linked yet" unless links are manually created.

Impact:

- Parent users have no self-serve or guided next step after login.
- Teachers/org admins may not know where to fulfill the promise made to parents.

Recommendation:

- Pick the product owner for parent links:
  - Org admin-managed is safer for school data governance.
  - Teacher-requested, org-admin-approved is a good next step if teachers know the family context.
- Update onboarding copy to match the shipped workflow.
- Add an admin/org page for parent-child links before broadly enabling parent accounts.

## Positive Observations

- Teacher primary navigation no longer exposes Schedule/Reports placeholders.
- Org-admin primary navigation no longer points into teacher-only Units/Sessions routes.
- Teacher courses now use "Focus Areas" terminology, matching the new taxonomy direction.
- Ended sessions in teacher dashboard/session list are no longer linked into the live-session workspace.
- `/s/not-a-real-token` produces a clean invalid-invite page.
- Student My Work now has route logic for reopening live-session work or class work when documents exist.

## Recommended Next Cycle

1. Fix environment correctness first:
   - remove `DEV_SKIP_AUTH` from the tunneled review server,
   - apply `drizzle/0024_parent_links.sql`,
   - configure `HOCUSPOCUS_TOKEN_SECRET` for Go and Hocuspocus.
2. Rerun a short acceptance pass for the same five roles.
3. Build platform-admin unit detail/edit or remove admin-unit edit links.
4. Define and implement the problem/unit/focus-area workflow.
5. Build parent-link management in the org/admin portal.
6. Add E2E assertions for identity parity:
   - `/api/auth/session.user.id`,
   - `/api/me/identity.userId`,
   - role-specific portal route access,
   - role-specific class/session visibility.


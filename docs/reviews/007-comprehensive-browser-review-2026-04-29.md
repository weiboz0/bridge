# 007 Comprehensive Browser Review - 2026-04-29

## Scope

Reviewed current `main` after plan 047 was merged, using the in-app browser against the SSH-tunneled `http://localhost:3003` site plus source tracing for the related handlers/components.

Browser coverage included:

- Login and protected portal rendering.
- Teacher dashboard, sessions, class detail, live session workspace, courses, units, and focus-area editor.
- Student dashboard, classes, My Work, Help, and direct live-session route.
- Go-backed API spot checks for `/api/auth/session`, `/api/me/identity`, `/api/me/memberships`, `/api/classes/mine`, `/api/sessions/{id}/teacher-page`, `/api/sessions/{id}/topics`, and parent reports.

## Fixed Since Review 006

- `src/middleware.ts` now uses a literal matcher, so `/login` and portal pages render again.
- Go email/password registration now accepts and persists `intendedRole`.
- Teacher onboarding no longer redirects to missing `/register-org`.
- Parent report endpoints are disabled with structured `501 not_implemented` responses until parent-child linking exists.
- Create-class malformed course IDs map to a clean 404.
- Student problem malformed IDs are guarded before API calls.
- Teacher unit org picker and unit creation scope loading behave much better after async data settles.
- The class-bound start-session flow has an unlinked-focus-area guard.

## Findings

### [P0] Review/runtime environment still makes Go APIs ignore the logged-in browser user

**File:** `platform/internal/auth/middleware.go:88-103`

After signing in as Alice Student, `/api/auth/session` correctly returned Alice (`242fea26-1527-4a10-b208-af4cad1e1102`), but Go-backed endpoints still resolved as `Dev User` / `da5cef74-66e5-4946-bf56-409b23f34503`. `/api/me/memberships` then returned the teacher/org-admin memberships for `da5...`, and `/api/classes/mine` returned instructor classes instead of Alice's student classes.

This is no longer the old stale-cookie precedence bug: source now reads canonical cookies, but the browser-visible Go identity matches the `DEV_SKIP_AUTH` bypass path. In this tunneled environment, any Go-backed browser test can render a Next shell for one user while fetching backend data as the dev bypass user.

**Recommendation:** Disable `DEV_SKIP_AUTH` in the shared/tunneled review server, or expose a separate clearly named dev-bypass environment. Add a visible response header or `/api/me/identity` warning when bypass auth is active, because this state invalidates role and privacy testing.

### [P1] Teacher and student session material resolve from different topic sources

**Files:** `platform/internal/handlers/sessions.go:731-768`, `platform/internal/handlers/sessions.go:1205-1245`, `src/components/session/teacher/teacher-dashboard.tsx:280-318`, `src/components/session/student/student-session.tsx:65-122`

The teacher live page gets `courseTopics` from all topics on the class course. In browser, `/api/sessions/312c0024-383f-4df4-ad18-4ac20a932206/teacher-page` returned `Review Topic acb3a714` with `unitId: null`. But `/api/sessions/312c0024-383f-4df4-ad18-4ac20a932206/topics` returned `[]`, and the student live-session page rendered only the editor controls with no material panel.

That means teacher presentation and student material delivery are not looking at the same session agenda. Plan 047's create guard checks course topics, while the student panel reads explicit `session_topics`; if new sessions do not auto-populate `session_topics` from the course/focus-area selection, teachers can think material is present while students see none.

**Recommendation:** Pick one source of truth for live-session agenda. Prefer explicit session focus areas, but then create/start must snapshot or link the selected focus areas into `session_topics`, and both teacher and student pages should render from that same list.

### [P2] Focus-area editor still hangs forever for invalid or inaccessible routes

**File:** `src/app/(portal)/teacher/courses/[id]/topics/[topicId]/page.tsx:46-54`

Opening `/teacher/courses/does-not-exist/topics/also-bad` stayed on `Loading...` after the fetch failed. The page still only calls `setTopic` when `res.ok`; `400`, `403`, `404`, and network failures leave `topic` as `null` forever. This is now the main editor for linking focus areas to teaching units, so failure states need to be explicit.

**Recommendation:** Validate both UUID params before fetching, add unavailable/error state, and update copy from "Topic" to "Focus Area" or "Agenda Item" to match the intended taxonomy.

### [P2] Dashboard and class detail still route ended sessions into a placeholder page

**Files:** `src/app/(portal)/teacher/page.tsx:140-165`, `src/app/(portal)/teacher/classes/[id]/page.tsx:138-159`

The dedicated `/teacher/sessions` list correctly renders ended sessions as non-links, but the teacher dashboard and class detail still wrap ended sessions in links. Opening one lands on a "Session ended" placeholder that says a read-only review surface is coming later.

That is safer than the old live-control-room bug, but it is still inconsistent navigation. Teachers see "reopen one of your recent sessions" on the dashboard and "View ->" in class history, but the destination has no review value.

**Recommendation:** Either make ended sessions non-clickable everywhere until review exists, or ship a minimal read-only review page with attendance, focus areas, and links to student work.

### [P2] Theme bootstrap still triggers a route-wide Next dev overlay issue

**File:** `src/app/layout.tsx:42-47`

Every portal route tested emitted the browser console error:

> Encountered a script tag while rendering React component. Scripts inside React components are never executed when rendering on the client.

The issue badge was visibly present on live teacher/student session pages. The code comment says the theme bootstrap was moved to `next/script` to avoid this warning, but the warning still reproduces in-browser.

**Recommendation:** Move the bootstrap outside a React-rendered body script path, or use the App Router-supported inline bootstrap pattern that does not surface the dev overlay. This matters because the overlay masks real runtime issues during browser QA.

### [P2] My Work still cannot reopen saved work

**File:** `src/app/(portal)/student/code/page.tsx:24-42`

The My Work page still renders document cards/snippets without a link back to the class, problem, attempt, or session context. In this browser run Alice had no saved work because Go auth was bypassed, so this is source-confirmed rather than data-confirmed, but the UX issue remains: a primary student nav item should let students resume work.

**Recommendation:** Extend `/api/documents` or replace the page with problem attempts so each item has an actionable "Open" target.

## Review Notes

- Current tunneled browser role testing is not trustworthy until the Go backend stops using the dev bypass identity.
- The Unit taxonomy direction is still right: Topics should be renamed in product copy to Focus Areas or Agenda Items, and Units should remain the canonical teaching material.
- I did not run automated tests during this review pass; this report is based on in-app browser observations and source review.


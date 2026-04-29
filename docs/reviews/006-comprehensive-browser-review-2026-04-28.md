# 006 Comprehensive Browser Review - 2026-04-28

## Scope

Reviewed the current `main` after the plan 043-046 follow-up work, using the in-app browser against `http://localhost:3003` and source inspection for the routes/components that could not render.

Browser page rendering was blocked by a Next.js middleware compilation error, so this pass could not complete a normal role-by-role UX walkthrough. I still verified the reachable Go-backed API surfaces through the in-app browser:

- `/api/me/identity` returned the Go dev-bypass identity `dev@localhost` / `da5cef74-66e5-4946-bf56-409b23f34503`.
- `/api/classes/mine` returned instructor classes including `Codex Review Class acb3a714`.
- `/api/sessions?status=live&limit=2` returned live sessions.
- `/api/sessions/312c0024-383f-4df4-ad18-4ac20a932206/teacher-page` returned a valid live teacher-page payload with one topic whose linked unit fields were all `null`.

## Taxonomy Recommendation

Use this vocabulary consistently in product copy, data contracts, and routes:

- **Course**: reusable curriculum container.
- **Focus Area** (current `topic` table/API): syllabus-level subdivision of a course or session agenda. It should organize what a session is about, not store teaching material.
- **Teaching Unit**: canonical teaching material. Units own instructional content, projected student/teacher views, versioning, overlays, and lifecycle state.
- **Session**: live or historical classroom event. A session should select one or more focus areas and resolve those focus areas to their linked teaching units at runtime or via a session snapshot.
- **Problem/Assignment**: student work items. They may be linked from a unit or session, but should not replace unit content.

The code is partway through this cutover: student and teacher session panels now render linked Units rather than deprecated Topic lesson content. The remaining product risk is that focus areas can still exist without linked units, so a session can look configured while delivering no material.

## Findings

### [P0] Next portal pages do not render because middleware matcher is not statically analyzable

**Files:** `src/middleware.ts:16-20`, `src/lib/portal/middleware-matcher.ts`

The browser could not render `/login` or any portal page. Console logs repeatedly showed:

> Next.js can't recognize the exported `config` field in route. `matcher` needs to be a static string or array of static strings or array of static objects.

`src/middleware.ts` imports `middlewareMatcher` and then exports `config = { matcher: middlewareMatcher }`. Next/Turbopack needs the matcher value to be statically parseable in the middleware file itself. Until this is fixed, browser QA, login, role navigation, and all App Router UX are blocked even though proxied API routes may still respond.

**Recommendation:** Inline the literal matcher array/object in `src/middleware.ts`. Keep a duplicated test fixture or exported helper for unit tests if needed, but do not import the runtime matcher into `config`.

### [P0] Parent report APIs expose child reports without parent-child authorization

**File:** `platform/internal/handlers/parent.go:33-39`

`ListReports` only checks that the caller is authenticated, then returns reports for the `childId` path parameter. The code comment says any authenticated user can view any student's reports until parent-child linking is implemented. `CreateReport` has the same issue and can create placeholder reports for arbitrary `childId` values with `GeneratedBy` set to the current caller.

This is high-impact student/parent data and should not be shipped behind route obscurity.

**Recommendation:** Require a `parent_links` relationship, class/org staff authorization, or platform-admin authorization before list/create. Until that relationship exists, remove or disable the endpoints and keep parent report UI hidden.

### [P1] Email/password registration ignores the selected role and teacher onboarding points to a missing route

**Files:** `platform/internal/handlers/auth.go:23-59`, `platform/internal/store/users.go:91-120`, `src/app/onboarding/page.tsx:35-36`

`next.config.ts` proxies `/api/auth/register` to Go. The Go registration handler accepts only `name`, `email`, and `password`, and `RegisterUser` inserts no `intended_role`. The Next/Auth OAuth path still handles signup intent, but the active email/password registration path discards the selected teacher/student role.

The onboarding page then redirects `intendedRole === "teacher"` to `/register-org`, but no `src/app/register-org` route exists. The result is a broken first-run path for new teachers and a misleading role selector for email/password signups.

**Recommendation:** Make Go registration persist `intendedRole` or remove the selector from email/password registration until role onboarding is implemented. Add the org registration route or redirect teachers to an existing org onboarding surface.

### [P1] Sessions can still be configured with focus areas that have no teaching unit

**Files:** `platform/internal/handlers/sessions.go:1205-1245`, `src/components/session/teacher/teacher-dashboard.tsx:280-318`, `src/components/session/student/student-session.tsx:65-120`

The new taxonomy is correct directionally: session panels now render linked Units, not Topic lesson content. But the browser-confirmed live teacher-page payload for session `312c0024-383f-4df4-ad18-4ac20a932206` returned:

```json
{
  "title": "Review Topic acb3a714",
  "unitId": null,
  "unitTitle": null,
  "unitMaterialType": null
}
```

The teacher dashboard renders this as "No teaching unit linked"; the student panel renders "No material yet for this topic." That is a valid empty state for authoring, but not for a started live session. A teacher can enter class with a syllabus focus area selected and still have no material to teach from.

**Recommendation:** Add a workflow guard: before starting a session, warn or block when selected focus areas have no linked unit. Also add a backfill/migration task for seeded/demo courses so existing topics are linked to units. Product copy should call these rows "Focus Areas" or "Agenda Items" if their job is syllabus organization.

### [P2] Topic/focus-area editor can hang forever on load failure

**File:** `src/app/(portal)/teacher/courses/[id]/topics/[topicId]/page.tsx:46-54`

The topic editor calls `/api/courses/{id}/topics/{topicId}` and only updates state when `res.ok`. Any `400`, `403`, `404`, or network failure leaves `topic` as `null`, so the page stays on `Loading...` indefinitely. This matters more after the taxonomy shift because the topic editor is now the place teachers link focus areas to teaching units.

**Recommendation:** Add UUID validation, explicit unavailable/error states, and a stable link back to the course page. Treat the page as "Edit Focus Area" rather than "Edit Topic" to clarify the new model.

### [P2] My Work still cannot reopen saved work

**File:** `src/app/(portal)/student/code/page.tsx:24-42`

The student "My Work" route renders saved documents as static cards with language, timestamp, and a text snippet. There is no link or action to reopen the underlying class/problem/session context. As a primary student nav item, it should support resuming work, not just viewing a snippet.

**Recommendation:** Include enough metadata in `/api/documents` to link each document back to its problem/session/class context, then render "Open" or make each card navigable.

## Positive Notes Since The Previous Pass

- Class detail access is now guarded by backend authorization rather than returning any class by UUID.
- Direct live-session joins now require class membership, invitation/participant state, or authorized staff/admin context.
- The student join-class dialog now verifies enrollment against `/api/classes/mine` before closing.
- Teacher/org/student navigation is cleaner; previous org-admin links into inaccessible teacher routes are gone.
- Teacher unit org pickers now dedupe organization membership rows.
- Student and teacher session material panels no longer render deprecated Topic `lessonContent`.
- Ended sessions are no longer linked from the teacher sessions list into the live control room.

## Review Limitations

- Visual portal usability, route navigation, responsive behavior, login, and role switching could not be fully tested because of the middleware compile error.
- The in-app browser could reach Go APIs, but those APIs resolved as the `DEV_SKIP_AUTH` dev identity. That is useful for API spot checks, but not trustworthy for role-specific behavior validation.
- No automated test suite was run for this review-only doc update.


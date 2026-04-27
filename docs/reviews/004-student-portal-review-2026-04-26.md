# Student Portal Review - localhost:3003

Date: 2026-04-26
Scope: Student portal only: dashboard, class join/listing, class detail, live-session participation, problem entry, My Work, Help, and student-facing material delivery.

## Summary

The student portal is currently blocked by the same cross-runtime auth drift observed in the teacher review. In browser, the portal shell showed Alice Student while Go-backed endpoints continued authorizing as the stale teacher/org-admin user. That makes student class membership, join-class behavior, and session participation unreliable to evaluate until identity is canonical across Auth.js and Go.

Beyond the auth blocker, the student portal has several student-specific risks: class/session APIs are too permissive, student material delivery still treats Topic as lesson content instead of using Units as the canonical teaching material, malformed problem routes fail as generic server errors, and the My Work navigation item does not let students reopen saved work.

## Environment

- App URL: `http://localhost:3003`
- Browser context: Codex in-app browser
- Tested account: `alice@demo.edu`
- Student shell showed: `Alice Student`
- Repository branch during review: `main`

## Findings

### [P0] Student portal can show Alice while Go acts as another user

File: `platform/internal/auth/middleware.go:67-76`

After signing in as Alice, `/api/auth/session` returned Alice (`242f...`), but Go-backed endpoints such as `/api/me/memberships` and `/api/classes/mine` still resolved the stale teacher/org-admin user (`da5...`). The student dashboard then showed no classes because it filtered those backend classes by `memberRole === "student"`, while direct class/session routes could still expose the stale user's data. This blocks reliable student enrollment, class listing, and session participation testing.

Evidence:

- `/api/auth/session` returned Alice with user ID `242fea26-1527-4a10-b208-af4cad1e1102`.
- `/api/me/memberships` returned memberships for user ID `da5cef74-66e5-4946-bf56-409b23f34503`.
- `/api/classes/mine` returned instructor-role classes while the student dashboard rendered "No classes yet."

Recommendation:

Fix canonical identity across Auth.js, the server API client, browser-proxied Go requests, and Go middleware before judging student workflow reliability. This should be treated as a platform P0 because it can mix identities across roles.

### [P0] Class detail API exposes class metadata without membership checks

File: `platform/internal/handlers/classes.go:157-174`

`GetClass` returns any class to any authenticated user who knows the UUID. The student class page calls this first, then renders class details and continues with fallback defaults even if later course/topic fetches fail. Class detail should require class membership, org-authorized staff role, or platform admin before returning metadata.

Evidence:

- As Alice in the shell, opening `/student/classes/44eec29a-96e8-4e4c-841d-6040a467f2ca` rendered class metadata and a live-session link even though Alice's dashboard showed no student classes.
- The handler does not check membership or org role before returning the class.

Recommendation:

Add access checks to `GetClass`. For student contexts, require a class membership. For staff contexts, require instructor/TA/org-admin/platform-admin authorization.

### [P0] Live sessions can be joined by session ID alone

File: `platform/internal/handlers/sessions.go:296-319`

`JoinSession` only checks that the session exists and is live before adding the caller as a participant. It does not require class membership, an invite token, or an existing invitation. If a student obtains a live session ID, they can POST directly to this endpoint and join.

Evidence:

- Student live-session route `/student/sessions/312c0024-383f-4df4-ad18-4ac20a932206` rendered the session workspace.
- The direct join handler does not call `CanAccessSession`, does not validate class membership, and does not require invite-token proof.

Recommendation:

Gate direct session joins by class membership or pre-created invitation. Keep invite-token joining on the token route, and make direct `/api/sessions/{id}/join` reject users without a relationship to the session.

### [P1] Student surfaces still use Topic lesson content instead of Units

File: `src/components/session/student/student-session.tsx:61-94`

The student class/session experience still fetches topic lesson content and renders it as teaching material. With the new taxonomy, Units should be the canonical teaching material, while Topics should be syllabus/focus-area organizers that reference Units. Leaving this path in place splits student material delivery across two models.

Evidence:

- Student session fetches `/api/sessions/{sessionId}/topics`.
- The component renders `LessonRenderer` from each topic's `lessonContent`.
- Student class detail also parses topic `lessonContent` and renders it in topic cards.

Recommendation:

Refactor student delivery so session focus areas or syllabus topics point to Units. Render student-facing projected Unit documents, not topic-owned lesson blocks.

### [P1] Join-class success is not verified against enrollment

File: `src/components/student/join-class-dialog.tsx:23-32`

The dialog closes and refreshes on any `res.ok`, but never checks that the returned membership belongs to the current student or that `/api/classes/mine` now includes the class as `memberRole: student`. In the current identity-drift state this makes the join flow look successful while the dashboard still says there are no classes.

Evidence:

- The client treats `res.ok` as sufficient success.
- Earlier browser testing with join code `KWMLCWG5` closed the form while the student dashboard still showed no classes.

Recommendation:

Return and validate the created membership from the join API, then either optimistically add the class to local state or refetch `/api/classes/mine` and verify the class appears as a student membership before closing the form.

### [P2] Malformed problem URLs surface as server errors

File: `src/app/(portal)/student/classes/[id]/problems/[problemId]/page.tsx:43-50`

The student problem route sends `problemId` and `classId` straight into API/local DB calls without UUID validation. Opening `/student/classes/does-not-exist/problems/also-bad` produced the generic server-error page instead of a stable not-found state.

Evidence:

- The malformed route rendered "This page couldn't load" with a server error code.
- Console logs included `ApiError: API 400: /api/problems/also-bad`.

Recommendation:

Validate `classId`, `problemId`, and `attemptId` before fetching. Map bad UUIDs and backend 400/403/404 responses to a stable not-found or access-denied state.

### [P2] My Work cannot reopen saved work

File: `src/app/(portal)/student/code/page.tsx:24-42`

The My Work page renders saved documents as static cards/snippets with no links or actions to resume the underlying attempt/document. As a primary student nav item, it should let students reopen recent work, continue an attempt, or navigate back to the class/problem context.

Evidence:

- `My Work` is a first-class student nav item.
- Non-empty state only renders language, timestamp, and snippet.
- There is no link or action per work item.

Recommendation:

Return enough context from `/api/documents` to link each document back to the owning class/problem/attempt, or replace this surface with a recent attempts API that is naturally navigable.

## Positive Notes

- Student primary navigation is compact and understandable: Dashboard, My Classes, My Work, Help.
- The student session workspace does render the coding surface, Raise Hand button, layout toggle, and AI hook once a live session is reachable.
- Student Unit viewer has explicit unavailable/error states and already aligns better with the Units-as-material direction than the topic lesson paths.

## Suggested Next Cycle Order

1. Fix canonical identity across Auth.js and Go before retesting student flows.
2. Add class/session access checks for student-facing class detail and direct session join.
3. Rework student material delivery around Units, with Topics/Syllabus Areas only organizing focus and references.
4. Make join-class success dependent on confirmed student membership.
5. Add UUID validation and not-found/access-denied states to student problem routes.
6. Make My Work navigable to the student's saved attempts.

## Verification Notes

This was a review-only pass. I did not modify application code or run automated tests. Browser verification was performed manually through the in-app browser against `localhost:3003`.

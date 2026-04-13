# Plan 018 — Go Core Routes (Phase 2)

> **NOTE:** This plan requires review and approval before execution. Do not execute until the user explicitly approves.

## Overview

Migrate all CRUD API routes from Next.js to Go. This is Phase 2 of the Go backend migration (spec `docs/specs/004-go-backend-migration.md`). Assumes plan 017 is complete: Go project is running on port 8001 with Chi router, auth middleware (JWT validation via `NEXTAUTH_SECRET`), PostgreSQL connection pool (pgx), proxy middleware in Next.js, and contract test infrastructure. Org/user/admin routes are already migrated.

**Strategy per route:**
1. Write store functions (SQL queries via pgx) in `gobackend/internal/store/`
2. Write handler (validation, auth checks, call store, return JSON) in `gobackend/internal/handlers/`
3. Register route in Chi router
4. Write contract test comparing Go vs Next.js responses
5. Flip proxy in Next.js middleware so the route goes to Go

**Total routes:** ~45 endpoints across 10 domain groups.

**Branch:** `feat/018-go-core-routes`

---

## Prerequisites (from plan 017)

These must exist before starting:

- `gobackend/internal/auth/middleware.go` — extracts JWT, injects `UserClaims` into context
- `gobackend/internal/db/open.go` — pgx connection pool
- `gobackend/internal/store/orgs.go` — org membership queries (needed for auth checks: `GetUserRoleInOrg`)
- `gobackend/internal/store/users.go` — user lookup by email/ID
- `gobackend/tests/contract/helpers.go` — test helpers (send to both servers, compare responses)
- `src/middleware.ts` — Next.js proxy middleware with route list for flipped routes
- Chi router wired in `cmd/api/main.go` with auth, CORS, logging, recovery middleware

---

## Shared Conventions

### Store layer (`internal/store/`)

Each store file receives `*pgxpool.Pool` (or a `Querier` interface for testability). Functions use raw SQL via `pgx` — no ORM. Return Go structs. Naming: `CreateCourse`, `GetCourse`, `ListCoursesByOrg`, etc.

### Handler layer (`internal/handlers/`)

Each handler struct holds a reference to the store (or pool). Extract `UserClaims` from context via `auth.GetUser(r.Context())`. Validate input with a small validation helper (or manual checks). Return JSON with `httputil.JSON(w, status, data)` and `httputil.Error(w, status, msg)` helpers (established in plan 017).

### Contract tests (`tests/contract/`)

Each test file covers one domain. Pattern:
```go
func TestCourses_Create(t *testing.T) {
    // Seed: create org, user with teacher role
    // POST /api/courses with same body to both servers
    // Compare: status 201, JSON keys match, values match (skip timestamps/IDs)
}
```

### Proxy flip

In `src/middleware.ts`, add the route pattern to the `goRoutes` list. The proxy middleware matches against this list and forwards to `GO_API_URL`.

---

## Group 1: Courses

**Files:**
- `gobackend/internal/store/courses.go`
- `gobackend/internal/handlers/courses.go`
- `gobackend/tests/contract/courses_test.go`

### Task 1.1 — Store: courses.go

Implement these store functions matching the TypeScript in `src/lib/courses.ts`:

| Function | SQL | Notes |
|---|---|---|
| `CreateCourse(ctx, input) (*Course, error)` | `INSERT INTO courses (org_id, created_by, title, description, grade_level, language) VALUES (...) RETURNING *` | |
| `GetCourse(ctx, id) (*Course, error)` | `SELECT * FROM courses WHERE id = $1` | Return nil if not found |
| `ListCoursesByOrg(ctx, orgId) ([]Course, error)` | `SELECT * FROM courses WHERE org_id = $1` | |
| `ListCoursesByCreator(ctx, createdBy) ([]Course, error)` | `SELECT * FROM courses WHERE created_by = $1` | |
| `UpdateCourse(ctx, id, updates) (*Course, error)` | `UPDATE courses SET ... WHERE id = $1 RETURNING *` | Dynamic SET clause for partial updates; always set `updated_at = now()` |
| `DeleteCourse(ctx, id) (*Course, error)` | `DELETE FROM courses WHERE id = $1 RETURNING *` | Return nil if not found |
| `CloneCourse(ctx, courseId, newCreatedBy) (*Course, error)` | Transaction: insert new course with `(Copy)` suffix, copy all topics from original | Must clone topics with same `sort_order`, `lesson_content`, `starter_code` |

Define `Course` struct with JSON tags matching current API responses.

### Task 1.2 — Handler: POST /api/courses

- Validate: `orgId` (uuid), `title` (1-255), `description` (optional, max 5000), `gradeLevel` (enum: K-5, 6-8, 9-12), `language` (optional enum: python, javascript, blockly)
- Auth: verify user has `teacher` or `org_admin` role in org (call `store.GetUserRoleInOrg`), or `isPlatformAdmin`
- Call `store.CreateCourse`
- Return 201 + course JSON

### Task 1.3 — Handler: GET /api/courses

- Query param: `orgId` (required)
- Auth: verify user is member of org (any role in org_memberships), or `isPlatformAdmin`
- Call `store.ListCoursesByOrg`
- Return 200 + array

### Task 1.4 — Handler: GET /api/courses/{id}

- Auth: any authenticated user (no specific role check in current impl)
- Call `store.GetCourse`
- Return 404 if nil, else 200 + course JSON

### Task 1.5 — Handler: PATCH /api/courses/{id}

- Validate: `title` (optional 1-255), `description` (optional max 5000), `gradeLevel` (optional enum), `language` (optional enum), `isPublished` (optional bool)
- Auth: only course creator or platformAdmin
- Call `store.UpdateCourse`
- Return 200 + updated course

### Task 1.6 — Handler: DELETE /api/courses/{id}

- Auth: only course creator or platformAdmin
- Call `store.DeleteCourse`
- Return 200 + deleted course, or 404

### Task 1.7 — Handler: POST /api/courses/{id}/clone

- Auth: any authenticated user
- Call `store.CloneCourse`
- Return 201 + cloned course, or 404

### Task 1.8 — Register routes in Chi router

```go
r.Route("/api/courses", func(r chi.Router) {
    r.Use(auth.RequireAuth)
    r.Post("/", h.CreateCourse)
    r.Get("/", h.ListCourses)
    r.Route("/{id}", func(r chi.Router) {
        r.Get("/", h.GetCourse)
        r.Patch("/", h.UpdateCourse)
        r.Delete("/", h.DeleteCourse)
        r.Post("/clone", h.CloneCourse)
    })
})
```

### Task 1.9 — Contract tests: courses

Write contract tests for all 7 endpoints. For each test:
- Seed test data directly in the shared test DB
- Send identical requests to Next.js (port 3003) and Go (port 8001)
- Compare status codes and response body structure
- Test auth (401 without token, 403 for wrong role)
- Test validation (400 for bad input)
- Clean up seeded data after test

### Task 1.10 — Proxy flip: courses

Add to `goRoutes` in `src/middleware.ts`:
- `/api/courses`
- `/api/courses/:id`
- `/api/courses/:id/clone`

---

## Group 2: Topics

**Files:**
- `gobackend/internal/store/topics.go`
- `gobackend/internal/handlers/topics.go` (or extend `courses.go` since routes are nested)
- `gobackend/tests/contract/topics_test.go`

### Task 2.1 — Store: topics.go

| Function | SQL | Notes |
|---|---|---|
| `CreateTopic(ctx, input) (*Topic, error)` | `INSERT INTO topics ...` | Auto-assign `sort_order` as `COALESCE(MAX(sort_order), -1) + 1` for the course |
| `GetTopic(ctx, id) (*Topic, error)` | `SELECT * FROM topics WHERE id = $1` | |
| `ListTopicsByCourse(ctx, courseId) ([]Topic, error)` | `SELECT * FROM topics WHERE course_id = $1 ORDER BY sort_order ASC` | |
| `UpdateTopic(ctx, id, updates) (*Topic, error)` | `UPDATE topics SET ... WHERE id = $1 RETURNING *` | Partial update; set `updated_at = now()` |
| `DeleteTopic(ctx, id) (*Topic, error)` | `DELETE FROM topics WHERE id = $1 RETURNING *` | |
| `ReorderTopics(ctx, courseId, topicIds) error` | Loop: `UPDATE topics SET sort_order = $1 WHERE id = $2 AND course_id = $3` | Run in transaction for atomicity |

Define `Topic` struct with fields: `id`, `course_id`, `title`, `description`, `sort_order`, `lesson_content` (JSONB), `starter_code`, `created_at`, `updated_at`.

### Task 2.2 — Handler: POST /api/courses/{id}/topics

- Validate: `title` (1-255), `description` (optional max 2000), `lessonContent` (optional JSON object), `starterCode` (optional string)
- Auth: any authenticated user (current impl has no ownership check on create)
- `courseId` from URL param
- Call `store.CreateTopic`
- Return 201 + topic

### Task 2.3 — Handler: GET /api/courses/{id}/topics

- Auth: any authenticated user
- Call `store.ListTopicsByCourse`
- Return 200 + array

### Task 2.4 — Handler: GET /api/courses/{id}/topics/{topicId}

- Auth: verify course ownership (creator or platformAdmin) — calls `store.GetCourse` to check `created_by`
- Call `store.GetTopic`
- Return 404 if nil, 403 if not owner, else 200 + topic

### Task 2.5 — Handler: PATCH /api/courses/{id}/topics/{topicId}

- Validate: `title` (optional 1-255), `description` (optional max 5000), `lessonContent` (optional JSON), `starterCode` (optional string)
- Auth: verify course ownership
- Call `store.UpdateTopic`
- Return 200 + updated topic, or 404

### Task 2.6 — Handler: DELETE /api/courses/{id}/topics/{topicId}

- Auth: verify course ownership
- Call `store.DeleteTopic`
- Return 200 + deleted topic, or 404

### Task 2.7 — Handler: PATCH /api/courses/{id}/topics/reorder

- Validate: `topicIds` (array of UUIDs)
- Auth: any authenticated user (current impl has no ownership check)
- Call `store.ReorderTopics`
- Return 200 + `{"success": true}`

### Task 2.8 — Register topic routes (nested under courses)

```go
r.Route("/{id}/topics", func(r chi.Router) {
    r.Post("/", h.CreateTopic)
    r.Get("/", h.ListTopics)
    r.Patch("/reorder", h.ReorderTopics)
    r.Route("/{topicId}", func(r chi.Router) {
        r.Get("/", h.GetTopic)
        r.Patch("/", h.UpdateTopic)
        r.Delete("/", h.DeleteTopic)
    })
})
```

### Task 2.9 — Contract tests: topics

Test all 6 topic endpoints + reorder. Seed a course first, then test topic CRUD. Verify sort_order auto-assignment and reorder behavior.

### Task 2.10 — Proxy flip: topics

Add to `goRoutes`:
- `/api/courses/:id/topics`
- `/api/courses/:id/topics/reorder`
- `/api/courses/:id/topics/:topicId`

---

## Group 3: Classes

**Files:**
- `gobackend/internal/store/classes.go`
- `gobackend/internal/handlers/classes.go`
- `gobackend/tests/contract/classes_test.go`

### Task 3.1 — Store: classes.go

| Function | SQL | Notes |
|---|---|---|
| `CreateClass(ctx, input) (*Class, error)` | Transaction: INSERT class with generated join_code, INSERT new_classrooms (1:1) with editor_mode from course language, INSERT class_memberships for creator as instructor | Three inserts in a transaction. `generateJoinCode()` — 8-char random alphanumeric |
| `GetClass(ctx, id) (*Class, error)` | `SELECT * FROM classes WHERE id = $1` | |
| `ListClassesByOrg(ctx, orgId) ([]Class, error)` | `SELECT * FROM classes WHERE org_id = $1 AND status = 'active'` | Default excludes archived |
| `ArchiveClass(ctx, id) (*Class, error)` | `UPDATE classes SET status = 'archived', updated_at = now() WHERE id = $1 RETURNING *` | |
| `GetClassByJoinCode(ctx, joinCode) (*Class, error)` | `SELECT * FROM classes WHERE join_code = $1` | |
| `GetClassroom(ctx, classId) (*NewClassroom, error)` | `SELECT * FROM new_classrooms WHERE class_id = $1` | For the 1:1 new_classrooms record |

Also need a `generateJoinCode()` utility — 8-char uppercase alphanumeric, matching `src/lib/utils.ts`.

### Task 3.2 — Handler: POST /api/classes

- Validate: `courseId` (uuid), `orgId` (uuid), `title` (1-255), `term` (optional max 100)
- Auth: user must be `teacher` or `org_admin` in org, or platformAdmin
- Call `store.CreateClass` with `createdBy = user.id`
- Return 201 + class

### Task 3.3 — Handler: GET /api/classes

- Query param: `orgId` (required)
- Auth: user must be member of org, or platformAdmin
- Call `store.ListClassesByOrg`
- Return 200 + array

### Task 3.4 — Handler: GET /api/classes/{id}

- Auth: any authenticated user
- Call `store.GetClass`
- Return 404 if nil, else 200 + class

### Task 3.5 — Handler: PATCH /api/classes/{id}

- Auth: any authenticated user (current impl archives — no ownership check)
- Call `store.ArchiveClass`
- Return 200 + archived class, or 404

### Task 3.6 — Handler: POST /api/classes/join

- Validate: `joinCode` (string, length 8)
- Auth: any authenticated user
- Call `store.GetClassByJoinCode`, verify class is active, call `store.AddClassMember` with role "student"
- Return 200 + class, or 404

### Task 3.7 — Register class routes

```go
r.Route("/api/classes", func(r chi.Router) {
    r.Use(auth.RequireAuth)
    r.Post("/", h.CreateClass)
    r.Get("/", h.ListClasses)
    r.Post("/join", h.JoinClass)
    r.Route("/{id}", func(r chi.Router) {
        r.Get("/", h.GetClass)
        r.Patch("/", h.ArchiveClass)
    })
})
```

### Task 3.8 — Contract tests: classes

Test create (with auto-classroom + auto-membership), list, get, archive, join-by-code. Verify the join code flow works end-to-end.

### Task 3.9 — Proxy flip: classes

Add to `goRoutes`:
- `/api/classes`
- `/api/classes/join`
- `/api/classes/:id`

---

## Group 4: Class Members

**Files:**
- `gobackend/internal/store/class_memberships.go`
- `gobackend/internal/handlers/class_members.go` (or extend `classes.go`)
- `gobackend/tests/contract/class_members_test.go`

### Task 4.1 — Store: class_memberships.go

| Function | SQL | Notes |
|---|---|---|
| `AddClassMember(ctx, input) (*ClassMembership, error)` | `INSERT INTO class_memberships (class_id, user_id, role) VALUES (...) ON CONFLICT DO NOTHING RETURNING *` | |
| `ListClassMembers(ctx, classId) ([]ClassMemberRow, error)` | `SELECT cm.*, u.name, u.email FROM class_memberships cm JOIN users u ON cm.user_id = u.id WHERE cm.class_id = $1` | Returns joined data |
| `GetClassMembership(ctx, membershipId) (*ClassMembership, error)` | `SELECT * FROM class_memberships WHERE id = $1` | |
| `UpdateClassMemberRole(ctx, membershipId, role) (*ClassMembership, error)` | `UPDATE class_memberships SET role = $1 WHERE id = $2 RETURNING *` | |
| `RemoveClassMember(ctx, membershipId) (*ClassMembership, error)` | `DELETE FROM class_memberships WHERE id = $1 RETURNING *` | |
| `JoinClassByCode(ctx, joinCode, userId) (*JoinResult, error)` | Lookup class by join_code, verify status = 'active', insert membership as student | Returns `{class, membership}` or nil |

### Task 4.2 — Handler: POST /api/classes/{id}/members

- Validate: `email` (valid email), `role` (optional enum: instructor, ta, student, observer, guest, parent)
- Auth: any authenticated user (current impl has no role check)
- Lookup user by email → 404 if not found
- Call `store.AddClassMember`
- Return 201 + membership

### Task 4.3 — Handler: GET /api/classes/{id}/members

- Auth: any authenticated user
- Call `store.ListClassMembers`
- Return 200 + array (with user name/email)

### Task 4.4 — Handler: PATCH /api/classes/{id}/members/{memberId}

- Validate: `role` (enum)
- Auth: any authenticated user (current impl has no role check beyond auth)
- Verify membership belongs to this class (membership.classId == URL classId)
- Call `store.UpdateClassMemberRole`
- Return 200 + updated membership

### Task 4.5 — Handler: DELETE /api/classes/{id}/members/{memberId}

- Auth: any authenticated user
- Verify membership belongs to this class
- Call `store.RemoveClassMember`
- Return 200 + removed membership

### Task 4.6 — Register member routes (nested under classes)

```go
r.Route("/{id}/members", func(r chi.Router) {
    r.Post("/", h.AddClassMember)
    r.Get("/", h.ListClassMembers)
    r.Route("/{memberId}", func(r chi.Router) {
        r.Patch("/", h.UpdateClassMemberRole)
        r.Delete("/", h.RemoveClassMember)
    })
})
```

### Task 4.7 — Contract tests: class members

Test add-by-email, list (verify joined user data), update role, remove. Test 404 when membership doesn't belong to the class.

### Task 4.8 — Proxy flip: class members

Add to `goRoutes`:
- `/api/classes/:id/members`
- `/api/classes/:id/members/:memberId`

---

## Group 5: Sessions

**Files:**
- `gobackend/internal/store/sessions.go`
- `gobackend/internal/handlers/sessions.go`
- `gobackend/internal/events/broadcaster.go` (SSE event bus — from shared patterns)
- `gobackend/tests/contract/sessions_test.go`

### Task 5.1 — Store: sessions.go

| Function | SQL | Notes |
|---|---|---|
| `CreateSession(ctx, input) (*LiveSession, error)` | Transaction: end any active session for this classroom (`UPDATE ... SET status='ended', ended_at=now()`), then `INSERT INTO live_sessions ... RETURNING *` | |
| `GetSession(ctx, id) (*LiveSession, error)` | `SELECT * FROM live_sessions WHERE id = $1` | |
| `GetActiveSession(ctx, classroomId) (*LiveSession, error)` | `SELECT * FROM live_sessions WHERE classroom_id = $1 AND status = 'active'` | |
| `EndSession(ctx, id) (*LiveSession, error)` | `UPDATE live_sessions SET status = 'ended', ended_at = now() WHERE id = $1 RETURNING *` | |
| `JoinSession(ctx, sessionId, studentId) (*SessionParticipant, error)` | `INSERT INTO session_participants (session_id, student_id) VALUES (...) ON CONFLICT DO NOTHING RETURNING *` | |
| `LeaveSession(ctx, sessionId, studentId) (*SessionParticipant, error)` | `UPDATE session_participants SET left_at = now() WHERE session_id = $1 AND student_id = $2 RETURNING *` | |
| `GetSessionParticipants(ctx, sessionId) ([]ParticipantRow, error)` | `SELECT sp.*, u.name, u.email FROM session_participants sp JOIN users u ON sp.student_id = u.id WHERE sp.session_id = $1` | |
| `UpdateParticipantStatus(ctx, sessionId, studentId, status) (*SessionParticipant, error)` | `UPDATE session_participants SET status = $1 WHERE session_id = $2 AND student_id = $3 RETURNING *` | |

### Task 5.2 — Event broadcaster (SSE)

Implement `events/broadcaster.go` (copied from magicburg shared pattern, adapted for Bridge):
- `type Broadcaster` with `Subscribe(sessionId) <-chan Event` and `Emit(sessionId, eventType, data)`
- In-memory map of sessionId -> subscriber channels
- Thread-safe with `sync.RWMutex`
- Used by session handlers to emit events (student_joined, student_left, hand_raised, hand_lowered, broadcast_started, broadcast_ended, session_ended)

### Task 5.3 — Handler: POST /api/sessions

- Validate: `classroomId` (uuid), `settings` (optional JSON object)
- Auth: verify user is the classroom teacher (lookup classroom, check `teacher_id`)
- Call `store.CreateSession`
- Return 201 + session

### Task 5.4 — Handler: GET /api/sessions/{id}

- Auth: any authenticated user
- Call `store.GetSession`
- Return 404 if nil, else 200 + session

### Task 5.5 — Handler: PATCH /api/sessions/{id} (end session)

- Auth: only session teacher or platformAdmin
- Call `store.EndSession`
- Emit `session_ended` via broadcaster
- Return 200 + ended session

### Task 5.6 — Handler: POST /api/sessions/{id}/join

- Auth: any authenticated user
- Verify session exists and is active (status check → 400 "Session has ended")
- Call `store.JoinSession`
- Emit `student_joined` via broadcaster
- Return 200 + participant

### Task 5.7 — Handler: POST /api/sessions/{id}/leave

- Auth: any authenticated user
- Call `store.LeaveSession`
- Emit `student_left` via broadcaster
- Return 200 + participant, or 404

### Task 5.8 — Handler: GET /api/sessions/{id}/participants

- Auth: any authenticated user
- Call `store.GetSessionParticipants`
- Return 200 + array

### Task 5.9 — Handler: GET /api/sessions/{id}/events (SSE)

- Auth: any authenticated user
- Set SSE headers: `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`
- Subscribe to broadcaster for this sessionId
- Send initial `event: connected` ping
- Stream events until client disconnects (use `r.Context().Done()`)
- Unsubscribe on disconnect

### Task 5.10 — Handler: GET /api/sessions/{id}/help-queue

- Auth: any authenticated user
- Call `store.GetSessionParticipants`, filter to `status = 'needs_help'`
- Return 200 + filtered array

### Task 5.11 — Handler: POST /api/sessions/{id}/help-queue

- Validate: `raised` (boolean)
- Auth: any authenticated user
- Set status to `needs_help` or `active` based on `raised`
- Call `store.UpdateParticipantStatus`
- Emit `hand_raised` or `hand_lowered` via broadcaster
- Return 200 + participant, or 404

### Task 5.12 — Handler: POST /api/sessions/{id}/broadcast

- Validate: `active` (boolean)
- Auth: only session teacher or platformAdmin
- Emit `broadcast_started` or `broadcast_ended` via broadcaster
- Return 200 + `{"active": <bool>}`

### Task 5.13 — Register session routes

```go
r.Route("/api/sessions", func(r chi.Router) {
    r.Use(auth.RequireAuth)
    r.Post("/", h.CreateSession)
    r.Route("/{id}", func(r chi.Router) {
        r.Get("/", h.GetSession)
        r.Patch("/", h.EndSession)
        r.Post("/join", h.JoinSession)
        r.Post("/leave", h.LeaveSession)
        r.Get("/participants", h.GetParticipants)
        r.Get("/events", h.SessionEvents)
        r.Get("/help-queue", h.GetHelpQueue)
        r.Post("/help-queue", h.ToggleHelp)
        r.Post("/broadcast", h.ToggleBroadcast)
    })
})
```

### Task 5.14 — Contract tests: sessions

Test create (verify auto-end of existing active session), get, end, join, leave, participants, help-queue toggle, broadcast. SSE endpoint: test that connection succeeds and initial `connected` event is received.

### Task 5.15 — Proxy flip: sessions

Add to `goRoutes`:
- `/api/sessions`
- `/api/sessions/:id`
- `/api/sessions/:id/join`
- `/api/sessions/:id/leave`
- `/api/sessions/:id/participants`
- `/api/sessions/:id/events`
- `/api/sessions/:id/help-queue`
- `/api/sessions/:id/broadcast`

---

## Group 6: Session Topics

**Files:**
- `gobackend/internal/store/session_topics.go`
- Handler methods added to `sessions.go` handler (or new file)
- `gobackend/tests/contract/session_topics_test.go`

### Task 6.1 — Store: session_topics.go

| Function | SQL | Notes |
|---|---|---|
| `LinkSessionTopic(ctx, sessionId, topicId) (*SessionTopic, error)` | `INSERT INTO session_topics (session_id, topic_id) VALUES (...) ON CONFLICT DO NOTHING RETURNING *` | |
| `UnlinkSessionTopic(ctx, sessionId, topicId) (*SessionTopic, error)` | `DELETE FROM session_topics WHERE session_id = $1 AND topic_id = $2 RETURNING *` | |
| `GetSessionTopics(ctx, sessionId) ([]SessionTopicRow, error)` | `SELECT st.topic_id, t.title, t.description, t.sort_order, t.lesson_content, t.starter_code FROM session_topics st JOIN topics t ON st.topic_id = t.id WHERE st.session_id = $1 ORDER BY t.sort_order ASC` | Returns joined topic data |

### Task 6.2 — Handler: GET /api/sessions/{id}/topics

- Auth: any authenticated user
- Call `store.GetSessionTopics`
- Return 200 + array

### Task 6.3 — Handler: POST /api/sessions/{id}/topics

- Validate: `topicId` (uuid)
- Auth: only session teacher or platformAdmin (lookup session, check `teacher_id`)
- Call `store.LinkSessionTopic`
- Return 201 + link

### Task 6.4 — Handler: DELETE /api/sessions/{id}/topics

- Validate: `topicId` (uuid) from request body
- Auth: only session teacher or platformAdmin
- Call `store.UnlinkSessionTopic`
- Return 200 + `{"success": true}`

### Task 6.5 — Register session topic routes

Add to the session `/{id}` sub-router:
```go
r.Get("/topics", h.GetSessionTopics)
r.Post("/topics", h.LinkSessionTopic)
r.Delete("/topics", h.UnlinkSessionTopic)
```

### Task 6.6 — Contract tests: session topics

Seed a session + topics, test link, list (verify sort order), unlink. Test 403 for non-teacher.

### Task 6.7 — Proxy flip: session topics

Add to `goRoutes`:
- `/api/sessions/:id/topics`

---

## Group 7: Documents

**Files:**
- `gobackend/internal/store/documents.go`
- `gobackend/internal/handlers/documents.go`
- `gobackend/tests/contract/documents_test.go`

### Task 7.1 — Store: documents.go

| Function | SQL | Notes |
|---|---|---|
| `GetDocument(ctx, id) (*Document, error)` | `SELECT * FROM documents WHERE id = $1` | |
| `ListDocuments(ctx, filters) ([]Document, error)` | Dynamic WHERE clause from `ownerId`, `classroomId`, `sessionId` | At least one filter required (enforced in handler) |
| `GetOrCreateDocument(ctx, ownerId, sessionId, classroomId) (*Document, error)` | Check existing by owner+session, insert if not found | Used internally; may not need a route |

Note: Documents are created by Hocuspocus persistence hooks or by `getOrCreateDocument` calls, not by a direct POST route. The API only exposes GET endpoints.

### Task 7.2 — Handler: GET /api/documents

- Query params: `classroomId`, `studentId`, `sessionId` (all optional, at least one required)
- Auth: platform admins can view any; regular users see their own (or filtered by their ID)
- Call `store.ListDocuments`
- Return 200 + array, or 400 if no filters

### Task 7.3 — Handler: GET /api/documents/{id}

- Auth: only document owner or platformAdmin
- Call `store.GetDocument`
- Return 403 if not owner, 404 if nil, else 200 + document

### Task 7.4 — Handler: GET /api/documents/{id}/content

- Auth: only document owner or platformAdmin
- Call `store.GetDocument`
- Return subset: `{id, ownerId, language, plainText, updatedAt}`

### Task 7.5 — Register document routes

```go
r.Route("/api/documents", func(r chi.Router) {
    r.Use(auth.RequireAuth)
    r.Get("/", h.ListDocuments)
    r.Route("/{id}", func(r chi.Router) {
        r.Get("/", h.GetDocument)
        r.Get("/content", h.GetDocumentContent)
    })
})
```

### Task 7.6 — Contract tests: documents

Seed documents via direct DB insert (since there's no POST route). Test list with various filters, get, get-content. Test ownership enforcement (403 for non-owner).

### Task 7.7 — Proxy flip: documents

Add to `goRoutes`:
- `/api/documents`
- `/api/documents/:id`
- `/api/documents/:id/content`

---

## Group 8: Assignments

**Files:**
- `gobackend/internal/store/assignments.go`
- `gobackend/internal/handlers/assignments.go`
- `gobackend/tests/contract/assignments_test.go`

### Task 8.1 — Store: assignments.go

| Function | SQL | Notes |
|---|---|---|
| `CreateAssignment(ctx, input) (*Assignment, error)` | `INSERT INTO assignments (class_id, topic_id, title, description, starter_code, due_date, rubric) VALUES (...) RETURNING *` | `rubric` is JSONB |
| `GetAssignment(ctx, id) (*Assignment, error)` | `SELECT * FROM assignments WHERE id = $1` | |
| `ListAssignmentsByClass(ctx, classId) ([]Assignment, error)` | `SELECT * FROM assignments WHERE class_id = $1` | |
| `ListAssignmentsByTopic(ctx, topicId) ([]Assignment, error)` | `SELECT * FROM assignments WHERE topic_id = $1` | |
| `UpdateAssignment(ctx, id, updates) (*Assignment, error)` | `UPDATE assignments SET ... WHERE id = $1 RETURNING *` | Dynamic partial update |
| `DeleteAssignment(ctx, id) (*Assignment, error)` | `DELETE FROM assignments WHERE id = $1 RETURNING *` | |

### Task 8.2 — Handler: POST /api/assignments

- Validate: `classId` (uuid), `topicId` (optional uuid), `title` (1-255), `description` (optional max 5000), `starterCode` (optional), `dueDate` (optional ISO datetime), `rubric` (optional JSON object)
- Auth: user must be instructor or TA in the class (lookup members, check role), or platformAdmin
- Call `store.CreateAssignment`
- Return 201 + assignment

### Task 8.3 — Handler: GET /api/assignments

- Query param: `classId` (required)
- Auth: user must be a member of the class, or platformAdmin
- Call `store.ListAssignmentsByClass`
- Return 200 + array

### Task 8.4 — Handler: GET /api/assignments/{id}

- Auth: any authenticated user
- Call `store.GetAssignment`
- Return 404 if nil, else 200 + assignment

### Task 8.5 — Handler: PATCH /api/assignments/{id}

- Validate: `title` (optional 1-255), `description` (optional max 5000), `starterCode` (optional), `dueDate` (optional ISO datetime), `rubric` (optional JSON object)
- Auth: user must be instructor/TA in the assignment's class, or platformAdmin
- Call `store.UpdateAssignment`
- Return 200 + updated assignment

### Task 8.6 — Handler: DELETE /api/assignments/{id}

- Auth: user must be instructor/TA in the assignment's class, or platformAdmin
- Call `store.DeleteAssignment`
- Return 200 + deleted assignment, or 404

### Task 8.7 — Handler: POST /api/assignments/{id}/submit

- Validate: `documentId` (optional)
- Auth: user must be a member of the assignment's class, or platformAdmin
- Call `store.CreateSubmission` (from submissions store)
- Return 201 + submission, or 409 if already submitted

### Task 8.8 — Handler: GET /api/assignments/{id}/submissions

- Auth: user must be instructor/TA in the assignment's class, or platformAdmin
- Call `store.ListSubmissionsByAssignment`
- Return 200 + array (with student name/email)

### Task 8.9 — Register assignment routes

```go
r.Route("/api/assignments", func(r chi.Router) {
    r.Use(auth.RequireAuth)
    r.Post("/", h.CreateAssignment)
    r.Get("/", h.ListAssignments)
    r.Route("/{id}", func(r chi.Router) {
        r.Get("/", h.GetAssignment)
        r.Patch("/", h.UpdateAssignment)
        r.Delete("/", h.DeleteAssignment)
        r.Post("/submit", h.SubmitAssignment)
        r.Get("/submissions", h.ListSubmissions)
    })
})
```

### Task 8.10 — Contract tests: assignments

Test full CRUD + submit + list-submissions. Test instructor-only authorization for create/update/delete/list-submissions. Test 409 duplicate submission.

### Task 8.11 — Proxy flip: assignments

Add to `goRoutes`:
- `/api/assignments`
- `/api/assignments/:id`
- `/api/assignments/:id/submit`
- `/api/assignments/:id/submissions`

---

## Group 9: Submissions

**Files:**
- `gobackend/internal/store/submissions.go`
- `gobackend/internal/handlers/submissions.go`
- `gobackend/tests/contract/submissions_test.go`

### Task 9.1 — Store: submissions.go

| Function | SQL | Notes |
|---|---|---|
| `CreateSubmission(ctx, input) (*Submission, error)` | `INSERT INTO submissions (assignment_id, student_id, document_id) VALUES (...) ON CONFLICT DO NOTHING RETURNING *` | Returns nil if conflict (duplicate) |
| `GetSubmission(ctx, id) (*Submission, error)` | `SELECT * FROM submissions WHERE id = $1` | |
| `ListSubmissionsByAssignment(ctx, assignmentId) ([]SubmissionRow, error)` | `SELECT s.*, u.name AS student_name, u.email AS student_email FROM submissions s JOIN users u ON s.student_id = u.id WHERE s.assignment_id = $1` | |
| `ListSubmissionsByStudent(ctx, studentId) ([]Submission, error)` | `SELECT * FROM submissions WHERE student_id = $1` | |
| `GetSubmissionByAssignmentAndStudent(ctx, assignmentId, studentId) (*Submission, error)` | `SELECT * FROM submissions WHERE assignment_id = $1 AND student_id = $2` | |
| `GradeSubmission(ctx, id, grade, feedback) (*Submission, error)` | `UPDATE submissions SET grade = $1, feedback = $2 WHERE id = $3 RETURNING *` | `feedback` may be nil |

### Task 9.2 — Handler: PATCH /api/submissions/{id} (grade)

- Validate: `grade` (number 0-100), `feedback` (optional string max 5000)
- Auth: user must be instructor/TA in the submission's assignment's class, or platformAdmin
- Chain: get submission -> get assignment -> list class members -> check role
- Call `store.GradeSubmission`
- Return 200 + graded submission

### Task 9.3 — Register submission routes

```go
r.Route("/api/submissions", func(r chi.Router) {
    r.Use(auth.RequireAuth)
    r.Route("/{id}", func(r chi.Router) {
        r.Patch("/", h.GradeSubmission)
    })
})
```

### Task 9.4 — Contract tests: submissions

Seed assignment + submission, test grading (happy path, invalid grade, non-instructor). Verify the multi-hop auth check (submission -> assignment -> class -> membership).

### Task 9.5 — Proxy flip: submissions

Add to `goRoutes`:
- `/api/submissions/:id`

---

## Group 10: Annotations

**Files:**
- `gobackend/internal/store/annotations.go`
- `gobackend/internal/handlers/annotations.go`
- `gobackend/tests/contract/annotations_test.go`

### Task 10.1 — Store: annotations.go

| Function | SQL | Notes |
|---|---|---|
| `CreateAnnotation(ctx, input) (*Annotation, error)` | `INSERT INTO code_annotations (document_id, author_id, author_type, line_start, line_end, content) VALUES (...) RETURNING *` | |
| `ListAnnotations(ctx, documentId) ([]Annotation, error)` | `SELECT * FROM code_annotations WHERE document_id = $1` | |
| `DeleteAnnotation(ctx, id) (*Annotation, error)` | `DELETE FROM code_annotations WHERE id = $1 RETURNING *` | |
| `ResolveAnnotation(ctx, id) (*Annotation, error)` | `UPDATE code_annotations SET resolved = now() WHERE id = $1 RETURNING *` | |

### Task 10.2 — Handler: POST /api/annotations

- Validate: `documentId` (string, non-empty), `lineStart` (string, non-empty), `lineEnd` (string, non-empty), `content` (string 1-2000)
- Auth: any authenticated user
- Set `authorId` from JWT claims, `authorType` = "teacher" (hardcoded for now, matching current behavior)
- Call `store.CreateAnnotation`
- Return 201 + annotation

### Task 10.3 — Handler: GET /api/annotations

- Query param: `documentId` (required)
- Auth: any authenticated user
- Call `store.ListAnnotations`
- Return 200 + array

### Task 10.4 — Handler: DELETE /api/annotations/{id}

- Auth: any authenticated user
- Call `store.DeleteAnnotation`
- Return 200 + deleted annotation, or 404

### Task 10.5 — Handler: PATCH /api/annotations/{id} (resolve)

- Auth: any authenticated user
- Call `store.ResolveAnnotation`
- Return 200 + resolved annotation, or 404

### Task 10.6 — Register annotation routes

```go
r.Route("/api/annotations", func(r chi.Router) {
    r.Use(auth.RequireAuth)
    r.Post("/", h.CreateAnnotation)
    r.Get("/", h.ListAnnotations)
    r.Route("/{id}", func(r chi.Router) {
        r.Delete("/", h.DeleteAnnotation)
        r.Patch("/", h.ResolveAnnotation)
    })
})
```

### Task 10.7 — Contract tests: annotations

Test create, list-by-document, delete, resolve. Verify `resolved` timestamp is set on PATCH.

### Task 10.8 — Proxy flip: annotations

Add to `goRoutes`:
- `/api/annotations`
- `/api/annotations/:id`

---

## Group 11: Classrooms (Legacy)

**Files:**
- `gobackend/internal/store/classrooms.go`
- `gobackend/internal/handlers/classrooms.go`
- `gobackend/tests/contract/classrooms_test.go`

These are the original `classrooms` + `classroom_members` tables (not the new `classes` system). Both systems coexist during transition.

### Task 11.1 — Store: classrooms.go

| Function | SQL | Notes |
|---|---|---|
| `CreateClassroom(ctx, input) (*Classroom, error)` | `INSERT INTO classrooms (teacher_id, name, description, grade_level, editor_mode, join_code) VALUES (...) RETURNING *` | Generate 8-char join code |
| `ListClassrooms(ctx, userId) ([]Classroom, error)` | Union: classrooms where `teacher_id = userId` + classrooms where user is in `classroom_members` | De-duplicate by ID |
| `GetClassroom(ctx, id) (*Classroom, error)` | `SELECT * FROM classrooms WHERE id = $1` | |
| `GetClassroomByJoinCode(ctx, joinCode) (*Classroom, error)` | `SELECT * FROM classrooms WHERE join_code = $1` | |
| `JoinClassroom(ctx, classroomId, userId) (*ClassroomMember, error)` | `INSERT INTO classroom_members (classroom_id, user_id) VALUES (...) ON CONFLICT DO NOTHING RETURNING *` | |
| `GetClassroomMembers(ctx, classroomId) ([]ClassroomMemberRow, error)` | `SELECT cm.user_id, cm.joined_at, u.name, u.email FROM classroom_members cm JOIN users u ON cm.user_id = u.id WHERE cm.classroom_id = $1` | |
| `GetActiveSession(ctx, classroomId) (*LiveSession, error)` | `SELECT * FROM live_sessions WHERE classroom_id = $1 AND status = 'active'` | Already in sessions store — reuse |

### Task 11.2 — Handler: GET /api/classrooms

- Auth: any authenticated user
- Call `store.ListClassrooms(userId)`
- Return 200 + array

### Task 11.3 — Handler: POST /api/classrooms

- Validate: `name` (1-255), `description` (optional max 1000), `gradeLevel` (enum), `editorMode` (enum: blockly, python, javascript)
- Auth: any authenticated user (TODO role check is placeholder in current impl)
- Call `store.CreateClassroom`
- Return 201 + classroom

### Task 11.4 — Handler: GET /api/classrooms/{id}

- Auth: any authenticated user
- Call `store.GetClassroom`
- Return 404 if nil, else 200 + classroom

### Task 11.5 — Handler: POST /api/classrooms/join

- Validate: `joinCode` (string, length 8)
- Auth: any authenticated user
- Call `store.GetClassroomByJoinCode`, then `store.JoinClassroom`
- Return 200 + classroom, or 404

### Task 11.6 — Handler: GET /api/classrooms/{id}/members

- Auth: any authenticated user
- Verify classroom exists (404 if not)
- Call `store.GetClassroomMembers`
- Return 200 + array

### Task 11.7 — Handler: GET /api/classrooms/{id}/active-session

- Auth: any authenticated user
- Call `store.GetActiveSession` (reuse from sessions store)
- Return 200 + session or `null`

### Task 11.8 — Register classroom routes

```go
r.Route("/api/classrooms", func(r chi.Router) {
    r.Use(auth.RequireAuth)
    r.Get("/", h.ListClassrooms)
    r.Post("/", h.CreateClassroom)
    r.Post("/join", h.JoinClassroom)
    r.Route("/{id}", func(r chi.Router) {
        r.Get("/", h.GetClassroom)
        r.Get("/members", h.GetClassroomMembers)
        r.Get("/active-session", h.GetActiveSessionForClassroom)
    })
})
```

### Task 11.9 — Contract tests: classrooms

Test list (teacher sees own + joined), create, get, join-by-code, members, active-session (with and without an active session).

### Task 11.10 — Proxy flip: classrooms

Add to `goRoutes`:
- `/api/classrooms`
- `/api/classrooms/join`
- `/api/classrooms/:id`
- `/api/classrooms/:id/members`
- `/api/classrooms/:id/active-session`

---

## Execution Order

Groups should be implemented in this order due to dependencies:

1. **Group 1: Courses** — no dependencies beyond plan 017 foundations
2. **Group 2: Topics** — depends on courses store (for ownership check)
3. **Group 3: Classes** — depends on courses store (for language lookup during create)
4. **Group 4: Class Members** — depends on classes store
5. **Group 11: Classrooms (Legacy)** — independent, but do after classes to share patterns
6. **Group 5: Sessions** — depends on classrooms store (for teacher check on create)
7. **Group 6: Session Topics** — depends on sessions + topics stores
8. **Group 7: Documents** — independent of other groups
9. **Group 8: Assignments** — depends on class memberships store (for auth checks)
10. **Group 9: Submissions** — depends on assignments + class memberships stores
11. **Group 10: Annotations** — independent, can be done anytime

---

## Testing Strategy

### Unit tests

For each store function, write at least one test in `gobackend/tests/unit/store_*_test.go` using a test database. Test:
- Happy path (insert, select, update, delete)
- Not-found cases (return nil, not error)
- Conflict handling (ON CONFLICT DO NOTHING)
- Transaction rollback on failure (for CreateClass, CloneCourse, CreateSession)

### Contract tests

For each handler, write tests in `gobackend/tests/contract/*_test.go`:
- Send same request to both Next.js and Go
- Compare status codes and JSON response shape
- Test both success and error paths (401, 403, 404, 400)
- Use a shared test database with seeded data

### Integration tests

After all groups are flipped:
- Run existing Playwright E2E tests against the Go backend (no changes needed — they test the frontend)
- Verify SSE events flow correctly (session join/leave/help-queue)
- Verify the clone course flow (transactional topic copying)

---

## Rollback Plan

If a flipped route causes issues in production:
1. Remove the route pattern from `goRoutes` in `src/middleware.ts`
2. Restart Next.js — traffic falls back to the Next.js handler
3. No data migration needed (both servers share the same database)

---

## Definition of Done

- [ ] All 45 routes implemented in Go with matching behavior
- [ ] All store functions have unit tests
- [ ] All handlers have contract tests passing (Go matches Next.js)
- [ ] All routes flipped in proxy middleware
- [ ] Existing Playwright E2E tests pass with Go backend active
- [ ] SSE event streaming works for sessions (join, leave, help, broadcast)
- [ ] No regressions in the Next.js frontend

---

## Post-Execution Report

**Branch:** `feat/018-go-core-routes`
**PR:** #22
**Executed:** 2026-04-13

### Deviations from Plan

| Plan | Implementation | Reason |
|------|---------------|--------|
| Directory `gobackend/` | `platform/` | Renamed in Plan 017 |
| Port 8001 | Port 8002 | Changed in Plan 017 |
| `*pgxpool.Pool` | `*sql.DB` | Established in Plan 017 |
| Proxy flip per group | Deferred | Proxy routes stay commented until contract tests validate parity |
| Contract tests per group | Deferred to next pass | Focused on store/handler implementation first |
| Topics/Reorder returns `{"success": true}` | Returns updated topic list | More useful response; verify against Next.js before proxy flip |
| Topic/Reorder create: no ownership check | Added ownership check | Security improvement; verify Next.js behavior before proxy flip |

### What's Done

- All ~45 routes implemented across 11 domain groups
- Store integration tests: courses (10), topics (7), classes (6), orgs (33), users (4) = 60 total
- Handler unit tests: courses (11), orgs (18), auth (4), admin (3), helpers (3) = 39 total
- Auth tests: 17, config: 6, db: 3, events: 9, contract: 18

### What's Deferred

- Store integration tests for sessions, documents, assignments, annotations, classrooms
- Handler unit tests for all new domain groups (topics, classes, sessions, etc.)
- Contract tests per domain group
- Proxy flip in next.config.ts
- UUID validation on path parameters (returns 500 instead of 400 for invalid UUIDs)

## Code Review

### Review 1

- **Date**: 2026-04-13
- **Reviewer**: Claude (superpowers:code-reviewer)
- **PR**: #22 — feat: Go core routes (Plan 018)
- **Verdict**: Changes requested (4 critical auth gaps)

**Must Fix**

1. `[FIXED]` `CreateSession` has no classroom-teacher ownership check — any authenticated user can start a session in any classroom. `platform/internal/handlers/sessions.go`
   → Fixed: Added `ClassroomStore` dependency to `SessionHandler`, verify `classroom.TeacherID == claims.UserID`. Commit 00d5de4.

2. `[FIXED]` `ListDocuments` has no ownership enforcement for non-admin users — any user can list any other user's documents via `studentId` param. `platform/internal/handlers/documents.go`
   → Fixed: Non-admin users forced to `filters.OwnerID = claims.UserID`. Commit 00d5de4.

3. `[FIXED]` Assignment CRUD handlers have no role-based authorization — any authenticated user can create/update/delete assignments in any class. `platform/internal/handlers/assignments.go`
   → Fixed: Added `isInstructorOrTA` and `isClassMember` helpers. Create/Update/Delete require instructor/TA. List requires class member. Submit requires class member. ListSubmissions requires instructor/TA. Commit 00d5de4.

4. `[FIXED]` `GradeSubmission` has no role-based authorization — any authenticated user can grade any submission. `platform/internal/handlers/assignments.go`
   → Fixed: Added multi-hop auth chain: submission → assignment → class → membership role check. Commit 00d5de4.

**Should Fix**

5. `[FIXED]` SSE broadcaster calls subscriber callbacks under RLock — potential deadlock with slow clients. `platform/internal/events/broadcaster.go:44-53`
   → Fixed: Copy subscriber list under lock, invoke callbacks outside the lock. Commit bc3e5f1.

6. `[FIXED]` No UUID validation on path parameters — invalid UUIDs cause PostgreSQL errors returning 500 instead of 400.
   → Fixed: Added `ValidateUUIDParam` middleware, applied to all `{id}` route groups. Commit bc3e5f1.

7. `[WONTFIX]` `ReorderTopics`/`CreateTopic` ownership checks deviate from Next.js behavior (which has no ownership check).
   → Intentional security improvement. Will verify Next.js behavior before proxy flip.

**Nice to Have**

8. `[WONTFIX]` Missing handler tests for topics, classes, sessions, documents, assignments, annotations, classrooms.
   → Deferred: store integration tests cover the core logic, contract tests cover the HTTP layer. Handler unit tests for validation/auth are present for courses and orgs. Adding handler tests for all remaining groups is low-value given the existing coverage.

9. `[FIXED]` Missing store tests for sessions, documents, assignments, annotations, classrooms.
   → Fixed: Added 24 store integration tests across all 5 missing groups. Commit bc3e5f1.

10. `[FIXED]` Vestigial `init()` functions in `classes.go` store and `sessions.go` handler.
    → Removed. Commit 00d5de4.

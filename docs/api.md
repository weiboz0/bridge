# API Reference

All API routes are under `/api/`. Protected routes require authentication via Auth.js session.

## Authentication

### `POST /api/auth/register`

Register a new user with email and password.

**Request:**
```json
{
  "name": "Alice Smith",
  "email": "alice@school.edu",
  "password": "password123",
  "role": "teacher"
}
```

**Validation:**
- `name`: 1-255 characters, required
- `email`: valid email, required
- `password`: 8-128 characters, required
- `role`: `"teacher"` or `"student"`, required

**Responses:**
- `201` — User created
- `400` — Invalid input
- `409` — Email already registered

### `GET/POST /api/auth/[...nextauth]`

Auth.js handler. Supports Google OAuth and credentials sign-in. Not called directly — use the Auth.js client (`signIn`, `signOut`).

## Courses, Classes, and Class Memberships

Course/class APIs are served by the Go platform service. See `platform/README.md` for endpoint details. The pre-course-hierarchy `/api/classrooms` routes were removed in Plan 027.

## Sessions

The canonical session API supports both class-linked sessions and orphan sessions:

- session rows now live in the `sessions` table, not `live_sessions`
- `classId` is nullable; `null` means an orphan session
- live session status is `"live"` or `"ended"`
- participant lifecycle status is `"invited"`, `"present"`, or `"left"`
- raised-hand/help-queue state is no longer stored in participant `status`; it is tracked separately and exposed through the existing help-queue endpoints

Compatibility notes:

- Existing HTTP payloads still use `studentId` in participant/event payloads.
- `POST /api/sessions/{id}/join` still returns `studentId` in the response payload when applicable.
- `GET /api/sessions/{id}/help-queue` still returns the raised-hand queue, but internally it is backed by `help_requested_at` rather than a `"needs_help"` participant status.
- `POST /api/sessions/{id}/end` ends a session (moved from `PATCH /api/sessions/{id}` in Plan 030b).
- `GET /api/sessions/by-class/{classId}` and `GET /api/sessions/active/{classId}` remain available as compatibility wrappers for class-scoped surfaces.

### `POST /api/sessions`

Create a live session.

**Request:**
```json
{
  "title": "Office hours",
  "classId": "uuid-or-null",
  "settings": "{\"mode\":\"collaborative\"}"
}
```

**Auth:**
- Orphan sessions (`classId` omitted or `null`): any teacher or platform admin
- Class-linked sessions: class instructor, org admin for the class org, or platform admin

**Validation:**
- `title`: required
- `classId`: optional
- `settings`: optional, defaults to `"{}"`

**Responses:**
- `201` — Created session object
- `400` — Invalid JSON or missing `title`
- `401` — Not authenticated
- `403` — Not authorized to create the requested session
- `404` — `classId` does not exist

### `GET /api/sessions`

List sessions ordered by `startedAt DESC`, with cursor pagination.

**Query parameters:**
- `teacherId` — optional; defaults to the caller's user ID. Only platform admins may query another teacher's sessions.
- `classId` — optional; limits results to one class
- `status` — optional; `"live"` or `"ended"`
- `limit` — optional; default `20`, max `100`
- `cursor` — optional opaque cursor from the prior response's `nextCursor`

**Response (`200`):**
```json
{
  "items": [
    {
      "id": "uuid",
      "classId": null,
      "teacherId": "uuid",
      "title": "Office hours",
      "status": "live",
      "settings": "{}",
      "startedAt": "2026-04-23T19:00:00Z",
      "endedAt": null
    }
  ],
  "nextCursor": "opaque-cursor"
}
```

`nextCursor` is omitted when there is no subsequent page.

**Errors:**
- `400` — Invalid `limit` or malformed `cursor`
- `401` — Not authenticated
- `403` — Non-admin attempting to query another teacher's sessions

## Session Participants (Direct Add/Revoke)

Teachers (and class instructors, org admins, platform admins) can directly add or remove participants from a session without requiring a join code or invite token.

### `POST /api/sessions/{id}/participants`

Add a participant directly by user ID or email.

**Auth:** Session teacher, class instructor/ta (if session is class-bound), org admin, or platform admin.

**Request (one of):**
```json
{ "userId": "uuid" }
```
```json
{ "email": "user@example.com" }
```

**Response (`201`):**
```json
{
  "sessionId": "uuid",
  "studentId": "uuid",
  "status": "invited",
  "invitedBy": "teacher-uuid",
  "invitedAt": "2026-01-15T12:00:00Z",
  "joinedAt": null,
  "leftAt": null
}
```

Adding a user who is already a participant is idempotent (returns 201 with the existing row). If a participant previously left, they are re-invited.

**Errors:**
- `400` — Neither `userId` nor `email` provided
- `401` — Not authenticated
- `403` — Not authorized
- `404` — Session not found, or user not found (when using `email`)

### `DELETE /api/sessions/{id}/participants/{userId}`

Remove a participant from the session entirely (deletes the participant row).

**Auth:** Same as add (session teacher, class instructor/ta, org admin, platform admin).

**Responses:**
- `204` — Participant removed (no body)
- `401` — Not authenticated
- `403` — Not authorized
- `404` — Session not found, or participant not found

## Session Access Control

### `GET /api/sessions/{id}` (tightened)

Fetch a session by ID. Access is now restricted:
- Session teacher: always allowed
- Class member (any role, if session is class-bound): allowed
- Session participant (invited or present): allowed
- Platform admin: always allowed
- Anyone else: returns **404** (does not leak session existence)

### `GET /api/sessions/{id}/participants` (tightened)

Fetch the participant roster. Restricted to authority roles only:
- Session teacher, class instructor/ta, org admin, platform admin: allowed
- Regular participants and other users: **403**

## Session Invites

Invite tokens allow students to join a session via a shareable link without needing to be pre-enrolled in the class. All invite endpoints require authentication.

### `PATCH /api/sessions/{id}`

Update mutable session fields. Only the session teacher or a platform admin can call this.

**Request:**
```json
{
  "title": "New title",
  "settings": "{\"key\":\"value\"}",
  "inviteExpiresAt": "2026-01-15T12:00:00Z"
}
```

All fields are optional. Send `"inviteExpiresAt": null` to clear the expiry (open lobby).

**Responses:**
- `200` — Updated session object
- `403` — Not the session teacher
- `404` — Session not found

### `POST /api/sessions/{id}/rotate-invite`

Generate a new invite token, invalidating any previous token immediately. Only the session teacher or a platform admin can call this.

**Request:** empty body

**Response (`200`):**
```json
{
  "id": "...",
  "inviteToken": "newRandomToken24chars",
  "inviteExpiresAt": null,
  "..."
}
```

**Errors:**
- `403` — Not the session teacher
- `404` — Session not found

### `DELETE /api/sessions/{id}/invite`

Revoke the invite token entirely, making any existing invite links dead.

**Responses:**
- `204` — Token revoked (no body)
- `403` — Not the session teacher
- `404` — Session not found

### `POST /api/s/{token}/join`

Join a session using an invite token. The user must be logged in.

**Request:** empty body

**Response (`200`):**
```json
{
  "sessionId": "uuid",
  "classId": "uuid-or-null",
  "participant": {
    "sessionId": "uuid",
    "studentId": "uuid",
    "status": "present",
    "joinedAt": "2026-01-15T12:00:00Z",
    "leftAt": null
  }
}
```

**Errors:**
- `401` — Not authenticated
- `404` — Invalid or unknown invite token
- `410` — Invite link expired or session has ended

---

## Platform Admin — Users

All endpoints require `is_platform_admin = true`. RequireAuth additionally returns 401 on `status = 'suspended'` users (admin or otherwise — the cached admin/status check shares a 60s TTL but is purged immediately on suspend / toggle-admin writes; see `platform/internal/auth/admin_check.go`).

### `GET /api/admin/users`

List platform-admin users with optional filters.

**Query parameters:**
- `role` (optional) — `org_admin` | `teacher` | `student` | `parent` | `platform_admin` | `unassigned`. Filters by primary org-membership role; `platform_admin` filters by `users.is_platform_admin`; `unassigned` returns users with no active org membership AND `is_platform_admin = false`.
- `orgId` (optional, UUID) — filter to users whose primary org-membership is in this org. Combinable with `role` (AND).

**Response (`200`):** array of `AdminUser`:
```json
[
  {
    "id": "uuid",
    "name": "Alice",
    "email": "alice@school.edu",
    "avatarUrl": null,
    "isPlatformAdmin": false,
    "status": "active",
    "orgRole": "teacher",
    "orgId": "uuid",
    "orgName": "Riverdale School",
    "hasPassword": true,
    "createdAt": "2026-04-01T00:00:00Z",
    "updatedAt": "2026-05-01T00:00:00Z"
  }
]
```

Primary org-membership = earliest active membership by `created_at` (LATERAL `LIMIT 1`). Users with no active membership have null `orgRole` / `orgId` / `orgName`.

**Errors:**
- `400` — Unknown role value or malformed orgId UUID
- `401` — Not authenticated (incl. suspended)
- `403` — Not a platform admin

### `GET /api/admin/users/{userID}`

Return a single enriched user.

**Response (`200`):** `AdminUser` (same shape as the list endpoint, single object).

**Errors:**
- `400` — Malformed UUID
- `401` — Not authenticated (incl. suspended)
- `403` — Not a platform admin
- `404` — User not found

### `PATCH /api/admin/users/{userID}/status`

Suspend or reactivate a user. Invalidates the admin-status cache entry for the target user immediately (`AdminChecker.Purge(userID)`), so the change takes effect on the next request — no 60s wait.

**Request body:**
```json
{ "status": "active" | "suspended" }
```

**Response (`200`):** the updated `AdminUser` row.

**Errors:**
- `400` — Invalid status value, malformed UUID, OR self-target (cannot change own status)
- `401` — Not authenticated (incl. suspended)
- `403` — Not a platform admin
- `404` — User not found

### `PATCH /api/admin/users/{userID}/platform-admin`

Promote / demote a user as platform admin. Invalidates the cache entry for the target user immediately.

**Request body:**
```json
{ "isPlatformAdmin": true | false }
```

**Response (`200`):** the updated `AdminUser` row.

**Errors:**
- `400` — Malformed UUID OR self-demote attempt (cannot remove own platform-admin role)
- `401` — Not authenticated (incl. suspended)
- `403` — Not a platform admin
- `404` — User not found

### Auth semantics for suspended users

- `RequireAuth` returns `401` on `status='suspended'` regardless of admin status.
- `OptionalAuth` treats suspended as unauthenticated (claims dropped, request proceeds without identity).
- NextAuth `authorize()` does NOT currently check `users.status` — a suspended user can still complete sign-in but will 401 on the first subsequent Go-proxied request. Acceptable v1; a future plan will add the check for graceful "Account suspended" sign-in failure.

---

## Platform Admin — Organizations

All endpoints require `is_platform_admin = true`. Same suspended-user semantics as the Platform Admin Users section.

### `GET /api/admin/orgs/{orgID}`

Return a single org enriched with active-membership counts per role.

**Response (`200`):** `AdminOrg`:
```json
{
  "id": "uuid",
  "name": "Riverdale School",
  "slug": "riverdale",
  "type": "k12",
  "status": "active",
  "contactEmail": "principal@riverdale.edu",
  "contactName": "Diana Riverdale",
  "domain": null,
  "settings": "{}",
  "verifiedAt": null,
  "createdAt": "2026-01-01T00:00:00Z",
  "updatedAt": "2026-05-12T00:00:00Z",
  "teacherCount": 5,
  "studentCount": 32,
  "parentCount": 3,
  "adminCount": 2,
  "totalActive": 42
}
```

Counts come from a `LATERAL` subquery over `org_memberships WHERE org_id = $1 AND status = 'active'`, FILTER-aggregated by role. Suspended memberships are excluded.

**Errors:**
- `400` — Malformed UUID
- `401` — Not authenticated (incl. suspended)
- `403` — Not a platform admin
- `404` — Organization not found

### `PATCH /api/admin/orgs/{orgID}/details`

Update an org's display fields. All three fields are required (no partial updates v1).

**Request body:**
```json
{
  "name": "Riverdale School District",
  "contactName": "Diana Riverdale",
  "contactEmail": "principal@riverdale.edu"
}
```

Validation: each field is `strings.TrimSpace`'d before the non-empty check (leading/trailing whitespace is acceptable and stripped). `contactEmail` is validated via `net/mail.ParseAddress`.

**Response (`200`):** the updated `AdminOrg` row (with current counts).

**Errors:**
- `400` — Empty field (post-trim), malformed email, or malformed UUID
- `401` — Not authenticated (incl. suspended)
- `403` — Not a platform admin
- `404` — Organization not found (including the POST-UPDATE race where the org was deleted between request and write)

### Existing org endpoints (unchanged)

- `GET /api/admin/orgs?status=...` — list orgs (optional status filter).
- `PATCH /api/admin/orgs/{orgID}` — status changes only (`{status: "active" | "suspended"}`). Distinct from `/details` to keep status-change semantics narrow.

---

## Books (library)

Plan 088 introduced a static curriculum library: a `book` contains many `chapters`. Distinct from courses (the per-class delivery layer that pulls from books). All endpoints require an authenticated user with appropriate scope access.

### `Book` shape

```json
{
  "id": "uuid",
  "title": "Python for K-8 Beginners",
  "description": "Curriculum for first-time Python learners",
  "scope": "platform" | "org",
  "scopeId": "uuid-or-null",
  "createdBy": "uuid",
  "createdAt": "2026-05-14T00:00:00Z",
  "updatedAt": "2026-05-14T00:00:00Z"
}
```

`scope = "platform"` → `scopeId` is null. `scope = "org"` → `scopeId` is the owning org UUID. There is no "personal" scope (plan 088 Decision #5).

### `POST /api/books`

Create a book. Body: `{title, description, scope, scopeId}`. Validation:
- `title` trimmed 1-255 chars.
- `scope` ∈ {`platform`, `org`}.
- `scopeId` nil iff `scope = "platform"`; required UUID for `scope = "org"`.

Returns `201` + Book. `400` on validation failure. `401`/`403` as usual.

### `GET /api/books?scope=&scopeId=`

List books visible to the caller. Optional filters narrow to a single scope bucket. Returns `200` + `{items: Book[]}`.

### `GET /api/books/{id}`

Single book by ID. `404` if not found, `403` if not visible to caller.

### `PATCH /api/books/{id}`

Update `title` + `description` (scope + scopeId are immutable post-create). Returns the updated `Book`. Same validation rules as create for the writable fields.

### `DELETE /api/books/{id}`

Delete a book. Chapters previously assigned to the book have their `book_id` set to NULL (`ON DELETE SET NULL`) — they become "unfiled".

Returns `204` on success. `404` if not found. `403` if not authorized.

---

## Chapters (formerly Units)

Plan 088 renamed `teaching_units` → `chapters` across the stack. All `/api/units/*` paths are removed; `/api/chapters/*` is the canonical path. Frontend bookmarks under `/teacher/units/*`, `/admin/units/*`, etc. are redirected via Next.js 308 to the new `/chapters/*` paths.

### Chapter list filter additions (plan 088)

`GET /api/chapters?scope=&scopeId=&bookId=` and `GET /api/chapters/search?...&bookId=` both accept a new `bookId` query parameter:

- Omit → no filter.
- UUID → `WHERE book_id = $1`.
- `unfiled` (literal string) → `WHERE book_id IS NULL`.

Bad value returns `400 "bookId must be a UUID or 'unfiled'"`.

The `Chapter` JSON response shape gains a `bookId: string | null` field.

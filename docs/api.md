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

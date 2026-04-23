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

The current session API is still the class-bound surface from earlier plans, but its underlying schema was updated in Plan 030a:

- session rows now live in the `sessions` table, not `live_sessions`
- live session status is `"live"` or `"ended"`
- participant lifecycle status is `"invited"`, `"present"`, or `"left"`
- raised-hand/help-queue state is no longer stored in participant `status`; it is tracked separately and exposed through the existing help-queue endpoints

Compatibility notes for the current API:

- Existing HTTP payloads still use `studentId` in participant/event payloads.
- `POST /api/sessions/{id}/join` still returns `studentId` in the response payload when applicable.
- `GET /api/sessions/{id}/help-queue` still returns the raised-hand queue, but internally it is backed by `help_requested_at` rather than a `"needs_help"` participant status.
- `POST /api/sessions/{id}/end` ends a session (moved from `PATCH /api/sessions/{id}` in Plan 030b).

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

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

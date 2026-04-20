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

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

## Classrooms

All classroom routes require authentication.

### `GET /api/classrooms`

List classrooms for the authenticated user (as teacher or member).

**Response:** `200`
```json
[
  {
    "id": "uuid",
    "schoolId": null,
    "teacherId": "uuid",
    "name": "Intro to Python",
    "description": "Learn Python basics",
    "gradeLevel": "6-8",
    "editorMode": "python",
    "joinCode": "ABC12345",
    "createdAt": "2026-04-10T...",
    "updatedAt": "2026-04-10T..."
  }
]
```

### `POST /api/classrooms`

Create a new classroom. Only teachers and admins can create classrooms.

**Request:**
```json
{
  "name": "Intro to Python",
  "description": "Learn Python basics",
  "gradeLevel": "6-8",
  "editorMode": "python"
}
```

**Validation:**
- `name`: 1-255 characters, required
- `description`: max 1000 characters, optional
- `gradeLevel`: `"K-5"`, `"6-8"`, or `"9-12"`, required
- `editorMode`: `"blockly"`, `"python"`, or `"javascript"`, required

**Responses:**
- `201` — Classroom created (includes auto-generated `joinCode`)
- `400` — Invalid input
- `401` — Not authenticated
- `403` — Not a teacher or admin

### `GET /api/classrooms/[id]`

Get a single classroom by ID.

**Responses:**
- `200` — Classroom object
- `401` — Not authenticated
- `404` — Not found

### `GET /api/classrooms/[id]/members`

List all members (students) of a classroom.

**Response:** `200`
```json
[
  {
    "userId": "uuid",
    "joinedAt": "2026-04-10T...",
    "name": "Bob Student",
    "email": "bob@school.edu",
    "role": "student"
  }
]
```

### `POST /api/classrooms/join`

Join a classroom by its join code.

**Request:**
```json
{
  "joinCode": "ABC12345"
}
```

**Responses:**
- `200` — Classroom object (joined successfully)
- `400` — Invalid join code format
- `401` — Not authenticated
- `404` — Classroom not found

# 021 — Frontend Cleanup: Remove Backend from Next.js

> **NOTE:** This plan requires review and approval before execution. Do not execute until the user explicitly approves.

**Phase:** 5 of the Go backend migration (spec `docs/specs/004-go-backend-migration.md`)

**Goal:** Remove all backend logic from Next.js — database access, API route handlers, server actions, AI modules — and make it a pure frontend that fetches data from the Go API. Auth.js stays for OAuth flow. Hocuspocus stays for Yjs sync. Everything else goes through Go.

**Prerequisite:** The Go backend must implement all API routes currently served by Next.js, including the new endpoints listed in Task 5. Phases 1-4 of the migration spec must be complete, with all contract tests passing.

**Depends on:** Plans 017-020 (Go backend implementation phases 1-4, not yet written)

---

## Inventory of Changes

### Files to convert (server components with DB access → Go API fetch)

| File | Current data source | New data source |
|------|-------------------|-----------------|
| `src/app/page.tsx` | `getEffectiveSession()` + `getUserMemberships(db)` + `buildUserRoles()` | `api("/api/me/roles")` |
| `src/components/portal/portal-shell.tsx` | `getEffectiveSession()` + `getUserMemberships(db)` + `buildUserRoles()` | `api("/api/me/portal-access")` |
| `src/app/(portal)/teacher/page.tsx` | `listCoursesByCreator(db)` + `listClassesByUser(db)` | `api("/api/courses?role=creator")` + `api("/api/classes?role=instructor")` |
| `src/app/(portal)/teacher/courses/page.tsx` | `listCoursesByCreator(db)` + `getUserMemberships(db)` + server action `handleCreateCourse` | `api("/api/courses?role=creator")` + `api("/api/me/memberships")` + client POST |
| `src/app/(portal)/teacher/courses/[id]/page.tsx` | `getCourse(db)` + `listTopicsByCourse(db)` + `listClassesByCourse(db)` + server actions `handleAddTopic`, `handleDeleteTopic` | `api("/api/courses/{id}")` + client POST/DELETE |
| `src/app/(portal)/teacher/courses/[id]/create-class/page.tsx` | `getCourse(db)` + server action `handleCreate` | `api("/api/courses/{id}")` + client POST |
| `src/app/(portal)/teacher/classes/page.tsx` | `listClassesByUser(db)` | `api("/api/classes?role=instructor")` |
| `src/app/(portal)/teacher/classes/[id]/page.tsx` | `getClass(db)` + `listClassMembers(db)` | `api("/api/classes/{id}")` + `api("/api/classes/{id}/members")` |
| `src/app/(portal)/teacher/classes/[id]/session/[sessionId]/dashboard/page.tsx` | `getClass(db)` + `listClassMembers(db)` + `getSession(db)` + `getClassroom(db)` + `getCourse(db)` + `listTopicsByCourse(db)` | `api("/api/classes/{id}/session-context/{sessionId}")` |
| `src/app/(portal)/student/page.tsx` | `listClassesByUser(db)` | `api("/api/classes?role=student")` |
| `src/app/(portal)/student/classes/page.tsx` | `listClassesByUser(db)` | `api("/api/classes?role=student")` |
| `src/app/(portal)/student/classes/[id]/page.tsx` | `getClass(db)` + `listClassMembers(db)` + `getCourse(db)` + `listTopicsByCourse(db)` | `api("/api/classes/{id}/student-view")` |
| `src/app/(portal)/student/classes/[id]/session/[sessionId]/page.tsx` | `getClass(db)` + `listClassMembers(db)` + `getSession(db)` + `joinSession(db)` + `getClassroom(db)` | `api("/api/sessions/{sessionId}/join")` (POST, returns context) |
| `src/app/(portal)/student/code/page.tsx` | `listDocuments(db)` | `api("/api/documents?owner=me")` |
| `src/app/(portal)/parent/page.tsx` | `getLinkedChildren(db)` + `getActiveSessionForStudent(db)` | `api("/api/parent/children")` |
| `src/app/(portal)/parent/children/[id]/page.tsx` | `getLinkedChildren(db)` + `db.select(users)` + `listClassesByUser(db)` + `listDocuments(db)` | `api("/api/parent/children/{id}")` |
| `src/app/(portal)/parent/children/[id]/live/page.tsx` | `getLinkedChildren(db)` + `getActiveSessionForStudent(db)` | `api("/api/parent/children/{id}/live")` |
| `src/app/(portal)/admin/page.tsx` | `countOrganizations(db)` + `countUsers(db)` | `api("/api/admin/stats")` |
| `src/app/(portal)/admin/orgs/page.tsx` | `listOrganizations(db)` + server actions `approveOrg`, `suspendOrg` | `api("/api/admin/orgs")` + client PATCH |
| `src/app/(portal)/admin/users/page.tsx` | `listUsers(db)` | `api("/api/admin/users")` |
| `src/app/(portal)/org/page.tsx` | `getUserMemberships(db)` + `getOrganization(db)` + `listOrgMembers(db)` + `listCoursesByOrg(db)` + `listClassesByOrg(db)` | `api("/api/me/org-dashboard")` |

### Files with server actions to convert → client-side fetch

| File | Server action | New approach |
|------|--------------|-------------|
| `src/app/(portal)/teacher/courses/page.tsx` | `handleCreateCourse` | Client component form, `POST /api/courses` |
| `src/app/(portal)/teacher/courses/[id]/page.tsx` | `handleAddTopic`, `handleDeleteTopic` | Client component forms, `POST /api/courses/{id}/topics`, `DELETE /api/courses/{id}/topics/{topicId}` |
| `src/app/(portal)/teacher/courses/[id]/create-class/page.tsx` | `handleCreate` | Client component form, `POST /api/classes` |
| `src/app/(portal)/admin/orgs/page.tsx` | `approveOrg`, `suspendOrg` | Client component buttons, `PATCH /api/admin/orgs/{id}` |

### Files already client-side (fetch to `/api/*` — just update base URL)

| File | Current fetch | Change needed |
|------|--------------|---------------|
| `src/app/(portal)/teacher/courses/[id]/topics/[topicId]/page.tsx` | `fetch(/api/courses/...)` | Prefix with `GO_API_URL` or rely on proxy (no change needed if proxy remains during transition) |
| `src/app/(portal)/parent/children/[id]/reports/page.tsx` | `fetch(/api/parent/children/...)` | Same — already client-side fetch |

### Files to delete

| Category | Files |
|----------|-------|
| **Database layer** | `src/lib/db/index.ts`, `src/lib/db/schema.ts` |
| **Backend modules** | `src/lib/annotations.ts`, `src/lib/sessions.ts`, `src/lib/sse.ts`, `src/lib/classrooms.ts`, `src/lib/org-memberships.ts`, `src/lib/topics.ts`, `src/lib/class-memberships.ts`, `src/lib/documents.ts`, `src/lib/parent-links.ts`, `src/lib/organizations.ts`, `src/lib/users.ts`, `src/lib/classes.ts`, `src/lib/courses.ts`, `src/lib/assignments.ts`, `src/lib/submissions.ts`, `src/lib/session-topics.ts`, `src/lib/parent-reports.ts`, `src/lib/attendance.ts` |
| **AI modules** | `src/lib/ai/interactions.ts`, `src/lib/ai/system-prompts.ts`, `src/lib/ai/guardrails.ts`, `src/lib/ai/client.ts`, `src/lib/ai/providers.ts`, `src/lib/ai/report-prompts.ts` |
| **API routes** | All files under `src/app/api/` **except** `src/app/api/auth/[...nextauth]/route.ts` |
| **Impersonation** | `src/lib/impersonate.ts` (Go handles impersonation via cookie reading) |

### Files to keep unchanged

| File | Reason |
|------|--------|
| `src/lib/auth.ts` | Auth.js config — stays for OAuth flow. But needs modification (see Task 2.1) |
| `src/app/api/auth/[...nextauth]/route.ts` | Auth.js route handler — stays |
| `src/lib/portal/roles.ts` | Pure logic, no DB access — stays (used for client-side role resolution from API response) |
| `src/lib/portal/nav-config.ts` | Pure config — stays |
| `src/lib/portal/types.ts` | Type definitions — stays |
| `src/lib/utils.ts` | UI utility (cn function) — stays |
| `src/lib/lesson-content.ts` | Pure parsing logic — stays |
| All `src/components/**` | React components, no DB access — stay |
| All `e2e/**` | Playwright tests — stay (updated in Task 6) |
| All placeholder pages | `teacher/reports`, `teacher/schedule`, `student/help`, `org/settings`, `org/courses`, `org/classes`, `org/teachers`, `org/students`, `admin/settings`, `parent/reports`, `parent/children/page.tsx` — no DB access, stay as-is |

### Tests to delete

| File | Reason |
|------|--------|
| `tests/unit/annotations.test.ts` | Tests deleted backend module |
| `tests/unit/organizations.test.ts` | Tests deleted backend module |
| `tests/unit/org-memberships.test.ts` | Tests deleted backend module |
| `tests/unit/classes.test.ts` | Tests deleted backend module |
| `tests/unit/courses.test.ts` | Tests deleted backend module |
| `tests/unit/topics.test.ts` | Tests deleted backend module |
| `tests/unit/documents.test.ts` | Tests deleted backend module |
| `tests/unit/users.test.ts` | Tests deleted backend module |
| `tests/unit/parent-links.test.ts` | Tests deleted backend module |
| `tests/unit/submissions.test.ts` | Tests deleted backend module |
| `tests/unit/assignments.test.ts` | Tests deleted backend module |
| `tests/unit/session-topics.test.ts` | Tests deleted backend module |
| `tests/unit/attendance.test.ts` | Tests deleted backend module |
| `tests/unit/schema.test.ts` | Tests deleted DB schema |
| `tests/unit/guardrails.test.ts` | Tests deleted AI module |
| `tests/unit/system-prompts.test.ts` | Tests deleted AI module |
| `tests/unit/providers.test.ts` | Tests deleted AI module |
| `tests/unit/sse.test.ts` | Tests deleted SSE module |
| `tests/llm/providers.test.ts` | Tests deleted LLM module |
| `tests/llm/guardrails.test.ts` | Tests deleted LLM module |
| `tests/api/sessions.test.ts` | Tests deleted API routes |
| `tests/api/db-connection.test.ts` | Tests deleted DB connection |
| `tests/api/classrooms-join.test.ts` | Tests deleted API routes |
| `tests/api/classrooms.test.ts` | Tests deleted API routes |
| `tests/api-helpers.ts` | Used by deleted API tests |
| `tests/integration/classrooms-api.test.ts` | Tests deleted API routes |
| `tests/integration/ai-toggle-api.test.ts` | Tests deleted API routes |
| `tests/integration/admin-orgs-api.test.ts` | Tests deleted API routes |
| `tests/integration/auth-register.test.ts` | Tests deleted API routes |
| `tests/integration/annotations-api.test.ts` | Tests deleted API routes |
| `tests/integration/orgs-api.test.ts` | Tests deleted API routes |
| `tests/integration/sessions-api.test.ts` | Tests deleted API routes |
| `tests/integration/documents-api.test.ts` | Tests deleted API routes |
| `tests/integration/courses-api.test.ts` | Tests deleted API routes |
| `tests/integration/classes-api.test.ts` | Tests deleted API routes |
| `tests/integration/org-members-api.test.ts` | Tests deleted API routes |
| `tests/integration/class-members-api.test.ts` | Tests deleted API routes |

### Tests to keep (no DB dependency)

| File | Reason |
|------|--------|
| `tests/unit/utils.test.ts` | Tests `src/lib/utils.ts` — stays |
| `tests/unit/portal-roles.test.ts` | Tests pure logic in `src/lib/portal/roles.ts` — stays |
| `tests/unit/nav-config.test.ts` | Tests pure config — stays |
| `tests/unit/lesson-content.test.ts` | Tests pure parsing — stays |
| `tests/unit/hooks.test.ts` | Tests React hooks — stays |
| `tests/unit/ai-activity-feed.test.tsx` | Tests React component — stays |
| `tests/unit/output-panel.test.tsx` | Tests React component — stays |
| `tests/unit/annotation-list.test.tsx` | Tests React component — stays |
| `tests/unit/use-pyodide.test.ts` | Tests client hook — stays |
| `tests/unit/diff-viewer.test.tsx` | Tests React component — stays |
| `tests/unit/monaco-themes.test.ts` | Tests client config — stays |
| `tests/unit/student-tile.test.tsx` | Tests React component — stays |
| `tests/unit/code-editor.test.tsx` | Tests React component — stays |
| `tests/unit/js-runner.test.ts` | Tests client runtime — stays |
| `tests/unit/blockly-toolbox.test.ts` | Tests client config — stays |
| `tests/helpers.ts` | Shared test helpers — stays (review for DB references) |

---

## Tasks

### Task 1: Create API client (`src/lib/api-client.ts`)

**New file:** `src/lib/api-client.ts`

A shared fetch wrapper that forwards the Auth.js session token to the Go backend. Used by server components (reads cookie from headers) and can be imported by client components too.

```typescript
// src/lib/api-client.ts
import { cookies } from "next/headers";

const GO_API_URL = process.env.GO_API_URL || "http://localhost:8001";

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public body?: unknown
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/**
 * Server-side API client. Reads the session cookie and forwards it
 * as a Bearer token to the Go backend.
 *
 * Usage (server component):
 *   const data = await api<MyType>("/api/courses");
 *   const data = await api<MyType>("/api/courses", { method: "POST", body: { title: "..." } });
 */
export async function api<T = unknown>(
  path: string,
  options: {
    method?: string;
    body?: unknown;
    headers?: Record<string, string>;
  } = {}
): Promise<T> {
  const cookieStore = await cookies();
  const sessionToken =
    cookieStore.get("__Secure-next-auth.session-token")?.value ||
    cookieStore.get("next-auth.session-token")?.value;

  const impersonateCookie = cookieStore.get("bridge-impersonate")?.value;

  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...options.headers,
  };

  if (sessionToken) {
    headers["Authorization"] = `Bearer ${sessionToken}`;
  }

  if (impersonateCookie) {
    headers["X-Bridge-Impersonate"] = impersonateCookie;
  }

  const url = `${GO_API_URL}${path}`;
  const res = await fetch(url, {
    method: options.method || "GET",
    headers,
    body: options.body ? JSON.stringify(options.body) : undefined,
    cache: "no-store",
  });

  if (!res.ok) {
    const body = await res.json().catch(() => null);
    throw new ApiError(res.status, `API ${res.status}: ${path}`, body);
  }

  // 204 No Content
  if (res.status === 204) return undefined as T;

  return res.json() as Promise<T>;
}

/**
 * Client-side API path builder. Client components fetch relative paths
 * which Next.js proxies to Go, so no base URL needed.
 * If the proxy is removed, set NEXT_PUBLIC_GO_API_URL.
 */
export function clientApiUrl(path: string): string {
  const base = process.env.NEXT_PUBLIC_GO_API_URL || "";
  return `${base}${path}`;
}
```

**Tests to write:** `tests/unit/api-client.test.ts`

- [ ] Test `api()` constructs correct URL with `GO_API_URL`
- [ ] Test `api()` forwards session token as Bearer header
- [ ] Test `api()` forwards impersonation cookie as `X-Bridge-Impersonate` header
- [ ] Test `api()` throws `ApiError` on non-2xx responses
- [ ] Test `api()` returns `undefined` for 204 responses
- [ ] Test `api()` sends JSON body for POST/PATCH methods
- [ ] Test `clientApiUrl()` with and without `NEXT_PUBLIC_GO_API_URL`

---

### Task 2: Modify Auth.js to remove DB writes (keep OAuth flow)

**File:** `src/lib/auth.ts`

Auth.js stays for the OAuth flow, but its callbacks currently write to the database (user upsert, provider linkage, JWT enrichment). After migration, Go handles user creation on first login. Auth.js only needs to issue a JWT with the Google profile info. Go enriches it with `user.id` and `isPlatformAdmin` on the API side.

**Before (current):**
```typescript
import { db } from "@/lib/db";
import { users, authProviders } from "@/lib/db/schema";
import { eq, and } from "drizzle-orm";

// ... callbacks that do db.select(), db.insert() ...
```

**After:**
```typescript
// No db imports. No drizzle. No bcryptjs.
// signIn callback: POST to Go /api/auth/oauth-callback to handle user upsert
// jwt callback: call Go /api/auth/enrich-token to get user.id + isPlatformAdmin
// Credentials provider: POST to Go /api/auth/credentials to validate
```

The Credentials provider currently does `db.select()` + `bcrypt.compare()`. This moves to a Go endpoint `POST /api/auth/credentials` that returns the user object or 401.

The `signIn` callback currently upserts users and links providers. This moves to `POST /api/auth/oauth-callback` which Go handles.

The `jwt` callback currently reads `users` table. This moves to `GET /api/auth/enrich-token?email=...` which Go handles.

**Key implementation detail:** These Auth.js callback calls to Go happen server-to-server (Next.js → Go) during the login flow, not from the browser. Use a direct `fetch()` with a shared internal secret header, not the `api()` client (which reads cookies that don't exist yet during login).

**New Go endpoints needed:**

| Endpoint | Purpose |
|----------|---------|
| `POST /api/auth/credentials` | Validate email+password, return user or 401 |
| `POST /api/auth/oauth-callback` | Upsert user + link provider, return user |
| `GET /api/auth/enrich-token?email={email}` | Return `{ id, isPlatformAdmin }` for JWT enrichment |

**Tests to write:** `tests/unit/auth-callbacks.test.ts`

- [ ] Test signIn callback calls Go `/api/auth/oauth-callback` with correct payload
- [ ] Test jwt callback calls Go `/api/auth/enrich-token` and sets token fields
- [ ] Test credentials authorize calls Go `/api/auth/credentials`
- [ ] Test credentials authorize returns null on 401
- [ ] Test session callback maps token fields to session

---

### Task 3: Convert portal pages — Teacher portal

#### 3.1 Teacher Dashboard (`src/app/(portal)/teacher/page.tsx`)

**Before:**
```typescript
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { listCoursesByCreator } from "@/lib/courses";
import { listClassesByUser } from "@/lib/classes";

const session = await auth();
const [courses, classes] = await Promise.all([
  listCoursesByCreator(db, session!.user.id),
  listClassesByUser(db, session!.user.id),
]);
```

**After:**
```typescript
import { api } from "@/lib/api-client";

interface TeacherDashboardData {
  courses: Array<{ id: string; title: string; /* ... */ }>;
  classes: Array<{ id: string; title: string; term: string | null; status: string; memberRole: string; /* ... */ }>;
}

const data = await api<TeacherDashboardData>("/api/teacher/dashboard");
const courses = data.courses;
const myClasses = data.classes.filter((c) => c.memberRole === "instructor");
```

Remove imports: `auth`, `db`, `listCoursesByCreator`, `listClassesByUser`.

#### 3.2 Teacher Courses List (`src/app/(portal)/teacher/courses/page.tsx`)

This page has both data loading AND a server action (`handleCreateCourse`). Split into:
- Server component for data loading (calls Go API)
- Client component for the create-course form

**Before (data loading):**
```typescript
const courses = await listCoursesByCreator(db, session!.user.id);
const memberships = await getUserMemberships(db, session!.user.id);
```

**After (data loading):**
```typescript
import { api } from "@/lib/api-client";

const data = await api<TeacherCoursesData>("/api/teacher/courses");
// data.courses — list of courses by this creator
// data.teacherOrgs — deduplicated orgs where user is teacher/org_admin
```

**Before (server action):**
```typescript
async function handleCreateCourse(formData: FormData) {
  "use server";
  const { auth: getAuth } = await import("@/lib/auth");
  const { db: database } = await import("@/lib/db");
  const { createCourse: create } = await import("@/lib/courses");
  // ...
  const course = await create(database, { orgId, createdBy, title, gradeLevel });
  redirect(`/teacher/courses/${course.id}`);
}
```

**After:** Extract form into a client component `src/components/teacher/create-course-form.tsx`:
```typescript
"use client";

export function CreateCourseForm({ teacherOrgs }: Props) {
  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const formData = new FormData(e.currentTarget);
    const res = await fetch("/api/courses", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        title: formData.get("title"),
        orgId: formData.get("orgId"),
        gradeLevel: formData.get("gradeLevel"),
      }),
    });
    if (res.ok) {
      const course = await res.json();
      window.location.href = `/teacher/courses/${course.id}`;
    }
  }
  // ... render form ...
}
```

Remove: `"use server"` block, `revalidatePath`, `redirect` (from server action), `db` import, `createCourse` import.

#### 3.3 Teacher Course Detail (`src/app/(portal)/teacher/courses/[id]/page.tsx`)

Two server actions to convert: `handleAddTopic` and `handleDeleteTopic`.

**Before:**
```typescript
const course = await getCourse(db, id);
const [topicList, classList] = await Promise.all([
  listTopicsByCourse(db, id),
  listClassesByCourse(db, id),
]);
```

**After:**
```typescript
const data = await api<CourseDetailData>(`/api/courses/${id}`);
// data.course, data.topics, data.classes
```

Extract into client components:
- `src/components/teacher/add-topic-form.tsx` — `POST /api/courses/{id}/topics`
- `src/components/teacher/delete-topic-button.tsx` — `DELETE /api/courses/{id}/topics/{topicId}`

Both use `useRouter().refresh()` after mutation to re-fetch server component data.

#### 3.4 Create Class from Course (`src/app/(portal)/teacher/courses/[id]/create-class/page.tsx`)

**Before:**
```typescript
const course = await getCourse(db, id);
// server action: createClass(database, { courseId, orgId, title, term, createdBy })
```

**After:**
```typescript
const course = await api<Course>(`/api/courses/${id}`);
// Extract form to client component: POST /api/classes
```

New client component: `src/components/teacher/create-class-form.tsx`

#### 3.5 Teacher Classes List (`src/app/(portal)/teacher/classes/page.tsx`)

**Before:**
```typescript
const classes = await listClassesByUser(db, session!.user.id);
```

**After:**
```typescript
const classes = await api<ClassListItem[]>("/api/classes?role=instructor");
```

#### 3.6 Teacher Class Detail (`src/app/(portal)/teacher/classes/[id]/page.tsx`)

**Before:**
```typescript
const cls = await getClass(db, id);
const members = await listClassMembers(db, id);
```

**After:**
```typescript
const data = await api<ClassDetailData>(`/api/classes/${id}`);
// data.class, data.members
```

#### 3.7 Teacher Session Dashboard (`src/app/(portal)/teacher/classes/[id]/session/[sessionId]/dashboard/page.tsx`)

**Before:**
```typescript
const cls = await getClass(db, classId);
const members = await listClassMembers(db, classId);
const liveSession = await getSession(db, sessionId);
const classroom = await getClassroom(db, classId);
const course = await getCourse(db, cls.courseId);
const courseTopics = course ? await listTopicsByCourse(db, course.id) : [];
```

**After:**
```typescript
const data = await api<SessionDashboardData>(
  `/api/classes/${classId}/sessions/${sessionId}/dashboard`
);
// data.classId, data.classroomId, data.editorMode, data.courseTopics
```

#### 3.8 Topic Editor (`src/app/(portal)/teacher/courses/[id]/topics/[topicId]/page.tsx`)

Already a client component using `fetch(/api/...)`. **No changes needed** — these fetches go through the Next.js proxy to Go (or directly if `NEXT_PUBLIC_GO_API_URL` is set).

**Tests to write:** `tests/unit/teacher-pages.test.ts`

- [ ] Test teacher dashboard renders with mocked API data
- [ ] Test teacher courses page renders course list from API
- [ ] Test CreateCourseForm submits POST to /api/courses
- [ ] Test AddTopicForm submits POST to /api/courses/{id}/topics
- [ ] Test DeleteTopicButton submits DELETE to /api/courses/{id}/topics/{topicId}
- [ ] Test CreateClassForm submits POST to /api/classes

---

### Task 4: Convert portal pages — Student portal

#### 4.1 Student Dashboard (`src/app/(portal)/student/page.tsx`)

**Before:**
```typescript
const classes = await listClassesByUser(db, session!.user.id);
```

**After:**
```typescript
const classes = await api<ClassListItem[]>("/api/classes?role=student");
```

#### 4.2 Student Classes List (`src/app/(portal)/student/classes/page.tsx`)

Same pattern as 4.1 — `api("/api/classes?role=student")`.

#### 4.3 Student Class Detail (`src/app/(portal)/student/classes/[id]/page.tsx`)

**Before:**
```typescript
const cls = await getClass(db, id);
const members = await listClassMembers(db, id);
const course = await getCourse(db, cls.courseId);
const topics = course ? await listTopicsByCourse(db, course.id) : [];
```

**After:**
```typescript
const data = await api<StudentClassView>(`/api/classes/${id}/student-view`);
// data.class, data.course, data.topics, data.isEnrolled
```

#### 4.4 Student Session Page (`src/app/(portal)/student/classes/[id]/session/[sessionId]/page.tsx`)

**Before:**
```typescript
const cls = await getClass(db, classId);
const members = await listClassMembers(db, classId);
const liveSession = await getSession(db, sessionId);
await joinSession(db, sessionId, session!.user.id);
const classroom = await getClassroom(db, classId);
```

**After:**
```typescript
// POST joins the session and returns context in one call
const data = await api<StudentSessionContext>(
  `/api/sessions/${sessionId}/join`,
  { method: "POST" }
);
// data.sessionId, data.classId, data.editorMode
```

#### 4.5 Student Code Page (`src/app/(portal)/student/code/page.tsx`)

**Before:**
```typescript
const docs = await listDocuments(db, { ownerId: session!.user.id });
```

**After:**
```typescript
const docs = await api<Document[]>("/api/documents?owner=me");
```

**Tests to write:** `tests/unit/student-pages.test.ts`

- [ ] Test student dashboard renders classes from API
- [ ] Test student class detail renders topics from API
- [ ] Test student session page calls POST /api/sessions/{id}/join
- [ ] Test student code page renders documents from API

---

### Task 5: Convert portal pages — Parent portal

#### 5.1 Parent Dashboard (`src/app/(portal)/parent/page.tsx`)

**Before:**
```typescript
const children = await getLinkedChildren(db, session!.user.id);
const childrenWithStatus = await Promise.all(
  children.map(async (child) => {
    const activeSession = await getActiveSessionForStudent(db, child.userId);
    return { ...child, isLive: !!activeSession, sessionId: activeSession?.sessionId };
  })
);
```

**After:**
```typescript
const children = await api<ChildWithStatus[]>("/api/parent/children");
// Go returns children with live status already computed
```

#### 5.2 Child Detail (`src/app/(portal)/parent/children/[id]/page.tsx`)

**Before:**
```typescript
const children = await getLinkedChildren(db, session!.user.id);
const [child] = await db.select().from(users).where(eq(users.id, id));
const classes = await listClassesByUser(db, id);
const docs = await listDocuments(db, { ownerId: id });
```

**After:**
```typescript
const data = await api<ChildDetailData>(`/api/parent/children/${id}`);
// data.child, data.classes, data.recentCode
```

Note: This page has a direct `db.select().from(users)` — the only page that bypasses the lib layer. The Go endpoint replaces all of it.

#### 5.3 Child Live View (`src/app/(portal)/parent/children/[id]/live/page.tsx`)

**Before:**
```typescript
const children = await getLinkedChildren(db, session!.user.id);
const activeSession = await getActiveSessionForStudent(db, childId);
```

**After:**
```typescript
const data = await api<ChildLiveData>(`/api/parent/children/${id}/live`);
// data.activeSession (null if not live), data.sessionId, data.childId
```

#### 5.4 Child Reports (`src/app/(portal)/parent/children/[id]/reports/page.tsx`)

Already a client component using `fetch(/api/parent/children/${id}/reports)`. **No changes needed.**

**Tests to write:** `tests/unit/parent-pages.test.ts`

- [ ] Test parent dashboard renders children with live status from API
- [ ] Test child detail page renders child info, classes, code from API
- [ ] Test child live view page handles active and inactive session states

---

### Task 6: Convert portal pages — Admin portal

#### 6.1 Admin Dashboard (`src/app/(portal)/admin/page.tsx`)

**Before:**
```typescript
const [pendingOrgs, activeOrgs, totalUsers] = await Promise.all([
  countOrganizations(db, "pending"),
  countOrganizations(db, "active"),
  countUsers(db),
]);
```

**After:**
```typescript
const stats = await api<AdminStats>("/api/admin/stats");
// stats.pendingOrgs, stats.activeOrgs, stats.totalUsers
```

#### 6.2 Admin Orgs (`src/app/(portal)/admin/orgs/page.tsx`)

Data loading + two server actions (`approveOrg`, `suspendOrg`).

**Before (data):**
```typescript
const orgs = await listOrganizations(db, status);
```

**After (data):**
```typescript
const orgs = await api<Org[]>(`/api/admin/orgs${status ? `?status=${status}` : ""}`);
```

**Before (server actions):**
```typescript
async function approveOrg(formData: FormData) {
  "use server";
  await updateOrgStatus(db, orgId, "active");
  revalidatePath("/admin/orgs");
}
```

**After:** Extract to client component `src/components/admin/org-actions.tsx`:
```typescript
"use client";

export function OrgActions({ org }: { org: Org }) {
  async function handleApprove() {
    await fetch(`/api/admin/orgs/${org.id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: "active" }),
    });
    window.location.reload();
  }
  // ... render approve/suspend buttons ...
}
```

#### 6.3 Admin Users (`src/app/(portal)/admin/users/page.tsx`)

**Before:**
```typescript
const userList = await listUsers(db);
```

**After:**
```typescript
const userList = await api<User[]>("/api/admin/users");
```

**Tests to write:** `tests/unit/admin-pages.test.ts`

- [ ] Test admin dashboard renders stats from API
- [ ] Test admin orgs page renders org list from API
- [ ] Test OrgActions component sends PATCH to /api/admin/orgs/{id}
- [ ] Test admin users page renders user list from API

---

### Task 7: Convert portal pages — Org Admin portal

#### 7.1 Org Dashboard (`src/app/(portal)/org/page.tsx`)

**Before:**
```typescript
const memberships = await getUserMemberships(db, session!.user.id);
const orgAdminMembership = memberships.find(/*...*/);
const org = await getOrganization(db, orgAdminMembership.orgId);
const [members, courses, classes] = await Promise.all([
  listOrgMembers(db, org.id),
  listCoursesByOrg(db, org.id),
  listClassesByOrg(db, org.id),
]);
```

**After:**
```typescript
const data = await api<OrgDashboardData>("/api/org/dashboard");
// data.org, data.teacherCount, data.studentCount, data.courseCount, data.classCount
```

The Go endpoint determines the user's org_admin membership from the JWT, so the client doesn't need to resolve it.

**Tests to write:** `tests/unit/org-pages.test.ts`

- [ ] Test org dashboard renders stats from API

---

### Task 8: Convert root page and portal shell

#### 8.1 Root Page (`src/app/page.tsx`)

**Before:**
```typescript
import { getEffectiveSession } from "@/lib/impersonate";
import { db } from "@/lib/db";
import { getUserMemberships } from "@/lib/org-memberships";
import { buildUserRoles, getPrimaryPortalPath } from "@/lib/portal/roles";

const session = await getEffectiveSession();
if (session?.user?.id) {
  const memberships = await getUserMemberships(db, session.user.id);
  const roles = buildUserRoles(session.user.isPlatformAdmin, memberships);
  const path = getPrimaryPortalPath(roles);
  redirect(path);
}
```

**After:**
```typescript
import { api, ApiError } from "@/lib/api-client";
import { redirect } from "next/navigation";

interface MeRoles {
  authenticated: boolean;
  primaryPortalPath: string;
}

export default async function Home() {
  try {
    const me = await api<MeRoles>("/api/me/roles");
    if (me.authenticated) {
      redirect(me.primaryPortalPath);
    }
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) {
      // Not authenticated — show landing page
    } else {
      throw e;
    }
  }

  return (
    <main>/* ... landing page unchanged ... */</main>
  );
}
```

Remove imports: `getEffectiveSession`, `db`, `getUserMemberships`, `buildUserRoles`, `getPrimaryPortalPath`.

#### 8.2 Portal Shell (`src/components/portal/portal-shell.tsx`)

**Before:**
```typescript
import { getEffectiveSession } from "@/lib/impersonate";
import { db } from "@/lib/db";
import { getUserMemberships } from "@/lib/org-memberships";
import { buildUserRoles, isAuthorizedForPortal } from "@/lib/portal/roles";

const session = await getEffectiveSession();
const memberships = await getUserMemberships(db, session.user.id);
const roles = buildUserRoles(session.user.isPlatformAdmin, memberships);
if (!isAuthorizedForPortal(roles, portalRole)) redirect("/");
```

**After:**
```typescript
import { api, ApiError } from "@/lib/api-client";
import { redirect } from "next/navigation";

interface PortalAccess {
  authorized: boolean;
  userName: string;
  roles: UserRole[];
  currentRole: PortalRole;
}

const access = await api<PortalAccess>(
  `/api/me/portal-access?role=${portalRole}`
);

if (!access.authorized) redirect("/");
```

Remove imports: `getEffectiveSession`, `db`, `getUserMemberships`, `buildUserRoles`, `isAuthorizedForPortal`.

**Tests to write:** `tests/unit/portal-shell.test.ts`

- [ ] Test portal shell redirects to /login when API returns 401
- [ ] Test portal shell redirects to / when not authorized for role
- [ ] Test portal shell renders sidebar when authorized
- [ ] Test root page redirects to primary portal path for authenticated users
- [ ] Test root page shows landing page for unauthenticated users

---

### Task 9: New Go API endpoints for frontend

These endpoints are required by the converted frontend but may not exist from prior migration phases (which focused on migrating existing API routes, not adding new aggregation endpoints).

| Endpoint | Method | Response | Used by |
|----------|--------|----------|---------|
| `/api/me/roles` | GET | `{ authenticated, primaryPortalPath }` | Root page |
| `/api/me/portal-access` | GET | `{ authorized, userName, roles[], currentRole }` | Portal shell |
| `/api/me/memberships` | GET | `{ memberships[] }` | Teacher courses page (org list for create form) |
| `/api/teacher/dashboard` | GET | `{ courses[], classes[] }` | Teacher dashboard |
| `/api/teacher/courses` | GET | `{ courses[], teacherOrgs[] }` | Teacher courses list |
| `/api/org/dashboard` | GET | `{ org, teacherCount, studentCount, courseCount, classCount }` | Org dashboard |
| `/api/admin/stats` | GET | `{ pendingOrgs, activeOrgs, totalUsers }` | Admin dashboard |
| `/api/classes/{id}/student-view` | GET | `{ class, course, topics[], isEnrolled }` | Student class detail |
| `/api/classes/{id}/sessions/{sid}/dashboard` | GET | `{ classId, classroomId, editorMode, courseTopics[] }` | Teacher session dashboard |
| `/api/auth/credentials` | POST | `{ id, name, email }` or 401 | Auth.js credentials provider |
| `/api/auth/oauth-callback` | POST | `{ id, name, email }` | Auth.js signIn callback |
| `/api/auth/enrich-token` | GET | `{ id, isPlatformAdmin }` | Auth.js jwt callback |

Existing Go endpoints that the frontend already calls via client-side fetch (migrated in prior phases):
- `GET /api/courses/{id}`, `POST /api/courses`, `GET /api/courses/{id}/topics`, `POST /api/courses/{id}/topics`, etc.
- `GET /api/classes/{id}`, `POST /api/classes`, `GET /api/classes/{id}/members`, etc.
- `GET /api/documents`, `GET /api/parent/children/{id}/reports`, etc.
- `PATCH /api/admin/orgs/{id}` — used by new OrgActions client component

**Implementation:** Add these handlers in `gobackend/internal/handlers/`. They are lightweight aggregation endpoints that compose existing store queries.

**Tests to write:** Go-side unit tests in `gobackend/tests/unit/` for each new handler.

---

### Task 10: Delete backend modules

Execute deletions in this order to catch any missed references:

#### 10.1 Delete API routes (except auth)

```bash
# Delete all API route directories except auth
rm -rf src/app/api/classrooms/
rm -rf src/app/api/sessions/
rm -rf src/app/api/annotations/
rm -rf src/app/api/ai/
rm -rf src/app/api/admin/orgs/
rm -rf src/app/api/admin/impersonate/
rm -rf src/app/api/orgs/
rm -rf src/app/api/classes/
rm -rf src/app/api/courses/
rm -rf src/app/api/documents/
rm -rf src/app/api/submissions/
rm -rf src/app/api/assignments/
rm -rf src/app/api/parent/

# Keep: src/app/api/auth/[...nextauth]/route.ts
# Delete: src/app/api/auth/register/route.ts (registration moves to Go)
rm src/app/api/auth/register/route.ts
```

#### 10.2 Delete backend lib modules

```bash
rm src/lib/annotations.ts
rm src/lib/sessions.ts
rm src/lib/sse.ts
rm src/lib/classrooms.ts
rm src/lib/org-memberships.ts
rm src/lib/topics.ts
rm src/lib/class-memberships.ts
rm src/lib/documents.ts
rm src/lib/parent-links.ts
rm src/lib/organizations.ts
rm src/lib/users.ts
rm src/lib/classes.ts
rm src/lib/courses.ts
rm src/lib/assignments.ts
rm src/lib/submissions.ts
rm src/lib/session-topics.ts
rm src/lib/parent-reports.ts
rm src/lib/attendance.ts
rm src/lib/impersonate.ts
```

#### 10.3 Delete AI modules

```bash
rm -rf src/lib/ai/
```

#### 10.4 Delete database layer

```bash
rm -rf src/lib/db/
```

#### 10.5 Delete backend tests

```bash
# Delete all test files for deleted modules
rm tests/unit/annotations.test.ts
rm tests/unit/organizations.test.ts
rm tests/unit/org-memberships.test.ts
rm tests/unit/classes.test.ts
rm tests/unit/courses.test.ts
rm tests/unit/topics.test.ts
rm tests/unit/documents.test.ts
rm tests/unit/users.test.ts
rm tests/unit/parent-links.test.ts
rm tests/unit/submissions.test.ts
rm tests/unit/assignments.test.ts
rm tests/unit/session-topics.test.ts
rm tests/unit/attendance.test.ts
rm tests/unit/schema.test.ts
rm tests/unit/guardrails.test.ts
rm tests/unit/system-prompts.test.ts
rm tests/unit/providers.test.ts
rm tests/unit/sse.test.ts
rm tests/llm/providers.test.ts
rm tests/llm/guardrails.test.ts
rm tests/api/sessions.test.ts
rm tests/api/db-connection.test.ts
rm tests/api/classrooms-join.test.ts
rm tests/api/classrooms.test.ts
rm tests/api-helpers.ts
rm -rf tests/integration/
```

#### 10.6 Clean up test helpers

**File:** `tests/helpers.ts` — review and remove any references to `db`, schema, or deleted modules. Keep generic test utilities.

**File:** `tests/setup.ts` — remove any DB connection setup.

---

### Task 11: Remove backend dependencies from package.json

**File:** `package.json`

Remove from `dependencies`:
```
"@anthropic-ai/sdk": "^0.88.0"
"@auth/drizzle-adapter": "^1.11.1"
"bcryptjs": "^3.0.3"
"drizzle-orm": "^0.45.2"
"openai": "^6.34.0"
"postgres": "^3.4.9"
```

Remove from `devDependencies`:
```
"@types/bcryptjs": "^3.0.0"
"drizzle-kit": "^0.31.10"
```

Remove from `scripts`:
```
"db:generate": "drizzle-kit generate"
"db:migrate": "drizzle-kit migrate"
"db:studio": "drizzle-kit studio"
```

After edits, run `npm install` to update `package-lock.json`.

Also delete `drizzle.config.ts` if it exists:
```bash
rm -f drizzle.config.ts
```

---

### Task 12: Update configuration files

#### 12.1 Update `.env.example`

Add:
```
# Go Backend API
GO_API_URL=http://localhost:8001
```

Remove (moved to Go):
```
# LLM Backend Configuration
LLM_BACKEND=anthropic
# LLM_MODEL=...
# LLM_BASE_URL=...
ANTHROPIC_API_KEY=
# OPENAI_API_KEY=...
# etc.
```

Keep:
```
DATABASE_URL=...          # Still needed by Hocuspocus
NEXTAUTH_URL=...
NEXTAUTH_SECRET=...
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
REDIS_URL=...
NEXT_PUBLIC_HOCUSPOCUS_URL=...
```

#### 12.2 Update `next.config.ts` (or `next.config.js`)

Add proxy rewrite for `/api/*` (except `/api/auth/*`) to Go backend during development:

```typescript
async rewrites() {
  return [
    {
      source: "/api/:path((?!auth).*)",
      destination: `${process.env.GO_API_URL || "http://localhost:8001"}/api/:path*`,
    },
  ];
},
```

This ensures client-side `fetch("/api/courses")` reaches Go without needing `NEXT_PUBLIC_GO_API_URL`. Auth routes stay handled by Next.js.

---

### Task 13: Update Playwright E2E tests

**Files:** `e2e/*.spec.ts`

The E2E tests interact with the browser and should not need major changes since the UI stays the same. However:

1. **Verify all tests still pass** — run the full suite against the new architecture (Next.js + Go)
2. **Update any test that directly hits `/api/*`** — if any E2E test does direct API calls (e.g., setting up test data via `fetch("/api/...")`), those still work because the proxy forwards to Go
3. **Update `e2e/playwright.config.ts`** — may need to add `GO_API_URL` to the env, and ensure Go backend is running as a dependency alongside the Next.js dev server
4. **Update `e2e/auth.setup.ts`** — if it creates test users via API, verify the Go endpoints match the old request format

**Tests to write:** No new E2E tests. Run existing suite and fix any failures.

- [ ] Run `npx playwright test` — all existing tests pass
- [ ] Fix any test that breaks due to response shape changes from Go vs Next.js

---

### Task 14: Final verification

- [ ] `npm run build` — Next.js builds without errors (no dead imports)
- [ ] `npm run test` — all remaining Vitest tests pass
- [ ] `npm run test:e2e` — all Playwright tests pass
- [ ] `npm run lint` — no lint errors
- [ ] Verify no file in `src/` imports from `@/lib/db`, `drizzle-orm`, `postgres`, `bcryptjs`, `@anthropic-ai/sdk`, or `openai`
- [ ] Verify no file in `src/` (outside `src/app/api/auth/`) uses `"use server"` blocks
- [ ] Verify `src/app/api/` only contains `auth/[...nextauth]/route.ts`
- [ ] Manual smoke test: login, navigate each portal, create course, add topic, create class, join as student

---

## Execution Order

1. **Task 9** — Implement new Go endpoints (prerequisite for everything else)
2. **Task 1** — Create `api-client.ts`
3. **Task 2** — Modify Auth.js (remove DB writes)
4. **Tasks 3-7** — Convert portal pages (can be done in parallel across portals)
5. **Task 8** — Convert root page and portal shell
6. **Task 10** — Delete backend modules (only after all pages are converted)
7. **Task 11** — Remove dependencies from package.json
8. **Task 12** — Update configuration
9. **Task 13** — Verify E2E tests
10. **Task 14** — Final verification

---

## New client components created

| File | Purpose | Replaces |
|------|---------|----------|
| `src/components/teacher/create-course-form.tsx` | Course creation form | Server action in `teacher/courses/page.tsx` |
| `src/components/teacher/add-topic-form.tsx` | Topic creation form | Server action in `teacher/courses/[id]/page.tsx` |
| `src/components/teacher/delete-topic-button.tsx` | Topic deletion button | Server action in `teacher/courses/[id]/page.tsx` |
| `src/components/teacher/create-class-form.tsx` | Class creation form | Server action in `teacher/courses/[id]/create-class/page.tsx` |
| `src/components/admin/org-actions.tsx` | Approve/suspend org buttons | Server actions in `admin/orgs/page.tsx` |

---

## Risk mitigation

- **Gradual rollout:** Convert one portal at a time, verify E2E tests pass after each portal is done.
- **Proxy fallback:** The Next.js rewrite proxy means client-side fetches work regardless of whether Go or Next.js handles them. Remove the proxy only after all routes are verified.
- **Auth.js is the riskiest change:** Test login flows (Google OAuth and credentials) thoroughly after Task 2. If Auth.js changes break login, all other work is blocked. Consider doing Task 2 last or having a rollback plan.
- **Impersonation:** The `bridge-impersonate` cookie is now forwarded as an `X-Bridge-Impersonate` header to Go. Go must read this header and apply the same override logic. Verify impersonation E2E test passes.

---

## Stage 1 — Post-Execution Report

**Branch:** `feat/021a-api-client-endpoints`
**PR:** #30
**Executed:** 2026-04-14

### What was done

- 7 new Go aggregation endpoints: /api/me/roles, /api/me/portal-access, /api/me/memberships, /api/teacher/dashboard, /api/teacher/courses, /api/org/dashboard, /api/admin/stats
- StatsStore for admin and org dashboard queries
- OptionalAuth middleware for endpoints that work with or without token
- src/lib/api-client.ts — server-side fetch wrapper for Next.js server components
- Stage 3 (delete backend) deferred — keeping Next.js backend for debugging

### Code Review — Stage 1

#### Review 1

- **Date**: 2026-04-14
- **Reviewer**: Claude (superpowers:code-reviewer)
- **PR**: #30
- **Verdict**: Changes requested (1 critical, 3 important)

**Must Fix**

1. `[FIXED]` No tests for 539 new lines of code.
   → Added 16 tests: me_test.go (12), teacher_test.go (2), stats_test.go (2).

**Should Fix**

2. `[FIXED]` N+1 query in teacher dashboard — per-org ListClassesByOrg loop.
   → Added ListClassesByOrgIDs with IN clause, single batch query.

3. `[WONTFIX]` Teacher endpoints have no role-check middleware.
   → Data is scoped to claims.UserID; students get empty results. Acceptable for now.

4. `[FIXED]` /api/me/roles behind RequireAuth but handles nil claims.
   → Moved to OptionalAuth middleware group. Added OptionalAuth middleware.

**Nice to Have**

5. `[WONTFIX]` Impersonation cookie forwarding uses Cookie header instead of X-Bridge-Impersonate.
   → Works correctly with Go middleware. Documented.

6. `[WONTFIX]` OrgDashboardStats counts only active members vs TypeScript counts all.
   → Active-only is more correct. Acceptable behavioral difference.

---

## Stage 2 Batch 1 — Post-Execution Report

**Branch:** `feat/021b-convert-pages`
**PR:** #32
**Executed:** 2026-04-14

### What was done

- Converted 5 core pages from Drizzle to Go API
- Added ListClassesByUser store method + GET /api/classes/mine endpoint
- Added JWE auth bridge (separate PR #31)

### Code Review — Stage 2 Batch 1

#### Review 1

- **Date**: 2026-04-14
- **Reviewer**: Claude (superpowers:code-reviewer)
- **PR**: #32
- **Verdict**: Changes requested (1 critical, 4 important)

**Must Fix**

1. `[FIXED]` Teacher dashboard showed all org classes instead of teacher's own classes.
   → Changed to use /api/classes/mine filtered by memberRole=instructor.

**Should Fix**

2. `[FIXED]` Root page swallowed all errors including infrastructure failures.
   → Only catch 401; re-throw everything else.

3. `[FIXED]` Portal shell redirected to /login on any error.
   → Only redirect on 401; let infrastructure errors propagate.

4. `[FIXED]` Missing tests for ListClassesByUser and ListMyClasses.
   → Added store integration test + handler auth test.

5. `[FIXED]` Ambiguous column in ListClassesByUser SQL JOIN.
   → Qualified all columns with table name.

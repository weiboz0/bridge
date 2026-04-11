# Real-time Sessions Implementation Plan (Plan 3 of 4)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable real-time collaborative coding sessions where teachers can monitor all students' code live, collaboratively edit with students, and broadcast their own code to the class.

**Architecture:** Yjs provides CRDT-based document sync through a Hocuspocus WebSocket server running as a sidecar process alongside Next.js. Each student editor in a live session maps to a Yjs document keyed by `session:{sessionId}:user:{userId}`. The teacher dashboard subscribes to all student documents via Yjs providers and renders a grid of miniaturized CodeMirror instances. SSE (Server-Sent Events) handles non-document events like student join/leave notifications. LiveSession and SessionParticipant tables track session state in PostgreSQL.

**Tech Stack:** Yjs, @hocuspocus/server, @hocuspocus/provider, y-codemirror.next, Server-Sent Events, Drizzle ORM, CodeMirror 6

**Spec:** `docs/superpowers/specs/001-bridge-platform-design.md`

**Depends on:** Plan 1 (Foundation), Plan 2 (Live Editor)

---

## File Structure

```
src/
├── app/
│   ├── api/
│   │   ├── sessions/
│   │   │   ├── route.ts                               # POST: create session
│   │   │   └── [id]/
│   │   │       ├── route.ts                           # GET session, PATCH end session
│   │   │       ├── join/
│   │   │       │   └── route.ts                       # POST: student joins session
│   │   │       ├── leave/
│   │   │       │   └── route.ts                       # POST: student leaves session
│   │   │       ├── participants/
│   │   │       │   └── route.ts                       # GET: list participants
│   │   │       └── events/
│   │   │           └── route.ts                       # GET: SSE stream
│   │   └── classrooms/
│   │       └── [id]/
│   │           └── active-session/
│   │               └── route.ts                       # GET: active session for classroom
│   └── dashboard/
│       └── classrooms/
│           └── [id]/
│               ├── page.tsx                           # Modified — add Start/Join Session button
│               ├── editor/
│               │   └── page.tsx                       # Modified — accept Yjs binding
│               └── session/
│                   └── [sessionId]/
│                       ├── page.tsx                   # Student session view (editor + Yjs)
│                       └── dashboard/
│                           └── page.tsx               # Teacher live dashboard
├── components/
│   ├── editor/
│   │   └── code-editor.tsx                            # Modified — optional Yjs binding
│   └── session/
│       ├── session-controls.tsx                       # Start/End/Join session buttons
│       ├── student-tile.tsx                           # Single student code tile
│       ├── student-grid.tsx                           # Grid of student tiles
│       ├── participant-list.tsx                       # Sidebar participant list
│       ├── broadcast-banner.tsx                       # "Teacher is broadcasting" banner
│       └── collaborative-editor.tsx                   # Full-screen collab editor view
├── lib/
│   ├── db/
│   │   └── schema.ts                                 # Modified — add LiveSession, SessionParticipant
│   ├── sessions.ts                                   # Session CRUD operations
│   └── sse.ts                                        # SSE utility helpers
└── workers/
    (existing pyodide-worker.ts)

server/
└── hocuspocus.ts                                     # Hocuspocus server script

tests/
├── api/
│   ├── sessions.test.ts                              # Session CRUD tests
│   └── session-participants.test.ts                   # Join/leave/list tests
└── unit/
    ├── sessions-lib.test.ts                           # Session library function tests
    └── sse.test.ts                                    # SSE utility tests
```

---

## Task 1: Install Real-time Dependencies

**Files:**
- Modify: `package.json`

- [ ] **Step 1: Install Yjs and Hocuspocus packages**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun add yjs @hocuspocus/server @hocuspocus/provider y-codemirror.next y-protocols
```

- [ ] **Step 2: Verify installation**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun run build
```

Expected: Build completes without errors.

- [ ] **Step 3: Commit**

```bash
git add package.json bun.lock
git commit -m "chore: install Yjs, Hocuspocus, and real-time sync dependencies"
```

---

## Task 2: LiveSession and SessionParticipant Database Schema

**Files:**
- Modify: `src/lib/db/schema.ts`

- [ ] **Step 1: Add session status and participant status enums plus LiveSession and SessionParticipant tables**

In `src/lib/db/schema.ts`, add the following after the existing `editorModeEnum` definition:

```typescript
export const sessionStatusEnum = pgEnum("session_status", [
  "active",
  "ended",
]);

export const participantStatusEnum = pgEnum("participant_status", [
  "active",
  "idle",
  "needs_help",
]);
```

Then add the following tables after the `classroomMembers` table:

```typescript
export const liveSessions = pgTable(
  "live_sessions",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    classroomId: uuid("classroom_id")
      .notNull()
      .references(() => classrooms.id, { onDelete: "cascade" }),
    teacherId: uuid("teacher_id")
      .notNull()
      .references(() => users.id),
    status: sessionStatusEnum("status").notNull().default("active"),
    settings: jsonb("settings").default({}),
    startedAt: timestamp("started_at").defaultNow().notNull(),
    endedAt: timestamp("ended_at"),
  },
  (table) => [
    index("live_sessions_classroom_idx").on(table.classroomId),
    index("live_sessions_status_idx").on(table.classroomId, table.status),
  ]
);

export const sessionParticipants = pgTable(
  "session_participants",
  {
    sessionId: uuid("session_id")
      .notNull()
      .references(() => liveSessions.id, { onDelete: "cascade" }),
    studentId: uuid("student_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    status: participantStatusEnum("status").notNull().default("active"),
    joinedAt: timestamp("joined_at").defaultNow().notNull(),
    leftAt: timestamp("left_at"),
  },
  (table) => [
    uniqueIndex("session_participant_unique_idx").on(
      table.sessionId,
      table.studentId
    ),
    index("session_participants_session_idx").on(table.sessionId),
  ]
);
```

- [ ] **Step 2: Generate the database migration**

```bash
export PATH="$HOME/.bun/bin:$PATH"
cd /home/chris/workshop/Bridge
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge" bun run db:generate
```

Verify a new migration SQL file appears in `drizzle/` containing `CREATE TABLE "live_sessions"` and `CREATE TABLE "session_participants"`.

- [ ] **Step 3: Run the migration against the dev database**

```bash
export PATH="$HOME/.bun/bin:$PATH"
cd /home/chris/workshop/Bridge
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge" bun run db:migrate
```

- [ ] **Step 4: Run the migration against the test database**

```bash
export PATH="$HOME/.bun/bin:$PATH"
cd /home/chris/workshop/Bridge
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run db:migrate
```

- [ ] **Step 5: Commit**

```bash
git add src/lib/db/schema.ts drizzle/
git commit -m "feat: add LiveSession and SessionParticipant database tables with migration"
```

---

## Task 3: Schema Unit Tests

**Files:**
- Modify: `tests/unit/schema.test.ts`

- [ ] **Step 1: Read the existing schema tests to understand the pattern**

Read `tests/unit/schema.test.ts` and follow the same pattern.

- [ ] **Step 2: Add tests for the new tables**

Append to `tests/unit/schema.test.ts`:

```typescript
import { liveSessions, sessionParticipants, sessionStatusEnum, participantStatusEnum } from "@/lib/db/schema";

describe("liveSessions table", () => {
  it("has expected columns", () => {
    expect(liveSessions.id).toBeDefined();
    expect(liveSessions.classroomId).toBeDefined();
    expect(liveSessions.teacherId).toBeDefined();
    expect(liveSessions.status).toBeDefined();
    expect(liveSessions.settings).toBeDefined();
    expect(liveSessions.startedAt).toBeDefined();
    expect(liveSessions.endedAt).toBeDefined();
  });
});

describe("sessionParticipants table", () => {
  it("has expected columns", () => {
    expect(sessionParticipants.sessionId).toBeDefined();
    expect(sessionParticipants.studentId).toBeDefined();
    expect(sessionParticipants.status).toBeDefined();
    expect(sessionParticipants.joinedAt).toBeDefined();
    expect(sessionParticipants.leftAt).toBeDefined();
  });
});

describe("session enums", () => {
  it("sessionStatusEnum has correct values", () => {
    expect(sessionStatusEnum.enumValues).toEqual(["active", "ended"]);
  });

  it("participantStatusEnum has correct values", () => {
    expect(participantStatusEnum.enumValues).toEqual(["active", "idle", "needs_help"]);
  });
});
```

- [ ] **Step 3: Run the schema tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
cd /home/chris/workshop/Bridge
bun run test tests/unit/schema.test.ts
```

Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add tests/unit/schema.test.ts
git commit -m "test: add schema tests for LiveSession and SessionParticipant tables"
```

---

## Task 4: Session Library Functions

**Files:**
- Create: `src/lib/sessions.ts`
- Modify: `tests/helpers.ts`

- [ ] **Step 1: Update test helpers with session factories**

Add the following to `tests/helpers.ts`. First add the new imports at the top (add `liveSessions` and `sessionParticipants` to the existing schema import):

```typescript
import * as schema from "@/lib/db/schema";
```

Update the `cleanupDatabase` function to delete the new tables (order matters for foreign keys — delete session_participants before live_sessions):

```typescript
export async function cleanupDatabase() {
  await testDb.delete(schema.sessionParticipants);
  await testDb.delete(schema.liveSessions);
  await testDb.delete(schema.classroomMembers);
  await testDb.delete(schema.classrooms);
  await testDb.delete(schema.authProviders);
  await testDb.delete(schema.users);
  await testDb.delete(schema.schools);
}
```

Add the following helper functions at the end of `tests/helpers.ts`:

```typescript
export async function createTestSession(
  classroomId: string,
  teacherId: string,
  overrides: Partial<typeof schema.liveSessions.$inferInsert> = {}
) {
  const [session] = await testDb
    .insert(schema.liveSessions)
    .values({
      classroomId,
      teacherId,
      ...overrides,
    })
    .returning();
  return session;
}

export async function addTestParticipant(
  sessionId: string,
  studentId: string,
  overrides: Partial<typeof schema.sessionParticipants.$inferInsert> = {}
) {
  const [participant] = await testDb
    .insert(schema.sessionParticipants)
    .values({
      sessionId,
      studentId,
      ...overrides,
    })
    .returning();
  return participant;
}
```

- [ ] **Step 2: Create the session library**

Create `src/lib/sessions.ts`:

```typescript
import { eq, and, isNull } from "drizzle-orm";
import { liveSessions, sessionParticipants, users } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

interface CreateSessionInput {
  classroomId: string;
  teacherId: string;
  settings?: Record<string, unknown>;
}

export async function createSession(db: Database, input: CreateSessionInput) {
  // Check for existing active session in this classroom
  const [existing] = await db
    .select()
    .from(liveSessions)
    .where(
      and(
        eq(liveSessions.classroomId, input.classroomId),
        eq(liveSessions.status, "active")
      )
    );

  if (existing) {
    throw new Error("Classroom already has an active session");
  }

  const [session] = await db
    .insert(liveSessions)
    .values({
      classroomId: input.classroomId,
      teacherId: input.teacherId,
      settings: input.settings ?? {},
    })
    .returning();

  return session;
}

export async function getSession(db: Database, sessionId: string) {
  const [session] = await db
    .select()
    .from(liveSessions)
    .where(eq(liveSessions.id, sessionId));
  return session || null;
}

export async function getActiveSession(db: Database, classroomId: string) {
  const [session] = await db
    .select()
    .from(liveSessions)
    .where(
      and(
        eq(liveSessions.classroomId, classroomId),
        eq(liveSessions.status, "active")
      )
    );
  return session || null;
}

export async function endSession(db: Database, sessionId: string) {
  // Mark all active participants as left
  await db
    .update(sessionParticipants)
    .set({ leftAt: new Date(), status: "active" })
    .where(
      and(
        eq(sessionParticipants.sessionId, sessionId),
        isNull(sessionParticipants.leftAt)
      )
    );

  const [session] = await db
    .update(liveSessions)
    .set({ status: "ended", endedAt: new Date() })
    .where(eq(liveSessions.id, sessionId))
    .returning();

  return session;
}

export async function joinSession(
  db: Database,
  sessionId: string,
  studentId: string
) {
  // Check if already participating (and hasn't left)
  const [existing] = await db
    .select()
    .from(sessionParticipants)
    .where(
      and(
        eq(sessionParticipants.sessionId, sessionId),
        eq(sessionParticipants.studentId, studentId),
        isNull(sessionParticipants.leftAt)
      )
    );

  if (existing) {
    return existing;
  }

  const [participant] = await db
    .insert(sessionParticipants)
    .values({ sessionId, studentId })
    .onConflictDoNothing()
    .returning();

  // If onConflictDoNothing returned nothing, the student previously left — update their record
  if (!participant) {
    const [updated] = await db
      .update(sessionParticipants)
      .set({ leftAt: null, status: "active", joinedAt: new Date() })
      .where(
        and(
          eq(sessionParticipants.sessionId, sessionId),
          eq(sessionParticipants.studentId, studentId)
        )
      )
      .returning();
    return updated;
  }

  return participant;
}

export async function leaveSession(
  db: Database,
  sessionId: string,
  studentId: string
) {
  const [participant] = await db
    .update(sessionParticipants)
    .set({ leftAt: new Date() })
    .where(
      and(
        eq(sessionParticipants.sessionId, sessionId),
        eq(sessionParticipants.studentId, studentId)
      )
    )
    .returning();
  return participant || null;
}

export async function getSessionParticipants(
  db: Database,
  sessionId: string
) {
  return db
    .select({
      studentId: sessionParticipants.studentId,
      status: sessionParticipants.status,
      joinedAt: sessionParticipants.joinedAt,
      leftAt: sessionParticipants.leftAt,
      name: users.name,
      email: users.email,
    })
    .from(sessionParticipants)
    .innerJoin(users, eq(sessionParticipants.studentId, users.id))
    .where(
      and(
        eq(sessionParticipants.sessionId, sessionId),
        isNull(sessionParticipants.leftAt)
      )
    );
}

export async function updateParticipantStatus(
  db: Database,
  sessionId: string,
  studentId: string,
  status: "active" | "idle" | "needs_help"
) {
  const [participant] = await db
    .update(sessionParticipants)
    .set({ status })
    .where(
      and(
        eq(sessionParticipants.sessionId, sessionId),
        eq(sessionParticipants.studentId, studentId)
      )
    )
    .returning();
  return participant || null;
}
```

- [ ] **Step 3: Commit**

```bash
git add src/lib/sessions.ts tests/helpers.ts
git commit -m "feat: add session library functions and test helpers"
```

---

## Task 5: Session Library Tests

**Files:**
- Create: `tests/api/sessions.test.ts`

- [ ] **Step 1: Write tests for all session library functions**

Create `tests/api/sessions.test.ts`:

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  createTestUser,
  createTestClassroom,
  createTestSession,
  addTestParticipant,
} from "../helpers";
import {
  createSession,
  getSession,
  getActiveSession,
  endSession,
  joinSession,
  leaveSession,
  getSessionParticipants,
  updateParticipantStatus,
} from "@/lib/sessions";

describe("session operations", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student1: Awaited<ReturnType<typeof createTestUser>>;
  let student2: Awaited<ReturnType<typeof createTestUser>>;
  let classroom: Awaited<ReturnType<typeof createTestClassroom>>;

  beforeEach(async () => {
    teacher = await createTestUser({ role: "teacher", email: "teacher@school.edu" });
    student1 = await createTestUser({ role: "student", email: "student1@school.edu" });
    student2 = await createTestUser({ role: "student", email: "student2@school.edu" });
    classroom = await createTestClassroom(teacher.id);
  });

  describe("createSession", () => {
    it("creates an active session for a classroom", async () => {
      const session = await createSession(testDb, {
        classroomId: classroom.id,
        teacherId: teacher.id,
      });

      expect(session.id).toBeDefined();
      expect(session.classroomId).toBe(classroom.id);
      expect(session.teacherId).toBe(teacher.id);
      expect(session.status).toBe("active");
      expect(session.startedAt).toBeInstanceOf(Date);
      expect(session.endedAt).toBeNull();
    });

    it("creates a session with custom settings", async () => {
      const session = await createSession(testDb, {
        classroomId: classroom.id,
        teacherId: teacher.id,
        settings: { aiEnabled: true, editorMode: "python" },
      });

      expect(session.settings).toEqual({ aiEnabled: true, editorMode: "python" });
    });

    it("rejects creating a second active session in the same classroom", async () => {
      await createSession(testDb, {
        classroomId: classroom.id,
        teacherId: teacher.id,
      });

      await expect(
        createSession(testDb, {
          classroomId: classroom.id,
          teacherId: teacher.id,
        })
      ).rejects.toThrow("Classroom already has an active session");
    });

    it("allows creating a new session after the previous one ended", async () => {
      const session1 = await createSession(testDb, {
        classroomId: classroom.id,
        teacherId: teacher.id,
      });
      await endSession(testDb, session1.id);

      const session2 = await createSession(testDb, {
        classroomId: classroom.id,
        teacherId: teacher.id,
      });

      expect(session2.id).not.toBe(session1.id);
      expect(session2.status).toBe("active");
    });
  });

  describe("getSession", () => {
    it("returns a session by ID", async () => {
      const created = await createTestSession(classroom.id, teacher.id);
      const found = await getSession(testDb, created.id);

      expect(found).not.toBeNull();
      expect(found!.id).toBe(created.id);
    });

    it("returns null for non-existent session", async () => {
      const found = await getSession(testDb, "00000000-0000-0000-0000-000000000000");
      expect(found).toBeNull();
    });
  });

  describe("getActiveSession", () => {
    it("returns the active session for a classroom", async () => {
      const created = await createTestSession(classroom.id, teacher.id);
      const active = await getActiveSession(testDb, classroom.id);

      expect(active).not.toBeNull();
      expect(active!.id).toBe(created.id);
    });

    it("returns null when no active session exists", async () => {
      const active = await getActiveSession(testDb, classroom.id);
      expect(active).toBeNull();
    });

    it("returns null when only ended sessions exist", async () => {
      const session = await createTestSession(classroom.id, teacher.id);
      await endSession(testDb, session.id);

      const active = await getActiveSession(testDb, classroom.id);
      expect(active).toBeNull();
    });
  });

  describe("endSession", () => {
    it("marks the session as ended with a timestamp", async () => {
      const session = await createTestSession(classroom.id, teacher.id);
      const ended = await endSession(testDb, session.id);

      expect(ended.status).toBe("ended");
      expect(ended.endedAt).toBeInstanceOf(Date);
    });

    it("marks all active participants as left when session ends", async () => {
      const session = await createTestSession(classroom.id, teacher.id);
      await addTestParticipant(session.id, student1.id);
      await addTestParticipant(session.id, student2.id);

      await endSession(testDb, session.id);

      const participants = await getSessionParticipants(testDb, session.id);
      // All participants should have leftAt set, so active list is empty
      expect(participants).toHaveLength(0);
    });
  });

  describe("joinSession", () => {
    it("adds a student as a participant", async () => {
      const session = await createTestSession(classroom.id, teacher.id);
      const participant = await joinSession(testDb, session.id, student1.id);

      expect(participant.sessionId).toBe(session.id);
      expect(participant.studentId).toBe(student1.id);
      expect(participant.status).toBe("active");
      expect(participant.leftAt).toBeNull();
    });

    it("returns existing participant if already joined", async () => {
      const session = await createTestSession(classroom.id, teacher.id);
      const first = await joinSession(testDb, session.id, student1.id);
      const second = await joinSession(testDb, session.id, student1.id);

      expect(second.studentId).toBe(first.studentId);
    });

    it("allows rejoining after leaving", async () => {
      const session = await createTestSession(classroom.id, teacher.id);
      await joinSession(testDb, session.id, student1.id);
      await leaveSession(testDb, session.id, student1.id);

      const rejoined = await joinSession(testDb, session.id, student1.id);
      expect(rejoined.leftAt).toBeNull();
      expect(rejoined.status).toBe("active");
    });
  });

  describe("leaveSession", () => {
    it("marks a participant as left with a timestamp", async () => {
      const session = await createTestSession(classroom.id, teacher.id);
      await addTestParticipant(session.id, student1.id);

      const left = await leaveSession(testDb, session.id, student1.id);
      expect(left).not.toBeNull();
      expect(left!.leftAt).toBeInstanceOf(Date);
    });

    it("returns null for a non-participant", async () => {
      const session = await createTestSession(classroom.id, teacher.id);
      const result = await leaveSession(testDb, session.id, student1.id);
      expect(result).toBeNull();
    });
  });

  describe("getSessionParticipants", () => {
    it("returns active participants with user info", async () => {
      const session = await createTestSession(classroom.id, teacher.id);
      await addTestParticipant(session.id, student1.id);
      await addTestParticipant(session.id, student2.id);

      const participants = await getSessionParticipants(testDb, session.id);
      expect(participants).toHaveLength(2);
      expect(participants[0].name).toBeDefined();
      expect(participants[0].email).toBeDefined();
    });

    it("excludes participants who have left", async () => {
      const session = await createTestSession(classroom.id, teacher.id);
      await addTestParticipant(session.id, student1.id);
      await addTestParticipant(session.id, student2.id, { leftAt: new Date() });

      const participants = await getSessionParticipants(testDb, session.id);
      expect(participants).toHaveLength(1);
      expect(participants[0].studentId).toBe(student1.id);
    });

    it("returns empty array for session with no participants", async () => {
      const session = await createTestSession(classroom.id, teacher.id);
      const participants = await getSessionParticipants(testDb, session.id);
      expect(participants).toHaveLength(0);
    });
  });

  describe("updateParticipantStatus", () => {
    it("updates status to needs_help", async () => {
      const session = await createTestSession(classroom.id, teacher.id);
      await addTestParticipant(session.id, student1.id);

      const updated = await updateParticipantStatus(
        testDb,
        session.id,
        student1.id,
        "needs_help"
      );

      expect(updated).not.toBeNull();
      expect(updated!.status).toBe("needs_help");
    });

    it("updates status to idle", async () => {
      const session = await createTestSession(classroom.id, teacher.id);
      await addTestParticipant(session.id, student1.id);

      const updated = await updateParticipantStatus(
        testDb,
        session.id,
        student1.id,
        "idle"
      );

      expect(updated).not.toBeNull();
      expect(updated!.status).toBe("idle");
    });

    it("returns null for non-existent participant", async () => {
      const session = await createTestSession(classroom.id, teacher.id);
      const updated = await updateParticipantStatus(
        testDb,
        session.id,
        "00000000-0000-0000-0000-000000000000",
        "needs_help"
      );
      expect(updated).toBeNull();
    });
  });
});
```

- [ ] **Step 2: Run the session tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
cd /home/chris/workshop/Bridge
bun run test tests/api/sessions.test.ts
```

Expected: All tests pass.

- [ ] **Step 3: Commit**

```bash
git add tests/api/sessions.test.ts
git commit -m "test: add comprehensive session library function tests"
```

---

## Task 6: Session API Routes

**Files:**
- Create: `src/app/api/sessions/route.ts`
- Create: `src/app/api/sessions/[id]/route.ts`
- Create: `src/app/api/sessions/[id]/join/route.ts`
- Create: `src/app/api/sessions/[id]/leave/route.ts`
- Create: `src/app/api/sessions/[id]/participants/route.ts`
- Create: `src/app/api/classrooms/[id]/active-session/route.ts`

- [ ] **Step 1: Create POST /api/sessions — start a session**

Create `src/app/api/sessions/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { createSession } from "@/lib/sessions";
import { getClassroom } from "@/lib/classrooms";

const createSchema = z.object({
  classroomId: z.string().uuid(),
  settings: z
    .object({
      aiEnabled: z.boolean().optional(),
      editorMode: z.enum(["blockly", "python", "javascript"]).optional(),
    })
    .optional(),
});

export async function POST(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  if (session.user.role !== "teacher" && session.user.role !== "admin") {
    return NextResponse.json(
      { error: "Only teachers can start sessions" },
      { status: 403 }
    );
  }

  const body = await request.json();
  const parsed = createSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const classroom = await getClassroom(db, parsed.data.classroomId);
  if (!classroom) {
    return NextResponse.json(
      { error: "Classroom not found" },
      { status: 404 }
    );
  }

  if (classroom.teacherId !== session.user.id && session.user.role !== "admin") {
    return NextResponse.json(
      { error: "You are not the teacher of this classroom" },
      { status: 403 }
    );
  }

  try {
    const liveSession = await createSession(db, {
      classroomId: parsed.data.classroomId,
      teacherId: session.user.id,
      settings: parsed.data.settings,
    });
    return NextResponse.json(liveSession, { status: 201 });
  } catch (error) {
    if (error instanceof Error && error.message.includes("already has an active session")) {
      return NextResponse.json(
        { error: error.message },
        { status: 409 }
      );
    }
    throw error;
  }
}
```

- [ ] **Step 2: Create GET/PATCH /api/sessions/[id] — get and end session**

Create `src/app/api/sessions/[id]/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSession, endSession } from "@/lib/sessions";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const liveSession = await getSession(db, id);

  if (!liveSession) {
    return NextResponse.json({ error: "Session not found" }, { status: 404 });
  }

  return NextResponse.json(liveSession);
}

export async function PATCH(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const liveSession = await getSession(db, id);

  if (!liveSession) {
    return NextResponse.json({ error: "Session not found" }, { status: 404 });
  }

  if (liveSession.teacherId !== session.user.id && session.user.role !== "admin") {
    return NextResponse.json(
      { error: "Only the session teacher can end the session" },
      { status: 403 }
    );
  }

  if (liveSession.status === "ended") {
    return NextResponse.json(
      { error: "Session is already ended" },
      { status: 400 }
    );
  }

  const ended = await endSession(db, id);
  return NextResponse.json(ended);
}
```

- [ ] **Step 3: Create POST /api/sessions/[id]/join — student joins session**

Create `src/app/api/sessions/[id]/join/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSession, joinSession } from "@/lib/sessions";

export async function POST(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const liveSession = await getSession(db, id);

  if (!liveSession) {
    return NextResponse.json({ error: "Session not found" }, { status: 404 });
  }

  if (liveSession.status !== "active") {
    return NextResponse.json(
      { error: "Session is not active" },
      { status: 400 }
    );
  }

  const participant = await joinSession(db, id, session.user.id);
  return NextResponse.json(participant);
}
```

- [ ] **Step 4: Create POST /api/sessions/[id]/leave — student leaves session**

Create `src/app/api/sessions/[id]/leave/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSession, leaveSession } from "@/lib/sessions";

export async function POST(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const liveSession = await getSession(db, id);

  if (!liveSession) {
    return NextResponse.json({ error: "Session not found" }, { status: 404 });
  }

  const participant = await leaveSession(db, id, session.user.id);

  if (!participant) {
    return NextResponse.json(
      { error: "Not a participant in this session" },
      { status: 404 }
    );
  }

  return NextResponse.json(participant);
}
```

- [ ] **Step 5: Create GET /api/sessions/[id]/participants — list active participants**

Create `src/app/api/sessions/[id]/participants/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSession, getSessionParticipants } from "@/lib/sessions";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const liveSession = await getSession(db, id);

  if (!liveSession) {
    return NextResponse.json({ error: "Session not found" }, { status: 404 });
  }

  const participants = await getSessionParticipants(db, id);
  return NextResponse.json(participants);
}
```

- [ ] **Step 6: Create GET /api/classrooms/[id]/active-session — get active session for a classroom**

Create `src/app/api/classrooms/[id]/active-session/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getActiveSession } from "@/lib/sessions";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const activeSession = await getActiveSession(db, id);

  if (!activeSession) {
    return NextResponse.json({ session: null });
  }

  return NextResponse.json({ session: activeSession });
}
```

- [ ] **Step 7: Verify build**

```bash
export PATH="$HOME/.bun/bin:$PATH"
cd /home/chris/workshop/Bridge
bun run build
```

Expected: Build completes without errors.

- [ ] **Step 8: Commit**

```bash
git add src/app/api/sessions/ src/app/api/classrooms/\[id\]/active-session/
git commit -m "feat: add session API routes for creating, ending, joining, and leaving sessions"
```

---

## Task 7: SSE Utility and Session Events Endpoint

**Files:**
- Create: `src/lib/sse.ts`
- Create: `src/app/api/sessions/[id]/events/route.ts`
- Create: `tests/unit/sse.test.ts`

- [ ] **Step 1: Create SSE utility module**

Create `src/lib/sse.ts`:

```typescript
export type SessionEvent =
  | { type: "student_joined"; studentId: string; studentName: string }
  | { type: "student_left"; studentId: string; studentName: string }
  | { type: "session_ended" }
  | { type: "participant_status"; studentId: string; status: string }
  | { type: "broadcast_started"; teacherId: string }
  | { type: "broadcast_ended"; teacherId: string };

type Listener = (event: SessionEvent) => void;

class SessionEventBus {
  private listeners = new Map<string, Set<Listener>>();

  subscribe(sessionId: string, listener: Listener): () => void {
    if (!this.listeners.has(sessionId)) {
      this.listeners.set(sessionId, new Set());
    }
    this.listeners.get(sessionId)!.add(listener);

    return () => {
      const set = this.listeners.get(sessionId);
      if (set) {
        set.delete(listener);
        if (set.size === 0) {
          this.listeners.delete(sessionId);
        }
      }
    };
  }

  emit(sessionId: string, event: SessionEvent): void {
    const set = this.listeners.get(sessionId);
    if (set) {
      for (const listener of set) {
        listener(event);
      }
    }
  }

  getListenerCount(sessionId: string): number {
    return this.listeners.get(sessionId)?.size ?? 0;
  }
}

// Singleton — shared across all API routes in the same process
export const sessionEventBus = new SessionEventBus();

export function formatSSE(event: SessionEvent): string {
  return `event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`;
}
```

- [ ] **Step 2: Write SSE utility tests**

Create `tests/unit/sse.test.ts`:

```typescript
import { describe, it, expect, vi } from "vitest";
import { sessionEventBus, formatSSE, type SessionEvent } from "@/lib/sse";

describe("formatSSE", () => {
  it("formats a student_joined event", () => {
    const event: SessionEvent = {
      type: "student_joined",
      studentId: "abc",
      studentName: "Alice",
    };
    const result = formatSSE(event);
    expect(result).toBe(
      `event: student_joined\ndata: ${JSON.stringify(event)}\n\n`
    );
  });

  it("formats a session_ended event", () => {
    const event: SessionEvent = { type: "session_ended" };
    const result = formatSSE(event);
    expect(result).toBe(
      `event: session_ended\ndata: ${JSON.stringify(event)}\n\n`
    );
  });
});

describe("SessionEventBus", () => {
  it("delivers events to subscribers", () => {
    const listener = vi.fn();
    const unsubscribe = sessionEventBus.subscribe("session-1", listener);

    const event: SessionEvent = {
      type: "student_joined",
      studentId: "s1",
      studentName: "Bob",
    };
    sessionEventBus.emit("session-1", event);

    expect(listener).toHaveBeenCalledWith(event);
    unsubscribe();
  });

  it("does not deliver events to other sessions", () => {
    const listener = vi.fn();
    const unsubscribe = sessionEventBus.subscribe("session-1", listener);

    sessionEventBus.emit("session-2", {
      type: "student_joined",
      studentId: "s1",
      studentName: "Bob",
    });

    expect(listener).not.toHaveBeenCalled();
    unsubscribe();
  });

  it("unsubscribe removes the listener", () => {
    const listener = vi.fn();
    const unsubscribe = sessionEventBus.subscribe("session-1", listener);
    unsubscribe();

    sessionEventBus.emit("session-1", {
      type: "student_joined",
      studentId: "s1",
      studentName: "Bob",
    });

    expect(listener).not.toHaveBeenCalled();
  });

  it("supports multiple listeners on the same session", () => {
    const listener1 = vi.fn();
    const listener2 = vi.fn();
    const unsub1 = sessionEventBus.subscribe("session-1", listener1);
    const unsub2 = sessionEventBus.subscribe("session-1", listener2);

    const event: SessionEvent = { type: "session_ended" };
    sessionEventBus.emit("session-1", event);

    expect(listener1).toHaveBeenCalledWith(event);
    expect(listener2).toHaveBeenCalledWith(event);

    unsub1();
    unsub2();
  });

  it("reports listener count correctly", () => {
    expect(sessionEventBus.getListenerCount("session-x")).toBe(0);

    const unsub1 = sessionEventBus.subscribe("session-x", vi.fn());
    expect(sessionEventBus.getListenerCount("session-x")).toBe(1);

    const unsub2 = sessionEventBus.subscribe("session-x", vi.fn());
    expect(sessionEventBus.getListenerCount("session-x")).toBe(2);

    unsub1();
    expect(sessionEventBus.getListenerCount("session-x")).toBe(1);

    unsub2();
    expect(sessionEventBus.getListenerCount("session-x")).toBe(0);
  });
});
```

- [ ] **Step 3: Run SSE tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
cd /home/chris/workshop/Bridge
bun run test tests/unit/sse.test.ts
```

Expected: All tests pass.

- [ ] **Step 4: Create SSE events endpoint**

Create `src/app/api/sessions/[id]/events/route.ts`:

```typescript
import { NextRequest } from "next/server";
import { auth } from "@/lib/auth";
import { getSession } from "@/lib/sessions";
import { db } from "@/lib/db";
import { sessionEventBus, formatSSE } from "@/lib/sse";

export const dynamic = "force-dynamic";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return new Response("Unauthorized", { status: 401 });
  }

  const { id } = await params;
  const liveSession = await getSession(db, id);

  if (!liveSession) {
    return new Response("Session not found", { status: 404 });
  }

  const stream = new ReadableStream({
    start(controller) {
      // Send initial connected event
      controller.enqueue(
        new TextEncoder().encode(
          `event: connected\ndata: ${JSON.stringify({ sessionId: id })}\n\n`
        )
      );

      const unsubscribe = sessionEventBus.subscribe(id, (event) => {
        try {
          controller.enqueue(new TextEncoder().encode(formatSSE(event)));

          // Close the stream if session ended
          if (event.type === "session_ended") {
            unsubscribe();
            controller.close();
          }
        } catch {
          // Stream may have been closed by the client
          unsubscribe();
        }
      });

      // Handle client disconnect
      _request.signal.addEventListener("abort", () => {
        unsubscribe();
        try {
          controller.close();
        } catch {
          // Already closed
        }
      });
    },
  });

  return new Response(stream, {
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache, no-transform",
      Connection: "keep-alive",
    },
  });
}
```

- [ ] **Step 5: Commit**

```bash
git add src/lib/sse.ts src/app/api/sessions/\[id\]/events/ tests/unit/sse.test.ts
git commit -m "feat: add SSE event bus for session events and /events streaming endpoint"
```

---

## Task 8: Hocuspocus Server

**Files:**
- Create: `server/hocuspocus.ts`
- Modify: `package.json` (add script)

- [ ] **Step 1: Create the Hocuspocus server script**

Create `server/hocuspocus.ts`:

```typescript
import { Hocuspocus } from "@hocuspocus/server";

const PORT = parseInt(process.env.HOCUSPOCUS_PORT || "1234", 10);

const server = new Hocuspocus({
  port: PORT,
  address: "0.0.0.0",

  async onAuthenticate({ token, documentName }) {
    // Token format: "userId:role"
    // documentName format: "session:{sessionId}:user:{userId}" or "broadcast:{sessionId}"
    if (!token) {
      throw new Error("Authentication required");
    }

    const [userId, role] = token.split(":");
    if (!userId || !role) {
      throw new Error("Invalid token format");
    }

    // Parse document name to extract session and owner info
    const parts = documentName.split(":");
    if (parts.length < 2) {
      throw new Error("Invalid document name format");
    }

    const docType = parts[0]; // "session" or "broadcast"

    if (docType === "session") {
      // Format: session:{sessionId}:user:{ownerId}
      const ownerId = parts[3];

      // Students can only access their own documents
      // Teachers can access any document (for monitoring/collaboration)
      if (role === "student" && userId !== ownerId) {
        throw new Error("Students can only access their own documents");
      }
    } else if (docType === "broadcast") {
      // Format: broadcast:{sessionId}
      // Anyone in the session can read, but only teachers write (enforced at client level)
    } else {
      throw new Error("Unknown document type");
    }

    return { userId, role };
  },

  async onConnect({ documentName }) {
    console.log(`[Hocuspocus] Client connected to: ${documentName}`);
  },

  async onDisconnect({ documentName }) {
    console.log(`[Hocuspocus] Client disconnected from: ${documentName}`);
  },
});

server.listen().then(() => {
  console.log(`[Hocuspocus] Server running on port ${PORT}`);
});
```

- [ ] **Step 2: Add scripts to package.json**

Add the following scripts to `package.json`:

```json
"hocuspocus": "bun run server/hocuspocus.ts",
"dev:all": "bun run dev & bun run hocuspocus"
```

- [ ] **Step 3: Verify the Hocuspocus server starts**

```bash
export PATH="$HOME/.bun/bin:$PATH"
cd /home/chris/workshop/Bridge
timeout 5 bun run hocuspocus || true
```

Expected: Server starts and prints the port message (will timeout after 5s since it runs forever — that's fine).

- [ ] **Step 4: Commit**

```bash
git add server/hocuspocus.ts package.json
git commit -m "feat: add Hocuspocus WebSocket server for Yjs document sync"
```

---

## Task 9: Yjs-enabled CodeEditor Component

**Files:**
- Modify: `src/components/editor/code-editor.tsx`

- [ ] **Step 1: Update CodeEditor to accept optional Yjs binding**

Replace the entire contents of `src/components/editor/code-editor.tsx`:

```typescript
"use client";

import { useRef, useEffect } from "react";
import {
  EditorView,
  keymap,
  lineNumbers,
  highlightActiveLine,
  highlightActiveLineGutter,
} from "@codemirror/view";
import { EditorState } from "@codemirror/state";
import {
  defaultKeymap,
  indentWithTab,
  history,
  historyKeymap,
} from "@codemirror/commands";
import { python } from "@codemirror/lang-python";
import {
  syntaxHighlighting,
  defaultHighlightStyle,
  bracketMatching,
  indentOnInput,
} from "@codemirror/language";
import { autocompletion, closeBrackets } from "@codemirror/autocomplete";
import type * as Y from "yjs";

interface YjsBinding {
  ytext: Y.Text;
  provider: { awareness: unknown };
  undoManager: Y.UndoManager;
}

interface CodeEditorProps {
  initialCode?: string;
  onChange?: (code: string) => void;
  readOnly?: boolean;
  yjsBinding?: YjsBinding;
  fontSize?: string;
}

export function CodeEditor({
  initialCode = "",
  onChange,
  readOnly = false,
  yjsBinding,
  fontSize = "14px",
}: CodeEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    const extensions = [
      lineNumbers(),
      highlightActiveLine(),
      highlightActiveLineGutter(),
      bracketMatching(),
      closeBrackets(),
      indentOnInput(),
      autocompletion(),
      python(),
      syntaxHighlighting(defaultHighlightStyle),
      EditorView.editable.of(!readOnly),
      EditorView.theme({
        "&": { height: "100%", fontSize },
        ".cm-scroller": {
          overflow: "auto",
          fontFamily: "var(--font-geist-mono), monospace",
        },
        ".cm-content": { minHeight: "200px" },
      }),
    ];

    if (yjsBinding) {
      // When using Yjs, import and configure y-codemirror.next dynamically
      let destroyed = false;
      let view: EditorView | null = null;

      (async () => {
        const { yCollab } = await import("y-codemirror.next");

        if (destroyed) return;

        const yjsExtensions = [
          keymap.of([...defaultKeymap, indentWithTab]),
          yCollab(yjsBinding.ytext, yjsBinding.provider.awareness as any, {
            undoManager: yjsBinding.undoManager,
          }),
        ];

        const state = EditorState.create({
          doc: yjsBinding.ytext.toString(),
          extensions: [...extensions, ...yjsExtensions],
        });

        view = new EditorView({
          state,
          parent: containerRef.current!,
        });

        viewRef.current = view;
      })();

      return () => {
        destroyed = true;
        if (view) {
          view.destroy();
          viewRef.current = null;
        }
      };
    } else {
      // Standard mode — no Yjs
      extensions.push(
        history(),
        keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab])
      );

      if (onChange) {
        extensions.push(
          EditorView.updateListener.of((update) => {
            if (update.docChanged) {
              onChange(update.state.doc.toString());
            }
          })
        );
      }

      const state = EditorState.create({
        doc: initialCode,
        extensions,
      });

      const view = new EditorView({
        state,
        parent: containerRef.current,
      });

      viewRef.current = view;

      return () => {
        view.destroy();
        viewRef.current = null;
      };
    }
  }, []);

  return (
    <div
      ref={containerRef}
      className="border rounded-lg overflow-hidden h-full"
    />
  );
}
```

- [ ] **Step 2: Verify the existing editor page still works (build check)**

```bash
export PATH="$HOME/.bun/bin:$PATH"
cd /home/chris/workshop/Bridge
bun run build
```

Expected: Build succeeds. The existing editor page uses `CodeEditor` without `yjsBinding`, so it should work unchanged.

- [ ] **Step 3: Commit**

```bash
git add src/components/editor/code-editor.tsx
git commit -m "feat: extend CodeEditor to accept optional Yjs document binding for real-time sync"
```

---

## Task 10: Yjs Provider Hook

**Files:**
- Create: `src/lib/yjs/use-yjs-provider.ts`

- [ ] **Step 1: Create the Yjs provider hook**

Create `src/lib/yjs/use-yjs-provider.ts`:

```typescript
"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import * as Y from "yjs";
import { HocuspocusProvider } from "@hocuspocus/provider";

interface UseYjsProviderOptions {
  documentName: string;
  token: string;
  serverUrl?: string;
  userName: string;
  userColor?: string;
}

interface UseYjsProviderReturn {
  doc: Y.Doc | null;
  provider: HocuspocusProvider | null;
  ytext: Y.Text | null;
  undoManager: Y.UndoManager | null;
  connected: boolean;
  synced: boolean;
}

const COLORS = [
  "#FF6B6B", "#4ECDC4", "#45B7D1", "#96CEB4",
  "#FFEAA7", "#DDA0DD", "#98D8C8", "#F7DC6F",
  "#BB8FCE", "#85C1E9", "#F1948A", "#82E0AA",
];

function pickColor(userId: string): string {
  let hash = 0;
  for (let i = 0; i < userId.length; i++) {
    hash = ((hash << 5) - hash + userId.charCodeAt(i)) | 0;
  }
  return COLORS[Math.abs(hash) % COLORS.length];
}

export function useYjsProvider({
  documentName,
  token,
  serverUrl,
  userName,
  userColor,
}: UseYjsProviderOptions): UseYjsProviderReturn {
  const [connected, setConnected] = useState(false);
  const [synced, setSynced] = useState(false);
  const docRef = useRef<Y.Doc | null>(null);
  const providerRef = useRef<HocuspocusProvider | null>(null);
  const ytextRef = useRef<Y.Text | null>(null);
  const undoManagerRef = useRef<Y.UndoManager | null>(null);

  // Use refs for the return values to avoid re-render loops
  const [state, setState] = useState<UseYjsProviderReturn>({
    doc: null,
    provider: null,
    ytext: null,
    undoManager: null,
    connected: false,
    synced: false,
  });

  useEffect(() => {
    const wsUrl = serverUrl || process.env.NEXT_PUBLIC_HOCUSPOCUS_URL || "ws://localhost:1234";

    const doc = new Y.Doc();
    const ytext = doc.getText("codemirror");
    const undoManager = new Y.UndoManager(ytext);

    const provider = new HocuspocusProvider({
      url: wsUrl,
      name: documentName,
      document: doc,
      token,
      onConnect() {
        setConnected(true);
      },
      onDisconnect() {
        setConnected(false);
      },
      onSynced() {
        setSynced(true);
      },
    });

    // Set awareness state with user info
    const color = userColor || pickColor(token.split(":")[0]);
    provider.setAwarenessField("user", {
      name: userName,
      color,
    });

    docRef.current = doc;
    providerRef.current = provider;
    ytextRef.current = ytext;
    undoManagerRef.current = undoManager;

    setState({
      doc,
      provider,
      ytext,
      undoManager,
      connected: false,
      synced: false,
    });

    return () => {
      undoManager.destroy();
      provider.destroy();
      doc.destroy();
      docRef.current = null;
      providerRef.current = null;
      ytextRef.current = null;
      undoManagerRef.current = null;
    };
  }, [documentName, token, serverUrl, userName, userColor]);

  // Sync connected/synced state
  useEffect(() => {
    setState((prev) => ({ ...prev, connected, synced }));
  }, [connected, synced]);

  return state;
}
```

- [ ] **Step 2: Verify build**

```bash
export PATH="$HOME/.bun/bin:$PATH"
cd /home/chris/workshop/Bridge
bun run build
```

- [ ] **Step 3: Commit**

```bash
git add src/lib/yjs/use-yjs-provider.ts
git commit -m "feat: add useYjsProvider hook for connecting to Hocuspocus documents"
```

---

## Task 11: Session Controls Component

**Files:**
- Create: `src/components/session/session-controls.tsx`

- [ ] **Step 1: Create the session controls component**

This component is used on the classroom detail page. It shows "Start Session" for teachers (when no active session) and "Join Session" for students (when a session is active).

Create `src/components/session/session-controls.tsx`:

```typescript
"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";

interface SessionControlsProps {
  classroomId: string;
  isTeacher: boolean;
}

interface SessionData {
  id: string;
  status: string;
  classroomId: string;
  teacherId: string;
}

export function SessionControls({ classroomId, isTeacher }: SessionControlsProps) {
  const router = useRouter();
  const [activeSession, setActiveSession] = useState<SessionData | null>(null);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetchActiveSession();
  }, [classroomId]);

  async function fetchActiveSession() {
    try {
      const res = await fetch(`/api/classrooms/${classroomId}/active-session`);
      if (res.ok) {
        const data = await res.json();
        setActiveSession(data.session);
      }
    } catch {
      // Ignore fetch errors on initial load
    } finally {
      setLoading(false);
    }
  }

  async function handleStartSession() {
    setActionLoading(true);
    setError(null);

    try {
      const res = await fetch("/api/sessions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ classroomId }),
      });

      if (!res.ok) {
        const data = await res.json();
        setError(data.error || "Failed to start session");
        return;
      }

      const session = await res.json();
      router.push(
        `/dashboard/classrooms/${classroomId}/session/${session.id}/dashboard`
      );
    } catch {
      setError("Failed to start session");
    } finally {
      setActionLoading(false);
    }
  }

  async function handleJoinSession() {
    if (!activeSession) return;

    setActionLoading(true);
    setError(null);

    try {
      const res = await fetch(`/api/sessions/${activeSession.id}/join`, {
        method: "POST",
      });

      if (!res.ok) {
        const data = await res.json();
        setError(data.error || "Failed to join session");
        return;
      }

      router.push(
        `/dashboard/classrooms/${classroomId}/session/${activeSession.id}`
      );
    } catch {
      setError("Failed to join session");
    } finally {
      setActionLoading(false);
    }
  }

  async function handleEndSession() {
    if (!activeSession) return;

    setActionLoading(true);
    setError(null);

    try {
      const res = await fetch(`/api/sessions/${activeSession.id}`, {
        method: "PATCH",
      });

      if (!res.ok) {
        const data = await res.json();
        setError(data.error || "Failed to end session");
        return;
      }

      setActiveSession(null);
    } catch {
      setError("Failed to end session");
    } finally {
      setActionLoading(false);
    }
  }

  if (loading) {
    return (
      <div className="text-sm text-muted-foreground">
        Checking session status...
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="flex gap-2 items-center">
        {isTeacher && !activeSession && (
          <Button
            onClick={handleStartSession}
            disabled={actionLoading}
            variant="default"
          >
            {actionLoading ? "Starting..." : "Start Live Session"}
          </Button>
        )}

        {isTeacher && activeSession && (
          <>
            <Button
              onClick={() =>
                router.push(
                  `/dashboard/classrooms/${classroomId}/session/${activeSession.id}/dashboard`
                )
              }
              variant="default"
            >
              Open Dashboard
            </Button>
            <Button
              onClick={handleEndSession}
              disabled={actionLoading}
              variant="destructive"
            >
              {actionLoading ? "Ending..." : "End Session"}
            </Button>
          </>
        )}

        {!isTeacher && activeSession && (
          <Button
            onClick={handleJoinSession}
            disabled={actionLoading}
            variant="default"
          >
            {actionLoading ? "Joining..." : "Join Live Session"}
          </Button>
        )}

        {!isTeacher && !activeSession && (
          <span className="text-sm text-muted-foreground">
            No active session. Wait for your teacher to start one.
          </span>
        )}
      </div>

      {activeSession && (
        <div className="flex items-center gap-2">
          <span className="relative flex h-2 w-2">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"></span>
            <span className="relative inline-flex rounded-full h-2 w-2 bg-green-500"></span>
          </span>
          <span className="text-sm text-green-600 font-medium">
            Live session active
          </span>
        </div>
      )}

      {error && (
        <p className="text-sm text-red-600">{error}</p>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add src/components/session/session-controls.tsx
git commit -m "feat: add SessionControls component for starting, joining, and ending sessions"
```

---

## Task 12: Update Classroom Detail Page

**Files:**
- Modify: `src/app/dashboard/classrooms/[id]/page.tsx`

- [ ] **Step 1: Add the SessionControls to the classroom detail page**

Update `src/app/dashboard/classrooms/[id]/page.tsx` to import and render the SessionControls. Add the import:

```typescript
import { SessionControls } from "@/components/session/session-controls";
```

Replace the existing `<div className="flex gap-2">` block (the one containing the "Open Editor" link) with:

```tsx
      <div className="flex gap-4 items-start">
        <Link
          href={`/dashboard/classrooms/${id}/editor`}
          className={buttonVariants()}
        >
          Open Editor
        </Link>
        <SessionControls classroomId={id} isTeacher={isTeacher} />
      </div>
```

- [ ] **Step 2: Verify build**

```bash
export PATH="$HOME/.bun/bin:$PATH"
cd /home/chris/workshop/Bridge
bun run build
```

- [ ] **Step 3: Commit**

```bash
git add src/app/dashboard/classrooms/\[id\]/page.tsx
git commit -m "feat: add session controls to classroom detail page"
```

---

## Task 13: Student Session Page (Editor with Yjs)

**Files:**
- Create: `src/app/dashboard/classrooms/[id]/session/[sessionId]/page.tsx`

- [ ] **Step 1: Create the student session page**

Create `src/app/dashboard/classrooms/[id]/session/[sessionId]/page.tsx`:

```typescript
"use client";

import { useState, useEffect, use } from "react";
import { CodeEditor } from "@/components/editor/code-editor";
import { OutputPanel } from "@/components/editor/output-panel";
import { RunButton } from "@/components/editor/run-button";
import { usePyodide } from "@/lib/pyodide/use-pyodide";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { BroadcastBanner } from "@/components/session/broadcast-banner";
import { Button } from "@/components/ui/button";

interface SessionPageProps {
  params: Promise<{ id: string; sessionId: string }>;
}

export default function StudentSessionPage({ params }: SessionPageProps) {
  const { id: classroomId, sessionId } = use(params);
  const [user, setUser] = useState<{ id: string; name: string; role: string } | null>(null);
  const [code, setCode] = useState("");
  const [showBroadcast, setShowBroadcast] = useState(false);
  const { ready, running, output, runCode, clearOutput } = usePyodide();

  // Fetch current user info
  useEffect(() => {
    fetch("/api/auth/session")
      .then((res) => res.json())
      .then((data) => {
        if (data?.user) {
          setUser(data.user);
        }
      })
      .catch(() => {});
  }, []);

  const documentName = user
    ? `session:${sessionId}:user:${user.id}`
    : "";

  const token = user ? `${user.id}:${user.role}` : "";

  const {
    ytext,
    provider,
    undoManager,
    connected,
    synced,
  } = useYjsProvider({
    documentName: documentName || "placeholder",
    token: token || "none",
    userName: user?.name || "Student",
  });

  // Subscribe to SSE for session events
  useEffect(() => {
    const eventSource = new EventSource(`/api/sessions/${sessionId}/events`);

    eventSource.addEventListener("session_ended", () => {
      // Redirect back to classroom when session ends
      window.location.href = `/dashboard/classrooms/${classroomId}`;
    });

    eventSource.addEventListener("broadcast_started", () => {
      setShowBroadcast(true);
    });

    eventSource.addEventListener("broadcast_ended", () => {
      setShowBroadcast(false);
    });

    return () => {
      eventSource.close();
    };
  }, [sessionId, classroomId]);

  // Track code changes from Yjs for running
  useEffect(() => {
    if (!ytext) return;

    const observer = () => {
      setCode(ytext.toString());
    };

    ytext.observe(observer);
    setCode(ytext.toString());

    return () => {
      ytext.unobserve(observer);
    };
  }, [ytext]);

  if (!user) {
    return (
      <div className="flex items-center justify-center h-[calc(100vh-3.5rem)]">
        <p className="text-muted-foreground">Loading session...</p>
      </div>
    );
  }

  const yjsBinding =
    ytext && provider && undoManager
      ? { ytext, provider, undoManager }
      : undefined;

  return (
    <div className="flex flex-col h-[calc(100vh-3.5rem)] gap-2 p-0">
      <div className="flex items-center justify-between px-4 pt-2">
        <div className="flex items-center gap-3">
          <h2 className="text-sm font-medium text-muted-foreground">
            Live Session
          </h2>
          <div className="flex items-center gap-1.5">
            <span
              className={`inline-block h-2 w-2 rounded-full ${
                connected ? "bg-green-500" : "bg-red-500"
              }`}
            />
            <span className="text-xs text-muted-foreground">
              {!synced ? "Syncing..." : connected ? "Connected" : "Disconnected"}
            </span>
          </div>
        </div>
        <div className="flex gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={clearOutput}
            disabled={running}
          >
            Clear
          </Button>
          <RunButton
            onRun={() => runCode(code)}
            running={running}
            ready={ready}
          />
        </div>
      </div>

      {showBroadcast && (
        <BroadcastBanner
          sessionId={sessionId}
          userName={user.name}
          token={token}
        />
      )}

      <div className="flex-1 min-h-0 px-4">
        {yjsBinding ? (
          <CodeEditor yjsBinding={yjsBinding} />
        ) : (
          <div className="flex items-center justify-center h-full text-muted-foreground">
            Connecting to session...
          </div>
        )}
      </div>

      <div className="h-[200px] shrink-0 px-4 pb-4">
        <OutputPanel output={output} running={running} />
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add src/app/dashboard/classrooms/\[id\]/session/
git commit -m "feat: add student session page with Yjs-synced editor"
```

---

## Task 14: Student Tile Component

**Files:**
- Create: `src/components/session/student-tile.tsx`

- [ ] **Step 1: Create the student tile component**

This shows a miniaturized read-only view of a student's code. The teacher dashboard will render a grid of these.

Create `src/components/session/student-tile.tsx`:

```typescript
"use client";

import { useEffect, useRef, useState } from "react";
import * as Y from "yjs";
import { HocuspocusProvider } from "@hocuspocus/provider";
import { EditorView, lineNumbers } from "@codemirror/view";
import { EditorState } from "@codemirror/state";
import { python } from "@codemirror/lang-python";
import {
  syntaxHighlighting,
  defaultHighlightStyle,
} from "@codemirror/language";

interface StudentTileProps {
  sessionId: string;
  studentId: string;
  studentName: string;
  status: "active" | "idle" | "needs_help";
  token: string;
  onClick: () => void;
}

export function StudentTile({
  sessionId,
  studentId,
  studentName,
  status,
  token,
  onClick,
}: StudentTileProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  const [lineCount, setLineCount] = useState(0);

  useEffect(() => {
    if (!containerRef.current) return;

    const doc = new Y.Doc();
    const ytext = doc.getText("codemirror");
    const documentName = `session:${sessionId}:user:${studentId}`;

    const provider = new HocuspocusProvider({
      url: process.env.NEXT_PUBLIC_HOCUSPOCUS_URL || "ws://localhost:1234",
      name: documentName,
      document: doc,
      token,
    });

    // Create a read-only miniaturized editor
    const state = EditorState.create({
      doc: "",
      extensions: [
        lineNumbers(),
        python(),
        syntaxHighlighting(defaultHighlightStyle),
        EditorView.editable.of(false),
        EditorView.theme({
          "&": { height: "100%", fontSize: "9px", cursor: "pointer" },
          ".cm-scroller": {
            overflow: "hidden",
            fontFamily: "var(--font-geist-mono), monospace",
          },
          ".cm-gutters": { fontSize: "8px", minWidth: "20px" },
          ".cm-content": { padding: "2px 0" },
          ".cm-line": { padding: "0 2px" },
        }),
      ],
    });

    const view = new EditorView({
      state,
      parent: containerRef.current,
    });

    viewRef.current = view;

    // Sync Yjs text to the read-only editor
    const observer = () => {
      const text = ytext.toString();
      setLineCount(text.split("\n").length);
      view.dispatch({
        changes: {
          from: 0,
          to: view.state.doc.length,
          insert: text,
        },
      });
    };

    ytext.observe(observer);

    // Also handle initial sync
    provider.on("synced", () => {
      observer();
    });

    return () => {
      ytext.unobserve(observer);
      provider.destroy();
      doc.destroy();
      view.destroy();
      viewRef.current = null;
    };
  }, [sessionId, studentId, token]);

  const statusColors = {
    active: "border-green-200 bg-green-50",
    idle: "border-yellow-200 bg-yellow-50",
    needs_help: "border-red-200 bg-red-50 ring-2 ring-red-300",
  };

  const statusLabels = {
    active: "Active",
    idle: "Idle",
    needs_help: "Needs Help",
  };

  const statusDots = {
    active: "bg-green-500",
    idle: "bg-yellow-500",
    needs_help: "bg-red-500",
  };

  return (
    <div
      onClick={onClick}
      className={`border rounded-lg overflow-hidden cursor-pointer hover:shadow-md transition-shadow ${statusColors[status]}`}
    >
      <div className="flex items-center justify-between px-2 py-1 border-b bg-white/80">
        <div className="flex items-center gap-1.5">
          <span className={`inline-block h-2 w-2 rounded-full ${statusDots[status]}`} />
          <span className="text-xs font-medium truncate max-w-[100px]">
            {studentName}
          </span>
        </div>
        <span className="text-[10px] text-muted-foreground">
          {statusLabels[status]}
        </span>
      </div>
      <div ref={containerRef} className="h-[120px]" />
      <div className="px-2 py-0.5 border-t bg-white/80">
        <span className="text-[10px] text-muted-foreground">
          {lineCount} lines
        </span>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add src/components/session/student-tile.tsx
git commit -m "feat: add StudentTile component with miniaturized live code preview"
```

---

## Task 15: Student Grid and Participant List Components

**Files:**
- Create: `src/components/session/student-grid.tsx`
- Create: `src/components/session/participant-list.tsx`

- [ ] **Step 1: Create the student grid component**

Create `src/components/session/student-grid.tsx`:

```typescript
"use client";

import { StudentTile } from "@/components/session/student-tile";

interface Participant {
  studentId: string;
  name: string;
  status: "active" | "idle" | "needs_help";
}

interface StudentGridProps {
  sessionId: string;
  participants: Participant[];
  token: string;
  onStudentClick: (studentId: string, studentName: string) => void;
}

export function StudentGrid({
  sessionId,
  participants,
  token,
  onStudentClick,
}: StudentGridProps) {
  if (participants.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground">
        <p>No students have joined yet. Share the session link with your class.</p>
      </div>
    );
  }

  // Sort: needs_help first, then active, then idle
  const sorted = [...participants].sort((a, b) => {
    const priority = { needs_help: 0, active: 1, idle: 2 };
    return priority[a.status] - priority[b.status];
  });

  return (
    <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-3 p-4">
      {sorted.map((participant) => (
        <StudentTile
          key={participant.studentId}
          sessionId={sessionId}
          studentId={participant.studentId}
          studentName={participant.name}
          status={participant.status}
          token={token}
          onClick={() =>
            onStudentClick(participant.studentId, participant.name)
          }
        />
      ))}
    </div>
  );
}
```

- [ ] **Step 2: Create the participant list component**

Create `src/components/session/participant-list.tsx`:

```typescript
"use client";

interface Participant {
  studentId: string;
  name: string;
  email: string;
  status: "active" | "idle" | "needs_help";
  joinedAt: string;
}

interface ParticipantListProps {
  participants: Participant[];
  onStudentClick: (studentId: string, studentName: string) => void;
}

export function ParticipantList({
  participants,
  onStudentClick,
}: ParticipantListProps) {
  const statusDots = {
    active: "bg-green-500",
    idle: "bg-yellow-500",
    needs_help: "bg-red-500",
  };

  const statusLabels = {
    active: "Active",
    idle: "Idle",
    needs_help: "Needs Help",
  };

  return (
    <div className="border-l bg-muted/30 w-64 flex-shrink-0 overflow-auto">
      <div className="p-3 border-b">
        <h3 className="text-sm font-semibold">
          Students ({participants.length})
        </h3>
      </div>
      <ul className="p-2 space-y-1">
        {participants.map((p) => (
          <li key={p.studentId}>
            <button
              onClick={() => onStudentClick(p.studentId, p.name)}
              className="w-full flex items-center gap-2 px-2 py-1.5 rounded hover:bg-muted text-left"
            >
              <span
                className={`inline-block h-2 w-2 rounded-full flex-shrink-0 ${statusDots[p.status]}`}
              />
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium truncate">{p.name}</p>
                <p className="text-[10px] text-muted-foreground">
                  {statusLabels[p.status]}
                </p>
              </div>
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
```

- [ ] **Step 3: Commit**

```bash
git add src/components/session/student-grid.tsx src/components/session/participant-list.tsx
git commit -m "feat: add StudentGrid and ParticipantList components for teacher dashboard"
```

---

## Task 16: Collaborative Editor Component

**Files:**
- Create: `src/components/session/collaborative-editor.tsx`

- [ ] **Step 1: Create the collaborative editor component**

When a teacher clicks on a student tile, this component opens a full-size collaborative editor view with both cursors visible.

Create `src/components/session/collaborative-editor.tsx`:

```typescript
"use client";

import { useEffect, useState } from "react";
import { CodeEditor } from "@/components/editor/code-editor";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { OutputPanel } from "@/components/editor/output-panel";
import { Button } from "@/components/ui/button";

interface CollaborativeEditorProps {
  sessionId: string;
  studentId: string;
  studentName: string;
  teacherName: string;
  token: string;
  onClose: () => void;
}

export function CollaborativeEditor({
  sessionId,
  studentId,
  studentName,
  teacherName,
  token,
  onClose,
}: CollaborativeEditorProps) {
  const documentName = `session:${sessionId}:user:${studentId}`;

  const { ytext, provider, undoManager, connected, synced } = useYjsProvider({
    documentName,
    token,
    userName: teacherName,
  });

  const yjsBinding =
    ytext && provider && undoManager
      ? { ytext, provider, undoManager }
      : undefined;

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 py-2 border-b bg-muted/30">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="sm" onClick={onClose}>
            Back to Dashboard
          </Button>
          <span className="text-sm font-medium">
            Editing with {studentName}
          </span>
          <div className="flex items-center gap-1.5">
            <span
              className={`inline-block h-2 w-2 rounded-full ${
                connected ? "bg-green-500" : "bg-red-500"
              }`}
            />
            <span className="text-xs text-muted-foreground">
              {!synced ? "Syncing..." : connected ? "Connected" : "Disconnected"}
            </span>
          </div>
        </div>
      </div>

      <div className="flex-1 min-h-0 px-4 py-2">
        {yjsBinding ? (
          <CodeEditor yjsBinding={yjsBinding} />
        ) : (
          <div className="flex items-center justify-center h-full text-muted-foreground">
            Connecting to student&apos;s editor...
          </div>
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add src/components/session/collaborative-editor.tsx
git commit -m "feat: add CollaborativeEditor component for teacher-student pair editing"
```

---

## Task 17: Broadcast Banner Component

**Files:**
- Create: `src/components/session/broadcast-banner.tsx`

- [ ] **Step 1: Create the broadcast banner component**

When the teacher broadcasts their code, students see a read-only CodeMirror view of the teacher's broadcast document above their own editor.

Create `src/components/session/broadcast-banner.tsx`:

```typescript
"use client";

import { useEffect, useRef, useState } from "react";
import * as Y from "yjs";
import { HocuspocusProvider } from "@hocuspocus/provider";
import { EditorView, lineNumbers } from "@codemirror/view";
import { EditorState } from "@codemirror/state";
import { python } from "@codemirror/lang-python";
import {
  syntaxHighlighting,
  defaultHighlightStyle,
} from "@codemirror/language";

interface BroadcastBannerProps {
  sessionId: string;
  userName: string;
  token: string;
}

export function BroadcastBanner({
  sessionId,
  userName,
  token,
}: BroadcastBannerProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [expanded, setExpanded] = useState(true);

  useEffect(() => {
    if (!containerRef.current || !expanded) return;

    const doc = new Y.Doc();
    const ytext = doc.getText("codemirror");
    const documentName = `broadcast:${sessionId}`;

    const provider = new HocuspocusProvider({
      url: process.env.NEXT_PUBLIC_HOCUSPOCUS_URL || "ws://localhost:1234",
      name: documentName,
      document: doc,
      token,
    });

    const state = EditorState.create({
      doc: "",
      extensions: [
        lineNumbers(),
        python(),
        syntaxHighlighting(defaultHighlightStyle),
        EditorView.editable.of(false),
        EditorView.theme({
          "&": { height: "100%", fontSize: "13px" },
          ".cm-scroller": {
            overflow: "auto",
            fontFamily: "var(--font-geist-mono), monospace",
          },
        }),
      ],
    });

    const view = new EditorView({
      state,
      parent: containerRef.current,
    });

    const observer = () => {
      const text = ytext.toString();
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: text },
      });
    };

    ytext.observe(observer);
    provider.on("synced", () => observer());

    return () => {
      ytext.unobserve(observer);
      provider.destroy();
      doc.destroy();
      view.destroy();
    };
  }, [sessionId, token, expanded]);

  return (
    <div className="mx-4 border rounded-lg overflow-hidden border-blue-300 bg-blue-50">
      <div className="flex items-center justify-between px-3 py-1.5 border-b border-blue-200 bg-blue-100">
        <div className="flex items-center gap-2">
          <span className="relative flex h-2 w-2">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-blue-400 opacity-75"></span>
            <span className="relative inline-flex rounded-full h-2 w-2 bg-blue-600"></span>
          </span>
          <span className="text-xs font-medium text-blue-800">
            Teacher is broadcasting live code
          </span>
        </div>
        <button
          onClick={() => setExpanded(!expanded)}
          className="text-xs text-blue-600 hover:text-blue-800"
        >
          {expanded ? "Minimize" : "Expand"}
        </button>
      </div>
      {expanded && (
        <div ref={containerRef} className="h-[200px]" />
      )}
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add src/components/session/broadcast-banner.tsx
git commit -m "feat: add BroadcastBanner component for teacher live code broadcast"
```

---

## Task 18: Teacher Live Dashboard Page

**Files:**
- Create: `src/app/dashboard/classrooms/[id]/session/[sessionId]/dashboard/page.tsx`

- [ ] **Step 1: Create the teacher live dashboard page**

Create `src/app/dashboard/classrooms/[id]/session/[sessionId]/dashboard/page.tsx`:

```typescript
"use client";

import { useState, useEffect, useCallback, use } from "react";
import { useRouter } from "next/navigation";
import { StudentGrid } from "@/components/session/student-grid";
import { ParticipantList } from "@/components/session/participant-list";
import { CollaborativeEditor } from "@/components/session/collaborative-editor";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { CodeEditor } from "@/components/editor/code-editor";
import { Button } from "@/components/ui/button";
import { sessionEventBus } from "@/lib/sse";

interface Participant {
  studentId: string;
  name: string;
  email: string;
  status: "active" | "idle" | "needs_help";
  joinedAt: string;
}

interface DashboardPageProps {
  params: Promise<{ id: string; sessionId: string }>;
}

export default function TeacherDashboardPage({ params }: DashboardPageProps) {
  const { id: classroomId, sessionId } = use(params);
  const router = useRouter();
  const [user, setUser] = useState<{ id: string; name: string; role: string } | null>(null);
  const [participants, setParticipants] = useState<Participant[]>([]);
  const [selectedStudent, setSelectedStudent] = useState<{
    id: string;
    name: string;
  } | null>(null);
  const [broadcasting, setBroadcasting] = useState(false);
  const [loading, setLoading] = useState(true);

  // Fetch user info
  useEffect(() => {
    fetch("/api/auth/session")
      .then((res) => res.json())
      .then((data) => {
        if (data?.user) {
          setUser(data.user);
        }
      })
      .catch(() => {});
  }, []);

  // Fetch participants
  const fetchParticipants = useCallback(async () => {
    try {
      const res = await fetch(`/api/sessions/${sessionId}/participants`);
      if (res.ok) {
        const data = await res.json();
        setParticipants(data);
      }
    } catch {
      // Ignore
    } finally {
      setLoading(false);
    }
  }, [sessionId]);

  useEffect(() => {
    fetchParticipants();
    // Poll every 5 seconds for participant updates
    const interval = setInterval(fetchParticipants, 5000);
    return () => clearInterval(interval);
  }, [fetchParticipants]);

  // Subscribe to SSE for real-time participant events
  useEffect(() => {
    const eventSource = new EventSource(`/api/sessions/${sessionId}/events`);

    eventSource.addEventListener("student_joined", (event) => {
      const data = JSON.parse(event.data);
      // Re-fetch full participant list to get complete data
      fetchParticipants();
    });

    eventSource.addEventListener("student_left", (event) => {
      const data = JSON.parse(event.data);
      setParticipants((prev) =>
        prev.filter((p) => p.studentId !== data.studentId)
      );
    });

    eventSource.addEventListener("participant_status", (event) => {
      const data = JSON.parse(event.data);
      setParticipants((prev) =>
        prev.map((p) =>
          p.studentId === data.studentId
            ? { ...p, status: data.status }
            : p
        )
      );
    });

    return () => {
      eventSource.close();
    };
  }, [sessionId, fetchParticipants]);

  // Broadcast Yjs provider — teacher's broadcast document
  const broadcastDocName = `broadcast:${sessionId}`;
  const token = user ? `${user.id}:${user.role}` : "";

  const {
    ytext: broadcastYtext,
    provider: broadcastProvider,
    undoManager: broadcastUndo,
  } = useYjsProvider({
    documentName: broadcastDocName,
    token: token || "none",
    userName: user?.name || "Teacher",
  });

  async function handleEndSession() {
    try {
      const res = await fetch(`/api/sessions/${sessionId}`, {
        method: "PATCH",
      });
      if (res.ok) {
        router.push(`/dashboard/classrooms/${classroomId}`);
      }
    } catch {
      // Ignore
    }
  }

  function handleToggleBroadcast() {
    const newState = !broadcasting;
    setBroadcasting(newState);

    // Emit SSE event so students know about broadcast
    // This is done via a fetch to a broadcast toggle endpoint
    fetch(`/api/sessions/${sessionId}/events`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        type: newState ? "broadcast_started" : "broadcast_ended",
      }),
    }).catch(() => {});
  }

  function handleStudentClick(studentId: string, studentName: string) {
    setSelectedStudent({ id: studentId, name: studentName });
  }

  if (!user) {
    return (
      <div className="flex items-center justify-center h-[calc(100vh-3.5rem)]">
        <p className="text-muted-foreground">Loading dashboard...</p>
      </div>
    );
  }

  // If a student is selected, show collaborative editor
  if (selectedStudent) {
    return (
      <div className="h-[calc(100vh-3.5rem)]">
        <CollaborativeEditor
          sessionId={sessionId}
          studentId={selectedStudent.id}
          studentName={selectedStudent.name}
          teacherName={user.name}
          token={token}
          onClose={() => setSelectedStudent(null)}
        />
      </div>
    );
  }

  const broadcastBinding =
    broadcastYtext && broadcastProvider && broadcastUndo
      ? { ytext: broadcastYtext, provider: broadcastProvider, undoManager: broadcastUndo }
      : undefined;

  return (
    <div className="flex flex-col h-[calc(100vh-3.5rem)]">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-2 border-b">
        <div className="flex items-center gap-3">
          <h2 className="text-sm font-semibold">Live Dashboard</h2>
          <div className="flex items-center gap-1.5">
            <span className="relative flex h-2 w-2">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"></span>
              <span className="relative inline-flex rounded-full h-2 w-2 bg-green-500"></span>
            </span>
            <span className="text-xs text-muted-foreground">
              {participants.length} student{participants.length !== 1 ? "s" : ""} connected
            </span>
          </div>
        </div>
        <div className="flex gap-2">
          <Button
            variant={broadcasting ? "default" : "outline"}
            size="sm"
            onClick={handleToggleBroadcast}
          >
            {broadcasting ? "Stop Broadcast" : "Broadcast Code"}
          </Button>
          <Button variant="destructive" size="sm" onClick={handleEndSession}>
            End Session
          </Button>
        </div>
      </div>

      {/* Broadcast editor (when broadcasting) */}
      {broadcasting && broadcastBinding && (
        <div className="border-b">
          <div className="px-4 py-1 bg-blue-50 border-b border-blue-200">
            <span className="text-xs font-medium text-blue-800">
              Broadcasting to all students
            </span>
          </div>
          <div className="h-[200px] px-4 py-2">
            <CodeEditor yjsBinding={broadcastBinding} />
          </div>
        </div>
      )}

      {/* Main content: grid + sidebar */}
      <div className="flex flex-1 min-h-0">
        <div className="flex-1 overflow-auto">
          {loading ? (
            <div className="flex items-center justify-center h-full text-muted-foreground">
              Loading participants...
            </div>
          ) : (
            <StudentGrid
              sessionId={sessionId}
              participants={participants}
              token={token}
              onStudentClick={handleStudentClick}
            />
          )}
        </div>
        <ParticipantList
          participants={participants}
          onStudentClick={handleStudentClick}
        />
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Verify build**

```bash
export PATH="$HOME/.bun/bin:$PATH"
cd /home/chris/workshop/Bridge
bun run build
```

- [ ] **Step 3: Commit**

```bash
git add src/app/dashboard/classrooms/\[id\]/session/\[sessionId\]/dashboard/
git commit -m "feat: add teacher live dashboard page with student grid and broadcast"
```

---

## Task 19: Broadcast Event API Endpoint

**Files:**
- Modify: `src/app/api/sessions/[id]/events/route.ts`

- [ ] **Step 1: Add POST handler for emitting broadcast events**

Add the following POST handler to `src/app/api/sessions/[id]/events/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { getSession } from "@/lib/sessions";
import { db } from "@/lib/db";
import { sessionEventBus, formatSSE, type SessionEvent } from "@/lib/sse";

// ... (existing GET handler stays unchanged)

const broadcastSchema = {
  broadcast_started: true,
  broadcast_ended: true,
} as const;

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  if (session.user.role !== "teacher" && session.user.role !== "admin") {
    return NextResponse.json(
      { error: "Only teachers can emit events" },
      { status: 403 }
    );
  }

  const { id } = await params;
  const liveSession = await getSession(db, id);

  if (!liveSession) {
    return NextResponse.json({ error: "Session not found" }, { status: 404 });
  }

  if (liveSession.teacherId !== session.user.id) {
    return NextResponse.json(
      { error: "You are not the teacher of this session" },
      { status: 403 }
    );
  }

  const body = await request.json();

  if (!body.type || !(body.type in broadcastSchema)) {
    return NextResponse.json({ error: "Invalid event type" }, { status: 400 });
  }

  const event: SessionEvent = {
    type: body.type,
    teacherId: session.user.id,
  } as SessionEvent;

  sessionEventBus.emit(id, event);

  return NextResponse.json({ ok: true });
}
```

Note: The full file should have both the existing `GET` handler and this new `POST` handler. When implementing, ensure the existing `GET` function and its imports remain intact. Add `NextResponse` to the imports from `"next/server"` and update the `import` line for `sessionEventBus` to also include `type SessionEvent`. The file should look like:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { getSession } from "@/lib/sessions";
import { db } from "@/lib/db";
import { sessionEventBus, formatSSE, type SessionEvent } from "@/lib/sse";

export const dynamic = "force-dynamic";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return new Response("Unauthorized", { status: 401 });
  }

  const { id } = await params;
  const liveSession = await getSession(db, id);

  if (!liveSession) {
    return new Response("Session not found", { status: 404 });
  }

  const stream = new ReadableStream({
    start(controller) {
      controller.enqueue(
        new TextEncoder().encode(
          `event: connected\ndata: ${JSON.stringify({ sessionId: id })}\n\n`
        )
      );

      const unsubscribe = sessionEventBus.subscribe(id, (event) => {
        try {
          controller.enqueue(new TextEncoder().encode(formatSSE(event)));

          if (event.type === "session_ended") {
            unsubscribe();
            controller.close();
          }
        } catch {
          unsubscribe();
        }
      });

      _request.signal.addEventListener("abort", () => {
        unsubscribe();
        try {
          controller.close();
        } catch {
          // Already closed
        }
      });
    },
  });

  return new Response(stream, {
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache, no-transform",
      Connection: "keep-alive",
    },
  });
}

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  if (session.user.role !== "teacher" && session.user.role !== "admin") {
    return NextResponse.json(
      { error: "Only teachers can emit events" },
      { status: 403 }
    );
  }

  const { id } = await params;
  const liveSession = await getSession(db, id);

  if (!liveSession) {
    return NextResponse.json({ error: "Session not found" }, { status: 404 });
  }

  if (liveSession.teacherId !== session.user.id) {
    return NextResponse.json(
      { error: "You are not the teacher of this session" },
      { status: 403 }
    );
  }

  const body = await request.json();

  if (!body.type || !["broadcast_started", "broadcast_ended"].includes(body.type)) {
    return NextResponse.json({ error: "Invalid event type" }, { status: 400 });
  }

  const event: SessionEvent = {
    type: body.type,
    teacherId: session.user.id,
  } as SessionEvent;

  sessionEventBus.emit(id, event);

  return NextResponse.json({ ok: true });
}
```

- [ ] **Step 2: Verify build**

```bash
export PATH="$HOME/.bun/bin:$PATH"
cd /home/chris/workshop/Bridge
bun run build
```

- [ ] **Step 3: Commit**

```bash
git add src/app/api/sessions/\[id\]/events/route.ts
git commit -m "feat: add POST handler for broadcast events to SSE events endpoint"
```

---

## Task 20: Emit SSE Events from Join/Leave API Routes

**Files:**
- Modify: `src/app/api/sessions/[id]/join/route.ts`
- Modify: `src/app/api/sessions/[id]/leave/route.ts`
- Modify: `src/app/api/sessions/[id]/route.ts`

- [ ] **Step 1: Emit student_joined SSE event from join route**

Update `src/app/api/sessions/[id]/join/route.ts` to emit an SSE event when a student joins. Add the import:

```typescript
import { sessionEventBus } from "@/lib/sse";
import { db as database } from "@/lib/db";
import { users } from "@/lib/db/schema";
import { eq } from "drizzle-orm";
```

After the `joinSession` call and before the `return`, add:

```typescript
  // Fetch student name for the event
  const [studentUser] = await database
    .select({ name: users.name })
    .from(users)
    .where(eq(users.id, session.user.id));

  sessionEventBus.emit(id, {
    type: "student_joined",
    studentId: session.user.id,
    studentName: studentUser?.name || "Unknown",
  });
```

Full file after modification:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSession, joinSession } from "@/lib/sessions";
import { sessionEventBus } from "@/lib/sse";
import { users } from "@/lib/db/schema";
import { eq } from "drizzle-orm";

export async function POST(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const liveSession = await getSession(db, id);

  if (!liveSession) {
    return NextResponse.json({ error: "Session not found" }, { status: 404 });
  }

  if (liveSession.status !== "active") {
    return NextResponse.json(
      { error: "Session is not active" },
      { status: 400 }
    );
  }

  const participant = await joinSession(db, id, session.user.id);

  // Fetch student name for the event
  const [studentUser] = await db
    .select({ name: users.name })
    .from(users)
    .where(eq(users.id, session.user.id));

  sessionEventBus.emit(id, {
    type: "student_joined",
    studentId: session.user.id,
    studentName: studentUser?.name || "Unknown",
  });

  return NextResponse.json(participant);
}
```

- [ ] **Step 2: Emit student_left SSE event from leave route**

Update `src/app/api/sessions/[id]/leave/route.ts` to emit an SSE event when a student leaves.

Full file after modification:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSession, leaveSession } from "@/lib/sessions";
import { sessionEventBus } from "@/lib/sse";
import { users } from "@/lib/db/schema";
import { eq } from "drizzle-orm";

export async function POST(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const liveSession = await getSession(db, id);

  if (!liveSession) {
    return NextResponse.json({ error: "Session not found" }, { status: 404 });
  }

  const participant = await leaveSession(db, id, session.user.id);

  if (!participant) {
    return NextResponse.json(
      { error: "Not a participant in this session" },
      { status: 404 }
    );
  }

  // Fetch student name for the event
  const [studentUser] = await db
    .select({ name: users.name })
    .from(users)
    .where(eq(users.id, session.user.id));

  sessionEventBus.emit(id, {
    type: "student_left",
    studentId: session.user.id,
    studentName: studentUser?.name || "Unknown",
  });

  return NextResponse.json(participant);
}
```

- [ ] **Step 3: Emit session_ended SSE event from end-session route**

Update `src/app/api/sessions/[id]/route.ts`. Add the import:

```typescript
import { sessionEventBus } from "@/lib/sse";
```

In the `PATCH` handler, after the `endSession` call and before the `return`, add:

```typescript
  sessionEventBus.emit(id, { type: "session_ended" });
```

Full PATCH handler in `src/app/api/sessions/[id]/route.ts` after modification:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSession, endSession } from "@/lib/sessions";
import { sessionEventBus } from "@/lib/sse";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const liveSession = await getSession(db, id);

  if (!liveSession) {
    return NextResponse.json({ error: "Session not found" }, { status: 404 });
  }

  return NextResponse.json(liveSession);
}

export async function PATCH(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const liveSession = await getSession(db, id);

  if (!liveSession) {
    return NextResponse.json({ error: "Session not found" }, { status: 404 });
  }

  if (liveSession.teacherId !== session.user.id && session.user.role !== "admin") {
    return NextResponse.json(
      { error: "Only the session teacher can end the session" },
      { status: 403 }
    );
  }

  if (liveSession.status === "ended") {
    return NextResponse.json(
      { error: "Session is already ended" },
      { status: 400 }
    );
  }

  const ended = await endSession(db, id);

  sessionEventBus.emit(id, { type: "session_ended" });

  return NextResponse.json(ended);
}
```

- [ ] **Step 4: Verify build**

```bash
export PATH="$HOME/.bun/bin:$PATH"
cd /home/chris/workshop/Bridge
bun run build
```

- [ ] **Step 5: Commit**

```bash
git add src/app/api/sessions/\[id\]/join/route.ts src/app/api/sessions/\[id\]/leave/route.ts src/app/api/sessions/\[id\]/route.ts
git commit -m "feat: emit SSE events on student join, leave, and session end"
```

---

## Task 21: Session Participant API Tests

**Files:**
- Create: `tests/api/session-participants.test.ts`

- [ ] **Step 1: Write integration tests for session participant operations**

Create `tests/api/session-participants.test.ts`:

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  createTestUser,
  createTestClassroom,
  createTestSession,
  addTestParticipant,
} from "../helpers";
import {
  joinSession,
  leaveSession,
  getSessionParticipants,
  updateParticipantStatus,
  endSession,
} from "@/lib/sessions";

describe("session participant operations", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student1: Awaited<ReturnType<typeof createTestUser>>;
  let student2: Awaited<ReturnType<typeof createTestUser>>;
  let student3: Awaited<ReturnType<typeof createTestUser>>;
  let classroom: Awaited<ReturnType<typeof createTestClassroom>>;
  let session: Awaited<ReturnType<typeof createTestSession>>;

  beforeEach(async () => {
    teacher = await createTestUser({ role: "teacher", email: "t@school.edu" });
    student1 = await createTestUser({ role: "student", email: "s1@school.edu" });
    student2 = await createTestUser({ role: "student", email: "s2@school.edu" });
    student3 = await createTestUser({ role: "student", email: "s3@school.edu" });
    classroom = await createTestClassroom(teacher.id);
    session = await createTestSession(classroom.id, teacher.id);
  });

  describe("multi-student session lifecycle", () => {
    it("handles multiple students joining and one leaving", async () => {
      await joinSession(testDb, session.id, student1.id);
      await joinSession(testDb, session.id, student2.id);
      await joinSession(testDb, session.id, student3.id);

      let participants = await getSessionParticipants(testDb, session.id);
      expect(participants).toHaveLength(3);

      await leaveSession(testDb, session.id, student2.id);

      participants = await getSessionParticipants(testDb, session.id);
      expect(participants).toHaveLength(2);
      expect(participants.find((p) => p.studentId === student2.id)).toBeUndefined();
    });

    it("correctly tracks needs_help status across participants", async () => {
      await joinSession(testDb, session.id, student1.id);
      await joinSession(testDb, session.id, student2.id);

      await updateParticipantStatus(testDb, session.id, student1.id, "needs_help");

      const participants = await getSessionParticipants(testDb, session.id);
      const s1 = participants.find((p) => p.studentId === student1.id);
      const s2 = participants.find((p) => p.studentId === student2.id);

      expect(s1?.status).toBe("needs_help");
      expect(s2?.status).toBe("active");
    });

    it("ending session marks all participants as left", async () => {
      await joinSession(testDb, session.id, student1.id);
      await joinSession(testDb, session.id, student2.id);
      await joinSession(testDb, session.id, student3.id);

      await endSession(testDb, session.id);

      const participants = await getSessionParticipants(testDb, session.id);
      expect(participants).toHaveLength(0);
    });
  });

  describe("rejoin scenarios", () => {
    it("student can rejoin after voluntarily leaving", async () => {
      await joinSession(testDb, session.id, student1.id);
      await leaveSession(testDb, session.id, student1.id);

      const participants = await getSessionParticipants(testDb, session.id);
      expect(participants).toHaveLength(0);

      const rejoined = await joinSession(testDb, session.id, student1.id);
      expect(rejoined.leftAt).toBeNull();

      const afterRejoin = await getSessionParticipants(testDb, session.id);
      expect(afterRejoin).toHaveLength(1);
    });
  });

  describe("status transitions", () => {
    it("cycles through all status values", async () => {
      await joinSession(testDb, session.id, student1.id);

      let updated = await updateParticipantStatus(testDb, session.id, student1.id, "idle");
      expect(updated?.status).toBe("idle");

      updated = await updateParticipantStatus(testDb, session.id, student1.id, "needs_help");
      expect(updated?.status).toBe("needs_help");

      updated = await updateParticipantStatus(testDb, session.id, student1.id, "active");
      expect(updated?.status).toBe("active");
    });
  });
});
```

- [ ] **Step 2: Run the participant tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
cd /home/chris/workshop/Bridge
bun run test tests/api/session-participants.test.ts
```

Expected: All tests pass.

- [ ] **Step 3: Commit**

```bash
git add tests/api/session-participants.test.ts
git commit -m "test: add session participant lifecycle and status transition tests"
```

---

## Task 22: Environment Variable Configuration

**Files:**
- Modify: `.env.example` (or create if it doesn't exist)

- [ ] **Step 1: Add Hocuspocus environment variables**

If `.env.example` exists, add to it. If not, create it with the following content:

```bash
# Hocuspocus WebSocket server
HOCUSPOCUS_PORT=1234
NEXT_PUBLIC_HOCUSPOCUS_URL=ws://localhost:1234
```

Also update `.env` (locally, not committed) with the same variables.

- [ ] **Step 2: Verify the env is not committed**

Check that `.env` is in `.gitignore`:

```bash
grep -q "\.env" .gitignore && echo "OK" || echo "MISSING"
```

- [ ] **Step 3: Commit env example only**

```bash
git add .env.example
git commit -m "docs: add Hocuspocus environment variables to .env.example"
```

---

## Task 23: Run All Tests and Final Build Verification

- [ ] **Step 1: Run the full test suite**

```bash
export PATH="$HOME/.bun/bin:$PATH"
cd /home/chris/workshop/Bridge
bun run test
```

Expected: All tests pass (schema, classrooms, sessions, session-participants, sse, output-panel, use-pyodide, utils).

- [ ] **Step 2: Run a production build**

```bash
export PATH="$HOME/.bun/bin:$PATH"
cd /home/chris/workshop/Bridge
bun run build
```

Expected: Build completes without errors.

- [ ] **Step 3: Fix any failing tests or build errors**

If there are test failures or build errors, address them before proceeding.

- [ ] **Step 4: Commit any remaining fixes**

Only if fixes were needed in Step 3:

```bash
git add -A
git commit -m "fix: resolve test failures and build errors in real-time sessions"
```

---

## Summary

This plan implements the following:

1. **Database** (Tasks 2-3): `LiveSession` and `SessionParticipant` tables with migration and schema tests
2. **Session CRUD** (Tasks 4-6): Library functions + API routes for creating, ending, joining, and leaving sessions, with comprehensive tests
3. **SSE Events** (Task 7): Event bus + streaming endpoint for real-time notifications (student join/leave, session end, broadcast toggle)
4. **Hocuspocus Server** (Task 8): Sidecar WebSocket server for Yjs document sync with auth and document access control
5. **Yjs-enabled Editor** (Tasks 9-10): Extended `CodeEditor` component with optional Yjs binding + `useYjsProvider` hook
6. **Session UI** (Tasks 11-13): Session controls on classroom page, student session page with Yjs editor
7. **Teacher Dashboard** (Tasks 14-18): Student tile grid, participant list sidebar, collaborative editor, broadcast banner, and the dashboard page itself
8. **Event Integration** (Tasks 19-20): SSE events emitted from join/leave/end API routes + broadcast event POST endpoint
9. **Testing** (Task 21): Session participant lifecycle integration tests
10. **Configuration** (Tasks 22-23): Environment variables, full test suite verification, production build check

Total estimated time: 90-120 minutes for an agentic worker.

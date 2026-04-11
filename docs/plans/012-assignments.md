# Assignment System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the assignment creation, submission, and grading system. Teachers create assignments tied to topics within a class, students submit their work (linked to documents), and teachers grade submissions with numeric scores and text feedback.

**Architecture:** Assignments belong to both a topic and a class. Submissions link a student to an assignment and optionally to a document. The system follows the existing pattern: Drizzle schema tables, lib CRUD functions, Next.js API routes, server-action-powered portal pages, and Vitest tests.

**Tech Stack:** Next.js 16 App Router, React Server Components + Server Actions, Drizzle ORM, shadcn/ui (Card, Button, Input, Label, Textarea), lucide-react icons, Zod v4, Vitest

**Depends on:** Plan 007 (course-hierarchy — schema, courses, topics, classes), Plan 008 (code-persistence — documents), Plan 010 (portal-pages — teacher/student class detail pages)

**Key constraints:**
- shadcn/ui uses `@base-ui/react` -- NO `asChild` prop; use `buttonVariants()` with `<Link>` instead
- Auth.js v5: `session.user.id`, `session.user.isPlatformAdmin`
- Drizzle ORM for all DB queries -- use existing lib functions, add new ones only when needed
- Zod v4: `z.record` requires two args: `z.record(z.string(), z.unknown())`
- `fileParallelism: false` in Vitest -- `.tsx` tests need `// @vitest-environment jsdom`
- Server actions use inline `"use server"` inside server component functions (see `src/app/(portal)/teacher/courses/[id]/page.tsx` pattern)
- Manual SQL migrations (drizzle-kit generate has TTY issues)
- Tests live under `tests/` directory, not alongside source files
- Test helpers in `tests/helpers.ts` (testDb, createTest*, cleanupDatabase) and `tests/api-helpers.ts` (setMockUser, createRequest, parseResponse)
- Next.js 16 params are `Promise<{ ... }>` and must be awaited

---

## File Structure

```
src/
├── lib/
│   ├── db/
│   │   └── schema.ts              # Modify: add assignments + submissions tables
│   ├── assignments.ts             # Create: assignment CRUD operations
│   └── submissions.ts             # Create: submission CRUD operations
├── app/
│   ├── api/
│   │   ├── assignments/
│   │   │   ├── route.ts           # Create: POST (create), GET (list by classId)
│   │   │   └── [id]/
│   │   │       ├── route.ts       # Create: GET, PATCH, DELETE
│   │   │       ├── submit/
│   │   │       │   └── route.ts   # Create: POST (student submits)
│   │   │       └── submissions/
│   │   │           └── route.ts   # Create: GET (list submissions for teacher)
│   │   └── submissions/
│   │       └── [id]/
│   │           └── route.ts       # Create: PATCH (grade a submission)
│   └── (portal)/
│       ├── teacher/
│       │   └── classes/
│       │       └── [id]/
│       │           ├── page.tsx   # Modify: add Assignments section
│       │           └── assignments/
│       │               └── [assignmentId]/
│       │                   └── page.tsx  # Create: assignment detail + grading
│       └── student/
│           └── classes/
│               └── [id]/
│                   └── page.tsx   # Modify: add Assignments section with submit
drizzle/
└── 0006_assignments.sql           # Create: migration SQL
tests/
├── helpers.ts                     # Modify: add createTestAssignment, createTestSubmission, update cleanupDatabase
├── unit/
│   ├── assignments.test.ts        # Create: assignment CRUD tests
│   └── submissions.test.ts        # Create: submission CRUD tests
└── integration/
    └── assignments-api.test.ts    # Create: API route integration tests
```

---

## Task 1: Schema — Add assignments and submissions tables

### File: `src/lib/db/schema.ts`

Add `doublePrecision` to the import from `drizzle-orm/pg-core`, then add the two new tables after the `documents` table definition.

```typescript
// Add doublePrecision to existing import:
import {
  pgTable,
  uuid,
  varchar,
  text,
  timestamp,
  jsonb,
  pgEnum,
  uniqueIndex,
  index,
  boolean,
  integer,
  doublePrecision,
} from "drizzle-orm/pg-core";

// --- After the documents table, add: ---

// --- Assignments ---

export const assignments = pgTable(
  "assignments",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    topicId: uuid("topic_id")
      .notNull()
      .references(() => topics.id, { onDelete: "cascade" }),
    classId: uuid("class_id")
      .notNull()
      .references(() => classes.id, { onDelete: "cascade" }),
    title: varchar("title", { length: 255 }).notNull(),
    description: text("description").default(""),
    starterCode: text("starter_code"),
    dueDate: timestamp("due_date"),
    rubric: jsonb("rubric").default({}),
    createdAt: timestamp("created_at").defaultNow().notNull(),
  },
  (table) => [
    index("assignments_class_idx").on(table.classId),
    index("assignments_topic_idx").on(table.topicId),
    index("assignments_class_topic_idx").on(table.classId, table.topicId),
  ]
);

export const submissions = pgTable(
  "submissions",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    assignmentId: uuid("assignment_id")
      .notNull()
      .references(() => assignments.id, { onDelete: "cascade" }),
    studentId: uuid("student_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    documentId: uuid("document_id").references(() => documents.id),
    grade: doublePrecision("grade"),
    feedback: text("feedback"),
    submittedAt: timestamp("submitted_at").defaultNow().notNull(),
  },
  (table) => [
    index("submissions_assignment_idx").on(table.assignmentId),
    index("submissions_student_idx").on(table.studentId),
    uniqueIndex("submissions_assignment_student_idx").on(
      table.assignmentId,
      table.studentId
    ),
  ]
);
```

### File: `drizzle/0006_assignments.sql`

```sql
-- Migration: Assignments
-- Adds assignments and submissions tables

CREATE TABLE "assignments" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  "topic_id" uuid NOT NULL REFERENCES "topics"("id") ON DELETE CASCADE,
  "class_id" uuid NOT NULL REFERENCES "classes"("id") ON DELETE CASCADE,
  "title" varchar(255) NOT NULL,
  "description" text DEFAULT '',
  "starter_code" text,
  "due_date" timestamp,
  "rubric" jsonb DEFAULT '{}'::jsonb,
  "created_at" timestamp DEFAULT now() NOT NULL
);

CREATE INDEX "assignments_class_idx" ON "assignments" USING btree ("class_id");
CREATE INDEX "assignments_topic_idx" ON "assignments" USING btree ("topic_id");
CREATE INDEX "assignments_class_topic_idx" ON "assignments" USING btree ("class_id", "topic_id");

CREATE TABLE "submissions" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  "assignment_id" uuid NOT NULL REFERENCES "assignments"("id") ON DELETE CASCADE,
  "student_id" uuid NOT NULL REFERENCES "users"("id") ON DELETE CASCADE,
  "document_id" uuid REFERENCES "documents"("id"),
  "grade" double precision,
  "feedback" text,
  "submitted_at" timestamp DEFAULT now() NOT NULL
);

CREATE INDEX "submissions_assignment_idx" ON "submissions" USING btree ("assignment_id");
CREATE INDEX "submissions_student_idx" ON "submissions" USING btree ("student_id");
CREATE UNIQUE INDEX "submissions_assignment_student_idx" ON "submissions" USING btree ("assignment_id", "student_id");
```

- [ ] Add `doublePrecision` to the import in `schema.ts`
- [ ] Add `assignments` table to `schema.ts`
- [ ] Add `submissions` table to `schema.ts`
- [ ] Create `drizzle/0006_assignments.sql` migration
- [ ] Run migration against test database: `psql bridge_test < drizzle/0006_assignments.sql`
- [ ] Commit: `"Add assignments and submissions schema tables"`

---

## Task 2: Test helpers — Add factory functions

### File: `tests/helpers.ts`

Add `createTestAssignment` and `createTestSubmission` helper functions, and update `cleanupDatabase` to delete submissions and assignments (in correct FK order).

**In `cleanupDatabase`**, add these two lines at the very top of the function (before `documents` delete, since submissions reference both assignments and documents, and assignments reference topics and classes):

```typescript
await testDb.delete(schema.submissions);
await testDb.delete(schema.assignments);
```

**Add two new factory functions** at the bottom of the file (before `closeTestDb`):

```typescript
export async function createTestAssignment(
  topicId: string,
  classId: string,
  overrides: Partial<typeof schema.assignments.$inferInsert> = {}
) {
  const [assignment] = await testDb
    .insert(schema.assignments)
    .values({
      topicId,
      classId,
      title: "Test Assignment",
      ...overrides,
    })
    .returning();
  return assignment;
}

export async function createTestSubmission(
  assignmentId: string,
  studentId: string,
  overrides: Partial<typeof schema.submissions.$inferInsert> = {}
) {
  const [submission] = await testDb
    .insert(schema.submissions)
    .values({
      assignmentId,
      studentId,
      ...overrides,
    })
    .returning();
  return submission;
}
```

- [ ] Update `cleanupDatabase` to delete submissions and assignments
- [ ] Add `createTestAssignment` helper
- [ ] Add `createTestSubmission` helper
- [ ] Commit: `"Add assignment and submission test helpers"`

---

## Task 3: Assignment CRUD — lib functions

### File: `src/lib/assignments.ts`

```typescript
import { eq, and } from "drizzle-orm";
import { assignments, topics } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

interface CreateAssignmentInput {
  topicId: string;
  classId: string;
  title: string;
  description?: string;
  starterCode?: string;
  dueDate?: Date | null;
  rubric?: Record<string, unknown>;
}

export async function createAssignment(db: Database, input: CreateAssignmentInput) {
  const [assignment] = await db
    .insert(assignments)
    .values(input)
    .returning();
  return assignment;
}

export async function getAssignment(db: Database, assignmentId: string) {
  const [assignment] = await db
    .select()
    .from(assignments)
    .where(eq(assignments.id, assignmentId));
  return assignment || null;
}

export async function listAssignmentsByClass(db: Database, classId: string) {
  return db
    .select()
    .from(assignments)
    .where(eq(assignments.classId, classId));
}

export async function listAssignmentsByTopic(
  db: Database,
  topicId: string,
  classId: string
) {
  return db
    .select()
    .from(assignments)
    .where(
      and(eq(assignments.topicId, topicId), eq(assignments.classId, classId))
    );
}

export async function updateAssignment(
  db: Database,
  assignmentId: string,
  updates: Partial<
    Pick<
      typeof assignments.$inferInsert,
      "title" | "description" | "starterCode" | "dueDate" | "rubric"
    >
  >
) {
  const [assignment] = await db
    .update(assignments)
    .set(updates)
    .where(eq(assignments.id, assignmentId))
    .returning();
  return assignment || null;
}

export async function deleteAssignment(db: Database, assignmentId: string) {
  const [deleted] = await db
    .delete(assignments)
    .where(eq(assignments.id, assignmentId))
    .returning();
  return deleted || null;
}
```

- [ ] Create `src/lib/assignments.ts` with all 6 functions
- [ ] Commit: `"Add assignment CRUD lib functions"`

---

## Task 4: Submission CRUD — lib functions

### File: `src/lib/submissions.ts`

```typescript
import { eq, and } from "drizzle-orm";
import { submissions, users } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

interface CreateSubmissionInput {
  assignmentId: string;
  studentId: string;
  documentId?: string | null;
}

export async function createSubmission(db: Database, input: CreateSubmissionInput) {
  const [submission] = await db
    .insert(submissions)
    .values(input)
    .returning();
  return submission;
}

export async function getSubmission(db: Database, submissionId: string) {
  const [submission] = await db
    .select()
    .from(submissions)
    .where(eq(submissions.id, submissionId));
  return submission || null;
}

export async function listSubmissionsByAssignment(db: Database, assignmentId: string) {
  return db
    .select({
      id: submissions.id,
      assignmentId: submissions.assignmentId,
      studentId: submissions.studentId,
      documentId: submissions.documentId,
      grade: submissions.grade,
      feedback: submissions.feedback,
      submittedAt: submissions.submittedAt,
      studentName: users.name,
      studentEmail: users.email,
    })
    .from(submissions)
    .innerJoin(users, eq(submissions.studentId, users.id))
    .where(eq(submissions.assignmentId, assignmentId));
}

export async function listSubmissionsByStudent(
  db: Database,
  studentId: string,
  assignmentId?: string
) {
  if (assignmentId) {
    return db
      .select()
      .from(submissions)
      .where(
        and(
          eq(submissions.studentId, studentId),
          eq(submissions.assignmentId, assignmentId)
        )
      );
  }
  return db
    .select()
    .from(submissions)
    .where(eq(submissions.studentId, studentId));
}

export async function getSubmissionByAssignmentAndStudent(
  db: Database,
  assignmentId: string,
  studentId: string
) {
  const [submission] = await db
    .select()
    .from(submissions)
    .where(
      and(
        eq(submissions.assignmentId, assignmentId),
        eq(submissions.studentId, studentId)
      )
    );
  return submission || null;
}

export async function gradeSubmission(
  db: Database,
  submissionId: string,
  grade: number,
  feedback?: string | null
) {
  const updates: Record<string, unknown> = { grade };
  if (feedback !== undefined) {
    updates.feedback = feedback;
  }
  const [submission] = await db
    .update(submissions)
    .set(updates)
    .where(eq(submissions.id, submissionId))
    .returning();
  return submission || null;
}
```

- [ ] Create `src/lib/submissions.ts` with all 6 functions
- [ ] Commit: `"Add submission CRUD lib functions"`

---

## Task 5: Unit tests — Assignments and submissions

### File: `tests/unit/assignments.test.ts`

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  createTestUser,
  createTestOrg,
  createTestCourse,
  createTestTopic,
  createTestClass,
  createTestAssignment,
} from "../helpers";
import {
  createAssignment,
  getAssignment,
  listAssignmentsByClass,
  listAssignmentsByTopic,
  updateAssignment,
  deleteAssignment,
} from "@/lib/assignments";

describe("assignment operations", () => {
  let topic: Awaited<ReturnType<typeof createTestTopic>>;
  let cls: Awaited<ReturnType<typeof createTestClass>>;

  beforeEach(async () => {
    const org = await createTestOrg();
    const teacher = await createTestUser({ email: "teacher@test.edu" });
    const course = await createTestCourse(org.id, teacher.id);
    topic = await createTestTopic(course.id);
    cls = await createTestClass(course.id, org.id);
  });

  it("creates an assignment", async () => {
    const assignment = await createAssignment(testDb, {
      topicId: topic.id,
      classId: cls.id,
      title: "Homework 1",
    });
    expect(assignment.id).toBeDefined();
    expect(assignment.title).toBe("Homework 1");
    expect(assignment.topicId).toBe(topic.id);
    expect(assignment.classId).toBe(cls.id);
    expect(assignment.description).toBe("");
    expect(assignment.starterCode).toBeNull();
    expect(assignment.dueDate).toBeNull();
  });

  it("creates an assignment with all fields", async () => {
    const dueDate = new Date("2026-12-31");
    const assignment = await createAssignment(testDb, {
      topicId: topic.id,
      classId: cls.id,
      title: "Full Assignment",
      description: "Do the thing",
      starterCode: "print('hello')",
      dueDate,
      rubric: { criteria: [{ name: "Correctness", points: 10 }] },
    });
    expect(assignment.description).toBe("Do the thing");
    expect(assignment.starterCode).toBe("print('hello')");
    expect(assignment.dueDate).toEqual(dueDate);
    expect(assignment.rubric).toEqual({
      criteria: [{ name: "Correctness", points: 10 }],
    });
  });

  it("gets an assignment by ID", async () => {
    const created = await createTestAssignment(topic.id, cls.id);
    const found = await getAssignment(testDb, created.id);
    expect(found).not.toBeNull();
    expect(found!.id).toBe(created.id);
  });

  it("returns null for non-existent assignment", async () => {
    const found = await getAssignment(
      testDb,
      "00000000-0000-0000-0000-000000000000"
    );
    expect(found).toBeNull();
  });

  it("lists assignments by class", async () => {
    await createTestAssignment(topic.id, cls.id, { title: "A1" });
    await createTestAssignment(topic.id, cls.id, { title: "A2" });

    const list = await listAssignmentsByClass(testDb, cls.id);
    expect(list).toHaveLength(2);
  });

  it("lists assignments by topic and class", async () => {
    const org = await createTestOrg();
    const teacher = await createTestUser({ email: "teacher2@test.edu" });
    const course = await createTestCourse(org.id, teacher.id);
    const topic2 = await createTestTopic(course.id, { title: "Other Topic" });
    const cls2 = await createTestClass(course.id, org.id);

    await createTestAssignment(topic.id, cls.id, { title: "Match" });
    await createTestAssignment(topic2.id, cls2.id, { title: "No Match" });

    const list = await listAssignmentsByTopic(testDb, topic.id, cls.id);
    expect(list).toHaveLength(1);
    expect(list[0].title).toBe("Match");
  });

  it("updates an assignment", async () => {
    const assignment = await createTestAssignment(topic.id, cls.id);
    const updated = await updateAssignment(testDb, assignment.id, {
      title: "Updated Title",
      description: "New description",
    });
    expect(updated!.title).toBe("Updated Title");
    expect(updated!.description).toBe("New description");
  });

  it("updates assignment due date", async () => {
    const assignment = await createTestAssignment(topic.id, cls.id);
    const dueDate = new Date("2026-06-15");
    const updated = await updateAssignment(testDb, assignment.id, { dueDate });
    expect(updated!.dueDate).toEqual(dueDate);
  });

  it("returns null when updating non-existent assignment", async () => {
    const updated = await updateAssignment(
      testDb,
      "00000000-0000-0000-0000-000000000000",
      { title: "Nope" }
    );
    expect(updated).toBeNull();
  });

  it("deletes an assignment", async () => {
    const assignment = await createTestAssignment(topic.id, cls.id);
    const deleted = await deleteAssignment(testDb, assignment.id);
    expect(deleted).not.toBeNull();

    const remaining = await listAssignmentsByClass(testDb, cls.id);
    expect(remaining).toHaveLength(0);
  });

  it("returns null when deleting non-existent assignment", async () => {
    const deleted = await deleteAssignment(
      testDb,
      "00000000-0000-0000-0000-000000000000"
    );
    expect(deleted).toBeNull();
  });
});
```

### File: `tests/unit/submissions.test.ts`

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  createTestUser,
  createTestOrg,
  createTestCourse,
  createTestTopic,
  createTestClass,
  createTestAssignment,
  createTestSubmission,
} from "../helpers";
import {
  createSubmission,
  getSubmission,
  listSubmissionsByAssignment,
  listSubmissionsByStudent,
  getSubmissionByAssignmentAndStudent,
  gradeSubmission,
} from "@/lib/submissions";
import { createDocument } from "@/lib/documents";

describe("submission operations", () => {
  let assignment: Awaited<ReturnType<typeof createTestAssignment>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;

  beforeEach(async () => {
    const org = await createTestOrg();
    const teacher = await createTestUser({ email: "teacher@test.edu" });
    const course = await createTestCourse(org.id, teacher.id);
    const topic = await createTestTopic(course.id);
    const cls = await createTestClass(course.id, org.id);
    assignment = await createTestAssignment(topic.id, cls.id);
    student = await createTestUser({ email: "student@test.edu" });
  });

  it("creates a submission", async () => {
    const submission = await createSubmission(testDb, {
      assignmentId: assignment.id,
      studentId: student.id,
    });
    expect(submission.id).toBeDefined();
    expect(submission.assignmentId).toBe(assignment.id);
    expect(submission.studentId).toBe(student.id);
    expect(submission.documentId).toBeNull();
    expect(submission.grade).toBeNull();
    expect(submission.feedback).toBeNull();
  });

  it("creates a submission with documentId", async () => {
    const doc = await createDocument(testDb, { ownerId: student.id });
    const submission = await createSubmission(testDb, {
      assignmentId: assignment.id,
      studentId: student.id,
      documentId: doc.id,
    });
    expect(submission.documentId).toBe(doc.id);
  });

  it("gets a submission by ID", async () => {
    const created = await createTestSubmission(assignment.id, student.id);
    const found = await getSubmission(testDb, created.id);
    expect(found).not.toBeNull();
    expect(found!.id).toBe(created.id);
  });

  it("returns null for non-existent submission", async () => {
    const found = await getSubmission(
      testDb,
      "00000000-0000-0000-0000-000000000000"
    );
    expect(found).toBeNull();
  });

  it("lists submissions by assignment with student info", async () => {
    await createTestSubmission(assignment.id, student.id);
    const student2 = await createTestUser({ email: "student2@test.edu" });
    await createTestSubmission(assignment.id, student2.id);

    const list = await listSubmissionsByAssignment(testDb, assignment.id);
    expect(list).toHaveLength(2);
    expect(list[0]).toHaveProperty("studentName");
    expect(list[0]).toHaveProperty("studentEmail");
  });

  it("lists submissions by student", async () => {
    await createTestSubmission(assignment.id, student.id);

    const list = await listSubmissionsByStudent(testDb, student.id);
    expect(list).toHaveLength(1);
    expect(list[0].studentId).toBe(student.id);
  });

  it("lists submissions by student filtered by assignment", async () => {
    await createTestSubmission(assignment.id, student.id);

    const org2 = await createTestOrg();
    const teacher2 = await createTestUser({ email: "teacher2@test.edu" });
    const course2 = await createTestCourse(org2.id, teacher2.id);
    const topic2 = await createTestTopic(course2.id);
    const cls2 = await createTestClass(course2.id, org2.id);
    const assignment2 = await createTestAssignment(topic2.id, cls2.id);
    await createTestSubmission(assignment2.id, student.id);

    const filtered = await listSubmissionsByStudent(
      testDb,
      student.id,
      assignment.id
    );
    expect(filtered).toHaveLength(1);
    expect(filtered[0].assignmentId).toBe(assignment.id);
  });

  it("gets submission by assignment and student", async () => {
    await createTestSubmission(assignment.id, student.id);
    const found = await getSubmissionByAssignmentAndStudent(
      testDb,
      assignment.id,
      student.id
    );
    expect(found).not.toBeNull();
    expect(found!.assignmentId).toBe(assignment.id);
    expect(found!.studentId).toBe(student.id);
  });

  it("returns null when no submission exists for assignment+student", async () => {
    const found = await getSubmissionByAssignmentAndStudent(
      testDb,
      assignment.id,
      student.id
    );
    expect(found).toBeNull();
  });

  it("grades a submission", async () => {
    const submission = await createTestSubmission(assignment.id, student.id);
    const graded = await gradeSubmission(
      testDb,
      submission.id,
      95.5,
      "Great work!"
    );
    expect(graded!.grade).toBe(95.5);
    expect(graded!.feedback).toBe("Great work!");
  });

  it("grades a submission without feedback", async () => {
    const submission = await createTestSubmission(assignment.id, student.id);
    const graded = await gradeSubmission(testDb, submission.id, 80);
    expect(graded!.grade).toBe(80);
    expect(graded!.feedback).toBeNull();
  });

  it("re-grades a submission", async () => {
    const submission = await createTestSubmission(assignment.id, student.id);
    await gradeSubmission(testDb, submission.id, 70, "Needs improvement");
    const regraded = await gradeSubmission(
      testDb,
      submission.id,
      85,
      "Much better on revision"
    );
    expect(regraded!.grade).toBe(85);
    expect(regraded!.feedback).toBe("Much better on revision");
  });

  it("returns null when grading non-existent submission", async () => {
    const graded = await gradeSubmission(
      testDb,
      "00000000-0000-0000-0000-000000000000",
      100
    );
    expect(graded).toBeNull();
  });

  it("enforces unique constraint on assignment+student", async () => {
    await createTestSubmission(assignment.id, student.id);
    await expect(
      createSubmission(testDb, {
        assignmentId: assignment.id,
        studentId: student.id,
      })
    ).rejects.toThrow();
  });
});
```

- [ ] Create `tests/unit/assignments.test.ts`
- [ ] Create `tests/unit/submissions.test.ts`
- [ ] Run tests: `bun run test tests/unit/assignments.test.ts tests/unit/submissions.test.ts`
- [ ] All tests pass
- [ ] Commit: `"Add unit tests for assignment and submission CRUD"`

---

## Task 6: API routes — Assignments

### File: `src/app/api/assignments/route.ts`

```typescript
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { createAssignment, listAssignmentsByClass } from "@/lib/assignments";
import { getClass } from "@/lib/classes";
import { listClassMembers } from "@/lib/class-memberships";

const createSchema = z.object({
  topicId: z.string().uuid(),
  classId: z.string().uuid(),
  title: z.string().min(1).max(255),
  description: z.string().max(5000).optional(),
  starterCode: z.string().optional(),
  dueDate: z.string().datetime().optional().nullable(),
  rubric: z.record(z.string(), z.unknown()).optional(),
});

export async function POST(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const body = await request.json();
  const parsed = createSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  // Verify user is instructor/TA in this class
  const members = await listClassMembers(db, parsed.data.classId);
  const isInstructor = members.some(
    (m) =>
      m.userId === session.user.id &&
      (m.role === "instructor" || m.role === "ta")
  );
  if (!isInstructor && !session.user.isPlatformAdmin) {
    return NextResponse.json(
      { error: "Only instructors can create assignments" },
      { status: 403 }
    );
  }

  const assignment = await createAssignment(db, {
    ...parsed.data,
    dueDate: parsed.data.dueDate ? new Date(parsed.data.dueDate) : undefined,
  });

  return NextResponse.json(assignment, { status: 201 });
}

export async function GET(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const classId = request.nextUrl.searchParams.get("classId");
  if (!classId) {
    return NextResponse.json({ error: "classId required" }, { status: 400 });
  }

  // Verify user is a member of this class
  const members = await listClassMembers(db, classId);
  const isMember = members.some((m) => m.userId === session.user.id);
  if (!isMember && !session.user.isPlatformAdmin) {
    return NextResponse.json(
      { error: "Not a member of this class" },
      { status: 403 }
    );
  }

  const list = await listAssignmentsByClass(db, classId);
  return NextResponse.json(list);
}
```

### File: `src/app/api/assignments/[id]/route.ts`

```typescript
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import {
  getAssignment,
  updateAssignment,
  deleteAssignment,
} from "@/lib/assignments";
import { listClassMembers } from "@/lib/class-memberships";

const updateSchema = z.object({
  title: z.string().min(1).max(255).optional(),
  description: z.string().max(5000).optional(),
  starterCode: z.string().optional().nullable(),
  dueDate: z.string().datetime().optional().nullable(),
  rubric: z.record(z.string(), z.unknown()).optional(),
});

async function verifyInstructor(
  classId: string,
  userId: string,
  isPlatformAdmin: boolean
) {
  if (isPlatformAdmin) return true;
  const members = await listClassMembers(db, classId);
  return members.some(
    (m) =>
      m.userId === userId && (m.role === "instructor" || m.role === "ta")
  );
}

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const assignment = await getAssignment(db, id);

  if (!assignment) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  return NextResponse.json(assignment);
}

export async function PATCH(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const assignment = await getAssignment(db, id);

  if (!assignment) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  if (
    !(await verifyInstructor(
      assignment.classId,
      session.user.id,
      session.user.isPlatformAdmin
    ))
  ) {
    return NextResponse.json({ error: "Access denied" }, { status: 403 });
  }

  const body = await request.json();
  const parsed = updateSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const updates: Record<string, unknown> = { ...parsed.data };
  if (parsed.data.dueDate !== undefined) {
    updates.dueDate = parsed.data.dueDate
      ? new Date(parsed.data.dueDate)
      : null;
  }

  const updated = await updateAssignment(db, id, updates);
  return NextResponse.json(updated);
}

export async function DELETE(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const assignment = await getAssignment(db, id);

  if (!assignment) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  if (
    !(await verifyInstructor(
      assignment.classId,
      session.user.id,
      session.user.isPlatformAdmin
    ))
  ) {
    return NextResponse.json({ error: "Access denied" }, { status: 403 });
  }

  const deleted = await deleteAssignment(db, id);
  return NextResponse.json(deleted);
}
```

### File: `src/app/api/assignments/[id]/submit/route.ts`

```typescript
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getAssignment } from "@/lib/assignments";
import {
  createSubmission,
  getSubmissionByAssignmentAndStudent,
} from "@/lib/submissions";
import { listClassMembers } from "@/lib/class-memberships";

const submitSchema = z.object({
  documentId: z.string().uuid().optional().nullable(),
});

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id: assignmentId } = await params;
  const assignment = await getAssignment(db, assignmentId);

  if (!assignment) {
    return NextResponse.json({ error: "Assignment not found" }, { status: 404 });
  }

  // Verify user is a student in this class
  const members = await listClassMembers(db, assignment.classId);
  const isStudent = members.some(
    (m) => m.userId === session.user.id && m.role === "student"
  );
  if (!isStudent) {
    return NextResponse.json(
      { error: "Only students can submit assignments" },
      { status: 403 }
    );
  }

  // Check for existing submission
  const existing = await getSubmissionByAssignmentAndStudent(
    db,
    assignmentId,
    session.user.id
  );
  if (existing) {
    return NextResponse.json(
      { error: "Already submitted" },
      { status: 409 }
    );
  }

  const body = await request.json().catch(() => ({}));
  const parsed = submitSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const submission = await createSubmission(db, {
    assignmentId,
    studentId: session.user.id,
    documentId: parsed.data.documentId,
  });

  return NextResponse.json(submission, { status: 201 });
}
```

### File: `src/app/api/assignments/[id]/submissions/route.ts`

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getAssignment } from "@/lib/assignments";
import { listSubmissionsByAssignment } from "@/lib/submissions";
import { listClassMembers } from "@/lib/class-memberships";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id: assignmentId } = await params;
  const assignment = await getAssignment(db, assignmentId);

  if (!assignment) {
    return NextResponse.json({ error: "Assignment not found" }, { status: 404 });
  }

  // Verify user is instructor/TA in this class
  const members = await listClassMembers(db, assignment.classId);
  const isInstructor = members.some(
    (m) =>
      m.userId === session.user.id &&
      (m.role === "instructor" || m.role === "ta")
  );
  if (!isInstructor && !session.user.isPlatformAdmin) {
    return NextResponse.json(
      { error: "Only instructors can view submissions" },
      { status: 403 }
    );
  }

  const list = await listSubmissionsByAssignment(db, assignmentId);
  return NextResponse.json(list);
}
```

### File: `src/app/api/submissions/[id]/route.ts`

```typescript
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSubmission, gradeSubmission } from "@/lib/submissions";
import { getAssignment } from "@/lib/assignments";
import { listClassMembers } from "@/lib/class-memberships";

const gradeSchema = z.object({
  grade: z.number().min(0).max(100),
  feedback: z.string().max(5000).optional().nullable(),
});

export async function PATCH(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const submission = await getSubmission(db, id);

  if (!submission) {
    return NextResponse.json({ error: "Submission not found" }, { status: 404 });
  }

  // Look up the assignment to get classId
  const assignment = await getAssignment(db, submission.assignmentId);
  if (!assignment) {
    return NextResponse.json({ error: "Assignment not found" }, { status: 404 });
  }

  // Verify user is instructor/TA in this class
  const members = await listClassMembers(db, assignment.classId);
  const isInstructor = members.some(
    (m) =>
      m.userId === session.user.id &&
      (m.role === "instructor" || m.role === "ta")
  );
  if (!isInstructor && !session.user.isPlatformAdmin) {
    return NextResponse.json(
      { error: "Only instructors can grade submissions" },
      { status: 403 }
    );
  }

  const body = await request.json();
  const parsed = gradeSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const graded = await gradeSubmission(
    db,
    id,
    parsed.data.grade,
    parsed.data.feedback
  );

  return NextResponse.json(graded);
}
```

- [ ] Create `src/app/api/assignments/route.ts`
- [ ] Create `src/app/api/assignments/[id]/route.ts`
- [ ] Create `src/app/api/assignments/[id]/submit/route.ts`
- [ ] Create `src/app/api/assignments/[id]/submissions/route.ts`
- [ ] Create `src/app/api/submissions/[id]/route.ts`
- [ ] Commit: `"Add assignment and submission API routes"`

---

## Task 7: Integration tests — API routes

### File: `tests/integration/assignments-api.test.ts`

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  createTestUser,
  createTestOrg,
  createTestOrgMembership,
  createTestCourse,
  createTestTopic,
  createTestClass,
  createTestAssignment,
  createTestSubmission,
} from "../helpers";
import { setMockUser, createRequest, parseResponse } from "../api-helpers";
import { addClassMember } from "@/lib/class-memberships";
import { createClass } from "@/lib/classes";
import { POST, GET } from "@/app/api/assignments/route";
import {
  GET as GET_ASSIGNMENT,
  PATCH as UPDATE_ASSIGNMENT,
  DELETE as DELETE_ASSIGNMENT,
} from "@/app/api/assignments/[id]/route";
import { POST as SUBMIT } from "@/app/api/assignments/[id]/submit/route";
import { GET as LIST_SUBMISSIONS } from "@/app/api/assignments/[id]/submissions/route";
import { PATCH as GRADE } from "@/app/api/submissions/[id]/route";

describe("Assignments API", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;
  let otherUser: Awaited<ReturnType<typeof createTestUser>>;
  let org: Awaited<ReturnType<typeof createTestOrg>>;
  let course: Awaited<ReturnType<typeof createTestCourse>>;
  let topic: Awaited<ReturnType<typeof createTestTopic>>;
  let cls: Awaited<ReturnType<typeof createTestClass>>;

  beforeEach(async () => {
    org = await createTestOrg();
    teacher = await createTestUser({ name: "Teacher", email: "teacher@test.edu" });
    student = await createTestUser({ name: "Student", email: "student@test.edu" });
    otherUser = await createTestUser({ name: "Other", email: "other@test.edu" });
    await createTestOrgMembership(org.id, teacher.id, { role: "teacher" });
    course = await createTestCourse(org.id, teacher.id);
    topic = await createTestTopic(course.id);
    cls = await createClass(testDb, {
      courseId: course.id,
      orgId: org.id,
      title: "Test Class",
      createdBy: teacher.id,
    });
    // Add student to class
    await addClassMember(testDb, {
      classId: cls.id,
      userId: student.id,
      role: "student",
    });
  });

  describe("POST /api/assignments", () => {
    it("instructor creates an assignment", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });

      const req = createRequest("/api/assignments", {
        method: "POST",
        body: {
          topicId: topic.id,
          classId: cls.id,
          title: "Homework 1",
          description: "Complete the exercises",
        },
      });
      const { status, body } = await parseResponse(await POST(req));
      expect(status).toBe(201);
      expect(body).toHaveProperty("title", "Homework 1");
      expect(body).toHaveProperty("classId", cls.id);
    });

    it("student cannot create assignment", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email });

      const req = createRequest("/api/assignments", {
        method: "POST",
        body: { topicId: topic.id, classId: cls.id, title: "Nope" },
      });
      const { status } = await parseResponse(await POST(req));
      expect(status).toBe(403);
    });

    it("unauthenticated user cannot create assignment", async () => {
      setMockUser(null);

      const req = createRequest("/api/assignments", {
        method: "POST",
        body: { topicId: topic.id, classId: cls.id, title: "Nope" },
      });
      const { status } = await parseResponse(await POST(req));
      expect(status).toBe(401);
    });

    it("rejects invalid input", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });

      const req = createRequest("/api/assignments", {
        method: "POST",
        body: { classId: cls.id },
      });
      const { status } = await parseResponse(await POST(req));
      expect(status).toBe(400);
    });
  });

  describe("GET /api/assignments", () => {
    it("class member lists assignments", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email });
      await createTestAssignment(topic.id, cls.id, { title: "A1" });
      await createTestAssignment(topic.id, cls.id, { title: "A2" });

      const req = createRequest("/api/assignments", {
        searchParams: { classId: cls.id },
      });
      const { status, body } = await parseResponse<any[]>(await GET(req));
      expect(status).toBe(200);
      expect(body).toHaveLength(2);
    });

    it("non-member cannot list", async () => {
      setMockUser({ id: otherUser.id, name: otherUser.name, email: otherUser.email });

      const req = createRequest("/api/assignments", {
        searchParams: { classId: cls.id },
      });
      const { status } = await parseResponse(await GET(req));
      expect(status).toBe(403);
    });

    it("requires classId", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });

      const req = createRequest("/api/assignments");
      const { status } = await parseResponse(await GET(req));
      expect(status).toBe(400);
    });
  });

  describe("GET/PATCH/DELETE /api/assignments/[id]", () => {
    it("gets an assignment", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const assignment = await createTestAssignment(topic.id, cls.id, {
        title: "Find Me",
      });

      const res = await GET_ASSIGNMENT(
        createRequest(`/api/assignments/${assignment.id}`),
        { params: Promise.resolve({ id: assignment.id }) }
      );
      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("title", "Find Me");
    });

    it("returns 404 for non-existent assignment", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const fakeId = "00000000-0000-0000-0000-000000000000";

      const res = await GET_ASSIGNMENT(
        createRequest(`/api/assignments/${fakeId}`),
        { params: Promise.resolve({ id: fakeId }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(404);
    });

    it("instructor updates an assignment", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const assignment = await createTestAssignment(topic.id, cls.id);

      const res = await UPDATE_ASSIGNMENT(
        createRequest(`/api/assignments/${assignment.id}`, {
          method: "PATCH",
          body: { title: "Updated Title", description: "New desc" },
        }),
        { params: Promise.resolve({ id: assignment.id }) }
      );
      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("title", "Updated Title");
    });

    it("student cannot update assignment", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email });
      const assignment = await createTestAssignment(topic.id, cls.id);

      const res = await UPDATE_ASSIGNMENT(
        createRequest(`/api/assignments/${assignment.id}`, {
          method: "PATCH",
          body: { title: "Hacked" },
        }),
        { params: Promise.resolve({ id: assignment.id }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(403);
    });

    it("instructor deletes an assignment", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const assignment = await createTestAssignment(topic.id, cls.id);

      const res = await DELETE_ASSIGNMENT(
        createRequest(`/api/assignments/${assignment.id}`, { method: "DELETE" }),
        { params: Promise.resolve({ id: assignment.id }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(200);
    });
  });

  describe("POST /api/assignments/[id]/submit", () => {
    it("student submits", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email });
      const assignment = await createTestAssignment(topic.id, cls.id);

      const res = await SUBMIT(
        createRequest(`/api/assignments/${assignment.id}/submit`, {
          method: "POST",
          body: {},
        }),
        { params: Promise.resolve({ id: assignment.id }) }
      );
      const { status, body } = await parseResponse(res);
      expect(status).toBe(201);
      expect(body).toHaveProperty("assignmentId", assignment.id);
      expect(body).toHaveProperty("studentId", student.id);
    });

    it("student cannot submit twice", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email });
      const assignment = await createTestAssignment(topic.id, cls.id);
      await createTestSubmission(assignment.id, student.id);

      const res = await SUBMIT(
        createRequest(`/api/assignments/${assignment.id}/submit`, {
          method: "POST",
          body: {},
        }),
        { params: Promise.resolve({ id: assignment.id }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(409);
    });

    it("non-student cannot submit", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const assignment = await createTestAssignment(topic.id, cls.id);

      const res = await SUBMIT(
        createRequest(`/api/assignments/${assignment.id}/submit`, {
          method: "POST",
          body: {},
        }),
        { params: Promise.resolve({ id: assignment.id }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(403);
    });

    it("returns 404 for non-existent assignment", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email });
      const fakeId = "00000000-0000-0000-0000-000000000000";

      const res = await SUBMIT(
        createRequest(`/api/assignments/${fakeId}/submit`, {
          method: "POST",
          body: {},
        }),
        { params: Promise.resolve({ id: fakeId }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(404);
    });
  });

  describe("GET /api/assignments/[id]/submissions", () => {
    it("instructor lists submissions", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const assignment = await createTestAssignment(topic.id, cls.id);
      await createTestSubmission(assignment.id, student.id);

      const res = await LIST_SUBMISSIONS(
        createRequest(`/api/assignments/${assignment.id}/submissions`),
        { params: Promise.resolve({ id: assignment.id }) }
      );
      const { status, body } = await parseResponse<any[]>(res);
      expect(status).toBe(200);
      expect(body).toHaveLength(1);
      expect(body[0]).toHaveProperty("studentName", "Student");
    });

    it("student cannot list submissions", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email });
      const assignment = await createTestAssignment(topic.id, cls.id);

      const res = await LIST_SUBMISSIONS(
        createRequest(`/api/assignments/${assignment.id}/submissions`),
        { params: Promise.resolve({ id: assignment.id }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(403);
    });
  });

  describe("PATCH /api/submissions/[id]", () => {
    it("instructor grades a submission", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const assignment = await createTestAssignment(topic.id, cls.id);
      const submission = await createTestSubmission(assignment.id, student.id);

      const res = await GRADE(
        createRequest(`/api/submissions/${submission.id}`, {
          method: "PATCH",
          body: { grade: 92, feedback: "Well done!" },
        }),
        { params: Promise.resolve({ id: submission.id }) }
      );
      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("grade", 92);
      expect(body).toHaveProperty("feedback", "Well done!");
    });

    it("student cannot grade", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email });
      const assignment = await createTestAssignment(topic.id, cls.id);
      const submission = await createTestSubmission(assignment.id, student.id);

      const res = await GRADE(
        createRequest(`/api/submissions/${submission.id}`, {
          method: "PATCH",
          body: { grade: 100 },
        }),
        { params: Promise.resolve({ id: submission.id }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(403);
    });

    it("rejects invalid grade", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const assignment = await createTestAssignment(topic.id, cls.id);
      const submission = await createTestSubmission(assignment.id, student.id);

      const res = await GRADE(
        createRequest(`/api/submissions/${submission.id}`, {
          method: "PATCH",
          body: { grade: 150 },
        }),
        { params: Promise.resolve({ id: submission.id }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(400);
    });

    it("returns 404 for non-existent submission", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const fakeId = "00000000-0000-0000-0000-000000000000";

      const res = await GRADE(
        createRequest(`/api/submissions/${fakeId}`, {
          method: "PATCH",
          body: { grade: 100 },
        }),
        { params: Promise.resolve({ id: fakeId }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(404);
    });
  });
});
```

- [ ] Create `tests/integration/assignments-api.test.ts`
- [ ] Run tests: `bun run test tests/integration/assignments-api.test.ts`
- [ ] All tests pass
- [ ] Commit: `"Add integration tests for assignment and submission API routes"`

---

## Task 8: Teacher UI — Assignments section on class detail page

### File: `src/app/(portal)/teacher/classes/[id]/page.tsx`

Modify the existing page to add an Assignments section. Import `listAssignmentsByClass` and `createAssignment`, add server actions for creating and deleting assignments, and render the list below the existing members section.

Replace the entire file with:

```typescript
import { notFound } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getClass } from "@/lib/classes";
import { getCourse } from "@/lib/courses";
import { listClassMembers } from "@/lib/class-memberships";
import { listTopicsByCourse } from "@/lib/topics";
import { listAssignmentsByClass, createAssignment, deleteAssignment } from "@/lib/assignments";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";
import { revalidatePath } from "next/cache";

export default async function TeacherClassDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const session = await auth();
  const { id } = await params;
  const cls = await getClass(db, id);
  if (!cls) notFound();

  const members = await listClassMembers(db, id);

  // Verify teacher is an instructor in this class
  const isInstructor = members.some(
    (m) => m.userId === session!.user.id && (m.role === "instructor" || m.role === "ta")
  );
  if (!isInstructor && !session!.user.isPlatformAdmin) notFound();

  const students = members.filter((m) => m.role === "student");
  const instructors = members.filter((m) => m.role === "instructor" || m.role === "ta");

  const course = await getCourse(db, cls.courseId);
  const topics = course ? await listTopicsByCourse(db, course.id) : [];
  const assignments = await listAssignmentsByClass(db, id);

  async function handleCreateAssignment(formData: FormData) {
    "use server";
    const { auth: getAuth } = await import("@/lib/auth");
    const { db: database } = await import("@/lib/db");
    const { getClass: get } = await import("@/lib/classes");
    const { listClassMembers: getMembers } = await import("@/lib/class-memberships");
    const { createAssignment: create } = await import("@/lib/assignments");
    const sess = await getAuth();
    if (!sess?.user?.id) return;
    const c = await get(database, id);
    if (!c) return;
    const m = await getMembers(database, id);
    const isInstr = m.some(
      (mem) => mem.userId === sess.user.id && (mem.role === "instructor" || mem.role === "ta")
    );
    if (!isInstr && !sess.user.isPlatformAdmin) return;

    const title = formData.get("title") as string;
    const topicId = formData.get("topicId") as string;
    const description = formData.get("description") as string;
    if (!title || !topicId) return;

    await create(database, { topicId, classId: id, title, description: description || undefined });
    revalidatePath(`/teacher/classes/${id}`);
  }

  async function handleDeleteAssignment(formData: FormData) {
    "use server";
    const { auth: getAuth } = await import("@/lib/auth");
    const { db: database } = await import("@/lib/db");
    const { listClassMembers: getMembers } = await import("@/lib/class-memberships");
    const { deleteAssignment: del } = await import("@/lib/assignments");
    const sess = await getAuth();
    if (!sess?.user?.id) return;
    const m = await getMembers(database, id);
    const isInstr = m.some(
      (mem) => mem.userId === sess.user.id && (mem.role === "instructor" || mem.role === "ta")
    );
    if (!isInstr && !sess.user.isPlatformAdmin) return;

    const assignmentId = formData.get("assignmentId") as string;
    if (!assignmentId) return;
    await del(database, assignmentId);
    revalidatePath(`/teacher/classes/${id}`);
  }

  return (
    <div className="p-6 space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{cls.title}</h1>
        <p className="text-muted-foreground">{cls.term || "No term"} · {cls.status}</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Join Code</CardTitle>
          <CardDescription>Share with students to join this class</CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-3xl font-mono tracking-widest font-bold text-center">{cls.joinCode}</p>
        </CardContent>
      </Card>

      <div className="grid gap-6 md:grid-cols-2">
        <div>
          <h2 className="text-lg font-semibold mb-3">Students ({students.length})</h2>
          {students.length === 0 ? (
            <p className="text-sm text-muted-foreground">No students have joined yet.</p>
          ) : (
            <div className="space-y-2">
              {students.map((m) => (
                <div key={m.id} className="flex items-center justify-between py-2 border-b last:border-0">
                  <span className="text-sm font-medium">{m.name}</span>
                  <span className="text-xs text-muted-foreground">{m.email}</span>
                </div>
              ))}
            </div>
          )}
        </div>

        <div>
          <h2 className="text-lg font-semibold mb-3">Instructors & TAs ({instructors.length})</h2>
          <div className="space-y-2">
            {instructors.map((m) => (
              <div key={m.id} className="flex items-center justify-between py-2 border-b last:border-0">
                <span className="text-sm font-medium">{m.name}</span>
                <span className="text-xs text-muted-foreground">{m.role}</span>
              </div>
            ))}
          </div>
        </div>
      </div>

      <div className="space-y-4">
        <h2 className="text-lg font-semibold">Assignments ({assignments.length})</h2>

        {topics.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Create Assignment</CardTitle>
            </CardHeader>
            <CardContent>
              <form action={handleCreateAssignment} className="flex gap-3 items-end flex-wrap">
                <div>
                  <Label className="text-xs">Title</Label>
                  <Input name="title" placeholder="e.g., Homework 1" required className="w-48" />
                </div>
                <div>
                  <Label className="text-xs">Topic</Label>
                  <select name="topicId" className="border rounded px-2 py-1.5 text-sm bg-background" required>
                    {topics.map((t) => (
                      <option key={t.id} value={t.id}>{t.title}</option>
                    ))}
                  </select>
                </div>
                <div>
                  <Label className="text-xs">Description (optional)</Label>
                  <Input name="description" placeholder="Instructions..." className="w-64" />
                </div>
                <Button type="submit" size="sm">Create</Button>
              </form>
            </CardContent>
          </Card>
        )}

        {assignments.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No assignments yet.{topics.length === 0 ? " Add topics to the course first." : " Create one above."}
          </p>
        ) : (
          <div className="space-y-2">
            {assignments.map((a) => (
              <Card key={a.id}>
                <CardContent className="py-3 flex items-center justify-between">
                  <Link href={`/teacher/classes/${id}/assignments/${a.id}`} className="flex-1">
                    <p className="font-medium hover:text-primary">{a.title}</p>
                    <p className="text-sm text-muted-foreground">
                      {a.description || "No description"}
                      {a.dueDate && ` · Due: ${new Date(a.dueDate).toLocaleDateString()}`}
                    </p>
                  </Link>
                  <form action={handleDeleteAssignment}>
                    <input type="hidden" name="assignmentId" value={a.id} />
                    <Button type="submit" variant="ghost" size="sm" className="text-destructive">
                      ×
                    </Button>
                  </form>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
```

- [ ] Replace `src/app/(portal)/teacher/classes/[id]/page.tsx` with updated version
- [ ] Verify no import errors
- [ ] Commit: `"Add assignments section to teacher class detail page"`

---

## Task 9: Teacher UI — Assignment detail and grading page

### File: `src/app/(portal)/teacher/classes/[id]/assignments/[assignmentId]/page.tsx`

```typescript
import { notFound } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getAssignment } from "@/lib/assignments";
import { listSubmissionsByAssignment, gradeSubmission } from "@/lib/submissions";
import { listClassMembers } from "@/lib/class-memberships";
import { getTopic } from "@/lib/topics";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";
import { revalidatePath } from "next/cache";

export default async function TeacherAssignmentDetailPage({
  params,
}: {
  params: Promise<{ id: string; assignmentId: string }>;
}) {
  const session = await auth();
  const { id: classId, assignmentId } = await params;

  const assignment = await getAssignment(db, assignmentId);
  if (!assignment || assignment.classId !== classId) notFound();

  const members = await listClassMembers(db, classId);
  const isInstructor = members.some(
    (m) => m.userId === session!.user.id && (m.role === "instructor" || m.role === "ta")
  );
  if (!isInstructor && !session!.user.isPlatformAdmin) notFound();

  const topic = await getTopic(db, assignment.topicId);
  const submissions = await listSubmissionsByAssignment(db, assignmentId);
  const students = members.filter((m) => m.role === "student");
  const submittedCount = submissions.length;

  async function handleGrade(formData: FormData) {
    "use server";
    const { auth: getAuth } = await import("@/lib/auth");
    const { db: database } = await import("@/lib/db");
    const { getAssignment: getA } = await import("@/lib/assignments");
    const { gradeSubmission: grade } = await import("@/lib/submissions");
    const { listClassMembers: getMembers } = await import("@/lib/class-memberships");
    const sess = await getAuth();
    if (!sess?.user?.id) return;

    const a = await getA(database, assignmentId);
    if (!a) return;
    const m = await getMembers(database, a.classId);
    const isInstr = m.some(
      (mem) => mem.userId === sess.user.id && (mem.role === "instructor" || mem.role === "ta")
    );
    if (!isInstr && !sess.user.isPlatformAdmin) return;

    const submissionId = formData.get("submissionId") as string;
    const gradeValue = parseFloat(formData.get("grade") as string);
    const feedback = formData.get("feedback") as string;
    if (!submissionId || isNaN(gradeValue) || gradeValue < 0 || gradeValue > 100) return;

    await grade(database, submissionId, gradeValue, feedback || null);
    revalidatePath(`/teacher/classes/${classId}/assignments/${assignmentId}`);
  }

  return (
    <div className="p-6 space-y-6">
      <div>
        <Link href={`/teacher/classes/${classId}`} className="text-sm text-muted-foreground hover:text-primary">
          Back to class
        </Link>
        <h1 className="text-2xl font-bold mt-2">{assignment.title}</h1>
        <p className="text-muted-foreground">
          Topic: {topic?.title || "Unknown"}
          {assignment.dueDate && ` · Due: ${new Date(assignment.dueDate).toLocaleDateString()}`}
        </p>
      </div>

      {assignment.description && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Description</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm">{assignment.description}</p>
          </CardContent>
        </Card>
      )}

      {assignment.starterCode && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Starter Code</CardTitle>
          </CardHeader>
          <CardContent>
            <pre className="bg-muted p-3 rounded text-sm font-mono overflow-x-auto">{assignment.starterCode}</pre>
          </CardContent>
        </Card>
      )}

      <div className="space-y-4">
        <h2 className="text-lg font-semibold">
          Submissions ({submittedCount} / {students.length} students)
        </h2>

        {submissions.length === 0 ? (
          <p className="text-sm text-muted-foreground">No submissions yet.</p>
        ) : (
          <div className="space-y-3">
            {submissions.map((s) => (
              <Card key={s.id}>
                <CardContent className="py-4 space-y-3">
                  <div className="flex items-center justify-between">
                    <div>
                      <p className="font-medium">{s.studentName}</p>
                      <p className="text-xs text-muted-foreground">
                        {s.studentEmail} · Submitted {new Date(s.submittedAt).toLocaleString()}
                      </p>
                    </div>
                    {s.grade !== null && (
                      <span className="text-lg font-bold">{s.grade}/100</span>
                    )}
                  </div>

                  {s.feedback && (
                    <p className="text-sm bg-muted p-2 rounded">{s.feedback}</p>
                  )}

                  <form action={handleGrade} className="flex gap-3 items-end flex-wrap">
                    <input type="hidden" name="submissionId" value={s.id} />
                    <div>
                      <Label className="text-xs">Grade (0-100)</Label>
                      <Input
                        name="grade"
                        type="number"
                        min="0"
                        max="100"
                        step="0.5"
                        defaultValue={s.grade !== null ? String(s.grade) : ""}
                        required
                        className="w-24"
                      />
                    </div>
                    <div className="flex-1">
                      <Label className="text-xs">Feedback (optional)</Label>
                      <Input
                        name="feedback"
                        placeholder="Comments..."
                        defaultValue={s.feedback || ""}
                      />
                    </div>
                    <Button type="submit" size="sm">
                      {s.grade !== null ? "Update Grade" : "Grade"}
                    </Button>
                  </form>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
```

- [ ] Create `src/app/(portal)/teacher/classes/[id]/assignments/[assignmentId]/page.tsx`
- [ ] Verify no import errors
- [ ] Commit: `"Add teacher assignment detail page with grading"`

---

## Task 10: Student UI — Assignments section on class detail page

### File: `src/app/(portal)/student/classes/[id]/page.tsx`

Modify the existing student class detail page to show assignments with due dates, submission status, and a submit button. Replace the entire file:

```typescript
import { notFound } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getClass } from "@/lib/classes";
import { getCourse } from "@/lib/courses";
import { listTopicsByCourse } from "@/lib/topics";
import { listClassMembers } from "@/lib/class-memberships";
import { listAssignmentsByClass } from "@/lib/assignments";
import { listSubmissionsByStudent, createSubmission } from "@/lib/submissions";
import { parseLessonContent } from "@/lib/lesson-content";
import { LessonRenderer } from "@/components/lesson/lesson-renderer";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";
import { revalidatePath } from "next/cache";

export default async function StudentClassDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const session = await auth();
  const { id } = await params;
  const cls = await getClass(db, id);
  if (!cls) notFound();

  const members = await listClassMembers(db, id);
  const isEnrolled = members.some((m) => m.userId === session!.user.id);
  if (!isEnrolled && !session!.user.isPlatformAdmin) notFound();

  const course = await getCourse(db, cls.courseId);
  const topics = course ? await listTopicsByCourse(db, course.id) : [];
  const assignments = await listAssignmentsByClass(db, id);
  const mySubmissions = await listSubmissionsByStudent(db, session!.user.id);

  // Build a map of assignmentId -> submission for quick lookup
  const submissionMap = new Map(
    mySubmissions.map((s) => [s.assignmentId, s])
  );

  async function handleSubmit(formData: FormData) {
    "use server";
    const { auth: getAuth } = await import("@/lib/auth");
    const { db: database } = await import("@/lib/db");
    const { getAssignment } = await import("@/lib/assignments");
    const {
      createSubmission: create,
      getSubmissionByAssignmentAndStudent,
    } = await import("@/lib/submissions");
    const { listClassMembers: getMembers } = await import("@/lib/class-memberships");
    const sess = await getAuth();
    if (!sess?.user?.id) return;

    const assignmentId = formData.get("assignmentId") as string;
    if (!assignmentId) return;

    const a = await getAssignment(database, assignmentId);
    if (!a) return;

    // Verify student is in the class
    const m = await getMembers(database, a.classId);
    const isStudent = m.some(
      (mem) => mem.userId === sess.user.id && mem.role === "student"
    );
    if (!isStudent) return;

    // Check not already submitted
    const existing = await getSubmissionByAssignmentAndStudent(
      database,
      assignmentId,
      sess.user.id
    );
    if (existing) return;

    await create(database, {
      assignmentId,
      studentId: sess.user.id,
    });
    revalidatePath(`/student/classes/${id}`);
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{cls.title}</h1>
          <p className="text-muted-foreground">
            {course?.title || ""} · {cls.term || "No term"}
          </p>
        </div>
        <Link
          href={`/dashboard/classrooms/${id}/editor`}
          className={buttonVariants()}
        >
          Open Editor
        </Link>
      </div>

      {assignments.length > 0 && (
        <div className="space-y-4">
          <h2 className="text-lg font-semibold">Assignments ({assignments.length})</h2>
          <div className="space-y-2">
            {assignments.map((a) => {
              const submission = submissionMap.get(a.id);
              const isOverdue = a.dueDate && new Date(a.dueDate) < new Date();
              return (
                <Card key={a.id}>
                  <CardContent className="py-4">
                    <div className="flex items-center justify-between">
                      <div className="flex-1">
                        <p className="font-medium">{a.title}</p>
                        <p className="text-sm text-muted-foreground">
                          {a.description || "No description"}
                        </p>
                        {a.dueDate && (
                          <p className={`text-xs mt-1 ${isOverdue && !submission ? "text-destructive" : "text-muted-foreground"}`}>
                            Due: {new Date(a.dueDate).toLocaleDateString()}
                            {isOverdue && !submission && " (Overdue)"}
                          </p>
                        )}
                      </div>
                      <div className="ml-4 text-right">
                        {submission ? (
                          <div>
                            <span className="text-xs bg-green-100 dark:bg-green-900 text-green-700 dark:text-green-300 px-2 py-1 rounded">
                              Submitted
                            </span>
                            {submission.grade !== null && (
                              <p className="text-lg font-bold mt-1">{submission.grade}/100</p>
                            )}
                            {submission.feedback && (
                              <p className="text-xs text-muted-foreground mt-1 max-w-48 truncate">
                                {submission.feedback}
                              </p>
                            )}
                          </div>
                        ) : (
                          <form action={handleSubmit}>
                            <input type="hidden" name="assignmentId" value={a.id} />
                            <Button type="submit" size="sm">
                              Submit
                            </Button>
                          </form>
                        )}
                      </div>
                    </div>
                  </CardContent>
                </Card>
              );
            })}
          </div>
        </div>
      )}

      {topics.length > 0 && (
        <div className="space-y-4">
          <h2 className="text-lg font-semibold">Topics</h2>
          {topics.map((topic, i) => {
            const content = parseLessonContent(topic.lessonContent);
            return (
              <Card key={topic.id}>
                <CardHeader>
                  <CardTitle className="text-base">{i + 1}. {topic.title}</CardTitle>
                  {topic.description && (
                    <CardDescription>{topic.description}</CardDescription>
                  )}
                </CardHeader>
                {content.blocks.length > 0 && (
                  <CardContent>
                    <LessonRenderer content={content} />
                  </CardContent>
                )}
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}
```

- [ ] Replace `src/app/(portal)/student/classes/[id]/page.tsx` with updated version
- [ ] Verify no import errors
- [ ] Commit: `"Add assignments section to student class detail page with submit"`

---

## Task 11: Final verification

- [ ] Run full test suite: `bun run test`
- [ ] All tests pass (unit + integration)
- [ ] Run `bun run build` to verify TypeScript compilation
- [ ] Commit any remaining fixes if needed
- [ ] Final commit: `"Complete assignment system implementation (plan 012)"`

---

## Post-Execution Report

*(To be filled in after implementation)*

- [ ] All tasks completed
- [ ] All tests passing
- [ ] Build succeeds
- [ ] Migration applied

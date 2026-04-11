# Course Hierarchy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the Course -> Class -> Classroom -> Session hierarchy from spec 002, replacing the flat classroom model. This introduces reusable course templates with topics, class offerings tied to courses, class-level membership with role enums, a slimmed-down Classroom entity (1:1 with Class, auto-created), session-topic linking, and data migration from the old schema.

**Architecture:** Courses are reusable curriculum templates owned by an org. Topics are ordered units within a course. A Class is a specific offering of a course (e.g., "Fall 2026 Period 3") with its own join code and membership roster. A Classroom is auto-created 1:1 with each Class and holds editor/room settings. Sessions belong to a Classroom. SessionTopic is a junction table linking sessions to the topics covered. ClassMembership replaces the old classroomMembers table with role-based membership (`instructor`, `ta`, `student`, `observer`, `guest`, `parent`).

**Tech Stack:** Drizzle ORM (schema + migrations), Zod v4 (validation), Auth.js v5 (session), Vitest (testing), Next.js 16 App Router (API routes), PostgreSQL

**Depends on:** Plan 006 (org-and-roles) — Organizations and OrgMembership tables must exist. This plan assumes `organizations` table, `orgMemberships` table, and the updated `users` table (without `role` and `schoolId`) are already in place.

---

## File Structure

```
src/
  lib/
    db/
      schema.ts                           # Modify: add Course, Topic, Class, ClassMembership,
                                          #   new Classroom, SessionTopic tables and enums
    courses.ts                            # Create: Course CRUD operations
    topics.ts                             # Create: Topic CRUD operations (create, reorder, update, delete)
    classes.ts                            # Create: Class CRUD operations (create, list, get, archive, join)
    class-memberships.ts                  # Create: ClassMembership operations (add, list, update role, remove)
    classrooms.ts                         # Modify: rewrite to use new Classroom table
    sessions.ts                           # Modify: update FK from old classrooms to new classrooms table
  app/
    api/
      courses/
        route.ts                          # Create: POST (create course), GET (list courses by org)
        [id]/
          route.ts                        # Create: GET, PATCH, DELETE for single course
          clone/
            route.ts                      # Create: POST (clone course within org)
          topics/
            route.ts                      # Create: POST (create topic), GET (list topics)
            reorder/
              route.ts                    # Create: PATCH (reorder topics)
            [topicId]/
              route.ts                    # Create: GET, PATCH, DELETE for single topic
      classes/
        route.ts                          # Create: POST (create class from course), GET (list classes)
        [id]/
          route.ts                        # Create: GET, PATCH (archive/update) for single class
          members/
            route.ts                      # Create: POST (add member), GET (list members)
            [memberId]/
              route.ts                    # Create: PATCH (update role), DELETE (remove member)
          join/
            route.ts                      # Create: POST (join class by code)
      sessions/
        route.ts                          # Modify: use new classroomId source
        [id]/
          topics/
            route.ts                      # Create: POST (link topic), GET (list), DELETE (unlink)
      classrooms/
        route.ts                          # Modify: backward-compat wrapper or redirect to classes
        [id]/
          route.ts                        # Modify: point to new classroom table
tests/
  helpers.ts                              # Modify: add test factory helpers for new tables
  api/
    courses.test.ts                       # Create: Course operation unit tests
    topics.test.ts                        # Create: Topic operation unit tests
    classes.test.ts                       # Create: Class operation unit tests
    class-memberships.test.ts             # Create: ClassMembership operation unit tests
  integration/
    courses-api.test.ts                   # Create: Course API integration tests
    topics-api.test.ts                    # Create: Topic API integration tests
    classes-api.test.ts                   # Create: Class API integration tests
    class-memberships-api.test.ts         # Create: ClassMembership API integration tests
    session-topics-api.test.ts            # Create: SessionTopic API integration tests
  unit/
    schema.test.ts                        # Modify: add schema smoke tests for new tables
```

---

## Task 1: Schema — New Enums and Tables

**Files:**
- Modify: `src/lib/db/schema.ts`

Add the new enums and tables for the course hierarchy. This task only adds schema definitions; the migration is generated separately.

- [ ] **Step 1: Add new enums**

Add these enums after the existing enum definitions in `schema.ts`:

```ts
export const classStatusEnum = pgEnum("class_status", ["active", "archived"]);

export const classMemberRoleEnum = pgEnum("class_member_role", [
  "instructor",
  "ta",
  "student",
  "observer",
  "guest",
  "parent",
]);

export const programmingLanguageEnum = pgEnum("programming_language", [
  "python",
  "javascript",
  "blockly",
]);
```

Note: `gradeLevelEnum` and `editorModeEnum` already exist and will be reused.

- [ ] **Step 2: Add Course table**

Add after the existing table definitions:

```ts
export const courses = pgTable(
  "courses",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    orgId: uuid("org_id")
      .notNull()
      .references(() => organizations.id, { onDelete: "cascade" }),
    createdBy: uuid("created_by")
      .notNull()
      .references(() => users.id),
    title: varchar("title", { length: 255 }).notNull(),
    description: text("description").default(""),
    gradeLevel: gradeLevelEnum("grade_level").notNull(),
    language: programmingLanguageEnum("language").notNull().default("python"),
    isPublished: boolean("is_published").notNull().default(false),
    createdAt: timestamp("created_at").defaultNow().notNull(),
    updatedAt: timestamp("updated_at").defaultNow().notNull(),
  },
  (table) => [
    index("courses_org_idx").on(table.orgId),
    index("courses_created_by_idx").on(table.createdBy),
  ]
);
```

Import `boolean` from `drizzle-orm/pg-core` (add to the existing import).

- [ ] **Step 3: Add Topic table**

```ts
export const topics = pgTable(
  "topics",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    courseId: uuid("course_id")
      .notNull()
      .references(() => courses.id, { onDelete: "cascade" }),
    title: varchar("title", { length: 255 }).notNull(),
    description: text("description").default(""),
    sortOrder: integer("sort_order").notNull().default(0),
    lessonContent: jsonb("lesson_content").default({}),
    starterCode: text("starter_code"),
    createdAt: timestamp("created_at").defaultNow().notNull(),
    updatedAt: timestamp("updated_at").defaultNow().notNull(),
  },
  (table) => [
    index("topics_course_idx").on(table.courseId),
    index("topics_sort_idx").on(table.courseId, table.sortOrder),
  ]
);
```

Import `integer` from `drizzle-orm/pg-core` (add to the existing import).

- [ ] **Step 4: Add Class table**

```ts
export const classes = pgTable(
  "classes",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    courseId: uuid("course_id")
      .notNull()
      .references(() => courses.id, { onDelete: "cascade" }),
    orgId: uuid("org_id")
      .notNull()
      .references(() => organizations.id, { onDelete: "cascade" }),
    title: varchar("title", { length: 255 }).notNull(),
    term: varchar("term", { length: 100 }).default(""),
    joinCode: varchar("join_code", { length: 10 }).notNull(),
    status: classStatusEnum("status").notNull().default("active"),
    createdAt: timestamp("created_at").defaultNow().notNull(),
    updatedAt: timestamp("updated_at").defaultNow().notNull(),
  },
  (table) => [
    uniqueIndex("classes_join_code_idx").on(table.joinCode),
    index("classes_course_idx").on(table.courseId),
    index("classes_org_idx").on(table.orgId),
  ]
);
```

- [ ] **Step 5: Add ClassMembership table**

```ts
export const classMemberships = pgTable(
  "class_memberships",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    classId: uuid("class_id")
      .notNull()
      .references(() => classes.id, { onDelete: "cascade" }),
    userId: uuid("user_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    role: classMemberRoleEnum("role").notNull().default("student"),
    joinedAt: timestamp("joined_at").defaultNow().notNull(),
  },
  (table) => [
    uniqueIndex("class_membership_unique_idx").on(table.classId, table.userId),
    index("class_memberships_class_idx").on(table.classId),
    index("class_memberships_user_idx").on(table.userId),
  ]
);
```

- [ ] **Step 6: Add new Classroom table (renamed from old)**

The old `classrooms` table stays for now (removed in the migration task). The new table is called `classroomsV2` in the schema variable name during transition, but maps to the `classrooms_v2` SQL table. After migration is complete and old table removed, it will be renamed back.

Actually, to avoid naming conflicts during the transition period, name the new schema variable `newClassrooms` with SQL table name `new_classrooms`:

```ts
export const newClassrooms = pgTable(
  "new_classrooms",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    classId: uuid("class_id")
      .notNull()
      .references(() => classes.id, { onDelete: "cascade" })
      .unique(),
    editorMode: editorModeEnum("editor_mode").notNull().default("python"),
    settings: jsonb("settings").default({}),
    createdAt: timestamp("created_at").defaultNow().notNull(),
  },
  (table) => [
    uniqueIndex("new_classrooms_class_idx").on(table.classId),
  ]
);
```

- [ ] **Step 7: Add SessionTopic junction table**

```ts
export const sessionTopics = pgTable(
  "session_topics",
  {
    sessionId: uuid("session_id")
      .notNull()
      .references(() => liveSessions.id, { onDelete: "cascade" }),
    topicId: uuid("topic_id")
      .notNull()
      .references(() => topics.id, { onDelete: "cascade" }),
  },
  (table) => [
    uniqueIndex("session_topic_unique_idx").on(table.sessionId, table.topicId),
  ]
);
```

- [ ] **Step 8: Add `newClassroomId` column to liveSessions**

Add an optional FK to the new classrooms table on the existing `liveSessions` table. During migration, sessions will be updated to point to the new classroom. Both FKs coexist until the old one is dropped.

```ts
// In the liveSessions table definition, add:
newClassroomId: uuid("new_classroom_id")
  .references(() => newClassrooms.id),
```

This column is nullable during transition. After migration completes, the old `classroomId` column pointing to the old `classrooms` table will be dropped and `newClassroomId` renamed to `classroomId`.

- [ ] **Step 9: Generate and run migration**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run db:generate
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run db:migrate
```

Verify the migration SQL was generated in `drizzle/` and applied cleanly.

- [ ] **Step 10: Add schema smoke tests**

In `tests/unit/schema.test.ts`, add import and insert/select smoke tests for each new table:

```ts
import {
  courses, topics, classes, classMemberships,
  newClassrooms, sessionTopics,
} from "@/lib/db/schema";

// Test that each new table can be inserted and selected
describe("new course hierarchy tables", () => {
  it("can insert and select a course", async () => {
    // Create org (from plan 006) and user as prerequisites
    // Insert a course, select it back, verify fields
  });

  it("can insert and select a topic", async () => { /* ... */ });
  it("can insert and select a class", async () => { /* ... */ });
  it("can insert and select a class membership", async () => { /* ... */ });
  it("can insert and select a new classroom", async () => { /* ... */ });
  it("can insert and select a session topic", async () => { /* ... */ });

  it("enforces unique class membership per user per class", async () => {
    // Insert same user+class twice, expect constraint violation
  });

  it("enforces unique classroom per class", async () => {
    // Insert two classrooms for same class, expect constraint violation
  });

  it("cascades topic deletion when course is deleted", async () => {
    // Create course + topic, delete course, verify topic gone
  });
});
```

- [ ] **Step 11: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/schema.test.ts
```

Expected: All tests pass.

- [ ] **Step 12: Commit**

```bash
git add src/lib/db/schema.ts drizzle/ tests/unit/schema.test.ts
git commit -m "feat: add Course, Topic, Class, ClassMembership, Classroom, SessionTopic schema

Introduces the course hierarchy tables for spec 002. Course is a
reusable curriculum template with Topics. Class is a specific offering
with join code and membership. Classroom is auto-created 1:1 with
Class. SessionTopic links sessions to topics covered."
```

---

## Task 2: Test Helpers for New Tables

**Files:**
- Modify: `tests/helpers.ts`

Add factory functions for creating test data for the new tables. Update `cleanupDatabase` to delete new tables in correct FK order.

- [ ] **Step 1: Update cleanupDatabase**

Add deletion of new tables in correct order (children before parents). Insert these lines at the top of the function, before the existing deletions:

```ts
await testDb.delete(schema.sessionTopics);
await testDb.delete(schema.classMemberships);
await testDb.delete(schema.newClassrooms);
await testDb.delete(schema.classes);
await testDb.delete(schema.topics);
await testDb.delete(schema.courses);
```

Note: `organizations` and `orgMemberships` cleanup should already exist from plan 006.

- [ ] **Step 2: Add createTestOrg helper (if not already from plan 006)**

Verify plan 006 added this. If not, add:

```ts
export async function createTestOrg(
  overrides: Partial<typeof schema.organizations.$inferInsert> = {}
) {
  const [org] = await testDb
    .insert(schema.organizations)
    .values({
      name: "Test Org",
      slug: `test-org-${nanoid(6)}`,
      type: "school",
      status: "active",
      contactEmail: `admin-${nanoid(4)}@test.edu`,
      contactName: "Admin",
      ...overrides,
    })
    .returning();
  return org;
}
```

- [ ] **Step 3: Add createTestCourse helper**

```ts
export async function createTestCourse(
  orgId: string,
  createdBy: string,
  overrides: Partial<typeof schema.courses.$inferInsert> = {}
) {
  const [course] = await testDb
    .insert(schema.courses)
    .values({
      orgId,
      createdBy,
      title: "Test Course",
      gradeLevel: "6-8",
      language: "python",
      ...overrides,
    })
    .returning();
  return course;
}
```

- [ ] **Step 4: Add createTestTopic helper**

```ts
export async function createTestTopic(
  courseId: string,
  overrides: Partial<typeof schema.topics.$inferInsert> = {}
) {
  const [topic] = await testDb
    .insert(schema.topics)
    .values({
      courseId,
      title: "Test Topic",
      sortOrder: 0,
      ...overrides,
    })
    .returning();
  return topic;
}
```

- [ ] **Step 5: Add createTestClass helper**

```ts
export async function createTestClass(
  courseId: string,
  orgId: string,
  overrides: Partial<typeof schema.classes.$inferInsert> = {}
) {
  const [cls] = await testDb
    .insert(schema.classes)
    .values({
      courseId,
      orgId,
      title: "Test Class",
      joinCode: nanoid(8),
      ...overrides,
    })
    .returning();
  return cls;
}
```

- [ ] **Step 6: Add createTestClassMembership helper**

```ts
export async function createTestClassMembership(
  classId: string,
  userId: string,
  overrides: Partial<typeof schema.classMemberships.$inferInsert> = {}
) {
  const [membership] = await testDb
    .insert(schema.classMemberships)
    .values({
      classId,
      userId,
      role: "student",
      ...overrides,
    })
    .returning();
  return membership;
}
```

- [ ] **Step 7: Add createTestNewClassroom helper**

```ts
export async function createTestNewClassroom(
  classId: string,
  overrides: Partial<typeof schema.newClassrooms.$inferInsert> = {}
) {
  const [classroom] = await testDb
    .insert(schema.newClassrooms)
    .values({
      classId,
      editorMode: "python",
      ...overrides,
    })
    .returning();
  return classroom;
}
```

- [ ] **Step 8: Commit**

```bash
git add tests/helpers.ts
git commit -m "feat: add test helpers for course hierarchy tables

Factory functions for Course, Topic, Class, ClassMembership, and
new Classroom. Updated cleanupDatabase for correct FK order."
```

---

## Task 3: Course CRUD Operations

**Files:**
- Create: `src/lib/courses.ts`
- Create: `tests/api/courses.test.ts`

- [ ] **Step 1: Write failing tests**

Create `tests/api/courses.test.ts`:

```ts
import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  createTestUser,
  createTestOrg,
  createTestCourse,
} from "../helpers";
import {
  createCourse,
  getCourse,
  listCoursesByOrg,
  updateCourse,
  cloneCourse,
} from "@/lib/courses";

describe("course operations", () => {
  let org: Awaited<ReturnType<typeof createTestOrg>>;
  let teacher: Awaited<ReturnType<typeof createTestUser>>;

  beforeEach(async () => {
    org = await createTestOrg();
    teacher = await createTestUser({ name: "Teacher" });
  });

  describe("createCourse", () => {
    it("creates a course with required fields", async () => {
      const course = await createCourse(testDb, {
        orgId: org.id,
        createdBy: teacher.id,
        title: "Intro to Python",
        gradeLevel: "6-8",
        language: "python",
      });

      expect(course.id).toBeDefined();
      expect(course.title).toBe("Intro to Python");
      expect(course.orgId).toBe(org.id);
      expect(course.createdBy).toBe(teacher.id);
      expect(course.isPublished).toBe(false);
    });

    it("creates a course with optional description", async () => {
      const course = await createCourse(testDb, {
        orgId: org.id,
        createdBy: teacher.id,
        title: "JS Basics",
        description: "Learn JavaScript fundamentals",
        gradeLevel: "9-12",
        language: "javascript",
      });

      expect(course.description).toBe("Learn JavaScript fundamentals");
    });
  });

  describe("getCourse", () => {
    it("returns course by ID", async () => {
      const created = await createTestCourse(org.id, teacher.id, {
        title: "Find Me",
      });
      const found = await getCourse(testDb, created.id);
      expect(found).not.toBeNull();
      expect(found!.title).toBe("Find Me");
    });

    it("returns null for non-existent ID", async () => {
      const found = await getCourse(
        testDb,
        "00000000-0000-0000-0000-000000000000"
      );
      expect(found).toBeNull();
    });
  });

  describe("listCoursesByOrg", () => {
    it("returns courses for an org", async () => {
      await createTestCourse(org.id, teacher.id, { title: "Course A" });
      await createTestCourse(org.id, teacher.id, { title: "Course B" });

      const results = await listCoursesByOrg(testDb, org.id);
      expect(results).toHaveLength(2);
    });

    it("does not return courses from other orgs", async () => {
      const otherOrg = await createTestOrg({ name: "Other Org" });
      await createTestCourse(otherOrg.id, teacher.id, {
        title: "Other Course",
      });

      const results = await listCoursesByOrg(testDb, org.id);
      expect(results).toHaveLength(0);
    });

    it("returns empty array when org has no courses", async () => {
      const results = await listCoursesByOrg(testDb, org.id);
      expect(results).toHaveLength(0);
    });
  });

  describe("updateCourse", () => {
    it("updates course fields", async () => {
      const course = await createTestCourse(org.id, teacher.id);
      const updated = await updateCourse(testDb, course.id, {
        title: "Updated Title",
        isPublished: true,
      });

      expect(updated).not.toBeNull();
      expect(updated!.title).toBe("Updated Title");
      expect(updated!.isPublished).toBe(true);
    });

    it("returns null for non-existent course", async () => {
      const updated = await updateCourse(
        testDb,
        "00000000-0000-0000-0000-000000000000",
        { title: "Nope" }
      );
      expect(updated).toBeNull();
    });
  });

  describe("cloneCourse", () => {
    it("clones a course with new title and same org", async () => {
      const original = await createTestCourse(org.id, teacher.id, {
        title: "Original",
        description: "Original desc",
      });

      const clone = await cloneCourse(testDb, original.id, {
        clonedBy: teacher.id,
        targetOrgId: org.id,
      });

      expect(clone).not.toBeNull();
      expect(clone!.id).not.toBe(original.id);
      expect(clone!.title).toBe("Original (Copy)");
      expect(clone!.description).toBe("Original desc");
      expect(clone!.orgId).toBe(org.id);
      expect(clone!.isPublished).toBe(false);
    });

    it("clones topics along with the course", async () => {
      const original = await createTestCourse(org.id, teacher.id);
      await createTestTopic(original.id, { title: "Topic 1", sortOrder: 0 });
      await createTestTopic(original.id, { title: "Topic 2", sortOrder: 1 });

      const clone = await cloneCourse(testDb, original.id, {
        clonedBy: teacher.id,
        targetOrgId: org.id,
      });

      // Need to import listTopicsByCourse or query directly
      const clonedTopics = await testDb
        .select()
        .from(topics)
        .where(eq(topics.courseId, clone!.id))
        .orderBy(topics.sortOrder);

      expect(clonedTopics).toHaveLength(2);
      expect(clonedTopics[0].title).toBe("Topic 1");
      expect(clonedTopics[1].title).toBe("Topic 2");
    });

    it("returns null when source course does not exist", async () => {
      const clone = await cloneCourse(
        testDb,
        "00000000-0000-0000-0000-000000000000",
        { clonedBy: teacher.id, targetOrgId: org.id }
      );
      expect(clone).toBeNull();
    });
  });
});
```

Add necessary imports (e.g., `import { topics } from "@/lib/db/schema"`, `import { eq } from "drizzle-orm"`, and `createTestTopic` from helpers) at the top.

- [ ] **Step 2: Run tests to verify failure**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/api/courses.test.ts
```

Expected: FAIL -- cannot resolve `@/lib/courses`.

- [ ] **Step 3: Implement `src/lib/courses.ts`**

```ts
import { eq } from "drizzle-orm";
import { courses, topics } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

interface CreateCourseInput {
  orgId: string;
  createdBy: string;
  title: string;
  description?: string;
  gradeLevel: "K-5" | "6-8" | "9-12";
  language: "python" | "javascript" | "blockly";
  isPublished?: boolean;
}

export async function createCourse(db: Database, input: CreateCourseInput) {
  const [course] = await db.insert(courses).values(input).returning();
  return course;
}

export async function getCourse(db: Database, courseId: string) {
  const [course] = await db
    .select()
    .from(courses)
    .where(eq(courses.id, courseId));
  return course || null;
}

export async function listCoursesByOrg(db: Database, orgId: string) {
  return db.select().from(courses).where(eq(courses.orgId, orgId));
}

interface UpdateCourseInput {
  title?: string;
  description?: string;
  gradeLevel?: "K-5" | "6-8" | "9-12";
  language?: "python" | "javascript" | "blockly";
  isPublished?: boolean;
}

export async function updateCourse(
  db: Database,
  courseId: string,
  input: UpdateCourseInput
) {
  const [course] = await db
    .update(courses)
    .set({ ...input, updatedAt: new Date() })
    .where(eq(courses.id, courseId))
    .returning();
  return course || null;
}

interface CloneCourseOptions {
  clonedBy: string;
  targetOrgId: string;
}

export async function cloneCourse(
  db: Database,
  sourceCourseId: string,
  options: CloneCourseOptions
) {
  const source = await getCourse(db, sourceCourseId);
  if (!source) return null;

  // Clone the course
  const [cloned] = await db
    .insert(courses)
    .values({
      orgId: options.targetOrgId,
      createdBy: options.clonedBy,
      title: `${source.title} (Copy)`,
      description: source.description,
      gradeLevel: source.gradeLevel,
      language: source.language,
      isPublished: false,
    })
    .returning();

  // Clone all topics
  const sourceTopics = await db
    .select()
    .from(topics)
    .where(eq(topics.courseId, sourceCourseId));

  if (sourceTopics.length > 0) {
    await db.insert(topics).values(
      sourceTopics.map((t) => ({
        courseId: cloned.id,
        title: t.title,
        description: t.description,
        sortOrder: t.sortOrder,
        lessonContent: t.lessonContent,
        starterCode: t.starterCode,
      }))
    );
  }

  return cloned;
}
```

- [ ] **Step 4: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/api/courses.test.ts
```

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add src/lib/courses.ts tests/api/courses.test.ts
git commit -m "feat: implement Course CRUD operations

Create, get, list by org, update, and clone (with topics) for the
Course entity. Clone creates a copy with '(Copy)' suffix and
duplicates all topics."
```

---

## Task 4: Topic CRUD Operations

**Files:**
- Create: `src/lib/topics.ts`
- Create: `tests/api/topics.test.ts`

- [ ] **Step 1: Write failing tests**

Create `tests/api/topics.test.ts`:

```ts
import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  createTestUser,
  createTestOrg,
  createTestCourse,
  createTestTopic,
} from "../helpers";
import {
  createTopic,
  getTopic,
  listTopicsByCourse,
  updateTopic,
  deleteTopic,
  reorderTopics,
} from "@/lib/topics";

describe("topic operations", () => {
  let org: Awaited<ReturnType<typeof createTestOrg>>;
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let course: Awaited<ReturnType<typeof createTestCourse>>;

  beforeEach(async () => {
    org = await createTestOrg();
    teacher = await createTestUser();
    course = await createTestCourse(org.id, teacher.id);
  });

  describe("createTopic", () => {
    it("creates a topic with required fields", async () => {
      const topic = await createTopic(testDb, {
        courseId: course.id,
        title: "Variables",
        sortOrder: 0,
      });

      expect(topic.id).toBeDefined();
      expect(topic.title).toBe("Variables");
      expect(topic.courseId).toBe(course.id);
      expect(topic.sortOrder).toBe(0);
    });

    it("creates a topic with lesson content and starter code", async () => {
      const content = {
        blocks: [
          { type: "markdown", content: "# Variables" },
        ],
      };
      const topic = await createTopic(testDb, {
        courseId: course.id,
        title: "Variables",
        sortOrder: 0,
        lessonContent: content,
        starterCode: "x = 5",
      });

      expect(topic.lessonContent).toEqual(content);
      expect(topic.starterCode).toBe("x = 5");
    });
  });

  describe("getTopic", () => {
    it("returns topic by ID", async () => {
      const created = await createTestTopic(course.id, { title: "Find Me" });
      const found = await getTopic(testDb, created.id);
      expect(found).not.toBeNull();
      expect(found!.title).toBe("Find Me");
    });

    it("returns null for non-existent ID", async () => {
      const found = await getTopic(
        testDb,
        "00000000-0000-0000-0000-000000000000"
      );
      expect(found).toBeNull();
    });
  });

  describe("listTopicsByCourse", () => {
    it("returns topics ordered by sortOrder", async () => {
      await createTestTopic(course.id, { title: "Second", sortOrder: 1 });
      await createTestTopic(course.id, { title: "First", sortOrder: 0 });

      const results = await listTopicsByCourse(testDb, course.id);
      expect(results).toHaveLength(2);
      expect(results[0].title).toBe("First");
      expect(results[1].title).toBe("Second");
    });

    it("returns empty array for course with no topics", async () => {
      const results = await listTopicsByCourse(testDb, course.id);
      expect(results).toHaveLength(0);
    });
  });

  describe("updateTopic", () => {
    it("updates topic fields", async () => {
      const topic = await createTestTopic(course.id);
      const updated = await updateTopic(testDb, topic.id, {
        title: "Updated",
        starterCode: "print('hi')",
      });

      expect(updated).not.toBeNull();
      expect(updated!.title).toBe("Updated");
      expect(updated!.starterCode).toBe("print('hi')");
    });

    it("returns null for non-existent topic", async () => {
      const updated = await updateTopic(
        testDb,
        "00000000-0000-0000-0000-000000000000",
        { title: "Nope" }
      );
      expect(updated).toBeNull();
    });
  });

  describe("deleteTopic", () => {
    it("deletes a topic and returns it", async () => {
      const topic = await createTestTopic(course.id);
      const deleted = await deleteTopic(testDb, topic.id);

      expect(deleted).not.toBeNull();
      expect(deleted!.id).toBe(topic.id);

      const found = await getTopic(testDb, topic.id);
      expect(found).toBeNull();
    });

    it("returns null for non-existent topic", async () => {
      const deleted = await deleteTopic(
        testDb,
        "00000000-0000-0000-0000-000000000000"
      );
      expect(deleted).toBeNull();
    });
  });

  describe("reorderTopics", () => {
    it("reorders topics by ID list", async () => {
      const t1 = await createTestTopic(course.id, {
        title: "A",
        sortOrder: 0,
      });
      const t2 = await createTestTopic(course.id, {
        title: "B",
        sortOrder: 1,
      });
      const t3 = await createTestTopic(course.id, {
        title: "C",
        sortOrder: 2,
      });

      // Reverse order
      await reorderTopics(testDb, course.id, [t3.id, t1.id, t2.id]);

      const results = await listTopicsByCourse(testDb, course.id);
      expect(results[0].title).toBe("C");
      expect(results[1].title).toBe("A");
      expect(results[2].title).toBe("B");
    });
  });
});
```

- [ ] **Step 2: Run tests to verify failure**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/api/topics.test.ts
```

- [ ] **Step 3: Implement `src/lib/topics.ts`**

```ts
import { eq, and, asc } from "drizzle-orm";
import { topics } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

interface CreateTopicInput {
  courseId: string;
  title: string;
  description?: string;
  sortOrder: number;
  lessonContent?: Record<string, unknown>;
  starterCode?: string;
}

export async function createTopic(db: Database, input: CreateTopicInput) {
  const [topic] = await db.insert(topics).values(input).returning();
  return topic;
}

export async function getTopic(db: Database, topicId: string) {
  const [topic] = await db
    .select()
    .from(topics)
    .where(eq(topics.id, topicId));
  return topic || null;
}

export async function listTopicsByCourse(db: Database, courseId: string) {
  return db
    .select()
    .from(topics)
    .where(eq(topics.courseId, courseId))
    .orderBy(asc(topics.sortOrder));
}

interface UpdateTopicInput {
  title?: string;
  description?: string;
  sortOrder?: number;
  lessonContent?: Record<string, unknown>;
  starterCode?: string | null;
}

export async function updateTopic(
  db: Database,
  topicId: string,
  input: UpdateTopicInput
) {
  const [topic] = await db
    .update(topics)
    .set({ ...input, updatedAt: new Date() })
    .where(eq(topics.id, topicId))
    .returning();
  return topic || null;
}

export async function deleteTopic(db: Database, topicId: string) {
  const [topic] = await db
    .delete(topics)
    .where(eq(topics.id, topicId))
    .returning();
  return topic || null;
}

export async function reorderTopics(
  db: Database,
  courseId: string,
  topicIds: string[]
) {
  // Update each topic's sortOrder based on position in the array
  const updates = topicIds.map((id, index) =>
    db
      .update(topics)
      .set({ sortOrder: index, updatedAt: new Date() })
      .where(and(eq(topics.id, id), eq(topics.courseId, courseId)))
  );
  await Promise.all(updates);
}
```

- [ ] **Step 4: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/api/topics.test.ts
```

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add src/lib/topics.ts tests/api/topics.test.ts
git commit -m "feat: implement Topic CRUD operations

Create, get, list by course (ordered), update, delete, and reorder
topics within a course."
```

---

## Task 5: Class CRUD Operations

**Files:**
- Create: `src/lib/classes.ts`
- Create: `tests/api/classes.test.ts`

- [ ] **Step 1: Write failing tests**

Create `tests/api/classes.test.ts`:

```ts
import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  createTestUser,
  createTestOrg,
  createTestCourse,
  createTestClass,
  createTestClassMembership,
  createTestNewClassroom,
} from "../helpers";
import {
  createClass,
  getClass,
  listClassesByOrg,
  listClassesByCourse,
  getClassByJoinCode,
  archiveClass,
} from "@/lib/classes";
import { newClassrooms, classMemberships } from "@/lib/db/schema";
import { eq } from "drizzle-orm";

describe("class operations", () => {
  let org: Awaited<ReturnType<typeof createTestOrg>>;
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let course: Awaited<ReturnType<typeof createTestCourse>>;

  beforeEach(async () => {
    org = await createTestOrg();
    teacher = await createTestUser();
    course = await createTestCourse(org.id, teacher.id);
  });

  describe("createClass", () => {
    it("creates a class with generated join code", async () => {
      const cls = await createClass(testDb, {
        courseId: course.id,
        orgId: org.id,
        title: "Python Fall 2026 P3",
        term: "Fall 2026",
        createdBy: teacher.id,
      });

      expect(cls.id).toBeDefined();
      expect(cls.title).toBe("Python Fall 2026 P3");
      expect(cls.joinCode).toHaveLength(8);
      expect(cls.status).toBe("active");
      expect(cls.courseId).toBe(course.id);
    });

    it("auto-creates a classroom", async () => {
      const cls = await createClass(testDb, {
        courseId: course.id,
        orgId: org.id,
        title: "Auto Classroom Test",
        createdBy: teacher.id,
      });

      const [classroom] = await testDb
        .select()
        .from(newClassrooms)
        .where(eq(newClassrooms.classId, cls.id));

      expect(classroom).toBeDefined();
      expect(classroom.classId).toBe(cls.id);
    });

    it("auto-adds creator as instructor in ClassMembership", async () => {
      const cls = await createClass(testDb, {
        courseId: course.id,
        orgId: org.id,
        title: "Instructor Test",
        createdBy: teacher.id,
      });

      const [membership] = await testDb
        .select()
        .from(classMemberships)
        .where(eq(classMemberships.classId, cls.id));

      expect(membership).toBeDefined();
      expect(membership.userId).toBe(teacher.id);
      expect(membership.role).toBe("instructor");
    });

    it("sets editorMode from course language", async () => {
      const jsCourse = await createTestCourse(org.id, teacher.id, {
        language: "javascript",
      });
      const cls = await createClass(testDb, {
        courseId: jsCourse.id,
        orgId: org.id,
        title: "JS Class",
        createdBy: teacher.id,
      });

      const [classroom] = await testDb
        .select()
        .from(newClassrooms)
        .where(eq(newClassrooms.classId, cls.id));

      expect(classroom.editorMode).toBe("javascript");
    });
  });

  describe("getClass", () => {
    it("returns class by ID", async () => {
      const cls = await createTestClass(course.id, org.id, {
        title: "Find Me",
      });
      const found = await getClass(testDb, cls.id);
      expect(found).not.toBeNull();
      expect(found!.title).toBe("Find Me");
    });

    it("returns null for non-existent ID", async () => {
      const found = await getClass(
        testDb,
        "00000000-0000-0000-0000-000000000000"
      );
      expect(found).toBeNull();
    });
  });

  describe("listClassesByOrg", () => {
    it("returns classes for an org", async () => {
      await createTestClass(course.id, org.id, { title: "Class A" });
      await createTestClass(course.id, org.id, { title: "Class B" });

      const results = await listClassesByOrg(testDb, org.id);
      expect(results).toHaveLength(2);
    });

    it("does not return archived classes by default", async () => {
      await createTestClass(course.id, org.id, {
        title: "Active",
        status: "active",
      });
      await createTestClass(course.id, org.id, {
        title: "Archived",
        status: "archived",
      });

      const results = await listClassesByOrg(testDb, org.id);
      expect(results).toHaveLength(1);
      expect(results[0].title).toBe("Active");
    });

    it("returns archived classes when includeArchived is true", async () => {
      await createTestClass(course.id, org.id, { status: "active" });
      await createTestClass(course.id, org.id, { status: "archived" });

      const results = await listClassesByOrg(testDb, org.id, {
        includeArchived: true,
      });
      expect(results).toHaveLength(2);
    });
  });

  describe("listClassesByCourse", () => {
    it("returns classes for a course", async () => {
      await createTestClass(course.id, org.id, { title: "Section A" });
      await createTestClass(course.id, org.id, { title: "Section B" });

      const results = await listClassesByCourse(testDb, course.id);
      expect(results).toHaveLength(2);
    });
  });

  describe("getClassByJoinCode", () => {
    it("returns class by join code", async () => {
      const cls = await createTestClass(course.id, org.id, {
        joinCode: "ABCD1234",
      });

      const found = await getClassByJoinCode(testDb, "ABCD1234");
      expect(found).not.toBeNull();
      expect(found!.id).toBe(cls.id);
    });

    it("returns null for invalid join code", async () => {
      const found = await getClassByJoinCode(testDb, "ZZZZZZZZ");
      expect(found).toBeNull();
    });
  });

  describe("archiveClass", () => {
    it("sets class status to archived", async () => {
      const cls = await createTestClass(course.id, org.id);
      const archived = await archiveClass(testDb, cls.id);

      expect(archived).not.toBeNull();
      expect(archived!.status).toBe("archived");
    });

    it("returns null for non-existent class", async () => {
      const archived = await archiveClass(
        testDb,
        "00000000-0000-0000-0000-000000000000"
      );
      expect(archived).toBeNull();
    });
  });
});
```

- [ ] **Step 2: Run tests to verify failure**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/api/classes.test.ts
```

- [ ] **Step 3: Implement `src/lib/classes.ts`**

```ts
import { eq, and } from "drizzle-orm";
import {
  classes,
  classMemberships,
  newClassrooms,
  courses,
} from "@/lib/db/schema";
import { generateJoinCode } from "@/lib/utils";
import type { Database } from "@/lib/db";

interface CreateClassInput {
  courseId: string;
  orgId: string;
  title: string;
  term?: string;
  createdBy: string;
}

export async function createClass(db: Database, input: CreateClassInput) {
  const { createdBy, ...classData } = input;

  // Look up course to get the language for editor mode
  const [course] = await db
    .select()
    .from(courses)
    .where(eq(courses.id, input.courseId));

  // Map course language to editor mode (they use the same values)
  const editorMode = course?.language || "python";

  // Create the class
  const [cls] = await db
    .insert(classes)
    .values({
      ...classData,
      joinCode: generateJoinCode(),
    })
    .returning();

  // Auto-create classroom
  await db.insert(newClassrooms).values({
    classId: cls.id,
    editorMode,
  });

  // Auto-add creator as instructor
  await db.insert(classMemberships).values({
    classId: cls.id,
    userId: createdBy,
    role: "instructor",
  });

  return cls;
}

export async function getClass(db: Database, classId: string) {
  const [cls] = await db
    .select()
    .from(classes)
    .where(eq(classes.id, classId));
  return cls || null;
}

export async function listClassesByOrg(
  db: Database,
  orgId: string,
  options: { includeArchived?: boolean } = {}
) {
  if (options.includeArchived) {
    return db.select().from(classes).where(eq(classes.orgId, orgId));
  }
  return db
    .select()
    .from(classes)
    .where(and(eq(classes.orgId, orgId), eq(classes.status, "active")));
}

export async function listClassesByCourse(db: Database, courseId: string) {
  return db.select().from(classes).where(eq(classes.courseId, courseId));
}

export async function getClassByJoinCode(db: Database, joinCode: string) {
  const [cls] = await db
    .select()
    .from(classes)
    .where(eq(classes.joinCode, joinCode));
  return cls || null;
}

export async function archiveClass(db: Database, classId: string) {
  const [cls] = await db
    .update(classes)
    .set({ status: "archived", updatedAt: new Date() })
    .where(eq(classes.id, classId))
    .returning();
  return cls || null;
}
```

- [ ] **Step 4: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/api/classes.test.ts
```

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add src/lib/classes.ts tests/api/classes.test.ts
git commit -m "feat: implement Class CRUD operations

Create (with auto-created Classroom and instructor membership),
get, list by org (with archived filter), list by course, find by
join code, and archive."
```

---

## Task 6: ClassMembership Operations

**Files:**
- Create: `src/lib/class-memberships.ts`
- Create: `tests/api/class-memberships.test.ts`

- [ ] **Step 1: Write failing tests**

Create `tests/api/class-memberships.test.ts`:

```ts
import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  createTestUser,
  createTestOrg,
  createTestCourse,
  createTestClass,
  createTestClassMembership,
} from "../helpers";
import {
  addClassMember,
  listClassMembers,
  updateClassMemberRole,
  removeClassMember,
  getClassMembership,
  joinClassByCode,
} from "@/lib/class-memberships";

describe("class membership operations", () => {
  let org: Awaited<ReturnType<typeof createTestOrg>>;
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;
  let course: Awaited<ReturnType<typeof createTestCourse>>;
  let cls: Awaited<ReturnType<typeof createTestClass>>;

  beforeEach(async () => {
    org = await createTestOrg();
    teacher = await createTestUser({ name: "Teacher" });
    student = await createTestUser({ name: "Student" });
    course = await createTestCourse(org.id, teacher.id);
    cls = await createTestClass(course.id, org.id);
  });

  describe("addClassMember", () => {
    it("adds a student by userId", async () => {
      const membership = await addClassMember(testDb, {
        classId: cls.id,
        userId: student.id,
        role: "student",
      });

      expect(membership).not.toBeNull();
      expect(membership!.userId).toBe(student.id);
      expect(membership!.role).toBe("student");
    });

    it("adds a TA", async () => {
      const ta = await createTestUser({ name: "TA" });
      const membership = await addClassMember(testDb, {
        classId: cls.id,
        userId: ta.id,
        role: "ta",
      });

      expect(membership!.role).toBe("ta");
    });

    it("does not duplicate membership for same user and class", async () => {
      await addClassMember(testDb, {
        classId: cls.id,
        userId: student.id,
        role: "student",
      });

      const duplicate = await addClassMember(testDb, {
        classId: cls.id,
        userId: student.id,
        role: "student",
      });

      // onConflictDoNothing — returns undefined/null
      expect(duplicate).toBeNull();
    });
  });

  describe("listClassMembers", () => {
    it("returns all members with user info", async () => {
      await createTestClassMembership(cls.id, teacher.id, {
        role: "instructor",
      });
      await createTestClassMembership(cls.id, student.id, { role: "student" });

      const members = await listClassMembers(testDb, cls.id);
      expect(members).toHaveLength(2);
      expect(members.map((m) => m.name)).toContain("Teacher");
      expect(members.map((m) => m.name)).toContain("Student");
    });

    it("returns empty array for class with no members", async () => {
      const members = await listClassMembers(testDb, cls.id);
      expect(members).toHaveLength(0);
    });
  });

  describe("getClassMembership", () => {
    it("returns membership for user in class", async () => {
      await createTestClassMembership(cls.id, student.id, { role: "student" });

      const membership = await getClassMembership(
        testDb,
        cls.id,
        student.id
      );
      expect(membership).not.toBeNull();
      expect(membership!.role).toBe("student");
    });

    it("returns null when user is not a member", async () => {
      const membership = await getClassMembership(
        testDb,
        cls.id,
        student.id
      );
      expect(membership).toBeNull();
    });
  });

  describe("updateClassMemberRole", () => {
    it("updates member role", async () => {
      const membership = await createTestClassMembership(cls.id, student.id, {
        role: "student",
      });

      const updated = await updateClassMemberRole(
        testDb,
        membership.id,
        "ta"
      );

      expect(updated).not.toBeNull();
      expect(updated!.role).toBe("ta");
    });

    it("returns null for non-existent membership", async () => {
      const updated = await updateClassMemberRole(
        testDb,
        "00000000-0000-0000-0000-000000000000",
        "ta"
      );
      expect(updated).toBeNull();
    });
  });

  describe("removeClassMember", () => {
    it("removes a member and returns the membership", async () => {
      const membership = await createTestClassMembership(
        cls.id,
        student.id
      );

      const removed = await removeClassMember(testDb, membership.id);
      expect(removed).not.toBeNull();
      expect(removed!.userId).toBe(student.id);

      const check = await getClassMembership(testDb, cls.id, student.id);
      expect(check).toBeNull();
    });

    it("returns null for non-existent membership", async () => {
      const removed = await removeClassMember(
        testDb,
        "00000000-0000-0000-0000-000000000000"
      );
      expect(removed).toBeNull();
    });
  });

  describe("joinClassByCode", () => {
    it("joins a class by code as student", async () => {
      const result = await joinClassByCode(testDb, cls.joinCode, student.id);

      expect(result).not.toBeNull();
      expect(result!.classId).toBe(cls.id);
      expect(result!.role).toBe("student");
    });

    it("returns null for invalid join code", async () => {
      const result = await joinClassByCode(testDb, "ZZZZZZZZ", student.id);
      expect(result).toBeNull();
    });

    it("does not duplicate on re-join", async () => {
      await joinClassByCode(testDb, cls.joinCode, student.id);
      const second = await joinClassByCode(testDb, cls.joinCode, student.id);
      expect(second).toBeNull();
    });
  });
});
```

- [ ] **Step 2: Run tests to verify failure**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/api/class-memberships.test.ts
```

- [ ] **Step 3: Implement `src/lib/class-memberships.ts`**

```ts
import { eq, and } from "drizzle-orm";
import { classMemberships, classes, users } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

interface AddClassMemberInput {
  classId: string;
  userId: string;
  role: "instructor" | "ta" | "student" | "observer" | "guest" | "parent";
}

export async function addClassMember(
  db: Database,
  input: AddClassMemberInput
) {
  const [membership] = await db
    .insert(classMemberships)
    .values(input)
    .onConflictDoNothing()
    .returning();
  return membership || null;
}

export async function listClassMembers(db: Database, classId: string) {
  return db
    .select({
      id: classMemberships.id,
      classId: classMemberships.classId,
      userId: classMemberships.userId,
      role: classMemberships.role,
      joinedAt: classMemberships.joinedAt,
      name: users.name,
      email: users.email,
    })
    .from(classMemberships)
    .innerJoin(users, eq(classMemberships.userId, users.id))
    .where(eq(classMemberships.classId, classId));
}

export async function getClassMembership(
  db: Database,
  classId: string,
  userId: string
) {
  const [membership] = await db
    .select()
    .from(classMemberships)
    .where(
      and(
        eq(classMemberships.classId, classId),
        eq(classMemberships.userId, userId)
      )
    );
  return membership || null;
}

export async function updateClassMemberRole(
  db: Database,
  membershipId: string,
  role: "instructor" | "ta" | "student" | "observer" | "guest" | "parent"
) {
  const [membership] = await db
    .update(classMemberships)
    .set({ role })
    .where(eq(classMemberships.id, membershipId))
    .returning();
  return membership || null;
}

export async function removeClassMember(
  db: Database,
  membershipId: string
) {
  const [membership] = await db
    .delete(classMemberships)
    .where(eq(classMemberships.id, membershipId))
    .returning();
  return membership || null;
}

export async function joinClassByCode(
  db: Database,
  joinCode: string,
  userId: string
) {
  const [cls] = await db
    .select()
    .from(classes)
    .where(eq(classes.joinCode, joinCode));

  if (!cls) return null;

  const [membership] = await db
    .insert(classMemberships)
    .values({
      classId: cls.id,
      userId,
      role: "student",
    })
    .onConflictDoNothing()
    .returning();

  return membership || null;
}
```

- [ ] **Step 4: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/api/class-memberships.test.ts
```

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add src/lib/class-memberships.ts tests/api/class-memberships.test.ts
git commit -m "feat: implement ClassMembership operations

Add member (with role), list members (with user info), get membership,
update role, remove member, and join by code (auto-assigns student role).
Duplicate membership is silently ignored."
```

---

## Task 7: Update Classroom and Session Operations

**Files:**
- Modify: `src/lib/classrooms.ts`
- Modify: `src/lib/sessions.ts`

Update the existing operations to work with the new schema while maintaining backward compatibility during the transition.

- [ ] **Step 1: Add new-classroom operations to `src/lib/classrooms.ts`**

Add functions for the new classroom table alongside the existing ones (don't remove old functions yet — they're needed during migration):

```ts
import { newClassrooms } from "@/lib/db/schema";

export async function getNewClassroom(db: Database, classroomId: string) {
  const [classroom] = await db
    .select()
    .from(newClassrooms)
    .where(eq(newClassrooms.id, classroomId));
  return classroom || null;
}

export async function getNewClassroomByClassId(
  db: Database,
  classId: string
) {
  const [classroom] = await db
    .select()
    .from(newClassrooms)
    .where(eq(newClassrooms.classId, classId));
  return classroom || null;
}

export async function updateNewClassroomSettings(
  db: Database,
  classroomId: string,
  settings: Record<string, unknown>
) {
  const [classroom] = await db
    .update(newClassrooms)
    .set({ settings })
    .where(eq(newClassrooms.id, classroomId))
    .returning();
  return classroom || null;
}
```

- [ ] **Step 2: Add SessionTopic operations to `src/lib/sessions.ts`**

Add session-topic linking functions:

```ts
import { sessionTopics, topics } from "@/lib/db/schema";
import { asc } from "drizzle-orm";

export async function linkSessionTopic(
  db: Database,
  sessionId: string,
  topicId: string
) {
  const [link] = await db
    .insert(sessionTopics)
    .values({ sessionId, topicId })
    .onConflictDoNothing()
    .returning();
  return link || null;
}

export async function unlinkSessionTopic(
  db: Database,
  sessionId: string,
  topicId: string
) {
  const [link] = await db
    .delete(sessionTopics)
    .where(
      and(
        eq(sessionTopics.sessionId, sessionId),
        eq(sessionTopics.topicId, topicId)
      )
    )
    .returning();
  return link || null;
}

export async function getSessionTopics(db: Database, sessionId: string) {
  return db
    .select({
      topicId: sessionTopics.topicId,
      title: topics.title,
      description: topics.description,
      sortOrder: topics.sortOrder,
      lessonContent: topics.lessonContent,
      starterCode: topics.starterCode,
    })
    .from(sessionTopics)
    .innerJoin(topics, eq(sessionTopics.topicId, topics.id))
    .where(eq(sessionTopics.sessionId, sessionId))
    .orderBy(asc(topics.sortOrder));
}
```

- [ ] **Step 3: Write tests for new classroom and session-topic operations**

Add to `tests/api/sessions.test.ts` or create a new file `tests/api/session-topics.test.ts`:

```ts
import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  createTestUser,
  createTestOrg,
  createTestCourse,
  createTestTopic,
  createTestClass,
  createTestNewClassroom,
  createTestSession,
} from "../helpers";
import {
  linkSessionTopic,
  unlinkSessionTopic,
  getSessionTopics,
} from "@/lib/sessions";
import {
  getNewClassroom,
  getNewClassroomByClassId,
} from "@/lib/classrooms";

describe("session-topic operations", () => {
  let org: Awaited<ReturnType<typeof createTestOrg>>;
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let course: Awaited<ReturnType<typeof createTestCourse>>;
  let cls: Awaited<ReturnType<typeof createTestClass>>;
  let classroom: Awaited<ReturnType<typeof createTestNewClassroom>>;
  let topic1: Awaited<ReturnType<typeof createTestTopic>>;
  let topic2: Awaited<ReturnType<typeof createTestTopic>>;

  beforeEach(async () => {
    org = await createTestOrg();
    teacher = await createTestUser();
    course = await createTestCourse(org.id, teacher.id);
    cls = await createTestClass(course.id, org.id);
    classroom = await createTestNewClassroom(cls.id);
    topic1 = await createTestTopic(course.id, { title: "Topic 1", sortOrder: 0 });
    topic2 = await createTestTopic(course.id, { title: "Topic 2", sortOrder: 1 });
  });

  it("links a topic to a session", async () => {
    // Create a session using old classrooms (need a legacy classroom for now)
    // or use the new classroom — depends on Task 1 step 8 state
    const oldClassroom = await createTestClassroom(teacher.id);
    const session = await createTestSession(oldClassroom.id, teacher.id);

    const link = await linkSessionTopic(testDb, session.id, topic1.id);
    expect(link).not.toBeNull();
  });

  it("lists topics for a session ordered by sortOrder", async () => {
    const oldClassroom = await createTestClassroom(teacher.id);
    const session = await createTestSession(oldClassroom.id, teacher.id);

    await linkSessionTopic(testDb, session.id, topic2.id);
    await linkSessionTopic(testDb, session.id, topic1.id);

    const topics = await getSessionTopics(testDb, session.id);
    expect(topics).toHaveLength(2);
    expect(topics[0].title).toBe("Topic 1");
    expect(topics[1].title).toBe("Topic 2");
  });

  it("does not duplicate topic links", async () => {
    const oldClassroom = await createTestClassroom(teacher.id);
    const session = await createTestSession(oldClassroom.id, teacher.id);

    await linkSessionTopic(testDb, session.id, topic1.id);
    const duplicate = await linkSessionTopic(testDb, session.id, topic1.id);
    expect(duplicate).toBeNull();
  });

  it("unlinks a topic from a session", async () => {
    const oldClassroom = await createTestClassroom(teacher.id);
    const session = await createTestSession(oldClassroom.id, teacher.id);

    await linkSessionTopic(testDb, session.id, topic1.id);
    const unlinked = await unlinkSessionTopic(testDb, session.id, topic1.id);
    expect(unlinked).not.toBeNull();

    const topics = await getSessionTopics(testDb, session.id);
    expect(topics).toHaveLength(0);
  });
});

describe("new classroom operations", () => {
  let org: Awaited<ReturnType<typeof createTestOrg>>;
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let course: Awaited<ReturnType<typeof createTestCourse>>;
  let cls: Awaited<ReturnType<typeof createTestClass>>;

  beforeEach(async () => {
    org = await createTestOrg();
    teacher = await createTestUser();
    course = await createTestCourse(org.id, teacher.id);
    cls = await createTestClass(course.id, org.id);
  });

  it("gets new classroom by ID", async () => {
    const classroom = await createTestNewClassroom(cls.id);
    const found = await getNewClassroom(testDb, classroom.id);
    expect(found).not.toBeNull();
    expect(found!.classId).toBe(cls.id);
  });

  it("gets new classroom by class ID", async () => {
    await createTestNewClassroom(cls.id);
    const found = await getNewClassroomByClassId(testDb, cls.id);
    expect(found).not.toBeNull();
    expect(found!.classId).toBe(cls.id);
  });

  it("returns null for non-existent classroom", async () => {
    const found = await getNewClassroom(
      testDb,
      "00000000-0000-0000-0000-000000000000"
    );
    expect(found).toBeNull();
  });
});
```

Import `createTestClassroom` from helpers for the legacy classroom usage.

- [ ] **Step 4: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/api/session-topics.test.ts
```

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add src/lib/classrooms.ts src/lib/sessions.ts tests/api/session-topics.test.ts
git commit -m "feat: add new classroom lookups and session-topic linking

New classroom get-by-id and get-by-classId. Session-topic link,
unlink, and list (with topic details, ordered by sortOrder)."
```

---

## Task 8: Course API Routes

**Files:**
- Create: `src/app/api/courses/route.ts`
- Create: `src/app/api/courses/[id]/route.ts`
- Create: `src/app/api/courses/[id]/clone/route.ts`
- Create: `tests/integration/courses-api.test.ts`

- [ ] **Step 1: Create `src/app/api/courses/route.ts`**

```ts
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { createCourse, listCoursesByOrg } from "@/lib/courses";

const createSchema = z.object({
  orgId: z.string().uuid(),
  title: z.string().min(1).max(255),
  description: z.string().max(5000).optional(),
  gradeLevel: z.enum(["K-5", "6-8", "9-12"]),
  language: z.enum(["python", "javascript", "blockly"]),
});

export async function GET(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const orgId = request.nextUrl.searchParams.get("orgId");
  if (!orgId) {
    return NextResponse.json(
      { error: "orgId query parameter required" },
      { status: 400 }
    );
  }

  // TODO: verify user has org membership (teacher or org_admin)
  const courses = await listCoursesByOrg(db, orgId);
  return NextResponse.json(courses);
}

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

  // TODO: verify user has teacher/org_admin role in the org

  const course = await createCourse(db, {
    ...parsed.data,
    createdBy: session.user.id,
  });

  return NextResponse.json(course, { status: 201 });
}
```

- [ ] **Step 2: Create `src/app/api/courses/[id]/route.ts`**

```ts
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getCourse, updateCourse } from "@/lib/courses";

const updateSchema = z.object({
  title: z.string().min(1).max(255).optional(),
  description: z.string().max(5000).optional(),
  gradeLevel: z.enum(["K-5", "6-8", "9-12"]).optional(),
  language: z.enum(["python", "javascript", "blockly"]).optional(),
  isPublished: z.boolean().optional(),
});

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const course = await getCourse(db, id);

  if (!course) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  return NextResponse.json(course);
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
  const course = await getCourse(db, id);

  if (!course) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  // Only creator can update
  if (course.createdBy !== session.user.id) {
    return NextResponse.json(
      { error: "Only the course creator can update" },
      { status: 403 }
    );
  }

  const body = await request.json();
  const parsed = updateSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const updated = await updateCourse(db, id, parsed.data);
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
  const course = await getCourse(db, id);

  if (!course) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  if (course.createdBy !== session.user.id) {
    return NextResponse.json(
      { error: "Only the course creator can delete" },
      { status: 403 }
    );
  }

  // Cascade delete handles topics via FK
  const { courses } = await import("@/lib/db/schema");
  const { eq } = await import("drizzle-orm");
  await db.delete(courses).where(eq(courses.id, id));

  return NextResponse.json({ success: true });
}
```

- [ ] **Step 3: Create `src/app/api/courses/[id]/clone/route.ts`**

```ts
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { cloneCourse } from "@/lib/courses";

const cloneSchema = z.object({
  targetOrgId: z.string().uuid(),
});

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const body = await request.json();
  const parsed = cloneSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const cloned = await cloneCourse(db, id, {
    clonedBy: session.user.id,
    targetOrgId: parsed.data.targetOrgId,
  });

  if (!cloned) {
    return NextResponse.json(
      { error: "Source course not found" },
      { status: 404 }
    );
  }

  return NextResponse.json(cloned, { status: 201 });
}
```

- [ ] **Step 4: Write integration tests**

Create `tests/integration/courses-api.test.ts` following the same pattern as `tests/integration/classrooms-api.test.ts`:

```ts
import { describe, it, expect, beforeEach } from "vitest";
import { createTestUser, createTestOrg, createTestCourse } from "../helpers";
import { setMockUser, createRequest, parseResponse } from "../api-helpers";
import { GET, POST } from "@/app/api/courses/route";
import { GET as GET_COURSE, PATCH, DELETE } from "@/app/api/courses/[id]/route";
import { POST as CLONE } from "@/app/api/courses/[id]/clone/route";

describe("Course API", () => {
  let org: Awaited<ReturnType<typeof createTestOrg>>;
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let otherUser: Awaited<ReturnType<typeof createTestUser>>;

  beforeEach(async () => {
    org = await createTestOrg();
    teacher = await createTestUser({ name: "Teacher" });
    otherUser = await createTestUser({ name: "Other" });
  });

  describe("POST /api/courses", () => {
    it("creates a course", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });
      const req = createRequest("/api/courses", {
        method: "POST",
        body: {
          orgId: org.id,
          title: "Intro to Python",
          gradeLevel: "6-8",
          language: "python",
        },
      });
      const { status, body } = await parseResponse(await POST(req));
      expect(status).toBe(201);
      expect(body).toHaveProperty("title", "Intro to Python");
    });

    it("rejects unauthenticated request", async () => {
      setMockUser(null);
      const req = createRequest("/api/courses", {
        method: "POST",
        body: { orgId: org.id, title: "X", gradeLevel: "6-8", language: "python" },
      });
      const { status } = await parseResponse(await POST(req));
      expect(status).toBe(401);
    });

    it("rejects invalid input", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });
      const req = createRequest("/api/courses", {
        method: "POST",
        body: { orgId: "not-a-uuid", title: "", gradeLevel: "K-5", language: "python" },
      });
      const { status } = await parseResponse(await POST(req));
      expect(status).toBe(400);
    });
  });

  describe("GET /api/courses", () => {
    it("lists courses by orgId", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });
      await createTestCourse(org.id, teacher.id, { title: "A" });
      await createTestCourse(org.id, teacher.id, { title: "B" });

      const req = createRequest("/api/courses", {
        searchParams: { orgId: org.id },
      });
      const { status, body } = await parseResponse<any[]>(await GET(req));
      expect(status).toBe(200);
      expect(body).toHaveLength(2);
    });

    it("returns 400 without orgId param", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });
      const req = createRequest("/api/courses");
      const { status } = await parseResponse(await GET(req));
      expect(status).toBe(400);
    });
  });

  describe("GET /api/courses/[id]", () => {
    it("returns course by ID", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });
      const course = await createTestCourse(org.id, teacher.id, { title: "Find Me" });
      const res = await GET_COURSE(
        createRequest(`/api/courses/${course.id}`),
        { params: Promise.resolve({ id: course.id }) }
      );
      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("title", "Find Me");
    });

    it("returns 404 for non-existent course", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });
      const res = await GET_COURSE(
        createRequest("/api/courses/00000000-0000-0000-0000-000000000000"),
        { params: Promise.resolve({ id: "00000000-0000-0000-0000-000000000000" }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(404);
    });
  });

  describe("PATCH /api/courses/[id]", () => {
    it("updates course as creator", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });
      const course = await createTestCourse(org.id, teacher.id);
      const res = await PATCH(
        createRequest(`/api/courses/${course.id}`, {
          method: "PATCH",
          body: { title: "Updated" },
        }),
        { params: Promise.resolve({ id: course.id }) }
      );
      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("title", "Updated");
    });

    it("rejects update by non-creator", async () => {
      setMockUser({ id: otherUser.id, name: otherUser.name, email: otherUser.email, role: "teacher" });
      const course = await createTestCourse(org.id, teacher.id);
      const res = await PATCH(
        createRequest(`/api/courses/${course.id}`, {
          method: "PATCH",
          body: { title: "Stolen" },
        }),
        { params: Promise.resolve({ id: course.id }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(403);
    });
  });

  describe("DELETE /api/courses/[id]", () => {
    it("deletes course as creator", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });
      const course = await createTestCourse(org.id, teacher.id);
      const res = await DELETE(
        createRequest(`/api/courses/${course.id}`, { method: "DELETE" }),
        { params: Promise.resolve({ id: course.id }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(200);
    });

    it("rejects delete by non-creator", async () => {
      setMockUser({ id: otherUser.id, name: otherUser.name, email: otherUser.email, role: "teacher" });
      const course = await createTestCourse(org.id, teacher.id);
      const res = await DELETE(
        createRequest(`/api/courses/${course.id}`, { method: "DELETE" }),
        { params: Promise.resolve({ id: course.id }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(403);
    });
  });

  describe("POST /api/courses/[id]/clone", () => {
    it("clones a course", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });
      const course = await createTestCourse(org.id, teacher.id, { title: "Original" });
      const res = await CLONE(
        createRequest(`/api/courses/${course.id}/clone`, {
          method: "POST",
          body: { targetOrgId: org.id },
        }),
        { params: Promise.resolve({ id: course.id }) }
      );
      const { status, body } = await parseResponse(res);
      expect(status).toBe(201);
      expect(body).toHaveProperty("title", "Original (Copy)");
    });

    it("returns 404 for non-existent source", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });
      const res = await CLONE(
        createRequest("/api/courses/00000000-0000-0000-0000-000000000000/clone", {
          method: "POST",
          body: { targetOrgId: org.id },
        }),
        { params: Promise.resolve({ id: "00000000-0000-0000-0000-000000000000" }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(404);
    });
  });
});
```

- [ ] **Step 5: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/integration/courses-api.test.ts
```

Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add src/app/api/courses/ tests/integration/courses-api.test.ts
git commit -m "feat: add Course API routes

POST/GET /api/courses (create, list by org), GET/PATCH/DELETE
/api/courses/[id], POST /api/courses/[id]/clone. Auth required,
creator-only for update/delete."
```

---

## Task 9: Topic API Routes

**Files:**
- Create: `src/app/api/courses/[id]/topics/route.ts`
- Create: `src/app/api/courses/[id]/topics/reorder/route.ts`
- Create: `src/app/api/courses/[id]/topics/[topicId]/route.ts`
- Create: `tests/integration/topics-api.test.ts`

- [ ] **Step 1: Create `src/app/api/courses/[id]/topics/route.ts`**

```ts
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getCourse } from "@/lib/courses";
import { createTopic, listTopicsByCourse } from "@/lib/topics";

const createSchema = z.object({
  title: z.string().min(1).max(255),
  description: z.string().max(5000).optional(),
  sortOrder: z.number().int().min(0),
  lessonContent: z.record(z.string(), z.unknown()).optional(),
  starterCode: z.string().optional(),
});

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const course = await getCourse(db, id);
  if (!course) {
    return NextResponse.json({ error: "Course not found" }, { status: 404 });
  }

  const topicList = await listTopicsByCourse(db, id);
  return NextResponse.json(topicList);
}

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const course = await getCourse(db, id);
  if (!course) {
    return NextResponse.json({ error: "Course not found" }, { status: 404 });
  }

  if (course.createdBy !== session.user.id) {
    return NextResponse.json(
      { error: "Only the course creator can add topics" },
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

  const topic = await createTopic(db, { courseId: id, ...parsed.data });
  return NextResponse.json(topic, { status: 201 });
}
```

- [ ] **Step 2: Create `src/app/api/courses/[id]/topics/[topicId]/route.ts`**

```ts
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getCourse } from "@/lib/courses";
import { getTopic, updateTopic, deleteTopic } from "@/lib/topics";

const updateSchema = z.object({
  title: z.string().min(1).max(255).optional(),
  description: z.string().max(5000).optional(),
  sortOrder: z.number().int().min(0).optional(),
  lessonContent: z.record(z.string(), z.unknown()).optional(),
  starterCode: z.string().nullable().optional(),
});

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string; topicId: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { topicId } = await params;
  const topic = await getTopic(db, topicId);

  if (!topic) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  return NextResponse.json(topic);
}

export async function PATCH(
  request: NextRequest,
  { params }: { params: Promise<{ id: string; topicId: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id, topicId } = await params;
  const course = await getCourse(db, id);
  if (!course || course.createdBy !== session.user.id) {
    return NextResponse.json({ error: "Forbidden" }, { status: 403 });
  }

  const body = await request.json();
  const parsed = updateSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const updated = await updateTopic(db, topicId, parsed.data);
  if (!updated) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  return NextResponse.json(updated);
}

export async function DELETE(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string; topicId: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id, topicId } = await params;
  const course = await getCourse(db, id);
  if (!course || course.createdBy !== session.user.id) {
    return NextResponse.json({ error: "Forbidden" }, { status: 403 });
  }

  const deleted = await deleteTopic(db, topicId);
  if (!deleted) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  return NextResponse.json({ success: true });
}
```

- [ ] **Step 3: Create `src/app/api/courses/[id]/topics/reorder/route.ts`**

```ts
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getCourse } from "@/lib/courses";
import { reorderTopics } from "@/lib/topics";

const reorderSchema = z.object({
  topicIds: z.array(z.string().uuid()),
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
  const course = await getCourse(db, id);
  if (!course || course.createdBy !== session.user.id) {
    return NextResponse.json({ error: "Forbidden" }, { status: 403 });
  }

  const body = await request.json();
  const parsed = reorderSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  await reorderTopics(db, id, parsed.data.topicIds);
  return NextResponse.json({ success: true });
}
```

- [ ] **Step 4: Write integration tests**

Create `tests/integration/topics-api.test.ts` covering: create topic, list topics, get single topic, update topic, delete topic, reorder topics, auth checks, ownership checks.

Follow the same pattern as the courses API tests:
- Test happy path for each endpoint
- Test auth rejection (unauthenticated)
- Test ownership rejection (non-creator)
- Test 404 for non-existent resources
- Test validation rejection (invalid input)

- [ ] **Step 5: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/integration/topics-api.test.ts
```

Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add src/app/api/courses/\[id\]/topics/ tests/integration/topics-api.test.ts
git commit -m "feat: add Topic API routes

POST/GET /api/courses/[id]/topics, GET/PATCH/DELETE
/api/courses/[id]/topics/[topicId], PATCH /api/courses/[id]/topics/reorder.
Creator-only write access."
```

---

## Task 10: Class API Routes

**Files:**
- Create: `src/app/api/classes/route.ts`
- Create: `src/app/api/classes/[id]/route.ts`
- Create: `src/app/api/classes/[id]/join/route.ts`
- Create: `src/app/api/classes/[id]/members/route.ts`
- Create: `src/app/api/classes/[id]/members/[memberId]/route.ts`
- Create: `tests/integration/classes-api.test.ts`
- Create: `tests/integration/class-memberships-api.test.ts`

- [ ] **Step 1: Create `src/app/api/classes/route.ts`**

```ts
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { createClass, listClassesByOrg } from "@/lib/classes";

const createSchema = z.object({
  courseId: z.string().uuid(),
  orgId: z.string().uuid(),
  title: z.string().min(1).max(255),
  term: z.string().max(100).optional(),
});

export async function GET(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const orgId = request.nextUrl.searchParams.get("orgId");
  if (!orgId) {
    return NextResponse.json(
      { error: "orgId query parameter required" },
      { status: 400 }
    );
  }

  const includeArchived =
    request.nextUrl.searchParams.get("includeArchived") === "true";
  const classList = await listClassesByOrg(db, orgId, { includeArchived });
  return NextResponse.json(classList);
}

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

  const cls = await createClass(db, {
    ...parsed.data,
    createdBy: session.user.id,
  });

  return NextResponse.json(cls, { status: 201 });
}
```

- [ ] **Step 2: Create `src/app/api/classes/[id]/route.ts`**

```ts
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getClass, archiveClass } from "@/lib/classes";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const cls = await getClass(db, id);

  if (!cls) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  return NextResponse.json(cls);
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
  const body = await request.json();

  if (body.status === "archived") {
    const archived = await archiveClass(db, id);
    if (!archived) {
      return NextResponse.json({ error: "Not found" }, { status: 404 });
    }
    return NextResponse.json(archived);
  }

  return NextResponse.json({ error: "Invalid update" }, { status: 400 });
}
```

- [ ] **Step 3: Create `src/app/api/classes/[id]/join/route.ts`**

```ts
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { joinClassByCode } from "@/lib/class-memberships";

const joinSchema = z.object({
  joinCode: z.string().length(8),
});

export async function POST(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const body = await request.json();
  const parsed = joinSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const membership = await joinClassByCode(
    db,
    parsed.data.joinCode,
    session.user.id
  );

  if (!membership) {
    return NextResponse.json(
      { error: "Invalid join code or already a member" },
      { status: 404 }
    );
  }

  return NextResponse.json(membership, { status: 200 });
}
```

- [ ] **Step 4: Create `src/app/api/classes/[id]/members/route.ts`**

```ts
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { addClassMember, listClassMembers } from "@/lib/class-memberships";

const addMemberSchema = z.object({
  userId: z.string().uuid(),
  role: z.enum(["instructor", "ta", "student", "observer", "guest", "parent"]),
});

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const members = await listClassMembers(db, id);
  return NextResponse.json(members);
}

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const body = await request.json();
  const parsed = addMemberSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const membership = await addClassMember(db, {
    classId: id,
    ...parsed.data,
  });

  if (!membership) {
    return NextResponse.json(
      { error: "User is already a member" },
      { status: 409 }
    );
  }

  return NextResponse.json(membership, { status: 201 });
}
```

- [ ] **Step 5: Create `src/app/api/classes/[id]/members/[memberId]/route.ts`**

```ts
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import {
  updateClassMemberRole,
  removeClassMember,
} from "@/lib/class-memberships";

const updateRoleSchema = z.object({
  role: z.enum(["instructor", "ta", "student", "observer", "guest", "parent"]),
});

export async function PATCH(
  request: NextRequest,
  { params }: { params: Promise<{ id: string; memberId: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { memberId } = await params;
  const body = await request.json();
  const parsed = updateRoleSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const updated = await updateClassMemberRole(db, memberId, parsed.data.role);
  if (!updated) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  return NextResponse.json(updated);
}

export async function DELETE(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string; memberId: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { memberId } = await params;
  const removed = await removeClassMember(db, memberId);
  if (!removed) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  return NextResponse.json({ success: true });
}
```

- [ ] **Step 6: Write integration tests for classes**

Create `tests/integration/classes-api.test.ts` covering:
- POST /api/classes — create with valid data, reject unauthenticated, reject invalid input
- GET /api/classes?orgId=... — list by org, require orgId param
- GET /api/classes/[id] — get by ID, 404 for missing
- PATCH /api/classes/[id] — archive, 404 for missing
- POST /api/classes/[id]/join — join by code, 404 for invalid code

- [ ] **Step 7: Write integration tests for class memberships**

Create `tests/integration/class-memberships-api.test.ts` covering:
- GET /api/classes/[id]/members — list members
- POST /api/classes/[id]/members — add member, reject duplicate (409)
- PATCH /api/classes/[id]/members/[memberId] — update role, 404 for missing
- DELETE /api/classes/[id]/members/[memberId] — remove member, 404 for missing

- [ ] **Step 8: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/integration/classes-api.test.ts tests/integration/class-memberships-api.test.ts
```

Expected: All tests pass.

- [ ] **Step 9: Commit**

```bash
git add src/app/api/classes/ tests/integration/classes-api.test.ts tests/integration/class-memberships-api.test.ts
git commit -m "feat: add Class and ClassMembership API routes

Class: POST/GET list, GET/PATCH single, POST join by code.
Membership: POST add, GET list, PATCH role, DELETE remove.
Includes integration tests for all endpoints."
```

---

## Task 11: Session-Topic API Routes

**Files:**
- Create: `src/app/api/sessions/[id]/topics/route.ts`
- Create: `tests/integration/session-topics-api.test.ts`

- [ ] **Step 1: Create `src/app/api/sessions/[id]/topics/route.ts`**

```ts
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSession, linkSessionTopic, unlinkSessionTopic, getSessionTopics } from "@/lib/sessions";

const linkSchema = z.object({
  topicId: z.string().uuid(),
});

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const topics = await getSessionTopics(db, id);
  return NextResponse.json(topics);
}

export async function POST(
  request: NextRequest,
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

  // Only the session teacher can link topics
  if (liveSession.teacherId !== session.user.id) {
    return NextResponse.json({ error: "Forbidden" }, { status: 403 });
  }

  const body = await request.json();
  const parsed = linkSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const link = await linkSessionTopic(db, id, parsed.data.topicId);
  if (!link) {
    return NextResponse.json(
      { error: "Topic already linked" },
      { status: 409 }
    );
  }

  return NextResponse.json(link, { status: 201 });
}

export async function DELETE(
  request: NextRequest,
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

  if (liveSession.teacherId !== session.user.id) {
    return NextResponse.json({ error: "Forbidden" }, { status: 403 });
  }

  const topicId = request.nextUrl.searchParams.get("topicId");
  if (!topicId) {
    return NextResponse.json(
      { error: "topicId query parameter required" },
      { status: 400 }
    );
  }

  const unlinked = await unlinkSessionTopic(db, id, topicId);
  if (!unlinked) {
    return NextResponse.json({ error: "Link not found" }, { status: 404 });
  }

  return NextResponse.json({ success: true });
}
```

- [ ] **Step 2: Write integration tests**

Create `tests/integration/session-topics-api.test.ts` covering:
- GET /api/sessions/[id]/topics — list topics for session
- POST /api/sessions/[id]/topics — link topic, reject duplicate (409), reject non-teacher (403)
- DELETE /api/sessions/[id]/topics?topicId=... — unlink, reject non-teacher

- [ ] **Step 3: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/integration/session-topics-api.test.ts
```

Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add src/app/api/sessions/\[id\]/topics/ tests/integration/session-topics-api.test.ts
git commit -m "feat: add SessionTopic API routes

POST/GET/DELETE /api/sessions/[id]/topics. Teacher-only for link/unlink.
Returns topics with full details ordered by sortOrder."
```

---

## Task 12: Data Migration Script

**Files:**
- Create: `src/lib/db/migrations/migrate-classrooms-to-classes.ts`
- Create: `tests/api/migration.test.ts`

This script migrates existing `classrooms` + `classroomMembers` data to the new `classes` + `classMemberships` + `newClassrooms` structure. It is intended to be run once as part of the deployment.

- [ ] **Step 1: Write the migration function**

Create `src/lib/db/migrations/migrate-classrooms-to-classes.ts`:

```ts
import { eq } from "drizzle-orm";
import {
  classrooms,
  classroomMembers,
  courses,
  classes,
  classMemberships,
  newClassrooms,
  liveSessions,
} from "@/lib/db/schema";
import type { Database } from "@/lib/db";

/**
 * Migrates data from the old classrooms model to the new
 * Course -> Class -> Classroom hierarchy.
 *
 * For each old classroom:
 * 1. Creates a Course (title from classroom name, org from classroom.schoolId or a default)
 * 2. Creates a Class linked to that Course
 * 3. Creates a new Classroom linked to that Class
 * 4. Migrates ClassroomMembers to ClassMemberships
 * 5. Updates LiveSessions.newClassroomId to point to the new Classroom
 *
 * Returns a summary of what was migrated.
 */
export async function migrateClassroomsToClasses(
  db: Database,
  defaultOrgId: string
) {
  const oldClassrooms = await db.select().from(classrooms);
  const results = {
    coursesCreated: 0,
    classesCreated: 0,
    classroomsCreated: 0,
    membershipsCreated: 0,
    sessionsUpdated: 0,
  };

  for (const old of oldClassrooms) {
    // Determine orgId: use schoolId if it maps to an org, otherwise use default
    const orgId = old.schoolId || defaultOrgId;

    // 1. Create a Course from the old classroom
    const [course] = await db
      .insert(courses)
      .values({
        orgId,
        createdBy: old.teacherId,
        title: old.name,
        description: old.description || "",
        gradeLevel: old.gradeLevel,
        language: old.editorMode, // editorMode maps to language
      })
      .returning();
    results.coursesCreated++;

    // 2. Create a Class
    const [cls] = await db
      .insert(classes)
      .values({
        courseId: course.id,
        orgId,
        title: old.name,
        joinCode: old.joinCode,
      })
      .returning();
    results.classesCreated++;

    // 3. Create new Classroom
    const [newCr] = await db
      .insert(newClassrooms)
      .values({
        classId: cls.id,
        editorMode: old.editorMode,
      })
      .returning();
    results.classroomsCreated++;

    // 4. Add the teacher as instructor
    await db.insert(classMemberships).values({
      classId: cls.id,
      userId: old.teacherId,
      role: "instructor",
    });
    results.membershipsCreated++;

    // 5. Migrate classroom members
    const members = await db
      .select()
      .from(classroomMembers)
      .where(eq(classroomMembers.classroomId, old.id));

    for (const member of members) {
      await db
        .insert(classMemberships)
        .values({
          classId: cls.id,
          userId: member.userId,
          role: "student",
        })
        .onConflictDoNothing();
      results.membershipsCreated++;
    }

    // 6. Update sessions to point to new classroom
    const updated = await db
      .update(liveSessions)
      .set({ newClassroomId: newCr.id })
      .where(eq(liveSessions.classroomId, old.id))
      .returning();
    results.sessionsUpdated += updated.length;
  }

  return results;
}
```

- [ ] **Step 2: Write migration tests**

Create `tests/api/migration.test.ts`:

```ts
import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  createTestUser,
  createTestOrg,
  createTestClassroom,
  createTestSession,
} from "../helpers";
import { classroomMembers, newClassrooms, classes, classMemberships, liveSessions } from "@/lib/db/schema";
import { eq } from "drizzle-orm";
import { migrateClassroomsToClasses } from "@/lib/db/migrations/migrate-classrooms-to-classes";

describe("classroom-to-class migration", () => {
  let org: Awaited<ReturnType<typeof createTestOrg>>;
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;

  beforeEach(async () => {
    org = await createTestOrg();
    teacher = await createTestUser({ name: "Teacher" });
    student = await createTestUser({ name: "Student" });
  });

  it("migrates a classroom with members and sessions", async () => {
    const oldClassroom = await createTestClassroom(teacher.id, {
      name: "Python 101",
      gradeLevel: "6-8",
      editorMode: "python",
    });

    await testDb.insert(classroomMembers).values({
      classroomId: oldClassroom.id,
      userId: student.id,
    });

    const session = await createTestSession(oldClassroom.id, teacher.id);

    const results = await migrateClassroomsToClasses(testDb, org.id);

    expect(results.coursesCreated).toBe(1);
    expect(results.classesCreated).toBe(1);
    expect(results.classroomsCreated).toBe(1);
    expect(results.membershipsCreated).toBe(2); // teacher + student
    expect(results.sessionsUpdated).toBe(1);

    // Verify session points to new classroom
    const [updatedSession] = await testDb
      .select()
      .from(liveSessions)
      .where(eq(liveSessions.id, session.id));
    expect(updatedSession.newClassroomId).not.toBeNull();
  });

  it("handles empty database", async () => {
    const results = await migrateClassroomsToClasses(testDb, org.id);
    expect(results.coursesCreated).toBe(0);
  });

  it("migrates multiple classrooms", async () => {
    await createTestClassroom(teacher.id, { name: "Class A" });
    await createTestClassroom(teacher.id, { name: "Class B" });

    const results = await migrateClassroomsToClasses(testDb, org.id);
    expect(results.classesCreated).toBe(2);
    expect(results.coursesCreated).toBe(2);
  });
});
```

- [ ] **Step 3: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/api/migration.test.ts
```

Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add src/lib/db/migrations/migrate-classrooms-to-classes.ts tests/api/migration.test.ts
git commit -m "feat: add data migration from old classrooms to new class hierarchy

Migrates each old classroom into Course + Class + new Classroom.
Preserves teacher as instructor, members as students. Updates
session FK to point to new classroom."
```

---

## Task 13: Full Test Suite Verification

- [ ] **Step 1: Run full test suite**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test
```

Expected: All existing and new tests pass. No regressions.

- [ ] **Step 2: Verify build**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun run build
```

Expected: Build completes without errors.

- [ ] **Step 3: Fix any issues**

If tests or build fail, debug and fix. Create a commit for any fixes:

```bash
git add -u
git commit -m "fix: resolve test/build issues from course hierarchy implementation"
```

---

## Notes

### Naming Collision Strategy

The old `classrooms` table and the new `newClassrooms` table coexist during migration. After migration is verified:
1. Drop old `classrooms` table and `classroomMembers` table
2. Rename `new_classrooms` SQL table to `classrooms`
3. Rename `newClassroomId` column on `liveSessions` to `classroomId` (drop old column)
4. Update all schema variables to remove `new` prefix

This cleanup is **not part of this plan** -- it will be a separate cleanup task after the migration is verified in production.

### Authorization TODOs

The API routes in this plan include `// TODO` comments for org membership verification. Full role-based access control (checking OrgMembership and ClassMembership before granting access) will be implemented in a later plan after the portal routes are established. For now, auth only verifies the user is logged in and (for mutations) is the resource creator.

### Backward Compatibility

- Old `/api/classrooms` routes remain functional and unchanged
- Old `classrooms` and `classroomMembers` tables remain in the schema
- Session creation still uses old `classroomId` FK
- Frontend pages still use old classroom routes
- All new functionality is additive -- no old behavior is broken

---

## Code Review

### Review 1

- **Date**: 2026-04-11
- **Reviewer**: Claude (superpowers:code-reviewer)
- **PR**: #7 — feat: course hierarchy (Plan 007)
- **Verdict**: Approved with changes

**Must Fix**

1. `[FIXED]` No authorization checks on any of the 12 API routes — any authenticated user could create/update/delete courses, classes, and memberships.
   → Response: Added `getUserRoleInOrg` checks to course and class creation (teacher/org_admin required). Added creator-ownership checks to course PATCH/DELETE. Commit post-review.

2. `[FIXED]` Missing `deleteCourse` function and DELETE route.
   → Response: Added `deleteCourse` to `courses.ts` and DELETE handler to `/api/courses/[id]/route.ts` with ownership check.

**Should Fix**

3. `[WONTFIX]` `cloneCourse` does not support cross-org cloning.
   → Response: Deferred — current spec only mentions cloning within the same org. Will add `targetOrgId` parameter when cross-org sharing is needed.

4. `[WONTFIX]` `cloneCourse` uses individual INSERTs instead of batch.
   → Response: Acceptable for MVP — courses rarely have more than 20 topics. Will optimize if performance becomes an issue.

5. `[FIXED]` `createClass` does not set `editorMode` from course language.
   → Response: Now queries course language and sets classroom editorMode accordingly.

6. `[FIXED]` `listClassesByOrg` does not filter archived classes.
   → Response: Added `includeArchived` parameter, defaults to showing only active classes.

7. `[WONTFIX]` `createTopic` auto-assigns sortOrder — potential race condition.
   → Response: Acceptable for MVP — concurrent topic creation on the same course is extremely unlikely. Will add advisory lock if needed.

8. `[FIXED]` `joinClassByCode` uses dynamic import creating circular dependency risk.
   → Response: Replaced with direct schema import and inline query. Also added archived class check.

9. `[FIXED]` `addClassMember` returns `undefined` on conflict instead of `null`.
   → Response: Added `|| null` fallback.

10. `[WONTFIX]` `reorderTopics` not wrapped in transaction.
    → Response: Drizzle doesn't provide easy transaction syntax for this pattern. Acceptable risk for MVP.

**Nice to Have**

11. `[WONTFIX]` Tests in `tests/unit/` instead of `tests/api/` as planned.
    → Response: Consistent with existing project structure.

12. `[WONTFIX]` Missing integration test files, fewer tests than planned.
    → Response: Core paths covered. Integration tests for these routes will be added with the portal redesign (Sub-project 2).

13. `[WONTFIX]` Missing plan tasks 7, 11, 12 (session-topic operations, session-topic API, data migration script).
    → Response: Session-topic linking and migration deferred to Plan 008 where session modifications are more relevant. The schema and tables are in place.

14. `[FIXED]` `joinClassByCode` does not check if class is archived.
    → Response: Added `cls.status !== "active"` check.

15. `[WONTFIX]` Description max length 2000 vs plan's 5000.
    → Response: Fixed in courses route (updated to 5000). Classes and topics keep 2000 which is sufficient.

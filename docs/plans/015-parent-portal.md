# Parent Portal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the parent portal with advanced features from spec 002 Sub-project 5. The current parent portal has a basic dashboard, a child detail page (classes + recent code), and a placeholder reports page. This plan adds: live session viewing with read-only Yjs, attendance tracking with statistics, AI-generated progress reports, and enhanced dashboard/detail pages with "Live Now" indicators and progress summaries.

**Architecture:** Server components for data fetching, client components for live/interactive features. The live session viewer is a client component that connects to the child's Yjs document as a read-only observer. AI reports are generated server-side using the existing LLM provider system and cached in a new `parent_reports` table. Attendance data is derived from the existing `session_participants` table. All parent access is gated by verifying the parent-child link via `getLinkedChildren()`.

**Tech Stack:** Next.js 16 App Router, React Server/Client Components, Drizzle ORM, Monaco Editor (read-only), Yjs + Hocuspocus (read-only subscription), LLM providers via `getOpenAIClient`/`getAnthropicClient`, shadcn/ui (Card, Button, Badge, Tabs, Progress), lucide-react, Tailwind CSS v4, Vitest + Testing Library

**Depends on:** Plan 010 (portal-pages -- existing parent portal scaffolding), Plan 014 (live-session-redesign -- session topics, teacher dashboard components), Plan 008 (code-persistence -- documents), Plan 011 (lesson-content -- LessonRenderer)

**Key constraints:**
- shadcn/ui uses `@base-ui/react` -- NO `asChild` prop; use `buttonVariants()` with `<Link>` instead
- Auth.js v5: `session.user.id`, `session.user.isPlatformAdmin`
- Drizzle ORM for all DB queries -- use existing lib functions, add new ones only when missing
- `fileParallelism: false` in Vitest -- `.tsx` tests need `// @vitest-environment jsdom`
- Hocuspocus auth uses token format `userId:role` -- parent needs `userId:parent` role to be allowed
- The Hocuspocus server currently allows `teacher` and `user` roles to view any doc in a session. Must add `parent` role support.
- `getLinkedChildren()` in `src/lib/parent-links.ts` is the single source of truth for parent-child authorization
- localStorage key convention: `bridge-<name>` (see `bridge-theme`, `bridge-sidebar-collapsed`)
- Existing SSE event bus at `src/lib/sse.ts` is the standard for server-to-client real-time events

---

## File Structure

```
server/
├── hocuspocus.ts                                         # Modify: allow "parent" role to view child docs (read-only)
src/
├── lib/
│   ├── parent-links.ts                                   # Modify: add getLinkedChildrenWithStatus(), isChildLinkedToParent()
│   ├── attendance.ts                                     # Create: attendance tracking queries
│   ├── parent-reports.ts                                 # Create: AI report generation + storage
│   ├── db/
│   │   └── schema.ts                                     # Modify: add parentReports table
│   └── ai/
│       └── report-prompts.ts                             # Create: system prompts for parent report generation
├── components/
│   └── parent/
│       ├── live-session-viewer.tsx                        # Create: read-only Yjs + Monaco + lesson content
│       ├── live-now-badge.tsx                             # Create: pulsing "Live Now" indicator
│       ├── attendance-summary.tsx                         # Create: attendance stats display
│       ├── attendance-history.tsx                         # Create: recent session list with topics
│       ├── grade-summary.tsx                              # Create: assignment grades list
│       ├── ai-interactions-summary.tsx                    # Create: recent AI usage summary
│       └── report-card.tsx                               # Create: single AI report display
├── app/
│   └── (portal)/
│       └── parent/
│           ├── page.tsx                                   # Modify: add Live Now indicators, last activity, progress
│           ├── children/
│           │   └── [id]/
│           │       ├── page.tsx                           # Modify: add attendance, grades, AI interactions, Live Now
│           │       ├── live/
│           │       │   └── page.tsx                       # Create: live session viewing page
│           │       └── reports/
│           │           └── page.tsx                       # Create: AI progress reports per child
│           └── reports/
│               └── page.tsx                               # Modify: redirect to per-child reports or show all children
├── app/
│   └── api/
│       └── parent/
│           ├── children/
│           │   └── [id]/
│           │       ├── live-session/
│           │       │   └── route.ts                       # Create: GET active session for child + Yjs connection info
│           │       └── reports/
│           │           ├── route.ts                       # Create: GET list / POST generate report
│           │           └── [reportId]/
│           │               └── route.ts                   # Create: GET single report
tests/
├── unit/
│   ├── attendance.test.ts                                 # Create: attendance query tests
│   ├── parent-reports.test.ts                             # Create: report generation + storage tests
│   └── parent-links-extended.test.ts                      # Create: extended parent-links tests
├── integration/
│   ├── parent-live-session-api.test.ts                    # Create: live session API tests
│   └── parent-reports-api.test.ts                         # Create: reports API tests
```

---

## Task 1: Database Schema -- Parent Reports Table

Add a table to cache AI-generated parent reports.

### Files

- [ ] `src/lib/db/schema.ts` -- add `parentReports` table
- [ ] `tests/helpers.ts` -- add `parentReports` to `cleanupDatabase()` and add `createTestParentReport()` helper

### Code

**`src/lib/db/schema.ts`** -- add after the `submissions` table definition:

```typescript
// --- Parent Reports ---

export const parentReports = pgTable(
  "parent_reports",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    parentId: uuid("parent_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    childId: uuid("child_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    weekStart: timestamp("week_start").notNull(),
    weekEnd: timestamp("week_end").notNull(),
    summary: text("summary").notNull(),
    details: jsonb("details").default({}),
    generatedAt: timestamp("generated_at").defaultNow().notNull(),
  },
  (table) => [
    index("parent_reports_parent_idx").on(table.parentId),
    index("parent_reports_child_idx").on(table.childId),
    index("parent_reports_week_idx").on(table.childId, table.weekStart),
  ]
);
```

The `details` JSONB field stores structured data that was used to generate the report:
```typescript
interface ReportDetails {
  sessionsAttended: number;
  sessionsTotal: number;
  topicsCovered: string[];
  assignmentsGraded: { title: string; grade: number }[];
  aiInteractionCount: number;
  annotationCount: number;
}
```

**`tests/helpers.ts`** -- add to `cleanupDatabase()` (insert before `await testDb.delete(schema.submissions);`):

```typescript
await testDb.delete(schema.parentReports);
```

Add helper function at end of file:

```typescript
export async function createTestParentReport(
  parentId: string,
  childId: string,
  overrides: Partial<typeof schema.parentReports.$inferInsert> = {}
) {
  const now = new Date();
  const weekStart = new Date(now);
  weekStart.setDate(weekStart.getDate() - weekStart.getDay()); // Sunday
  weekStart.setHours(0, 0, 0, 0);
  const weekEnd = new Date(weekStart);
  weekEnd.setDate(weekEnd.getDate() + 6);
  weekEnd.setHours(23, 59, 59, 999);

  const [report] = await testDb
    .insert(schema.parentReports)
    .values({
      parentId,
      childId,
      weekStart,
      weekEnd,
      summary: "Test report summary.",
      details: { sessionsAttended: 3, sessionsTotal: 5, topicsCovered: ["Variables"], assignmentsGraded: [], aiInteractionCount: 2, annotationCount: 1 },
      ...overrides,
    })
    .returning();
  return report;
}
```

### Commit
`git commit -m "add parentReports schema table and test helpers for parent portal reports"`

---

## Task 2: Attendance Tracking Library

Create `src/lib/attendance.ts` with functions to query session participation data for a student.

### Files

- [ ] `src/lib/attendance.ts` -- create
- [ ] `tests/unit/attendance.test.ts` -- create

### Code

**`src/lib/attendance.ts`**:

```typescript
import { eq, and, desc, sql, count } from "drizzle-orm";
import {
  sessionParticipants,
  liveSessions,
  sessionTopics,
  topics,
  newClassrooms,
  classes,
  classMemberships,
} from "@/lib/db/schema";
import type { Database } from "@/lib/db";

export interface AttendanceSummary {
  classId: string;
  classTitle: string;
  sessionsAttended: number;
  sessionsTotal: number;
  attendanceRate: number; // 0.0 - 1.0
}

export interface SessionHistory {
  sessionId: string;
  classTitle: string;
  startedAt: Date;
  endedAt: Date | null;
  topicsCovered: string[];
}

/**
 * Get attendance summary per class for a student.
 * Counts how many sessions the student participated in vs total sessions per class.
 */
export async function getAttendanceSummary(
  db: Database,
  studentId: string
): Promise<AttendanceSummary[]> {
  // Get all classes the student is in
  const studentClasses = await db
    .select({
      classId: classMemberships.classId,
      classTitle: classes.title,
    })
    .from(classMemberships)
    .innerJoin(classes, eq(classMemberships.classId, classes.id))
    .where(
      and(
        eq(classMemberships.userId, studentId),
        eq(classMemberships.role, "student")
      )
    );

  if (studentClasses.length === 0) return [];

  const summaries: AttendanceSummary[] = [];

  for (const cls of studentClasses) {
    // Get classroom for this class
    const [classroom] = await db
      .select({ id: newClassrooms.id })
      .from(newClassrooms)
      .where(eq(newClassrooms.classId, cls.classId));

    if (!classroom) {
      summaries.push({
        classId: cls.classId,
        classTitle: cls.classTitle,
        sessionsAttended: 0,
        sessionsTotal: 0,
        attendanceRate: 0,
      });
      continue;
    }

    // Count total sessions for this classroom
    const [totalResult] = await db
      .select({ count: count() })
      .from(liveSessions)
      .where(eq(liveSessions.classroomId, classroom.id));

    const sessionsTotal = totalResult?.count || 0;

    // Count sessions this student participated in
    const [attendedResult] = await db
      .select({ count: count() })
      .from(sessionParticipants)
      .innerJoin(liveSessions, eq(sessionParticipants.sessionId, liveSessions.id))
      .where(
        and(
          eq(sessionParticipants.studentId, studentId),
          eq(liveSessions.classroomId, classroom.id)
        )
      );

    const sessionsAttended = attendedResult?.count || 0;

    summaries.push({
      classId: cls.classId,
      classTitle: cls.classTitle,
      sessionsAttended,
      sessionsTotal,
      attendanceRate: sessionsTotal > 0 ? sessionsAttended / sessionsTotal : 0,
    });
  }

  return summaries;
}

/**
 * Get recent session history for a student -- sessions they participated in,
 * with topics covered.
 */
export async function getSessionHistory(
  db: Database,
  studentId: string,
  limit = 20
): Promise<SessionHistory[]> {
  // Get sessions the student participated in, most recent first
  const participations = await db
    .select({
      sessionId: sessionParticipants.sessionId,
      startedAt: liveSessions.startedAt,
      endedAt: liveSessions.endedAt,
      classroomId: liveSessions.classroomId,
    })
    .from(sessionParticipants)
    .innerJoin(liveSessions, eq(sessionParticipants.sessionId, liveSessions.id))
    .where(eq(sessionParticipants.studentId, studentId))
    .orderBy(desc(liveSessions.startedAt))
    .limit(limit);

  if (participations.length === 0) return [];

  // Resolve class titles and topics for each session
  const history: SessionHistory[] = [];

  for (const p of participations) {
    // Get class title via classroom -> class
    const [classroom] = await db
      .select({ classId: newClassrooms.classId })
      .from(newClassrooms)
      .where(eq(newClassrooms.id, p.classroomId));

    let classTitle = "Unknown Class";
    if (classroom) {
      const [cls] = await db
        .select({ title: classes.title })
        .from(classes)
        .where(eq(classes.id, classroom.classId));
      if (cls) classTitle = cls.title;
    }

    // Get topics covered in this session
    const sessionTopicRows = await db
      .select({ title: topics.title })
      .from(sessionTopics)
      .innerJoin(topics, eq(sessionTopics.topicId, topics.id))
      .where(eq(sessionTopics.sessionId, p.sessionId));

    history.push({
      sessionId: p.sessionId,
      classTitle,
      startedAt: p.startedAt,
      endedAt: p.endedAt,
      topicsCovered: sessionTopicRows.map((t) => t.title),
    });
  }

  return history;
}

/**
 * Check if a student is currently in an active session.
 * Returns the active session info or null.
 */
export async function getActiveSessionForStudent(
  db: Database,
  studentId: string
): Promise<{
  sessionId: string;
  classroomId: string;
  classId: string;
  classTitle: string;
  startedAt: Date;
  topicsCovered: string[];
} | null> {
  // Find active session participation
  const activeSessions = await db
    .select({
      sessionId: sessionParticipants.sessionId,
      classroomId: liveSessions.classroomId,
      startedAt: liveSessions.startedAt,
    })
    .from(sessionParticipants)
    .innerJoin(liveSessions, eq(sessionParticipants.sessionId, liveSessions.id))
    .where(
      and(
        eq(sessionParticipants.studentId, studentId),
        eq(liveSessions.status, "active"),
        sql`${sessionParticipants.leftAt} IS NULL`
      )
    );

  if (activeSessions.length === 0) return null;

  const active = activeSessions[0];

  // Get class info
  const [classroom] = await db
    .select({ classId: newClassrooms.classId })
    .from(newClassrooms)
    .where(eq(newClassrooms.id, active.classroomId));

  if (!classroom) return null;

  const [cls] = await db
    .select({ title: classes.title })
    .from(classes)
    .where(eq(classes.id, classroom.classId));

  // Get topics
  const sessionTopicRows = await db
    .select({ title: topics.title })
    .from(sessionTopics)
    .innerJoin(topics, eq(sessionTopics.topicId, topics.id))
    .where(eq(sessionTopics.sessionId, active.sessionId));

  return {
    sessionId: active.sessionId,
    classroomId: active.classroomId,
    classId: classroom.classId,
    classTitle: cls?.title || "Unknown Class",
    startedAt: active.startedAt,
    topicsCovered: sessionTopicRows.map((t) => t.title),
  };
}
```

**`tests/unit/attendance.test.ts`**:

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  cleanupDatabase,
  createTestUser,
  createTestOrg,
  createTestCourse,
  createTestClass,
  createTestSession,
  createTestClassroom,
  createTestTopic,
} from "../helpers";
import { classMemberships, newClassrooms, sessionParticipants, sessionTopics, liveSessions } from "@/lib/db/schema";
import { createClass } from "@/lib/classes";
import {
  getAttendanceSummary,
  getSessionHistory,
  getActiveSessionForStudent,
} from "@/lib/attendance";

describe("attendance", () => {
  let org: any;
  let teacher: any;
  let student: any;
  let course: any;

  beforeEach(async () => {
    await cleanupDatabase();
    org = await createTestOrg();
    teacher = await createTestUser({ email: "teacher@test.edu" });
    student = await createTestUser({ name: "Alice", email: "student@test.edu" });
    course = await createTestCourse(org.id, teacher.id);
  });

  describe("getAttendanceSummary", () => {
    it("returns empty array when student has no classes", async () => {
      const summary = await getAttendanceSummary(testDb, student.id);
      expect(summary).toHaveLength(0);
    });

    it("returns 0/0 attendance when class has no sessions", async () => {
      const cls = await createClass(testDb, {
        courseId: course.id,
        orgId: org.id,
        title: "Python 101",
        createdBy: teacher.id,
      });
      await testDb.insert(classMemberships).values({
        classId: cls.id,
        userId: student.id,
        role: "student",
      });

      const summary = await getAttendanceSummary(testDb, student.id);
      expect(summary).toHaveLength(1);
      expect(summary[0].classTitle).toBe("Python 101");
      expect(summary[0].sessionsAttended).toBe(0);
      expect(summary[0].sessionsTotal).toBe(0);
      expect(summary[0].attendanceRate).toBe(0);
    });

    it("tracks attended and total sessions", async () => {
      const cls = await createClass(testDb, {
        courseId: course.id,
        orgId: org.id,
        title: "Python 101",
        createdBy: teacher.id,
      });
      await testDb.insert(classMemberships).values({
        classId: cls.id,
        userId: student.id,
        role: "student",
      });

      // Get the auto-created classroom
      const [classroom] = await testDb
        .select()
        .from(newClassrooms)
        .where(eq(newClassrooms.classId, cls.id));

      // Create 3 sessions, student attended 2
      const s1 = await createTestSession(classroom.id, teacher.id, { status: "ended", endedAt: new Date() });
      const s2 = await createTestSession(classroom.id, teacher.id, { status: "ended", endedAt: new Date() });
      const _s3 = await createTestSession(classroom.id, teacher.id, { status: "ended", endedAt: new Date() });

      await testDb.insert(sessionParticipants).values({ sessionId: s1.id, studentId: student.id });
      await testDb.insert(sessionParticipants).values({ sessionId: s2.id, studentId: student.id });

      const summary = await getAttendanceSummary(testDb, student.id);
      expect(summary).toHaveLength(1);
      expect(summary[0].sessionsAttended).toBe(2);
      expect(summary[0].sessionsTotal).toBe(3);
      expect(summary[0].attendanceRate).toBeCloseTo(2 / 3);
    });

    it("handles multiple classes", async () => {
      const cls1 = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Class A", createdBy: teacher.id });
      const cls2 = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Class B", createdBy: teacher.id });

      await testDb.insert(classMemberships).values({ classId: cls1.id, userId: student.id, role: "student" });
      await testDb.insert(classMemberships).values({ classId: cls2.id, userId: student.id, role: "student" });

      const summary = await getAttendanceSummary(testDb, student.id);
      expect(summary).toHaveLength(2);
    });
  });

  describe("getSessionHistory", () => {
    it("returns empty when student has no participations", async () => {
      const history = await getSessionHistory(testDb, student.id);
      expect(history).toHaveLength(0);
    });

    it("returns sessions with topics covered", async () => {
      const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Python 101", createdBy: teacher.id });
      await testDb.insert(classMemberships).values({ classId: cls.id, userId: student.id, role: "student" });

      const [classroom] = await testDb.select().from(newClassrooms).where(eq(newClassrooms.classId, cls.id));

      const topic = await createTestTopic(course.id, { title: "Variables" });
      const session = await createTestSession(classroom.id, teacher.id, { status: "ended", endedAt: new Date() });

      await testDb.insert(sessionTopics).values({ sessionId: session.id, topicId: topic.id });
      await testDb.insert(sessionParticipants).values({ sessionId: session.id, studentId: student.id });

      const history = await getSessionHistory(testDb, student.id);
      expect(history).toHaveLength(1);
      expect(history[0].classTitle).toBe("Python 101");
      expect(history[0].topicsCovered).toContain("Variables");
    });

    it("respects limit parameter", async () => {
      const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Test", createdBy: teacher.id });
      await testDb.insert(classMemberships).values({ classId: cls.id, userId: student.id, role: "student" });

      const [classroom] = await testDb.select().from(newClassrooms).where(eq(newClassrooms.classId, cls.id));

      for (let i = 0; i < 5; i++) {
        const s = await createTestSession(classroom.id, teacher.id, { status: "ended", endedAt: new Date() });
        await testDb.insert(sessionParticipants).values({ sessionId: s.id, studentId: student.id });
      }

      const history = await getSessionHistory(testDb, student.id, 3);
      expect(history).toHaveLength(3);
    });
  });

  describe("getActiveSessionForStudent", () => {
    it("returns null when student has no active session", async () => {
      const result = await getActiveSessionForStudent(testDb, student.id);
      expect(result).toBeNull();
    });

    it("finds active session", async () => {
      const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Python 101", createdBy: teacher.id });
      await testDb.insert(classMemberships).values({ classId: cls.id, userId: student.id, role: "student" });

      const [classroom] = await testDb.select().from(newClassrooms).where(eq(newClassrooms.classId, cls.id));
      const session = await createTestSession(classroom.id, teacher.id, { status: "active" });

      await testDb.insert(sessionParticipants).values({ sessionId: session.id, studentId: student.id });

      const result = await getActiveSessionForStudent(testDb, student.id);
      expect(result).not.toBeNull();
      expect(result!.sessionId).toBe(session.id);
      expect(result!.classTitle).toBe("Python 101");
    });

    it("ignores sessions where student has left", async () => {
      const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Test", createdBy: teacher.id });
      await testDb.insert(classMemberships).values({ classId: cls.id, userId: student.id, role: "student" });

      const [classroom] = await testDb.select().from(newClassrooms).where(eq(newClassrooms.classId, cls.id));
      const session = await createTestSession(classroom.id, teacher.id, { status: "active" });

      await testDb.insert(sessionParticipants).values({
        sessionId: session.id,
        studentId: student.id,
        leftAt: new Date(),
      });

      const result = await getActiveSessionForStudent(testDb, student.id);
      expect(result).toBeNull();
    });

    it("ignores ended sessions", async () => {
      const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Test", createdBy: teacher.id });
      await testDb.insert(classMemberships).values({ classId: cls.id, userId: student.id, role: "student" });

      const [classroom] = await testDb.select().from(newClassrooms).where(eq(newClassrooms.classId, cls.id));
      const session = await createTestSession(classroom.id, teacher.id, { status: "ended", endedAt: new Date() });

      await testDb.insert(sessionParticipants).values({ sessionId: session.id, studentId: student.id });

      const result = await getActiveSessionForStudent(testDb, student.id);
      expect(result).toBeNull();
    });
  });
});
```

Note: The test file needs `import { eq } from "drizzle-orm";` at the top for the `newClassrooms` query.

### Commit
`git commit -m "add attendance tracking library with queries for summary, history, and active session detection"`

---

## Task 3: Extended Parent Links

Add `isChildLinkedToParent()` helper and `getLinkedChildrenWithStatus()` that includes live session status per child.

### Files

- [ ] `src/lib/parent-links.ts` -- add new functions
- [ ] `tests/unit/parent-links-extended.test.ts` -- create

### Code

**`src/lib/parent-links.ts`** -- add at the end of the file:

```typescript
import { getActiveSessionForStudent } from "@/lib/attendance";

/**
 * Check if a specific child is linked to a parent.
 * More efficient than fetching all children when you only need one.
 */
export async function isChildLinkedToParent(
  db: Database,
  parentUserId: string,
  childUserId: string
): boolean {
  const parentMemberships = await db
    .select({ classId: classMemberships.classId })
    .from(classMemberships)
    .where(
      and(
        eq(classMemberships.userId, parentUserId),
        eq(classMemberships.role, "parent")
      )
    );

  if (parentMemberships.length === 0) return false;

  for (const pm of parentMemberships) {
    const [student] = await db
      .select({ userId: classMemberships.userId })
      .from(classMemberships)
      .where(
        and(
          eq(classMemberships.classId, pm.classId),
          eq(classMemberships.userId, childUserId),
          eq(classMemberships.role, "student")
        )
      );
    if (student) return true;
  }

  return false;
}

export interface LinkedChildWithStatus {
  userId: string;
  name: string;
  email: string;
  classCount: number;
  activeSession: {
    sessionId: string;
    classTitle: string;
    topicsCovered: string[];
  } | null;
  lastActivity: Date | null;
}

/**
 * Get children linked to a parent, including live session status.
 * Used for the enhanced parent dashboard.
 */
export async function getLinkedChildrenWithStatus(
  db: Database,
  parentUserId: string
): Promise<LinkedChildWithStatus[]> {
  const children = await getLinkedChildren(db, parentUserId);
  if (children.length === 0) return [];

  const result: LinkedChildWithStatus[] = [];

  for (const child of children) {
    const activeSession = await getActiveSessionForStudent(db, child.userId);

    // Get last activity: most recent session participation
    const { desc: descOrder } = await import("drizzle-orm");
    const { sessionParticipants, liveSessions } = await import("@/lib/db/schema");

    const [lastParticipation] = await db
      .select({ startedAt: liveSessions.startedAt })
      .from(sessionParticipants)
      .innerJoin(liveSessions, eq(sessionParticipants.sessionId, liveSessions.id))
      .where(eq(sessionParticipants.studentId, child.userId))
      .orderBy(descOrder(liveSessions.startedAt))
      .limit(1);

    result.push({
      userId: child.userId,
      name: child.name,
      email: child.email,
      classCount: child.classCount,
      activeSession: activeSession
        ? {
            sessionId: activeSession.sessionId,
            classTitle: activeSession.classTitle,
            topicsCovered: activeSession.topicsCovered,
          }
        : null,
      lastActivity: lastParticipation?.startedAt || null,
    });
  }

  return result;
}
```

Also need to add import at the top of parent-links.ts. The existing file already imports `eq, and` from `drizzle-orm` and `classMemberships, classes, users` from schema. Add the additional import for `getActiveSessionForStudent`. The function signature for `isChildLinkedToParent` needs the `Promise<boolean>` return type annotation.

**`tests/unit/parent-links-extended.test.ts`**:

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import { eq } from "drizzle-orm";
import {
  testDb,
  cleanupDatabase,
  createTestUser,
  createTestOrg,
  createTestCourse,
  createTestSession,
} from "../helpers";
import { classMemberships, newClassrooms, sessionParticipants } from "@/lib/db/schema";
import { createClass } from "@/lib/classes";
import { isChildLinkedToParent, getLinkedChildrenWithStatus } from "@/lib/parent-links";

describe("parent-links extended", () => {
  let org: any;
  let teacher: any;
  let student: any;
  let parent: any;
  let course: any;

  beforeEach(async () => {
    await cleanupDatabase();
    org = await createTestOrg();
    teacher = await createTestUser({ email: "teacher@test.edu" });
    student = await createTestUser({ name: "Alice Student", email: "student@test.edu" });
    parent = await createTestUser({ email: "parent@test.edu" });
    course = await createTestCourse(org.id, teacher.id);
  });

  describe("isChildLinkedToParent", () => {
    it("returns false when no link exists", async () => {
      const linked = await isChildLinkedToParent(testDb, parent.id, student.id);
      expect(linked).toBe(false);
    });

    it("returns true when parent and student share a class", async () => {
      const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Test", createdBy: teacher.id });
      await testDb.insert(classMemberships).values({ classId: cls.id, userId: student.id, role: "student" });
      await testDb.insert(classMemberships).values({ classId: cls.id, userId: parent.id, role: "parent" });

      const linked = await isChildLinkedToParent(testDb, parent.id, student.id);
      expect(linked).toBe(true);
    });

    it("returns false when parent is in class but child is not", async () => {
      const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Test", createdBy: teacher.id });
      await testDb.insert(classMemberships).values({ classId: cls.id, userId: parent.id, role: "parent" });

      const linked = await isChildLinkedToParent(testDb, parent.id, student.id);
      expect(linked).toBe(false);
    });
  });

  describe("getLinkedChildrenWithStatus", () => {
    it("returns empty when no children linked", async () => {
      const result = await getLinkedChildrenWithStatus(testDb, parent.id);
      expect(result).toHaveLength(0);
    });

    it("returns child with no active session", async () => {
      const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Test", createdBy: teacher.id });
      await testDb.insert(classMemberships).values({ classId: cls.id, userId: student.id, role: "student" });
      await testDb.insert(classMemberships).values({ classId: cls.id, userId: parent.id, role: "parent" });

      const result = await getLinkedChildrenWithStatus(testDb, parent.id);
      expect(result).toHaveLength(1);
      expect(result[0].name).toBe("Alice Student");
      expect(result[0].activeSession).toBeNull();
    });

    it("returns child with active session info", async () => {
      const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Python 101", createdBy: teacher.id });
      await testDb.insert(classMemberships).values({ classId: cls.id, userId: student.id, role: "student" });
      await testDb.insert(classMemberships).values({ classId: cls.id, userId: parent.id, role: "parent" });

      const [classroom] = await testDb.select().from(newClassrooms).where(eq(newClassrooms.classId, cls.id));
      const session = await createTestSession(classroom.id, teacher.id, { status: "active" });
      await testDb.insert(sessionParticipants).values({ sessionId: session.id, studentId: student.id });

      const result = await getLinkedChildrenWithStatus(testDb, parent.id);
      expect(result).toHaveLength(1);
      expect(result[0].activeSession).not.toBeNull();
      expect(result[0].activeSession!.sessionId).toBe(session.id);
      expect(result[0].activeSession!.classTitle).toBe("Python 101");
    });

    it("populates lastActivity from most recent session", async () => {
      const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Test", createdBy: teacher.id });
      await testDb.insert(classMemberships).values({ classId: cls.id, userId: student.id, role: "student" });
      await testDb.insert(classMemberships).values({ classId: cls.id, userId: parent.id, role: "parent" });

      const [classroom] = await testDb.select().from(newClassrooms).where(eq(newClassrooms.classId, cls.id));
      const session = await createTestSession(classroom.id, teacher.id, { status: "ended", endedAt: new Date() });
      await testDb.insert(sessionParticipants).values({ sessionId: session.id, studentId: student.id });

      const result = await getLinkedChildrenWithStatus(testDb, parent.id);
      expect(result).toHaveLength(1);
      expect(result[0].lastActivity).not.toBeNull();
    });
  });
});
```

### Commit
`git commit -m "extend parent-links with isChildLinkedToParent and getLinkedChildrenWithStatus including live session status"`

---

## Task 4: Hocuspocus Auth -- Allow Parent Observer Role

Update the Hocuspocus server to allow `parent` role tokens to connect as read-only observers.

### Files

- [ ] `server/hocuspocus.ts` -- modify `onAuthenticate` to allow `parent` role

### Code

**`server/hocuspocus.ts`** -- modify the `onAuthenticate` handler. Replace the current access check block:

```typescript
// Current:
if (role !== "teacher" && role !== "user" && userId !== docOwner) {
  throw new Error("Access denied");
}

// Replace with:
if (role !== "teacher" && role !== "parent" && role !== "user" && userId !== docOwner) {
  throw new Error("Access denied");
}
```

The `parent` role token format is `{parentUserId}:parent`. The parent can connect to `session:{sessionId}:user:{childUserId}` documents to observe their child's code. The actual parent-child relationship authorization happens in the API layer (the page server component verifies the link before providing the session info and token). The Hocuspocus layer only checks that the role is allowed.

Note: Parents connect as observers with `readOnly` mode on the Monaco editor client side. The Hocuspocus server does not enforce read-only at the transport layer (it allows syncing), but the client-side `CodeEditor` component with `readOnly={true}` prevents any edits from being generated. This matches how teacher observation already works.

### Commit
`git commit -m "allow parent role to connect to Hocuspocus for read-only session observation"`

---

## Task 5: AI Report Prompts and Generation Library

Create the LLM prompt for generating parent-friendly progress reports and the library to generate/store/retrieve them.

### Files

- [ ] `src/lib/ai/report-prompts.ts` -- create
- [ ] `src/lib/parent-reports.ts` -- create
- [ ] `tests/unit/parent-reports.test.ts` -- create

### Code

**`src/lib/ai/report-prompts.ts`**:

```typescript
export interface ReportInputData {
  childName: string;
  weekStart: string; // ISO date
  weekEnd: string;
  classes: {
    title: string;
    sessionsAttended: number;
    sessionsTotal: number;
    topicsCovered: string[];
  }[];
  assignments: {
    title: string;
    className: string;
    grade: number | null;
    feedback: string | null;
  }[];
  aiInteractionCount: number;
  annotationsSample: {
    content: string;
    authorType: "teacher" | "ai";
  }[];
}

export function getReportSystemPrompt(): string {
  return `You are a helpful education assistant generating weekly progress reports for parents.

RULES:
- Write in clear, friendly, parent-appropriate language
- Avoid technical jargon — explain programming concepts in simple terms
- Be encouraging and positive while being honest about areas for improvement
- Keep the summary concise: 3-5 paragraphs maximum
- Structure the report with: attendance overview, topics learned, assignment performance, engagement notes
- If the student has not been active this week, say so briefly and encouragingly
- Do not fabricate data — only reference what is provided
- Use the student's first name`;
}

export function getReportUserPrompt(data: ReportInputData): string {
  let prompt = `Generate a weekly progress report for the parent of ${data.childName}.\n\n`;
  prompt += `Week: ${data.weekStart} to ${data.weekEnd}\n\n`;

  if (data.classes.length === 0) {
    prompt += `The student was not enrolled in any classes this week.\n\n`;
  } else {
    prompt += `ATTENDANCE:\n`;
    for (const cls of data.classes) {
      prompt += `- ${cls.title}: attended ${cls.sessionsAttended} of ${cls.sessionsTotal} sessions\n`;
      if (cls.topicsCovered.length > 0) {
        prompt += `  Topics covered: ${cls.topicsCovered.join(", ")}\n`;
      }
    }
    prompt += `\n`;
  }

  if (data.assignments.length > 0) {
    prompt += `ASSIGNMENTS:\n`;
    for (const a of data.assignments) {
      const gradeStr = a.grade !== null ? `Grade: ${a.grade}%` : "Not graded yet";
      prompt += `- ${a.title} (${a.className}): ${gradeStr}`;
      if (a.feedback) prompt += ` — Teacher feedback: "${a.feedback}"`;
      prompt += `\n`;
    }
    prompt += `\n`;
  }

  prompt += `AI TUTOR USAGE: ${data.aiInteractionCount} conversation${data.aiInteractionCount !== 1 ? "s" : ""} with the AI tutor this week.\n\n`;

  if (data.annotationsSample.length > 0) {
    prompt += `TEACHER ANNOTATIONS (sample):\n`;
    for (const ann of data.annotationsSample.slice(0, 5)) {
      prompt += `- [${ann.authorType}]: "${ann.content}"\n`;
    }
    prompt += `\n`;
  }

  prompt += `Write the progress report now.`;
  return prompt;
}
```

**`src/lib/parent-reports.ts`**:

```typescript
import { eq, and, desc, gte, lte } from "drizzle-orm";
import {
  parentReports,
  sessionParticipants,
  liveSessions,
  sessionTopics,
  topics,
  aiInteractions,
  codeAnnotations,
  submissions,
  assignments,
  documents,
  classes,
  classMemberships,
  newClassrooms,
} from "@/lib/db/schema";
import type { Database } from "@/lib/db";
import { getAttendanceSummary, getSessionHistory } from "@/lib/attendance";
import {
  getReportSystemPrompt,
  getReportUserPrompt,
  type ReportInputData,
} from "@/lib/ai/report-prompts";
import {
  isAnthropicBackend,
  getAnthropicClient,
  getOpenAIClient,
  getModel,
} from "@/lib/ai/client";

/**
 * Get all reports for a parent-child pair, most recent first.
 */
export async function listReports(
  db: Database,
  parentId: string,
  childId: string
) {
  return db
    .select()
    .from(parentReports)
    .where(
      and(
        eq(parentReports.parentId, parentId),
        eq(parentReports.childId, childId)
      )
    )
    .orderBy(desc(parentReports.weekStart));
}

/**
 * Get a single report by ID, verifying parent ownership.
 */
export async function getReport(
  db: Database,
  reportId: string,
  parentId: string
) {
  const [report] = await db
    .select()
    .from(parentReports)
    .where(
      and(
        eq(parentReports.id, reportId),
        eq(parentReports.parentId, parentId)
      )
    );
  return report || null;
}

/**
 * Check if a report already exists for this child for a given week.
 */
export async function getExistingReport(
  db: Database,
  parentId: string,
  childId: string,
  weekStart: Date
) {
  const [report] = await db
    .select()
    .from(parentReports)
    .where(
      and(
        eq(parentReports.parentId, parentId),
        eq(parentReports.childId, childId),
        eq(parentReports.weekStart, weekStart)
      )
    );
  return report || null;
}

/**
 * Compute the Monday-Sunday week boundaries for a given date.
 */
export function getWeekBounds(date: Date): { weekStart: Date; weekEnd: Date } {
  const weekStart = new Date(date);
  const day = weekStart.getDay();
  // Adjust to Monday (day 1). If Sunday (0), go back 6 days.
  const diff = day === 0 ? 6 : day - 1;
  weekStart.setDate(weekStart.getDate() - diff);
  weekStart.setHours(0, 0, 0, 0);

  const weekEnd = new Date(weekStart);
  weekEnd.setDate(weekEnd.getDate() + 6);
  weekEnd.setHours(23, 59, 59, 999);

  return { weekStart, weekEnd };
}

/**
 * Gather all data needed to generate a report for a child over a week.
 */
export async function gatherReportData(
  db: Database,
  childName: string,
  childId: string,
  weekStart: Date,
  weekEnd: Date
): Promise<ReportInputData> {
  // 1. Attendance summary
  const attendanceSummaries = await getAttendanceSummary(db, childId);

  // 2. Session history for the week
  const allHistory = await getSessionHistory(db, childId, 100);
  const weekHistory = allHistory.filter(
    (h) => h.startedAt >= weekStart && h.startedAt <= weekEnd
  );

  // Build per-class data
  const classMap = new Map<string, { title: string; sessionsAttended: number; sessionsTotal: number; topicsCovered: Set<string> }>();

  for (const summary of attendanceSummaries) {
    classMap.set(summary.classId, {
      title: summary.classTitle,
      sessionsAttended: 0, // We'll count only this week's
      sessionsTotal: summary.sessionsTotal,
      topicsCovered: new Set(),
    });
  }

  for (const h of weekHistory) {
    // Find matching class by title (session history returns classTitle)
    for (const [, data] of classMap) {
      if (data.title === h.classTitle) {
        data.sessionsAttended++;
        h.topicsCovered.forEach((t) => data.topicsCovered.add(t));
      }
    }
  }

  const classesData = Array.from(classMap.values()).map((d) => ({
    title: d.title,
    sessionsAttended: d.sessionsAttended,
    sessionsTotal: d.sessionsTotal,
    topicsCovered: Array.from(d.topicsCovered),
  }));

  // 3. AI interactions count for the week
  const weekSessions = weekHistory.map((h) => h.sessionId);
  let aiInteractionCount = 0;
  for (const sessionId of weekSessions) {
    const interactions = await db
      .select()
      .from(aiInteractions)
      .where(
        and(
          eq(aiInteractions.studentId, childId),
          eq(aiInteractions.sessionId, sessionId)
        )
      );
    aiInteractionCount += interactions.length;
  }

  // 4. Assignments submitted/graded this week
  const allSubmissions = await db
    .select({
      grade: submissions.grade,
      feedback: submissions.feedback,
      assignmentTitle: assignments.title,
      classId: assignments.classId,
      submittedAt: submissions.submittedAt,
    })
    .from(submissions)
    .innerJoin(assignments, eq(submissions.assignmentId, assignments.id))
    .where(eq(submissions.studentId, childId));

  const weekSubmissions = allSubmissions.filter(
    (s) => s.submittedAt >= weekStart && s.submittedAt <= weekEnd
  );

  const assignmentsData = weekSubmissions.map((s) => ({
    title: s.assignmentTitle,
    className: classMap.get(s.classId)?.title || "Unknown",
    grade: s.grade,
    feedback: s.feedback,
  }));

  // 5. Teacher annotations sample from this week's documents
  const weekDocs = await db
    .select({ id: documents.id })
    .from(documents)
    .where(eq(documents.ownerId, childId));

  const annotationsSample: { content: string; authorType: "teacher" | "ai" }[] = [];
  for (const doc of weekDocs.slice(0, 20)) {
    const annotations = await db
      .select({ content: codeAnnotations.content, authorType: codeAnnotations.authorType, createdAt: codeAnnotations.createdAt })
      .from(codeAnnotations)
      .where(eq(codeAnnotations.documentId, doc.id));

    for (const ann of annotations) {
      if (ann.createdAt >= weekStart && ann.createdAt <= weekEnd) {
        annotationsSample.push({ content: ann.content, authorType: ann.authorType });
      }
    }
    if (annotationsSample.length >= 5) break;
  }

  return {
    childName,
    weekStart: weekStart.toISOString().split("T")[0],
    weekEnd: weekEnd.toISOString().split("T")[0],
    classes: classesData,
    assignments: assignmentsData,
    aiInteractionCount,
    annotationsSample,
  };
}

/**
 * Generate a progress report using the LLM.
 */
export async function generateReportText(data: ReportInputData): Promise<string> {
  const systemPrompt = getReportSystemPrompt();
  const userPrompt = getReportUserPrompt(data);

  if (isAnthropicBackend()) {
    const client = getAnthropicClient();
    const response = await client.messages.create({
      model: getModel(),
      max_tokens: 1024,
      system: systemPrompt,
      messages: [{ role: "user", content: userPrompt }],
    });
    const textBlock = response.content.find((b) => b.type === "text");
    return textBlock?.text || "Unable to generate report.";
  } else {
    const client = getOpenAIClient();
    const response = await client.chat.completions.create({
      model: getModel(),
      max_tokens: 1024,
      messages: [
        { role: "system", content: systemPrompt },
        { role: "user", content: userPrompt },
      ],
    });
    return response.choices[0]?.message?.content || "Unable to generate report.";
  }
}

/**
 * Generate and store a weekly report for a parent-child pair.
 * Returns the existing report if one already exists for this week.
 */
export async function generateAndStoreReport(
  db: Database,
  parentId: string,
  childId: string,
  childName: string,
  targetDate: Date = new Date()
) {
  const { weekStart, weekEnd } = getWeekBounds(targetDate);

  // Check for existing report
  const existing = await getExistingReport(db, parentId, childId, weekStart);
  if (existing) return existing;

  // Gather data
  const data = await gatherReportData(db, childName, childId, weekStart, weekEnd);

  // Generate report text via LLM
  const summary = await generateReportText(data);

  // Store
  const details = {
    sessionsAttended: data.classes.reduce((sum, c) => sum + c.sessionsAttended, 0),
    sessionsTotal: data.classes.reduce((sum, c) => sum + c.sessionsTotal, 0),
    topicsCovered: data.classes.flatMap((c) => c.topicsCovered),
    assignmentsGraded: data.assignments
      .filter((a) => a.grade !== null)
      .map((a) => ({ title: a.title, grade: a.grade! })),
    aiInteractionCount: data.aiInteractionCount,
    annotationCount: data.annotationsSample.length,
  };

  const [report] = await db
    .insert(parentReports)
    .values({
      parentId,
      childId,
      weekStart,
      weekEnd,
      summary,
      details,
    })
    .returning();

  return report;
}
```

**`tests/unit/parent-reports.test.ts`**:

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  cleanupDatabase,
  createTestUser,
  createTestOrg,
  createTestCourse,
  createTestParentReport,
} from "../helpers";
import {
  listReports,
  getReport,
  getExistingReport,
  getWeekBounds,
  gatherReportData,
} from "@/lib/parent-reports";
import { getReportSystemPrompt, getReportUserPrompt } from "@/lib/ai/report-prompts";

describe("parent-reports", () => {
  let org: any;
  let teacher: any;
  let student: any;
  let parent: any;
  let course: any;

  beforeEach(async () => {
    await cleanupDatabase();
    org = await createTestOrg();
    teacher = await createTestUser({ email: "teacher@test.edu" });
    student = await createTestUser({ name: "Alice", email: "student@test.edu" });
    parent = await createTestUser({ email: "parent@test.edu" });
    course = await createTestCourse(org.id, teacher.id);
  });

  describe("getWeekBounds", () => {
    it("returns Monday-Sunday for a Wednesday", () => {
      // 2026-04-08 is a Wednesday
      const date = new Date("2026-04-08T12:00:00Z");
      const { weekStart, weekEnd } = getWeekBounds(date);

      expect(weekStart.getDay()).toBe(1); // Monday
      expect(weekEnd.getDay()).toBe(0); // Sunday
      expect(weekStart.getDate()).toBe(6); // April 6
      expect(weekEnd.getDate()).toBe(12); // April 12
    });

    it("handles Sunday correctly", () => {
      // 2026-04-12 is a Sunday
      const date = new Date("2026-04-12T12:00:00Z");
      const { weekStart, weekEnd } = getWeekBounds(date);

      expect(weekStart.getDay()).toBe(1); // Monday
      expect(weekStart.getDate()).toBe(6); // April 6
      expect(weekEnd.getDate()).toBe(12); // April 12
    });

    it("handles Monday correctly", () => {
      // 2026-04-06 is a Monday
      const date = new Date("2026-04-06T12:00:00Z");
      const { weekStart, weekEnd } = getWeekBounds(date);

      expect(weekStart.getDay()).toBe(1);
      expect(weekStart.getDate()).toBe(6);
      expect(weekEnd.getDate()).toBe(12);
    });
  });

  describe("listReports", () => {
    it("returns empty when no reports exist", async () => {
      const reports = await listReports(testDb, parent.id, student.id);
      expect(reports).toHaveLength(0);
    });

    it("returns reports for the correct parent-child pair", async () => {
      await createTestParentReport(parent.id, student.id);

      const reports = await listReports(testDb, parent.id, student.id);
      expect(reports).toHaveLength(1);
    });

    it("does not return reports for a different child", async () => {
      const otherStudent = await createTestUser({ email: "other@test.edu" });
      await createTestParentReport(parent.id, otherStudent.id);

      const reports = await listReports(testDb, parent.id, student.id);
      expect(reports).toHaveLength(0);
    });
  });

  describe("getReport", () => {
    it("returns null for non-existent report", async () => {
      const report = await getReport(testDb, "00000000-0000-0000-0000-000000000000", parent.id);
      expect(report).toBeNull();
    });

    it("returns report when parent matches", async () => {
      const created = await createTestParentReport(parent.id, student.id);
      const report = await getReport(testDb, created.id, parent.id);
      expect(report).not.toBeNull();
      expect(report!.id).toBe(created.id);
    });

    it("returns null when parent does not match", async () => {
      const otherParent = await createTestUser({ email: "other-parent@test.edu" });
      const created = await createTestParentReport(parent.id, student.id);
      const report = await getReport(testDb, created.id, otherParent.id);
      expect(report).toBeNull();
    });
  });

  describe("getExistingReport", () => {
    it("returns null when no report for the week", async () => {
      const report = await getExistingReport(testDb, parent.id, student.id, new Date("2026-01-05"));
      expect(report).toBeNull();
    });

    it("finds existing report for the same week", async () => {
      const weekStart = new Date("2026-04-06T00:00:00.000Z");
      await createTestParentReport(parent.id, student.id, { weekStart });

      const report = await getExistingReport(testDb, parent.id, student.id, weekStart);
      expect(report).not.toBeNull();
    });
  });

  describe("gatherReportData", () => {
    it("returns empty data when student has no activity", async () => {
      const weekStart = new Date("2026-04-06T00:00:00.000Z");
      const weekEnd = new Date("2026-04-12T23:59:59.999Z");

      const data = await gatherReportData(testDb, "Alice", student.id, weekStart, weekEnd);
      expect(data.childName).toBe("Alice");
      expect(data.classes).toHaveLength(0);
      expect(data.assignments).toHaveLength(0);
      expect(data.aiInteractionCount).toBe(0);
      expect(data.annotationsSample).toHaveLength(0);
    });
  });
});

describe("report-prompts", () => {
  describe("getReportSystemPrompt", () => {
    it("returns a non-empty system prompt", () => {
      const prompt = getReportSystemPrompt();
      expect(prompt.length).toBeGreaterThan(50);
      expect(prompt).toContain("parent");
    });
  });

  describe("getReportUserPrompt", () => {
    it("includes child name and week", () => {
      const prompt = getReportUserPrompt({
        childName: "Alice",
        weekStart: "2026-04-06",
        weekEnd: "2026-04-12",
        classes: [],
        assignments: [],
        aiInteractionCount: 0,
        annotationsSample: [],
      });
      expect(prompt).toContain("Alice");
      expect(prompt).toContain("2026-04-06");
    });

    it("includes class attendance data", () => {
      const prompt = getReportUserPrompt({
        childName: "Alice",
        weekStart: "2026-04-06",
        weekEnd: "2026-04-12",
        classes: [{ title: "Python 101", sessionsAttended: 3, sessionsTotal: 5, topicsCovered: ["Loops"] }],
        assignments: [],
        aiInteractionCount: 0,
        annotationsSample: [],
      });
      expect(prompt).toContain("Python 101");
      expect(prompt).toContain("3 of 5");
      expect(prompt).toContain("Loops");
    });

    it("includes assignment grades", () => {
      const prompt = getReportUserPrompt({
        childName: "Alice",
        weekStart: "2026-04-06",
        weekEnd: "2026-04-12",
        classes: [],
        assignments: [{ title: "Homework 1", className: "Python 101", grade: 85, feedback: "Good work!" }],
        aiInteractionCount: 0,
        annotationsSample: [],
      });
      expect(prompt).toContain("Homework 1");
      expect(prompt).toContain("85%");
      expect(prompt).toContain("Good work!");
    });

    it("includes AI interaction count", () => {
      const prompt = getReportUserPrompt({
        childName: "Alice",
        weekStart: "2026-04-06",
        weekEnd: "2026-04-12",
        classes: [],
        assignments: [],
        aiInteractionCount: 6,
        annotationsSample: [],
      });
      expect(prompt).toContain("6 conversations");
    });

    it("uses singular for 1 interaction", () => {
      const prompt = getReportUserPrompt({
        childName: "Alice",
        weekStart: "2026-04-06",
        weekEnd: "2026-04-12",
        classes: [],
        assignments: [],
        aiInteractionCount: 1,
        annotationsSample: [],
      });
      expect(prompt).toContain("1 conversation ");
      expect(prompt).not.toContain("1 conversations");
    });
  });
});
```

### Commit
`git commit -m "add AI report generation library with LLM prompts, data gathering, and caching"`

---

## Task 6: Live Session Viewer Component

Create the client component for parents to view their child's live session. Uses read-only Monaco editor with Yjs binding, shows lesson content, and displays annotations in real-time.

### Files

- [ ] `src/components/parent/live-session-viewer.tsx` -- create
- [ ] `src/components/parent/live-now-badge.tsx` -- create

### Code

**`src/components/parent/live-now-badge.tsx`**:

```typescript
"use client";

interface LiveNowBadgeProps {
  className?: string;
  size?: "sm" | "md";
}

export function LiveNowBadge({ className = "", size = "sm" }: LiveNowBadgeProps) {
  const sizeClasses = size === "sm" ? "text-xs px-2 py-0.5" : "text-sm px-3 py-1";

  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full bg-red-100 text-red-700 font-medium ${sizeClasses} ${className}`}
    >
      <span className="relative flex h-2 w-2">
        <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75" />
        <span className="relative inline-flex rounded-full h-2 w-2 bg-red-500" />
      </span>
      Live Now
    </span>
  );
}
```

**`src/components/parent/live-session-viewer.tsx`**:

```typescript
"use client";

import { useState, useEffect } from "react";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { CodeEditor } from "@/components/editor/code-editor";
import { LessonRenderer } from "@/components/lesson/lesson-renderer";
import { AnnotationList } from "@/components/annotations/annotation-list";
import { LiveNowBadge } from "./live-now-badge";
import { parseLessonContent } from "@/lib/lesson-content";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

interface SessionTopic {
  title: string;
  lessonContent: unknown;
}

interface LiveSessionViewerProps {
  sessionId: string;
  childId: string;
  childName: string;
  classTitle: string;
  parentId: string;
  topics: SessionTopic[];
  editorLanguage: string;
}

interface Annotation {
  id: string;
  lineStart: string;
  lineEnd: string;
  content: string;
  authorType: "teacher" | "ai";
  createdAt: string;
}

export function LiveSessionViewer({
  sessionId,
  childId,
  childName,
  classTitle,
  parentId,
  topics,
  editorLanguage,
}: LiveSessionViewerProps) {
  const [annotations, setAnnotations] = useState<Annotation[]>([]);
  const [sessionEnded, setSessionEnded] = useState(false);

  // Connect to the child's Yjs document as read-only observer
  const documentName = `session:${sessionId}:user:${childId}`;
  const token = `${parentId}:parent`;

  const { yText, provider, connected } = useYjsProvider({
    documentName,
    token,
  });

  // Poll for annotations
  useEffect(() => {
    const docId = `session:${sessionId}:user:${childId}`;

    async function fetchAnnotations() {
      const res = await fetch(`/api/annotations?documentId=${encodeURIComponent(docId)}`);
      if (res.ok) setAnnotations(await res.json());
    }

    fetchAnnotations();
    const interval = setInterval(fetchAnnotations, 5000);
    return () => clearInterval(interval);
  }, [sessionId, childId]);

  // Poll to detect session end
  useEffect(() => {
    async function checkSession() {
      const res = await fetch(`/api/parent/children/${childId}/live-session`);
      if (res.ok) {
        const data = await res.json();
        if (!data.active) setSessionEnded(true);
      } else {
        setSessionEnded(true);
      }
    }

    const interval = setInterval(checkSession, 10000);
    return () => clearInterval(interval);
  }, [childId]);

  if (sessionEnded) {
    return (
      <div className="p-6 text-center space-y-4">
        <h2 className="text-xl font-semibold">Session Ended</h2>
        <p className="text-muted-foreground">
          {childName}'s session has ended. You can view their work in the child detail page.
        </p>
        <a
          href={`/parent/children/${childId}`}
          className="inline-block px-4 py-2 rounded-lg bg-primary text-primary-foreground text-sm"
        >
          Back to {childName}'s Profile
        </a>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-[calc(100vh-4rem)]">
      {/* Header */}
      <div className="flex items-center justify-between px-6 py-3 border-b bg-card">
        <div className="flex items-center gap-3">
          <h2 className="text-lg font-semibold">{childName}</h2>
          <span className="text-muted-foreground">in</span>
          <span className="font-medium">{classTitle}</span>
          <LiveNowBadge size="md" />
        </div>
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <span className={`w-2 h-2 rounded-full ${connected ? "bg-green-500" : "bg-yellow-500"}`} />
          {connected ? "Connected" : "Connecting..."}
        </div>
      </div>

      {/* Main content: lesson + editor + annotations */}
      <div className="flex flex-1 min-h-0">
        {/* Left: Lesson content */}
        {topics.length > 0 && (
          <div className="w-1/3 border-r overflow-auto p-4 space-y-4">
            <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
              Lesson Content
            </h3>
            {topics.map((topic, i) => (
              <div key={i}>
                <h4 className="font-medium mb-2">{topic.title}</h4>
                <LessonRenderer content={parseLessonContent(topic.lessonContent)} />
              </div>
            ))}
          </div>
        )}

        {/* Center: Code editor (read-only) */}
        <div className={`flex-1 flex flex-col min-h-0 ${topics.length === 0 ? "" : ""}`}>
          <div className="px-4 py-2 border-b bg-muted/30">
            <p className="text-xs text-muted-foreground">
              Read-only view of {childName}'s code
            </p>
          </div>
          <div className="flex-1 min-h-0">
            <CodeEditor
              readOnly={true}
              language={editorLanguage}
              yText={yText}
              provider={provider}
            />
          </div>
        </div>

        {/* Right: Annotations */}
        <div className="w-64 border-l overflow-auto p-3">
          <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wide mb-3">
            Teacher Notes
          </h3>
          <AnnotationList
            annotations={annotations}
            onDelete={() => {}} // Parents cannot delete annotations
          />
        </div>
      </div>
    </div>
  );
}
```

### Commit
`git commit -m "add LiveSessionViewer and LiveNowBadge components for parent read-only session observation"`

---

## Task 7: Parent Dashboard and Detail Components

Create reusable components for the enhanced parent pages.

### Files

- [ ] `src/components/parent/attendance-summary.tsx` -- create
- [ ] `src/components/parent/attendance-history.tsx` -- create
- [ ] `src/components/parent/grade-summary.tsx` -- create
- [ ] `src/components/parent/ai-interactions-summary.tsx` -- create
- [ ] `src/components/parent/report-card.tsx` -- create

### Code

**`src/components/parent/attendance-summary.tsx`**:

```typescript
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import type { AttendanceSummary as AttendanceSummaryData } from "@/lib/attendance";

interface AttendanceSummaryProps {
  summaries: AttendanceSummaryData[];
}

export function AttendanceSummary({ summaries }: AttendanceSummaryProps) {
  if (summaries.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">No class enrollments yet.</p>
    );
  }

  return (
    <div className="grid gap-3 sm:grid-cols-2">
      {summaries.map((s) => {
        const pct = Math.round(s.attendanceRate * 100);
        return (
          <Card key={s.classId}>
            <CardContent className="py-3">
              <div className="flex items-center justify-between mb-1">
                <span className="font-medium text-sm">{s.classTitle}</span>
                <span className="text-sm text-muted-foreground">
                  {s.sessionsAttended}/{s.sessionsTotal}
                </span>
              </div>
              <div className="w-full bg-muted rounded-full h-2">
                <div
                  className={`h-2 rounded-full transition-all ${
                    pct >= 80 ? "bg-green-500" : pct >= 60 ? "bg-yellow-500" : "bg-red-500"
                  }`}
                  style={{ width: `${pct}%` }}
                />
              </div>
              <p className="text-xs text-muted-foreground mt-1">{pct}% attendance</p>
            </CardContent>
          </Card>
        );
      })}
    </div>
  );
}
```

**`src/components/parent/attendance-history.tsx`**:

```typescript
import { Card, CardContent } from "@/components/ui/card";
import type { SessionHistory } from "@/lib/attendance";

interface AttendanceHistoryProps {
  sessions: SessionHistory[];
}

export function AttendanceHistory({ sessions }: AttendanceHistoryProps) {
  if (sessions.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">No session history yet.</p>
    );
  }

  return (
    <div className="space-y-2">
      {sessions.map((s) => (
        <Card key={s.sessionId}>
          <CardContent className="py-3">
            <div className="flex items-center justify-between">
              <div>
                <span className="font-medium text-sm">{s.classTitle}</span>
                {s.topicsCovered.length > 0 && (
                  <p className="text-xs text-muted-foreground mt-0.5">
                    Topics: {s.topicsCovered.join(", ")}
                  </p>
                )}
              </div>
              <div className="text-right">
                <p className="text-xs text-muted-foreground">
                  {new Date(s.startedAt).toLocaleDateString()}
                </p>
                {s.endedAt && (
                  <p className="text-xs text-muted-foreground">
                    {formatDuration(s.startedAt, s.endedAt)}
                  </p>
                )}
              </div>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

function formatDuration(start: Date, end: Date): string {
  const ms = new Date(end).getTime() - new Date(start).getTime();
  const minutes = Math.round(ms / 60000);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  const remaining = minutes % 60;
  return `${hours}h ${remaining}m`;
}
```

**`src/components/parent/grade-summary.tsx`**:

```typescript
import { Card, CardContent } from "@/components/ui/card";

export interface GradeEntry {
  assignmentTitle: string;
  className: string;
  grade: number | null;
  feedback: string | null;
  submittedAt: Date;
}

interface GradeSummaryProps {
  grades: GradeEntry[];
}

export function GradeSummary({ grades }: GradeSummaryProps) {
  if (grades.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">No graded assignments yet.</p>
    );
  }

  return (
    <div className="space-y-2">
      {grades.map((g, i) => (
        <Card key={i}>
          <CardContent className="py-3">
            <div className="flex items-center justify-between">
              <div>
                <span className="font-medium text-sm">{g.assignmentTitle}</span>
                <p className="text-xs text-muted-foreground">{g.className}</p>
              </div>
              <div className="text-right">
                {g.grade !== null ? (
                  <span
                    className={`text-sm font-semibold ${
                      g.grade >= 80 ? "text-green-600" : g.grade >= 60 ? "text-yellow-600" : "text-red-600"
                    }`}
                  >
                    {g.grade}%
                  </span>
                ) : (
                  <span className="text-xs text-muted-foreground">Pending</span>
                )}
              </div>
            </div>
            {g.feedback && (
              <p className="text-xs text-muted-foreground mt-1 italic">
                "{g.feedback}"
              </p>
            )}
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
```

**`src/components/parent/ai-interactions-summary.tsx`**:

```typescript
import { Card, CardContent } from "@/components/ui/card";

export interface AIInteractionEntry {
  sessionDate: Date;
  className: string;
  messageCount: number;
}

interface AIInteractionsSummaryProps {
  interactions: AIInteractionEntry[];
  totalCount: number;
}

export function AIInteractionsSummary({ interactions, totalCount }: AIInteractionsSummaryProps) {
  if (interactions.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">No AI tutor conversations yet.</p>
    );
  }

  return (
    <div>
      <p className="text-sm text-muted-foreground mb-3">
        {totalCount} total conversation{totalCount !== 1 ? "s" : ""} with the AI tutor
      </p>
      <div className="space-y-2">
        {interactions.slice(0, 10).map((entry, i) => (
          <Card key={i}>
            <CardContent className="py-2 flex items-center justify-between">
              <div>
                <span className="text-sm">{entry.className}</span>
                <p className="text-xs text-muted-foreground">
                  {new Date(entry.sessionDate).toLocaleDateString()}
                </p>
              </div>
              <span className="text-xs text-muted-foreground">
                {entry.messageCount} message{entry.messageCount !== 1 ? "s" : ""}
              </span>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}
```

**`src/components/parent/report-card.tsx`**:

```typescript
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import ReactMarkdown from "react-markdown";

interface ReportDetails {
  sessionsAttended: number;
  sessionsTotal: number;
  topicsCovered: string[];
  assignmentsGraded: { title: string; grade: number }[];
  aiInteractionCount: number;
  annotationCount: number;
}

interface ReportCardProps {
  weekStart: Date;
  weekEnd: Date;
  summary: string;
  details: ReportDetails;
  generatedAt: Date;
}

export function ReportCard({ weekStart, weekEnd, summary, details, generatedAt }: ReportCardProps) {
  const startStr = new Date(weekStart).toLocaleDateString("en-US", { month: "short", day: "numeric" });
  const endStr = new Date(weekEnd).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" });

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">
          Week of {startStr} - {endStr}
        </CardTitle>
        <p className="text-xs text-muted-foreground">
          Generated {new Date(generatedAt).toLocaleDateString()}
        </p>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Quick stats */}
        <div className="flex flex-wrap gap-4 text-sm">
          <div className="flex items-center gap-1.5">
            <span className="text-muted-foreground">Attendance:</span>
            <span className="font-medium">{details.sessionsAttended}/{details.sessionsTotal}</span>
          </div>
          {details.topicsCovered.length > 0 && (
            <div className="flex items-center gap-1.5">
              <span className="text-muted-foreground">Topics:</span>
              <span className="font-medium">{details.topicsCovered.length}</span>
            </div>
          )}
          <div className="flex items-center gap-1.5">
            <span className="text-muted-foreground">AI Conversations:</span>
            <span className="font-medium">{details.aiInteractionCount}</span>
          </div>
        </div>

        {/* AI-generated summary */}
        <div className="prose prose-sm dark:prose-invert max-w-none border-t pt-4">
          <ReactMarkdown>{summary}</ReactMarkdown>
        </div>
      </CardContent>
    </Card>
  );
}
```

### Commit
`git commit -m "add parent portal display components: attendance, grades, AI interactions, and report card"`

---

## Task 8: API Routes for Parent Portal

Create API routes for live session info and report generation/retrieval.

### Files

- [ ] `src/app/api/parent/children/[id]/live-session/route.ts` -- create
- [ ] `src/app/api/parent/children/[id]/reports/route.ts` -- create
- [ ] `src/app/api/parent/children/[id]/reports/[reportId]/route.ts` -- create
- [ ] `tests/integration/parent-live-session-api.test.ts` -- create
- [ ] `tests/integration/parent-reports-api.test.ts` -- create

### Code

**`src/app/api/parent/children/[id]/live-session/route.ts`**:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { isChildLinkedToParent } from "@/lib/parent-links";
import { getActiveSessionForStudent } from "@/lib/attendance";
import { getSessionTopics } from "@/lib/session-topics";
import { getClassroom } from "@/lib/classes";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id: childId } = await params;

  // Verify parent-child link
  const linked = await isChildLinkedToParent(db, session.user.id, childId);
  if (!linked) {
    return NextResponse.json({ error: "Not linked to this child" }, { status: 403 });
  }

  // Get active session
  const activeSession = await getActiveSessionForStudent(db, childId);
  if (!activeSession) {
    return NextResponse.json({ active: false });
  }

  // Get session topics with lesson content
  const sessionTopicData = await getSessionTopics(db, activeSession.sessionId);

  // Get editor language from classroom
  const classroom = await getClassroom(db, activeSession.classId);
  const editorLanguage = classroom?.editorMode || "python";

  return NextResponse.json({
    active: true,
    sessionId: activeSession.sessionId,
    classId: activeSession.classId,
    classTitle: activeSession.classTitle,
    startedAt: activeSession.startedAt,
    editorLanguage,
    topics: sessionTopicData.map((t) => ({
      title: t.title,
      lessonContent: t.lessonContent,
    })),
  });
}
```

**`src/app/api/parent/children/[id]/reports/route.ts`**:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { users } from "@/lib/db/schema";
import { eq } from "drizzle-orm";
import { isChildLinkedToParent } from "@/lib/parent-links";
import { listReports, generateAndStoreReport } from "@/lib/parent-reports";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id: childId } = await params;

  const linked = await isChildLinkedToParent(db, session.user.id, childId);
  if (!linked) {
    return NextResponse.json({ error: "Not linked to this child" }, { status: 403 });
  }

  const reports = await listReports(db, session.user.id, childId);
  return NextResponse.json(reports);
}

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id: childId } = await params;

  const linked = await isChildLinkedToParent(db, session.user.id, childId);
  if (!linked) {
    return NextResponse.json({ error: "Not linked to this child" }, { status: 403 });
  }

  // Get child name
  const [child] = await db.select({ name: users.name }).from(users).where(eq(users.id, childId));
  if (!child) {
    return NextResponse.json({ error: "Child not found" }, { status: 404 });
  }

  // Parse optional targetDate from body, default to now
  let targetDate = new Date();
  try {
    const body = await request.json();
    if (body.targetDate) {
      targetDate = new Date(body.targetDate);
    }
  } catch {
    // Empty body is fine, use default
  }

  try {
    const report = await generateAndStoreReport(
      db,
      session.user.id,
      childId,
      child.name,
      targetDate
    );
    return NextResponse.json(report, { status: 201 });
  } catch (err) {
    console.error("Failed to generate report:", err);
    return NextResponse.json({ error: "Failed to generate report" }, { status: 500 });
  }
}
```

**`src/app/api/parent/children/[id]/reports/[reportId]/route.ts`**:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { isChildLinkedToParent } from "@/lib/parent-links";
import { getReport } from "@/lib/parent-reports";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string; reportId: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id: childId, reportId } = await params;

  const linked = await isChildLinkedToParent(db, session.user.id, childId);
  if (!linked) {
    return NextResponse.json({ error: "Not linked to this child" }, { status: 403 });
  }

  const report = await getReport(db, reportId, session.user.id);
  if (!report) {
    return NextResponse.json({ error: "Report not found" }, { status: 404 });
  }

  return NextResponse.json(report);
}
```

**`tests/integration/parent-live-session-api.test.ts`**:

```typescript
import { describe, it, expect, beforeEach, vi } from "vitest";
import { eq } from "drizzle-orm";
import {
  testDb,
  cleanupDatabase,
  createTestUser,
  createTestOrg,
  createTestCourse,
  createTestSession,
} from "../helpers";
import { classMemberships, newClassrooms, sessionParticipants } from "@/lib/db/schema";
import { createClass } from "@/lib/classes";
import { getActiveSessionForStudent } from "@/lib/attendance";
import { isChildLinkedToParent } from "@/lib/parent-links";

describe("parent live session API logic", () => {
  let org: any;
  let teacher: any;
  let student: any;
  let parent: any;
  let course: any;

  beforeEach(async () => {
    await cleanupDatabase();
    org = await createTestOrg();
    teacher = await createTestUser({ email: "teacher@test.edu" });
    student = await createTestUser({ name: "Alice", email: "student@test.edu" });
    parent = await createTestUser({ email: "parent@test.edu" });
    course = await createTestCourse(org.id, teacher.id);
  });

  it("returns null when child has no active session", async () => {
    const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Test", createdBy: teacher.id });
    await testDb.insert(classMemberships).values({ classId: cls.id, userId: student.id, role: "student" });
    await testDb.insert(classMemberships).values({ classId: cls.id, userId: parent.id, role: "parent" });

    const linked = await isChildLinkedToParent(testDb, parent.id, student.id);
    expect(linked).toBe(true);

    const session = await getActiveSessionForStudent(testDb, student.id);
    expect(session).toBeNull();
  });

  it("returns session info when child is in active session", async () => {
    const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Python 101", createdBy: teacher.id });
    await testDb.insert(classMemberships).values({ classId: cls.id, userId: student.id, role: "student" });
    await testDb.insert(classMemberships).values({ classId: cls.id, userId: parent.id, role: "parent" });

    const [classroom] = await testDb.select().from(newClassrooms).where(eq(newClassrooms.classId, cls.id));
    const liveSession = await createTestSession(classroom.id, teacher.id, { status: "active" });
    await testDb.insert(sessionParticipants).values({ sessionId: liveSession.id, studentId: student.id });

    const result = await getActiveSessionForStudent(testDb, student.id);
    expect(result).not.toBeNull();
    expect(result!.sessionId).toBe(liveSession.id);
    expect(result!.classTitle).toBe("Python 101");
  });

  it("denies access when parent is not linked to child", async () => {
    const otherStudent = await createTestUser({ email: "other@test.edu" });
    const linked = await isChildLinkedToParent(testDb, parent.id, otherStudent.id);
    expect(linked).toBe(false);
  });
});
```

**`tests/integration/parent-reports-api.test.ts`**:

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  cleanupDatabase,
  createTestUser,
  createTestOrg,
  createTestCourse,
  createTestParentReport,
} from "../helpers";
import { classMemberships } from "@/lib/db/schema";
import { createClass } from "@/lib/classes";
import { isChildLinkedToParent } from "@/lib/parent-links";
import { listReports, getReport, getWeekBounds } from "@/lib/parent-reports";

describe("parent reports API logic", () => {
  let org: any;
  let teacher: any;
  let student: any;
  let parent: any;
  let course: any;

  beforeEach(async () => {
    await cleanupDatabase();
    org = await createTestOrg();
    teacher = await createTestUser({ email: "teacher@test.edu" });
    student = await createTestUser({ name: "Alice", email: "student@test.edu" });
    parent = await createTestUser({ email: "parent@test.edu" });
    course = await createTestCourse(org.id, teacher.id);
  });

  it("parent can list reports for linked child", async () => {
    const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Test", createdBy: teacher.id });
    await testDb.insert(classMemberships).values({ classId: cls.id, userId: student.id, role: "student" });
    await testDb.insert(classMemberships).values({ classId: cls.id, userId: parent.id, role: "parent" });

    await createTestParentReport(parent.id, student.id);
    await createTestParentReport(parent.id, student.id, {
      weekStart: new Date("2026-03-30"),
      weekEnd: new Date("2026-04-05"),
    });

    const linked = await isChildLinkedToParent(testDb, parent.id, student.id);
    expect(linked).toBe(true);

    const reports = await listReports(testDb, parent.id, student.id);
    expect(reports).toHaveLength(2);
  });

  it("parent can get a specific report", async () => {
    const report = await createTestParentReport(parent.id, student.id);
    const fetched = await getReport(testDb, report.id, parent.id);
    expect(fetched).not.toBeNull();
    expect(fetched!.summary).toBe("Test report summary.");
  });

  it("another parent cannot access the report", async () => {
    const otherParent = await createTestUser({ email: "other-parent@test.edu" });
    const report = await createTestParentReport(parent.id, student.id);
    const fetched = await getReport(testDb, report.id, otherParent.id);
    expect(fetched).toBeNull();
  });

  it("prevents access to unlinked child's reports", async () => {
    const otherStudent = await createTestUser({ email: "other@test.edu" });
    const linked = await isChildLinkedToParent(testDb, parent.id, otherStudent.id);
    expect(linked).toBe(false);
  });
});
```

### Commit
`git commit -m "add API routes for parent live session info and progress report generation/retrieval"`

---

## Task 9: Enhanced Parent Dashboard Page

Update the parent dashboard to show "Live Now" indicators, last activity dates, and overall progress per child.

### Files

- [ ] `src/app/(portal)/parent/page.tsx` -- rewrite

### Code

**`src/app/(portal)/parent/page.tsx`**:

```typescript
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getLinkedChildrenWithStatus } from "@/lib/parent-links";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { LiveNowBadge } from "@/components/parent/live-now-badge";
import Link from "next/link";

export default async function ParentDashboard() {
  const session = await auth();
  const children = await getLinkedChildrenWithStatus(db, session!.user.id);

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">Parent Dashboard</h1>

      {children.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            <p>No children linked yet.</p>
            <p className="text-sm mt-2">
              Your child's teacher will link your account to your child's classes.
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {children.map((child) => (
            <Link key={child.userId} href={`/parent/children/${child.userId}`}>
              <Card className="hover:border-primary transition-colors cursor-pointer">
                <CardHeader>
                  <div className="flex items-center justify-between">
                    <CardTitle className="text-lg">{child.name}</CardTitle>
                    {child.activeSession && <LiveNowBadge />}
                  </div>
                  <CardDescription>
                    {child.classCount} class{child.classCount !== 1 ? "es" : ""}
                  </CardDescription>
                </CardHeader>
                <CardContent className="pt-0 space-y-2">
                  {child.activeSession && (
                    <div className="text-sm">
                      <span className="text-muted-foreground">In session:</span>{" "}
                      <span className="font-medium">{child.activeSession.classTitle}</span>
                      {child.activeSession.topicsCovered.length > 0 && (
                        <p className="text-xs text-muted-foreground mt-0.5">
                          Topics: {child.activeSession.topicsCovered.join(", ")}
                        </p>
                      )}
                    </div>
                  )}
                  {child.lastActivity ? (
                    <p className="text-xs text-muted-foreground">
                      Last active: {new Date(child.lastActivity).toLocaleDateString()}
                    </p>
                  ) : (
                    <p className="text-xs text-muted-foreground">No activity yet</p>
                  )}
                </CardContent>
              </Card>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
```

### Commit
`git commit -m "enhance parent dashboard with Live Now indicators, active session details, and last activity dates"`

---

## Task 10: Enhanced Child Detail Page

Update the child detail page to show attendance stats, assignment grades, AI interaction summary, and a "Live Now" badge with a link to the live viewer.

### Files

- [ ] `src/app/(portal)/parent/children/[id]/page.tsx` -- rewrite

### Code

**`src/app/(portal)/parent/children/[id]/page.tsx`**:

```typescript
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { users, submissions, assignments, classes, aiInteractions, liveSessions, sessionParticipants } from "@/lib/db/schema";
import { eq, and } from "drizzle-orm";
import { listClassesByUser } from "@/lib/classes";
import { getLinkedChildren } from "@/lib/parent-links";
import { getAttendanceSummary, getSessionHistory, getActiveSessionForStudent } from "@/lib/attendance";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { AttendanceSummary } from "@/components/parent/attendance-summary";
import { AttendanceHistory } from "@/components/parent/attendance-history";
import { GradeSummary, type GradeEntry } from "@/components/parent/grade-summary";
import { AIInteractionsSummary, type AIInteractionEntry } from "@/components/parent/ai-interactions-summary";
import { LiveNowBadge } from "@/components/parent/live-now-badge";
import { notFound } from "next/navigation";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";

export default async function ChildDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const session = await auth();
  const { id } = await params;

  // Verify this parent is linked to this child
  const children = await getLinkedChildren(db, session!.user.id);
  if (!children.some((c) => c.userId === id)) {
    notFound();
  }

  const [child] = await db.select().from(users).where(eq(users.id, id));
  if (!child) notFound();

  // Fetch all data in parallel
  const [
    classesList,
    attendanceSummaries,
    sessionHistoryList,
    activeSession,
  ] = await Promise.all([
    listClassesByUser(db, id),
    getAttendanceSummary(db, id),
    getSessionHistory(db, id, 10),
    getActiveSessionForStudent(db, id),
  ]);

  // Get grades -- submissions with assignment info
  const allSubmissions = await db
    .select({
      grade: submissions.grade,
      feedback: submissions.feedback,
      submittedAt: submissions.submittedAt,
      assignmentTitle: assignments.title,
      classId: assignments.classId,
    })
    .from(submissions)
    .innerJoin(assignments, eq(submissions.assignmentId, assignments.id))
    .where(eq(submissions.studentId, id));

  // Resolve class names for grades
  const classMap = new Map<string, string>();
  for (const cls of classesList) {
    classMap.set(cls.id, cls.title);
  }

  const grades: GradeEntry[] = allSubmissions.map((s) => ({
    assignmentTitle: s.assignmentTitle,
    className: classMap.get(s.classId) || "Unknown",
    grade: s.grade,
    feedback: s.feedback,
    submittedAt: s.submittedAt,
  }));

  // Get AI interaction summary
  const allInteractions = await db
    .select({
      id: aiInteractions.id,
      sessionId: aiInteractions.sessionId,
      messages: aiInteractions.messages,
      createdAt: aiInteractions.createdAt,
    })
    .from(aiInteractions)
    .where(eq(aiInteractions.studentId, id));

  // Resolve session dates and class names for interactions
  const interactionEntries: AIInteractionEntry[] = [];
  for (const interaction of allInteractions.slice(0, 20)) {
    const [sess] = await db
      .select({ startedAt: liveSessions.startedAt, classroomId: liveSessions.classroomId })
      .from(liveSessions)
      .where(eq(liveSessions.id, interaction.sessionId));

    if (sess) {
      // Find class name via classroom
      let className = "Unknown";
      const { newClassrooms } = await import("@/lib/db/schema");
      const [classroom] = await db
        .select({ classId: newClassrooms.classId })
        .from(newClassrooms)
        .where(eq(newClassrooms.id, sess.classroomId));
      if (classroom) {
        const classTitle = classMap.get(classroom.classId);
        if (classTitle) className = classTitle;
      }

      const messages = Array.isArray(interaction.messages) ? interaction.messages : [];
      interactionEntries.push({
        sessionDate: sess.startedAt,
        className,
        messageCount: messages.length,
      });
    }
  }

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-bold">{child.name}</h1>
            {activeSession && <LiveNowBadge size="md" />}
          </div>
          <p className="text-muted-foreground">{child.email}</p>
        </div>
        <div className="flex gap-2">
          {activeSession && (
            <Link
              href={`/parent/children/${id}/live`}
              className={buttonVariants({ variant: "default", size: "sm" })}
            >
              Watch Live Session
            </Link>
          )}
          <Link
            href={`/parent/children/${id}/reports`}
            className={buttonVariants({ variant: "outline", size: "sm" })}
          >
            View Reports
          </Link>
        </div>
      </div>

      {/* Active session banner */}
      {activeSession && (
        <Card className="border-red-200 bg-red-50 dark:bg-red-950/20 dark:border-red-900">
          <CardContent className="py-3">
            <div className="flex items-center justify-between">
              <div>
                <span className="font-medium">{child.name}</span> is currently in a live session:{" "}
                <span className="font-semibold">{activeSession.classTitle}</span>
                {activeSession.topicsCovered.length > 0 && (
                  <span className="text-sm text-muted-foreground ml-2">
                    ({activeSession.topicsCovered.join(", ")})
                  </span>
                )}
              </div>
              <Link
                href={`/parent/children/${id}/live`}
                className={buttonVariants({ variant: "default", size: "sm" })}
              >
                Watch Now
              </Link>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Classes */}
      <section>
        <h2 className="text-lg font-semibold mb-3">Classes ({classesList.length})</h2>
        {classesList.length === 0 ? (
          <p className="text-sm text-muted-foreground">Not enrolled in any classes.</p>
        ) : (
          <div className="space-y-2">
            {classesList.map((cls) => (
              <Card key={cls.id}>
                <CardContent className="py-3">
                  <p className="font-medium">{cls.title}</p>
                  <p className="text-sm text-muted-foreground">{cls.term || "No term"}</p>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </section>

      {/* Attendance */}
      <section>
        <h2 className="text-lg font-semibold mb-3">Attendance</h2>
        <AttendanceSummary summaries={attendanceSummaries} />
      </section>

      {/* Grades */}
      <section>
        <h2 className="text-lg font-semibold mb-3">Assignment Grades</h2>
        <GradeSummary grades={grades} />
      </section>

      {/* AI Interactions */}
      <section>
        <h2 className="text-lg font-semibold mb-3">AI Tutor Usage</h2>
        <AIInteractionsSummary
          interactions={interactionEntries}
          totalCount={allInteractions.length}
        />
      </section>

      {/* Recent Session History */}
      <section>
        <h2 className="text-lg font-semibold mb-3">Recent Sessions</h2>
        <AttendanceHistory sessions={sessionHistoryList} />
      </section>
    </div>
  );
}
```

### Commit
`git commit -m "enhance child detail page with attendance, grades, AI usage, session history, and Live Now banner"`

---

## Task 11: Live Session Viewing Page

Create the server component page that wraps the LiveSessionViewer client component.

### Files

- [ ] `src/app/(portal)/parent/children/[id]/live/page.tsx` -- create

### Code

**`src/app/(portal)/parent/children/[id]/live/page.tsx`**:

```typescript
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { users } from "@/lib/db/schema";
import { eq } from "drizzle-orm";
import { getLinkedChildren } from "@/lib/parent-links";
import { getActiveSessionForStudent } from "@/lib/attendance";
import { getSessionTopics } from "@/lib/session-topics";
import { getClassroom } from "@/lib/classes";
import { LiveSessionViewer } from "@/components/parent/live-session-viewer";
import { notFound, redirect } from "next/navigation";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";

export default async function LiveSessionPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const session = await auth();
  const { id: childId } = await params;

  // Verify parent-child link
  const children = await getLinkedChildren(db, session!.user.id);
  if (!children.some((c) => c.userId === childId)) {
    notFound();
  }

  const [child] = await db.select().from(users).where(eq(users.id, childId));
  if (!child) notFound();

  // Check for active session
  const activeSession = await getActiveSessionForStudent(db, childId);

  if (!activeSession) {
    return (
      <div className="p-6 text-center space-y-4">
        <h2 className="text-xl font-semibold">No Active Session</h2>
        <p className="text-muted-foreground">
          {child.name} is not currently in a live session.
        </p>
        <Link
          href={`/parent/children/${childId}`}
          className={buttonVariants({ variant: "outline" })}
        >
          Back to {child.name}'s Profile
        </Link>
      </div>
    );
  }

  // Get session topics with lesson content
  const sessionTopicData = await getSessionTopics(db, activeSession.sessionId);

  // Get editor language from classroom
  const classroom = await getClassroom(db, activeSession.classId);
  const editorLanguage = classroom?.editorMode || "python";

  return (
    <LiveSessionViewer
      sessionId={activeSession.sessionId}
      childId={childId}
      childName={child.name}
      classTitle={activeSession.classTitle}
      parentId={session!.user.id}
      topics={sessionTopicData.map((t) => ({
        title: t.title,
        lessonContent: t.lessonContent,
      }))}
      editorLanguage={editorLanguage}
    />
  );
}
```

### Commit
`git commit -m "add live session viewing page for parents with read-only Yjs code observation"`

---

## Task 12: AI Progress Reports Page

Create the per-child reports page where parents can view existing reports and generate new ones.

### Files

- [ ] `src/app/(portal)/parent/children/[id]/reports/page.tsx` -- create
- [ ] `src/app/(portal)/parent/reports/page.tsx` -- modify to redirect or list all children

### Code

**`src/app/(portal)/parent/children/[id]/reports/page.tsx`**:

```typescript
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { users } from "@/lib/db/schema";
import { eq } from "drizzle-orm";
import { getLinkedChildren } from "@/lib/parent-links";
import { listReports } from "@/lib/parent-reports";
import { ReportCard } from "@/components/parent/report-card";
import { notFound } from "next/navigation";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { GenerateReportButton } from "./generate-report-button";

export default async function ChildReportsPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const session = await auth();
  const { id: childId } = await params;

  // Verify parent-child link
  const children = await getLinkedChildren(db, session!.user.id);
  if (!children.some((c) => c.userId === childId)) {
    notFound();
  }

  const [child] = await db.select().from(users).where(eq(users.id, childId));
  if (!child) notFound();

  const reports = await listReports(db, session!.user.id, childId);

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Progress Reports</h1>
          <p className="text-muted-foreground">{child.name}</p>
        </div>
        <div className="flex gap-2">
          <GenerateReportButton childId={childId} />
          <Link
            href={`/parent/children/${childId}`}
            className={buttonVariants({ variant: "outline", size: "sm" })}
          >
            Back to Profile
          </Link>
        </div>
      </div>

      {reports.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            <p>No reports generated yet.</p>
            <p className="text-sm mt-2">
              Click "Generate Report" to create a weekly progress summary.
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-4">
          {reports.map((report) => (
            <ReportCard
              key={report.id}
              weekStart={report.weekStart}
              weekEnd={report.weekEnd}
              summary={report.summary}
              details={report.details as any}
              generatedAt={report.generatedAt}
            />
          ))}
        </div>
      )}
    </div>
  );
}
```

**`src/app/(portal)/parent/children/[id]/reports/generate-report-button.tsx`** (client component for the generate action):

```typescript
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";

interface GenerateReportButtonProps {
  childId: string;
}

export function GenerateReportButton({ childId }: GenerateReportButtonProps) {
  const [loading, setLoading] = useState(false);
  const router = useRouter();

  async function handleGenerate() {
    setLoading(true);
    try {
      const res = await fetch(`/api/parent/children/${childId}/reports`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({}),
      });

      if (res.ok) {
        router.refresh();
      } else {
        const data = await res.json();
        alert(data.error || "Failed to generate report");
      }
    } catch {
      alert("Failed to generate report");
    } finally {
      setLoading(false);
    }
  }

  return (
    <Button size="sm" onClick={handleGenerate} disabled={loading}>
      {loading ? "Generating..." : "Generate Report"}
    </Button>
  );
}
```

**`src/app/(portal)/parent/reports/page.tsx`** -- update to redirect to dashboard with child links:

```typescript
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getLinkedChildren } from "@/lib/parent-links";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";

export default async function ParentReportsPage() {
  const session = await auth();
  const children = await getLinkedChildren(db, session!.user.id);

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">Reports</h1>
      <p className="text-muted-foreground">
        View AI-generated weekly progress reports for each of your children.
      </p>

      {children.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            <p>No children linked yet.</p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {children.map((child) => (
            <Card key={child.userId}>
              <CardHeader>
                <CardTitle className="text-lg">{child.name}</CardTitle>
              </CardHeader>
              <CardContent>
                <Link
                  href={`/parent/children/${child.userId}/reports`}
                  className={buttonVariants({ variant: "outline", size: "sm" })}
                >
                  View Reports
                </Link>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
```

### Commit
`git commit -m "add AI progress reports pages with report generation, caching, and per-child views"`

---

## Task 13: Database Migration

Generate and run the Drizzle migration for the new `parentReports` table.

### Steps

- [ ] Run `bun run db:generate` to generate the migration
- [ ] Run `bun run db:migrate` to apply it
- [ ] Verify the table exists

### Commit
`git commit -m "add database migration for parent_reports table"`

---

## Task 14: Run All Tests and Fix Issues

Run the full test suite to catch any issues. Fix any import errors, type errors, or test failures.

### Steps

- [ ] Run `bun run test` and verify all new and existing tests pass
- [ ] Run `bun run lint` and fix any lint issues
- [ ] Run `bun run build` to verify the build succeeds

### Commit
`git commit -m "fix test and build issues for parent portal implementation"`

---

## Summary

**Total new files:** ~18

| Category | Files |
|---|---|
| Schema | 1 modified (schema.ts + migration) |
| Libraries | 3 new (attendance.ts, parent-reports.ts, report-prompts.ts), 1 modified (parent-links.ts) |
| Components | 7 new (live-session-viewer, live-now-badge, attendance-summary, attendance-history, grade-summary, ai-interactions-summary, report-card), 1 new (generate-report-button) |
| Pages | 2 modified (parent dashboard, child detail), 3 new (live session, child reports, generate button) |
| API Routes | 3 new (live-session, reports list+generate, report detail) |
| Server | 1 modified (hocuspocus.ts) |
| Tests | 5 new |
| Test Helpers | 1 modified |

**Key design decisions:**

1. **Parent Yjs access:** Parents connect to Hocuspocus with `userId:parent` token. The server allows this role alongside `teacher` and `user`. Read-only is enforced client-side via Monaco `readOnly={true}`, matching the existing teacher observation pattern.

2. **Report caching:** Reports are stored in `parent_reports` after generation. If a report for the same week already exists, the cached version is returned. This prevents redundant LLM calls and gives consistent results.

3. **Attendance derivation:** Rather than a separate attendance tracking table, attendance is derived from existing `session_participants` and `live_sessions` tables. This avoids data duplication and keeps the source of truth clear.

4. **Authorization model:** All parent access flows through `getLinkedChildren()` or `isChildLinkedToParent()`. These check class membership where the parent has `role="parent"` in the same class as the student. This is the existing pattern from plan 010.

5. **Live detection:** Active session detection uses `session_participants` with `leftAt IS NULL` joined to `live_sessions` with `status='active'`. This works with the existing session lifecycle.

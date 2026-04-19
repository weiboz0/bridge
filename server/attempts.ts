import { eq, sql } from "drizzle-orm";
import {
  pgTable,
  uuid,
  text,
  timestamp,
  varchar,
} from "drizzle-orm/pg-core";
import { serverDb } from "./db";

// Minimal table definition for server/ context (can't use @/ aliases).
// Keep in sync with drizzle/0008_problems.sql + 0009_attempts_yjs.sql.
const attempts = pgTable("attempts", {
  id: uuid("id").primaryKey(),
  problemId: uuid("problem_id").notNull(),
  userId: uuid("user_id").notNull(),
  yjsState: text("yjs_state"),
  plainText: text("plain_text").notNull().default(""),
  language: varchar("language", { length: 32 }).notNull(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).defaultNow().notNull(),
});

const problems = pgTable("problems", {
  id: uuid("id").primaryKey(),
  topicId: uuid("topic_id").notNull(),
});

const topics = pgTable("topics", {
  id: uuid("id").primaryKey(),
  courseId: uuid("course_id").notNull(),
});

export interface AttemptOwner {
  userId: string;
  problemId: string;
}

export async function loadAttemptOwner(attemptId: string): Promise<AttemptOwner | null> {
  const [row] = await serverDb
    .select({ userId: attempts.userId, problemId: attempts.problemId })
    .from(attempts)
    .where(eq(attempts.id, attemptId));
  return row ?? null;
}

export async function loadAttemptYjsState(attemptId: string): Promise<string | null> {
  const [row] = await serverDb
    .select({ yjsState: attempts.yjsState })
    .from(attempts)
    .where(eq(attempts.id, attemptId));
  return row?.yjsState ?? null;
}

export async function storeAttemptYjsState(
  attemptId: string,
  yjsState: string,
  plainText: string,
): Promise<void> {
  await serverDb.execute(sql`
    UPDATE attempts
    SET yjs_state = ${yjsState}, plain_text = ${plainText}, updated_at = now()
    WHERE id = ${attemptId}
  `);
}

/**
 * Mirror of `ClassStore.TeacherCanViewStudentInCourse` from the Go side.
 * Single-source-of-truth would be a Go API call, but doing it here avoids
 * a second-hop auth dance for every Hocuspocus connect. Acceptable cost:
 * if the Go policy changes, both sites must update.
 */
export async function teacherCanViewAttempt(
  teacherId: string,
  attemptId: string,
): Promise<boolean> {
  const result = await serverDb.execute<{ allowed: boolean }>(sql`
    WITH attempt_course AS (
      SELECT t.course_id
      FROM attempts a
      INNER JOIN problems p ON p.id = a.problem_id
      INNER JOIN topics t ON t.id = p.topic_id
      WHERE a.id = ${attemptId}
    )
    SELECT EXISTS (
      SELECT 1
      FROM class_memberships cm_t
      INNER JOIN class_memberships cm_s ON cm_s.class_id = cm_t.class_id
      INNER JOIN classes c ON c.id = cm_t.class_id
      INNER JOIN attempts a ON a.id = ${attemptId}
      INNER JOIN attempt_course ac ON true
      WHERE cm_t.user_id = ${teacherId}
        AND cm_t.role = 'instructor'
        AND cm_s.user_id = a.user_id
        AND c.course_id = ac.course_id
    ) AS allowed
  `);

  // postgres.js returns rows as an array on .rows or directly depending on driver
  // shape. drizzle's `execute` here returns the underlying client result.
  const rows = (result as unknown as { allowed: boolean }[]) ?? [];
  return rows[0]?.allowed === true;
}

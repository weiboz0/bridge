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

// Plan 053b — `problems.topic_id` was dropped in migration 0013;
// the broken column reference is gone. Topics→problems linking lives
// in `topic_problems` now (m:n).

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
 * Plan 053b Phase 2 — fixed query.
 *
 * Mirror of the Go-side `AttemptStore.IsTeacherOfAttempt`. Both sides
 * MUST stay in sync — single-source-of-truth would be a Go API
 * call, but doing it here avoids a second-hop auth dance for every
 * Hocuspocus connect. Pre-053b the query referenced `problems.topic_id`
 * which was dropped in migration 0013; the helper has been silently
 * failing for months (teacher-watch broken end-to-end). The new
 * query joins via `topic_problems` and verifies BOTH the teacher
 * AND the attempt owner share a class — same privacy boundary as
 * the Go version (Codex pass-1 catch on plan 053b).
 *
 * Note: this TS helper goes away when plan 053 phase 4 retires
 * the legacy Hocuspocus auth path; the Go side becomes the only
 * source of truth.
 */
export async function teacherCanViewAttempt(
  teacherId: string,
  attemptId: string,
): Promise<boolean> {
  const result = await serverDb.execute<{ allowed: boolean }>(sql`
    SELECT EXISTS (
      SELECT 1
      FROM attempts a
      INNER JOIN topic_problems tp ON tp.problem_id = a.problem_id
      INNER JOIN topics t          ON t.id = tp.topic_id
      INNER JOIN courses co        ON co.id = t.course_id
      INNER JOIN classes c         ON c.course_id = co.id
      INNER JOIN class_memberships cm_t
        ON cm_t.class_id = c.id
        AND cm_t.user_id = ${teacherId}
        AND cm_t.role IN ('instructor', 'ta')
      INNER JOIN class_memberships cm_s
        ON cm_s.class_id = c.id
        AND cm_s.user_id = a.user_id
      WHERE a.id = ${attemptId}
    ) AS allowed
  `);

  // postgres.js returns rows as an array on .rows or directly depending
  // on driver shape. drizzle's `execute` here returns the underlying
  // client result.
  const rows = (result as unknown as { allowed: boolean }[]) ?? [];
  return rows[0]?.allowed === true;
}

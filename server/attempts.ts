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


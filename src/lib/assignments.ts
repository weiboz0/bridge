import { eq } from "drizzle-orm";
import { assignments } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

interface CreateAssignmentInput {
  classId: string;
  topicId?: string;
  title: string;
  description?: string;
  starterCode?: string;
  dueDate?: Date;
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

export async function listAssignmentsByTopic(db: Database, topicId: string) {
  return db
    .select()
    .from(assignments)
    .where(eq(assignments.topicId, topicId));
}

export async function updateAssignment(
  db: Database,
  assignmentId: string,
  updates: Partial<Pick<typeof assignments.$inferInsert, "title" | "description" | "starterCode" | "dueDate" | "rubric">>
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

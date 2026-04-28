import { eq, and, asc } from "drizzle-orm";
import { topics } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

interface CreateTopicInput {
  courseId: string;
  title: string;
  description?: string;
}

export async function createTopic(db: Database, input: CreateTopicInput) {
  // Auto-assign sortOrder as max + 1
  const existing = await db
    .select()
    .from(topics)
    .where(eq(topics.courseId, input.courseId));

  const maxOrder = existing.reduce((max, t) => Math.max(max, t.sortOrder), -1);

  const [topic] = await db
    .insert(topics)
    .values({
      ...input,
      sortOrder: maxOrder + 1,
    })
    .returning();
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

export async function updateTopic(
  db: Database,
  topicId: string,
  updates: Partial<Pick<typeof topics.$inferInsert, "title" | "description">>
) {
  const [topic] = await db
    .update(topics)
    .set({ ...updates, updatedAt: new Date() })
    .where(eq(topics.id, topicId))
    .returning();
  return topic || null;
}

export async function deleteTopic(db: Database, topicId: string) {
  const [deleted] = await db
    .delete(topics)
    .where(eq(topics.id, topicId))
    .returning();
  return deleted || null;
}

export async function reorderTopics(db: Database, courseId: string, topicIds: string[]) {
  for (let i = 0; i < topicIds.length; i++) {
    await db
      .update(topics)
      .set({ sortOrder: i })
      .where(
        and(
          eq(topics.id, topicIds[i]),
          eq(topics.courseId, courseId)
        )
      );
  }
}

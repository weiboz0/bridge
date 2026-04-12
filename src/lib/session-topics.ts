import { eq, and } from "drizzle-orm";
import { sessionTopics, topics } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

export async function linkSessionTopic(db: Database, sessionId: string, topicId: string) {
  const [link] = await db
    .insert(sessionTopics)
    .values({ sessionId, topicId })
    .onConflictDoNothing()
    .returning();
  return link || null;
}

export async function unlinkSessionTopic(db: Database, sessionId: string, topicId: string) {
  const [deleted] = await db
    .delete(sessionTopics)
    .where(
      and(
        eq(sessionTopics.sessionId, sessionId),
        eq(sessionTopics.topicId, topicId)
      )
    )
    .returning();
  return deleted || null;
}

export async function getSessionTopics(db: Database, sessionId: string) {
  const links = await db
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
    .where(eq(sessionTopics.sessionId, sessionId));

  return links.sort((a, b) => a.sortOrder - b.sortOrder);
}

import { eq, and, or, inArray } from "drizzle-orm";
import {
  sessionTopics,
  topics,
  teachingUnits,
  courses,
} from "@/lib/db/schema";
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
  // Cross-org leak guard (added with the linked-Unit join in plan 044
  // phase 1): the Unit's scope must be platform OR its scope_id must
  // match the topic's course org_id. A misaligned topic_id won't
  // surface a Unit from another org.
  const links = await db
    .select({
      topicId: sessionTopics.topicId,
      title: topics.title,
      description: topics.description,
      sortOrder: topics.sortOrder,
      unitId: teachingUnits.id,
      unitTitle: teachingUnits.title,
      unitMaterialType: teachingUnits.materialType,
    })
    .from(sessionTopics)
    .innerJoin(topics, eq(sessionTopics.topicId, topics.id))
    .innerJoin(courses, eq(courses.id, topics.courseId))
    .leftJoin(
      teachingUnits,
      and(
        eq(teachingUnits.topicId, topics.id),
        or(
          eq(teachingUnits.scope, "platform"),
          eq(teachingUnits.scopeId, courses.orgId)
        )
      )
    )
    .where(eq(sessionTopics.sessionId, sessionId));

  return links.sort((a, b) => a.sortOrder - b.sortOrder);
}

// Plan 044 phase 1: thin lookup used by the teacher topic-edit page to
// render the linked Unit. Same cross-org guard as getSessionTopics.
// Returns null when no Unit is linked to the topic.
export async function getTopicLinkedUnit(db: Database, topicId: string) {
  const [unit] = await db
    .select({
      unitId: teachingUnits.id,
      unitTitle: teachingUnits.title,
      unitMaterialType: teachingUnits.materialType,
      unitStatus: teachingUnits.status,
    })
    .from(topics)
    .innerJoin(courses, eq(courses.id, topics.courseId))
    .innerJoin(
      teachingUnits,
      and(
        eq(teachingUnits.topicId, topics.id),
        or(
          eq(teachingUnits.scope, "platform"),
          eq(teachingUnits.scopeId, courses.orgId)
        )
      )
    )
    .where(eq(topics.id, topicId));

  return unit ?? null;
}

// Bulk version for course-detail and class-detail pages: returns a map
// of topicId → linked Unit. Single query, no N+1.
export async function listLinkedUnitsByTopicIds(
  db: Database,
  topicIds: string[]
): Promise<Record<string, { unitId: string; unitTitle: string; unitMaterialType: string }>> {
  if (topicIds.length === 0) return {};
  const rows = await db
    .select({
      topicId: topics.id,
      unitId: teachingUnits.id,
      unitTitle: teachingUnits.title,
      unitMaterialType: teachingUnits.materialType,
    })
    .from(topics)
    .innerJoin(courses, eq(courses.id, topics.courseId))
    .innerJoin(
      teachingUnits,
      and(
        eq(teachingUnits.topicId, topics.id),
        or(
          eq(teachingUnits.scope, "platform"),
          eq(teachingUnits.scopeId, courses.orgId)
        )
      )
    )
    .where(inArray(topics.id, topicIds));
  const out: Record<string, { unitId: string; unitTitle: string; unitMaterialType: string }> = {};
  for (const r of rows) {
    out[r.topicId] = {
      unitId: r.unitId,
      unitTitle: r.unitTitle,
      unitMaterialType: r.unitMaterialType,
    };
  }
  return out;
}

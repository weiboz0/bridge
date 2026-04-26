import { eq, and, desc, isNull } from "drizzle-orm";
import { sessions, sessionParticipants, sessionTopics, topics } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

export async function getAttendanceSummary(
  db: Database,
  studentId: string,
  classId: string
) {
  // Get all sessions for this class
  const allSessions = await db
    .select()
    .from(sessions)
    .where(eq(sessions.classId, classId));

  // Get sessions this student participated in
  const attended = await db
    .select({ sessionId: sessionParticipants.sessionId })
    .from(sessionParticipants)
    .where(eq(sessionParticipants.userId, studentId));

  const attendedIds = new Set(attended.map((a) => a.sessionId));
  const classSessionIds = new Set(allSessions.map((s) => s.id));

  return {
    total: allSessions.length,
    attended: attended.filter((a) => classSessionIds.has(a.sessionId)).length,
    rate: allSessions.length > 0
      ? Math.round((attended.filter((a) => classSessionIds.has(a.sessionId)).length / allSessions.length) * 100)
      : 0,
  };
}

export async function getSessionHistory(
  db: Database,
  studentId: string,
  limit = 10
) {
  const participations = await db
    .select({
      sessionId: sessionParticipants.sessionId,
      joinedAt: sessionParticipants.joinedAt,
      leftAt: sessionParticipants.leftAt,
      status: sessions.status,
      startedAt: sessions.startedAt,
      endedAt: sessions.endedAt,
    })
    .from(sessionParticipants)
    .innerJoin(sessions, eq(sessionParticipants.sessionId, sessions.id))
    .where(eq(sessionParticipants.userId, studentId))
    .orderBy(desc(sessions.startedAt))
    .limit(limit);

  // Get topics for each session
  const results = [];
  for (const p of participations) {
    const sessionTopicList = await db
      .select({ title: topics.title })
      .from(sessionTopics)
      .innerJoin(topics, eq(sessionTopics.topicId, topics.id))
      .where(eq(sessionTopics.sessionId, p.sessionId));

    results.push({
      ...p,
      topics: sessionTopicList.map((t) => t.title),
    });
  }

  return results;
}

export async function getActiveSessionForStudent(db: Database, studentId: string) {
  const active = await db
    .select({
      sessionId: sessionParticipants.sessionId,
      classId: sessions.classId,
      startedAt: sessions.startedAt,
    })
    .from(sessionParticipants)
    .innerJoin(sessions, eq(sessionParticipants.sessionId, sessions.id))
    .where(
      and(
        eq(sessionParticipants.userId, studentId),
        eq(sessions.status, "live"),
        isNull(sessionParticipants.leftAt)
      )
    )
    .limit(1);

  return active[0] || null;
}

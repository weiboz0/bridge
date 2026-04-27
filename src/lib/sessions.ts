import { eq, and, desc } from "drizzle-orm";
import { sessions, sessionParticipants, users } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

interface CreateSessionInput {
  classId: string;
  teacherId: string;
  settings?: Record<string, unknown>;
}

export async function createSession(db: Database, input: CreateSessionInput) {
  // End any live session for this class first
  const [existing] = await db
    .select()
    .from(sessions)
    .where(
      and(
        eq(sessions.classId, input.classId),
        eq(sessions.status, "live")
      )
    );

  if (existing) {
    await db
      .update(sessions)
      .set({ status: "ended", endedAt: new Date() })
      .where(eq(sessions.id, existing.id));
  }

  const [session] = await db
    .insert(sessions)
    .values({
      classId: input.classId,
      teacherId: input.teacherId,
      title: "Untitled session",
      settings: input.settings ?? {},
    })
    .returning();
  return session;
}

export async function getSession(db: Database, sessionId: string) {
  const [session] = await db
    .select()
    .from(sessions)
    .where(eq(sessions.id, sessionId));
  return session || null;
}

export async function getActiveSession(db: Database, classId: string) {
  const [session] = await db
    .select()
    .from(sessions)
    .where(
      and(
        eq(sessions.classId, classId),
        eq(sessions.status, "live")
      )
    );
  return session || null;
}

export async function endSession(db: Database, sessionId: string) {
  const [session] = await db
    .update(sessions)
    .set({ status: "ended", endedAt: new Date() })
    .where(eq(sessions.id, sessionId))
    .returning();
  return session || null;
}

export async function joinSession(
  db: Database,
  sessionId: string,
  studentId: string
) {
  const [participant] = await db
    .insert(sessionParticipants)
    .values({ sessionId, userId: studentId })
    .onConflictDoNothing()
    .returning();
  return participant;
}

export async function leaveSession(
  db: Database,
  sessionId: string,
  studentId: string
) {
  const [participant] = await db
    .update(sessionParticipants)
    .set({ leftAt: new Date() })
    .where(
      and(
        eq(sessionParticipants.sessionId, sessionId),
        eq(sessionParticipants.userId, studentId)
      )
    )
    .returning();
  return participant || null;
}

export async function getSessionParticipants(db: Database, sessionId: string) {
  return db
    .select({
      studentId: sessionParticipants.userId,
      status: sessionParticipants.status,
      joinedAt: sessionParticipants.joinedAt,
      leftAt: sessionParticipants.leftAt,
      helpRequestedAt: sessionParticipants.helpRequestedAt,
      name: users.name,
      email: users.email,
    })
    .from(sessionParticipants)
    .innerJoin(users, eq(sessionParticipants.userId, users.id))
    .where(eq(sessionParticipants.sessionId, sessionId));
}

export async function updateParticipantStatus(
  db: Database,
  sessionId: string,
  studentId: string,
  status: "active" | "needs_help" | "present" | "left" | "invited"
) {
  const [participant] =
    status === "needs_help"
      ? await db
          .update(sessionParticipants)
          .set({ helpRequestedAt: new Date() })
          .where(
            and(
              eq(sessionParticipants.sessionId, sessionId),
              eq(sessionParticipants.userId, studentId)
            )
          )
          .returning()
      : status === "active"
        ? await db
            .update(sessionParticipants)
            .set({ helpRequestedAt: null })
            .where(
              and(
                eq(sessionParticipants.sessionId, sessionId),
                eq(sessionParticipants.userId, studentId)
              )
            )
            .returning()
        : await db
            .update(sessionParticipants)
            .set({ status })
            .where(
              and(
                eq(sessionParticipants.sessionId, sessionId),
                eq(sessionParticipants.userId, studentId)
              )
            )
            .returning();
  return participant || null;
}

import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, createTestClassroom, createTestSession } from "../helpers";
import { sessionParticipants } from "@/lib/db/schema";
import { getAttendanceSummary, getSessionHistory, getActiveSessionForStudent } from "@/lib/attendance";

describe("attendance operations", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;
  let classroom: Awaited<ReturnType<typeof createTestClassroom>>;

  beforeEach(async () => {
    teacher = await createTestUser({ email: "teacher@test.edu" });
    student = await createTestUser({ email: "student@test.edu" });
    classroom = await createTestClassroom(teacher.id);
  });

  it("calculates attendance summary", async () => {
    const s1 = await createTestSession(classroom.id, teacher.id, { status: "ended" });
    const s2 = await createTestSession(classroom.id, teacher.id, { status: "ended" });

    // Student attended only s1
    await testDb.insert(sessionParticipants).values({
      sessionId: s1.id,
      studentId: student.id,
    });

    const summary = await getAttendanceSummary(testDb, student.id, classroom.id);
    expect(summary.total).toBe(2);
    expect(summary.attended).toBe(1);
    expect(summary.rate).toBe(50);
  });

  it("returns 0 rate for no sessions", async () => {
    const summary = await getAttendanceSummary(testDb, student.id, classroom.id);
    expect(summary.total).toBe(0);
    expect(summary.rate).toBe(0);
  });

  it("gets session history", async () => {
    const s1 = await createTestSession(classroom.id, teacher.id);
    await testDb.insert(sessionParticipants).values({
      sessionId: s1.id,
      studentId: student.id,
    });

    const history = await getSessionHistory(testDb, student.id);
    expect(history).toHaveLength(1);
    expect(history[0].sessionId).toBe(s1.id);
  });

  it("detects active session for student", async () => {
    const session = await createTestSession(classroom.id, teacher.id, { status: "active" });
    await testDb.insert(sessionParticipants).values({
      sessionId: session.id,
      studentId: student.id,
    });

    const active = await getActiveSessionForStudent(testDb, student.id);
    expect(active).not.toBeNull();
    expect(active!.sessionId).toBe(session.id);
  });

  it("returns null when no active session", async () => {
    const active = await getActiveSessionForStudent(testDb, student.id);
    expect(active).toBeNull();
  });
});

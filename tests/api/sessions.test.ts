import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, createTestClassroom, createTestSession } from "../helpers";
import {
  createSession,
  getSession,
  getActiveSession,
  endSession,
  joinSession,
  leaveSession,
  getSessionParticipants,
  updateParticipantStatus,
} from "@/lib/sessions";

describe("session operations", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;
  let classroom: Awaited<ReturnType<typeof createTestClassroom>>;

  beforeEach(async () => {
    teacher = await createTestUser({ email: "teacher@test.edu" });
    student = await createTestUser({ email: "student@test.edu" });
    classroom = await createTestClassroom(teacher.id);
  });

  describe("createSession", () => {
    it("creates a new active session", async () => {
      const session = await createSession(testDb, {
        classroomId: classroom.id,
        teacherId: teacher.id,
      });
      expect(session.id).toBeDefined();
      expect(session.status).toBe("active");
      expect(session.classroomId).toBe(classroom.id);
    });

    it("ends existing active session when creating a new one", async () => {
      const first = await createSession(testDb, {
        classroomId: classroom.id,
        teacherId: teacher.id,
      });
      const second = await createSession(testDb, {
        classroomId: classroom.id,
        teacherId: teacher.id,
      });

      const firstAfter = await getSession(testDb, first.id);
      expect(firstAfter!.status).toBe("ended");
      expect(second.status).toBe("active");
    });
  });

  describe("getActiveSession", () => {
    it("returns active session for classroom", async () => {
      const session = await createSession(testDb, {
        classroomId: classroom.id,
        teacherId: teacher.id,
      });
      const found = await getActiveSession(testDb, classroom.id);
      expect(found).not.toBeNull();
      expect(found!.id).toBe(session.id);
    });

    it("returns null when no active session", async () => {
      const found = await getActiveSession(testDb, classroom.id);
      expect(found).toBeNull();
    });
  });

  describe("endSession", () => {
    it("marks session as ended", async () => {
      const session = await createSession(testDb, {
        classroomId: classroom.id,
        teacherId: teacher.id,
      });
      const ended = await endSession(testDb, session.id);
      expect(ended!.status).toBe("ended");
      expect(ended!.endedAt).not.toBeNull();
    });
  });

  describe("participant management", () => {
    it("student joins a session", async () => {
      const session = await createSession(testDb, {
        classroomId: classroom.id,
        teacherId: teacher.id,
      });
      const participant = await joinSession(testDb, session.id, student.id);
      expect(participant).toBeDefined();
    });

    it("does not duplicate on re-join", async () => {
      const session = await createSession(testDb, {
        classroomId: classroom.id,
        teacherId: teacher.id,
      });
      await joinSession(testDb, session.id, student.id);
      await joinSession(testDb, session.id, student.id);

      const participants = await getSessionParticipants(testDb, session.id);
      expect(participants).toHaveLength(1);
    });

    it("student leaves a session", async () => {
      const session = await createSession(testDb, {
        classroomId: classroom.id,
        teacherId: teacher.id,
      });
      await joinSession(testDb, session.id, student.id);
      const left = await leaveSession(testDb, session.id, student.id);
      expect(left!.leftAt).not.toBeNull();
    });

    it("lists participants with user info", async () => {
      const session = await createSession(testDb, {
        classroomId: classroom.id,
        teacherId: teacher.id,
      });
      await joinSession(testDb, session.id, student.id);

      const participants = await getSessionParticipants(testDb, session.id);
      expect(participants).toHaveLength(1);
      expect(participants[0].name).toBe(student.name);
      expect(participants[0].status).toBe("active");
    });

    it("updates participant status", async () => {
      const session = await createSession(testDb, {
        classroomId: classroom.id,
        teacherId: teacher.id,
      });
      await joinSession(testDb, session.id, student.id);
      const updated = await updateParticipantStatus(
        testDb, session.id, student.id, "needs_help"
      );
      expect(updated!.status).toBe("needs_help");
    });
  });
});

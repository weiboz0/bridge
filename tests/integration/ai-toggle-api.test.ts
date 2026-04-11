import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, createTestClassroom, createTestSession } from "../helpers";
import { setMockUser, createRequest, parseResponse } from "../api-helpers";
import { POST as TOGGLE } from "@/app/api/ai/toggle/route";
import { GET as LIST_INTERACTIONS } from "@/app/api/ai/interactions/route";
import { aiInteractions } from "@/lib/db/schema";
import { eq } from "drizzle-orm";

describe("AI Toggle API", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;
  let classroom: Awaited<ReturnType<typeof createTestClassroom>>;

  beforeEach(async () => {
    teacher = await createTestUser({ name: "Teacher", role: "teacher", email: "teacher@test.edu" });
    student = await createTestUser({ name: "Student", role: "student", email: "student@test.edu" });
    classroom = await createTestClassroom(teacher.id);
  });

  describe("POST /api/ai/toggle", () => {
    it("teacher enables AI for a student", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });
      const session = await createTestSession(classroom.id, teacher.id);

      const req = createRequest("/api/ai/toggle", {
        method: "POST",
        body: {
          sessionId: session.id,
          studentId: student.id,
          enabled: true,
        },
      });

      const { status, body } = await parseResponse(await TOGGLE(req));
      expect(status).toBe(200);
      expect(body).toHaveProperty("studentId", student.id);
      expect(body).toHaveProperty("enabled", true);

      // Verify interaction was created in DB
      const interactions = await testDb
        .select()
        .from(aiInteractions)
        .where(eq(aiInteractions.studentId, student.id));
      expect(interactions).toHaveLength(1);
    });

    it("teacher disables AI for a student", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });
      const session = await createTestSession(classroom.id, teacher.id);

      const req = createRequest("/api/ai/toggle", {
        method: "POST",
        body: {
          sessionId: session.id,
          studentId: student.id,
          enabled: false,
        },
      });

      const { status, body } = await parseResponse(await TOGGLE(req));
      expect(status).toBe(200);
      expect(body).toHaveProperty("enabled", false);
    });

    it("rejects toggle by student", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email, role: "student" });
      const session = await createTestSession(classroom.id, teacher.id);

      const req = createRequest("/api/ai/toggle", {
        method: "POST",
        body: {
          sessionId: session.id,
          studentId: student.id,
          enabled: true,
        },
      });

      const { status } = await parseResponse(await TOGGLE(req));
      expect(status).toBe(403);
    });

    it("rejects toggle by non-owner teacher", async () => {
      const otherTeacher = await createTestUser({ name: "Other", role: "teacher", email: "other@test.edu" });
      setMockUser({ id: otherTeacher.id, name: otherTeacher.name, email: otherTeacher.email, role: "teacher" });
      const session = await createTestSession(classroom.id, teacher.id);

      const req = createRequest("/api/ai/toggle", {
        method: "POST",
        body: {
          sessionId: session.id,
          studentId: student.id,
          enabled: true,
        },
      });

      const { status } = await parseResponse(await TOGGLE(req));
      expect(status).toBe(403);
    });
  });

  describe("GET /api/ai/interactions", () => {
    it("teacher lists interactions for a session", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });
      const session = await createTestSession(classroom.id, teacher.id);

      // Create an interaction directly
      await testDb.insert(aiInteractions).values({
        studentId: student.id,
        sessionId: session.id,
        enabledByTeacherId: teacher.id,
        messages: [{ role: "user", content: "help me", timestamp: new Date().toISOString() }],
      });

      const req = createRequest("/api/ai/interactions", {
        searchParams: { sessionId: session.id },
      });

      const { status, body } = await parseResponse<any[]>(await LIST_INTERACTIONS(req));
      expect(status).toBe(200);
      expect(body).toHaveLength(1);
      expect(body[0]).toHaveProperty("studentId", student.id);
    });

    it("rejects listing by student", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email, role: "student" });

      const req = createRequest("/api/ai/interactions", {
        searchParams: { sessionId: "any-id" },
      });

      const { status } = await parseResponse(await LIST_INTERACTIONS(req));
      expect(status).toBe(403);
    });

    it("requires sessionId param", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });

      const req = createRequest("/api/ai/interactions");
      const { status } = await parseResponse(await LIST_INTERACTIONS(req));
      expect(status).toBe(400);
    });
  });
});

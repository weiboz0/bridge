import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, createTestClassroom, createTestSession } from "../helpers";
import { setMockUser, createRequest, parseResponse } from "../api-helpers";
import { POST as CREATE_SESSION } from "@/app/api/sessions/route";
import { GET as GET_SESSION, PATCH as END_SESSION } from "@/app/api/sessions/[id]/route";
import { POST as JOIN_SESSION } from "@/app/api/sessions/[id]/join/route";
import { POST as LEAVE_SESSION } from "@/app/api/sessions/[id]/leave/route";
import { GET as GET_PARTICIPANTS } from "@/app/api/sessions/[id]/participants/route";
import { GET as GET_ACTIVE } from "@/app/api/classrooms/[id]/active-session/route";
import { GET as GET_HELP_QUEUE, POST as RAISE_HAND } from "@/app/api/sessions/[id]/help-queue/route";
import { sessionParticipants } from "@/lib/db/schema";

describe("Session API", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;
  let classroom: Awaited<ReturnType<typeof createTestClassroom>>;

  beforeEach(async () => {
    teacher = await createTestUser({ name: "Teacher", email: "teacher@test.edu" });
    student = await createTestUser({ name: "Student", email: "student@test.edu" });
    classroom = await createTestClassroom(teacher.id);
  });

  describe("POST /api/sessions", () => {
    it("creates a session as the classroom teacher", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });

      const req = createRequest("/api/sessions", {
        method: "POST",
        body: { classroomId: classroom.id },
      });

      const { status, body } = await parseResponse(await CREATE_SESSION(req));
      expect(status).toBe(201);
      expect(body).toHaveProperty("status", "active");
      expect(body).toHaveProperty("classroomId", classroom.id);
    });

    // TODO: role-based rejection tests will be re-added with OrgMembership-based auth

    it("rejects invalid classroomId", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });

      const req = createRequest("/api/sessions", {
        method: "POST",
        body: { classroomId: "not-a-uuid" },
      });

      const { status } = await parseResponse(await CREATE_SESSION(req));
      expect(status).toBe(400);
    });
  });

  describe("GET /api/sessions/[id]", () => {
    it("returns session by ID", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const session = await createTestSession(classroom.id, teacher.id);

      const res = await GET_SESSION(
        createRequest(`/api/sessions/${session.id}`),
        { params: Promise.resolve({ id: session.id }) }
      );

      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("id", session.id);
    });

    it("returns 404 for non-existent session", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });

      const res = await GET_SESSION(
        createRequest("/api/sessions/00000000-0000-0000-0000-000000000000"),
        { params: Promise.resolve({ id: "00000000-0000-0000-0000-000000000000" }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(404);
    });
  });

  describe("PATCH /api/sessions/[id] (end session)", () => {
    it("ends session as the teacher", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const session = await createTestSession(classroom.id, teacher.id);

      const res = await END_SESSION(
        createRequest(`/api/sessions/${session.id}`, { method: "PATCH" }),
        { params: Promise.resolve({ id: session.id }) }
      );

      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("status", "ended");
    });

    it("rejects end by non-teacher", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email });
      const session = await createTestSession(classroom.id, teacher.id);

      const res = await END_SESSION(
        createRequest(`/api/sessions/${session.id}`, { method: "PATCH" }),
        { params: Promise.resolve({ id: session.id }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(403);
    });
  });

  describe("POST /api/sessions/[id]/join", () => {
    it("student joins a session", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email });
      const session = await createTestSession(classroom.id, teacher.id);

      const res = await JOIN_SESSION(
        createRequest(`/api/sessions/${session.id}/join`, { method: "POST" }),
        { params: Promise.resolve({ id: session.id }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(200);
    });

    it("rejects join for ended session", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email });
      const session = await createTestSession(classroom.id, teacher.id, { status: "ended" });

      const res = await JOIN_SESSION(
        createRequest(`/api/sessions/${session.id}/join`, { method: "POST" }),
        { params: Promise.resolve({ id: session.id }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(400);
    });
  });

  describe("POST /api/sessions/[id]/leave", () => {
    it("student leaves a session", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email });
      const session = await createTestSession(classroom.id, teacher.id);
      await testDb.insert(sessionParticipants).values({
        sessionId: session.id,
        studentId: student.id,
      });

      const res = await LEAVE_SESSION(
        createRequest(`/api/sessions/${session.id}/leave`, { method: "POST" }),
        { params: Promise.resolve({ id: session.id }) }
      );

      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("leftAt");
    });

    it("returns 404 if not a participant", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email });
      const session = await createTestSession(classroom.id, teacher.id);

      const res = await LEAVE_SESSION(
        createRequest(`/api/sessions/${session.id}/leave`, { method: "POST" }),
        { params: Promise.resolve({ id: session.id }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(404);
    });
  });

  describe("GET /api/sessions/[id]/participants", () => {
    it("lists session participants", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const session = await createTestSession(classroom.id, teacher.id);
      await testDb.insert(sessionParticipants).values({
        sessionId: session.id,
        studentId: student.id,
      });

      const res = await GET_PARTICIPANTS(
        createRequest(`/api/sessions/${session.id}/participants`),
        { params: Promise.resolve({ id: session.id }) }
      );

      const { status, body } = await parseResponse<any[]>(res);
      expect(status).toBe(200);
      expect(body).toHaveLength(1);
      expect(body[0]).toHaveProperty("name", "Student");
    });
  });

  describe("GET /api/classrooms/[id]/active-session", () => {
    it("returns active session", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const session = await createTestSession(classroom.id, teacher.id);

      const res = await GET_ACTIVE(
        createRequest(`/api/classrooms/${classroom.id}/active-session`),
        { params: Promise.resolve({ id: classroom.id }) }
      );

      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("id", session.id);
    });

    it("returns null when no active session", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });

      const res = await GET_ACTIVE(
        createRequest(`/api/classrooms/${classroom.id}/active-session`),
        { params: Promise.resolve({ id: classroom.id }) }
      );

      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toBeNull();
    });
  });

  describe("Help Queue API", () => {
    it("student raises hand", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email });
      const session = await createTestSession(classroom.id, teacher.id);
      await testDb.insert(sessionParticipants).values({
        sessionId: session.id,
        studentId: student.id,
      });

      const res = await RAISE_HAND(
        createRequest(`/api/sessions/${session.id}/help-queue`, {
          method: "POST",
          body: { raised: true },
        }),
        { params: Promise.resolve({ id: session.id }) }
      );

      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("status", "needs_help");
    });

    it("lists help queue", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const session = await createTestSession(classroom.id, teacher.id);
      await testDb.insert(sessionParticipants).values({
        sessionId: session.id,
        studentId: student.id,
        status: "needs_help",
      });

      const res = await GET_HELP_QUEUE(
        createRequest(`/api/sessions/${session.id}/help-queue`),
        { params: Promise.resolve({ id: session.id }) }
      );

      const { status, body } = await parseResponse<any[]>(res);
      expect(status).toBe(200);
      expect(body).toHaveLength(1);
      expect(body[0]).toHaveProperty("name", "Student");
    });
  });
});

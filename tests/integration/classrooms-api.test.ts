import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, createTestClassroom } from "../helpers";
import { setMockUser, createRequest, parseResponse } from "../api-helpers";
import { GET, POST } from "@/app/api/classrooms/route";
import { GET as GET_CLASSROOM } from "@/app/api/classrooms/[id]/route";
import { GET as GET_MEMBERS } from "@/app/api/classrooms/[id]/members/route";
import { POST as JOIN } from "@/app/api/classrooms/join/route";
import { classroomMembers } from "@/lib/db/schema";

describe("Classroom API", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;

  beforeEach(async () => {
    teacher = await createTestUser({ name: "Teacher", email: "teacher@test.edu" });
    student = await createTestUser({ name: "Student", email: "student@test.edu" });
  });

  describe("POST /api/classrooms", () => {
    it("creates a classroom as teacher", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });

      const req = createRequest("/api/classrooms", {
        method: "POST",
        body: {
          name: "Python 101",
          gradeLevel: "6-8",
          editorMode: "python",
        },
      });

      const { status, body } = await parseResponse(await POST(req));
      expect(status).toBe(201);
      expect(body).toHaveProperty("name", "Python 101");
      expect(body).toHaveProperty("joinCode");
      expect((body as any).joinCode).toHaveLength(8);
    });

    // TODO: role-based rejection tests will be re-added with OrgMembership-based auth

    it("rejects unauthenticated request", async () => {
      setMockUser(null);

      const req = createRequest("/api/classrooms", {
        method: "POST",
        body: {
          name: "No Auth",
          gradeLevel: "6-8",
          editorMode: "python",
        },
      });

      const { status } = await parseResponse(await POST(req));
      expect(status).toBe(401);
    });

    it("rejects invalid grade level", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });

      const req = createRequest("/api/classrooms", {
        method: "POST",
        body: {
          name: "Bad Grade",
          gradeLevel: "13-16",
          editorMode: "python",
        },
      });

      const { status } = await parseResponse(await POST(req));
      expect(status).toBe(400);
    });
  });

  describe("GET /api/classrooms", () => {
    it("lists classrooms for teacher", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      await createTestClassroom(teacher.id, { name: "Class A" });
      await createTestClassroom(teacher.id, { name: "Class B" });

      const req = createRequest("/api/classrooms");
      const { status, body } = await parseResponse<any[]>(await GET());
      expect(status).toBe(200);
      expect(body).toHaveLength(2);
    });

    it("lists classrooms where student is member", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email });
      const classroom = await createTestClassroom(teacher.id, { name: "Joined Class" });
      await testDb.insert(classroomMembers).values({
        classroomId: classroom.id,
        userId: student.id,
      });

      const req = createRequest("/api/classrooms");
      const { status, body } = await parseResponse<any[]>(await GET());
      expect(status).toBe(200);
      expect(body).toHaveLength(1);
      expect(body[0]).toHaveProperty("name", "Joined Class");
    });

    it("returns empty for student with no classrooms", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email });

      const { status, body } = await parseResponse<any[]>(await GET());
      expect(status).toBe(200);
      expect(body).toHaveLength(0);
    });
  });

  describe("GET /api/classrooms/[id]", () => {
    it("returns classroom by ID", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const classroom = await createTestClassroom(teacher.id, { name: "Find Me" });

      const res = await GET_CLASSROOM(
        createRequest(`/api/classrooms/${classroom.id}`),
        { params: Promise.resolve({ id: classroom.id }) }
      );

      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("name", "Find Me");
    });

    it("returns 404 for non-existent classroom", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });

      const res = await GET_CLASSROOM(
        createRequest("/api/classrooms/00000000-0000-0000-0000-000000000000"),
        { params: Promise.resolve({ id: "00000000-0000-0000-0000-000000000000" }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(404);
    });
  });

  describe("POST /api/classrooms/join", () => {
    it("joins a classroom by code", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email });
      const classroom = await createTestClassroom(teacher.id);

      const req = createRequest("/api/classrooms/join", {
        method: "POST",
        body: { joinCode: classroom.joinCode },
      });

      const { status, body } = await parseResponse(await JOIN(req));
      expect(status).toBe(200);
      expect(body).toHaveProperty("id", classroom.id);
    });

    it("returns 404 for invalid code", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email });

      const req = createRequest("/api/classrooms/join", {
        method: "POST",
        body: { joinCode: "ZZZZZZZZ" },
      });

      const { status } = await parseResponse(await JOIN(req));
      expect(status).toBe(404);
    });

    it("rejects malformed code", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email });

      const req = createRequest("/api/classrooms/join", {
        method: "POST",
        body: { joinCode: "short" },
      });

      const { status } = await parseResponse(await JOIN(req));
      expect(status).toBe(400);
    });
  });

  describe("GET /api/classrooms/[id]/members", () => {
    it("lists classroom members", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const classroom = await createTestClassroom(teacher.id);
      await testDb.insert(classroomMembers).values({
        classroomId: classroom.id,
        userId: student.id,
      });

      const res = await GET_MEMBERS(
        createRequest(`/api/classrooms/${classroom.id}/members`),
        { params: Promise.resolve({ id: classroom.id }) }
      );

      const { status, body } = await parseResponse<any[]>(res);
      expect(status).toBe(200);
      expect(body).toHaveLength(1);
      expect(body[0]).toHaveProperty("name", "Student");
    });
  });
});

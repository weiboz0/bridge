import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, createTestOrg, createTestCourse } from "../helpers";
import { setMockUser, createRequest, parseResponse } from "../api-helpers";
import { PATCH, DELETE } from "@/app/api/classes/[id]/members/[memberId]/route";
import { createClass } from "@/lib/classes";
import { addClassMember, listClassMembers } from "@/lib/class-memberships";

describe("Class Member Management API", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;
  let org: Awaited<ReturnType<typeof createTestOrg>>;
  let course: Awaited<ReturnType<typeof createTestCourse>>;

  beforeEach(async () => {
    org = await createTestOrg();
    teacher = await createTestUser({ name: "Teacher", email: "teacher@test.edu" });
    student = await createTestUser({ name: "Student", email: "student@test.edu" });
    course = await createTestCourse(org.id, teacher.id);
  });

  describe("PATCH /api/classes/[id]/members/[memberId]", () => {
    it("updates member role", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Test", createdBy: teacher.id });
      const member = await addClassMember(testDb, { classId: cls.id, userId: student.id, role: "student" });

      const res = await PATCH(
        createRequest(`/api/classes/${cls.id}/members/${member!.id}`, {
          method: "PATCH",
          body: { role: "ta" },
        }),
        { params: Promise.resolve({ id: cls.id, memberId: member!.id }) }
      );

      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("role", "ta");
    });

    it("returns 404 for membership from different class", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const cls1 = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Class 1", createdBy: teacher.id });
      const cls2 = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Class 2", createdBy: teacher.id });
      const member = await addClassMember(testDb, { classId: cls2.id, userId: student.id, role: "student" });

      const res = await PATCH(
        createRequest(`/api/classes/${cls1.id}/members/${member!.id}`, {
          method: "PATCH",
          body: { role: "ta" },
        }),
        { params: Promise.resolve({ id: cls1.id, memberId: member!.id }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(404);
    });

    it("rejects invalid role", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Test", createdBy: teacher.id });
      const member = await addClassMember(testDb, { classId: cls.id, userId: student.id, role: "student" });

      const res = await PATCH(
        createRequest(`/api/classes/${cls.id}/members/${member!.id}`, {
          method: "PATCH",
          body: { role: "superadmin" },
        }),
        { params: Promise.resolve({ id: cls.id, memberId: member!.id }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(400);
    });
  });

  describe("DELETE /api/classes/[id]/members/[memberId]", () => {
    it("removes a member", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Test", createdBy: teacher.id });
      const member = await addClassMember(testDb, { classId: cls.id, userId: student.id, role: "student" });

      const res = await DELETE(
        createRequest(`/api/classes/${cls.id}/members/${member!.id}`, { method: "DELETE" }),
        { params: Promise.resolve({ id: cls.id, memberId: member!.id }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(200);

      // Verify removed
      const members = await listClassMembers(testDb, cls.id);
      const studentMembers = members.filter((m) => m.role === "student");
      expect(studentMembers).toHaveLength(0);
    });

    it("returns 404 for membership from different class", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const cls1 = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Class 1", createdBy: teacher.id });
      const cls2 = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Class 2", createdBy: teacher.id });
      const member = await addClassMember(testDb, { classId: cls2.id, userId: student.id, role: "student" });

      const res = await DELETE(
        createRequest(`/api/classes/${cls1.id}/members/${member!.id}`, { method: "DELETE" }),
        { params: Promise.resolve({ id: cls1.id, memberId: member!.id }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(404);
    });
  });
});

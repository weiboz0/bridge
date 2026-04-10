import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  createTestUser,
  createTestClassroom,
} from "../helpers";
import { classroomMembers } from "@/lib/db/schema";
import {
  createClassroom,
  listClassrooms,
  getClassroom,
  getClassroomMembers,
} from "@/lib/classrooms";

describe("classroom operations", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;

  beforeEach(async () => {
    teacher = await createTestUser({ role: "teacher", email: "teacher@school.edu" });
    student = await createTestUser({ role: "student", email: "student@school.edu" });
  });

  describe("createClassroom", () => {
    it("creates a classroom with a generated join code", async () => {
      const classroom = await createClassroom(testDb, {
        teacherId: teacher.id,
        name: "Intro to Python",
        gradeLevel: "6-8" as const,
        editorMode: "python" as const,
      });

      expect(classroom.id).toBeDefined();
      expect(classroom.name).toBe("Intro to Python");
      expect(classroom.teacherId).toBe(teacher.id);
      expect(classroom.joinCode).toHaveLength(8);
    });

    it("creates unique join codes", async () => {
      const c1 = await createClassroom(testDb, {
        teacherId: teacher.id,
        name: "Class 1",
        gradeLevel: "6-8" as const,
        editorMode: "python" as const,
      });
      const c2 = await createClassroom(testDb, {
        teacherId: teacher.id,
        name: "Class 2",
        gradeLevel: "6-8" as const,
        editorMode: "python" as const,
      });

      expect(c1.joinCode).not.toBe(c2.joinCode);
    });
  });

  describe("listClassrooms", () => {
    it("returns classrooms where user is teacher", async () => {
      await createClassroom(testDb, {
        teacherId: teacher.id,
        name: "My Class",
        gradeLevel: "6-8" as const,
        editorMode: "python" as const,
      });

      const results = await listClassrooms(testDb, teacher.id);
      expect(results).toHaveLength(1);
      expect(results[0].name).toBe("My Class");
    });

    it("returns classrooms where user is a member", async () => {
      const classroom = await createClassroom(testDb, {
        teacherId: teacher.id,
        name: "Shared Class",
        gradeLevel: "6-8" as const,
        editorMode: "python" as const,
      });

      await testDb.insert(classroomMembers).values({
        classroomId: classroom.id,
        userId: student.id,
      });

      const results = await listClassrooms(testDb, student.id);
      expect(results).toHaveLength(1);
      expect(results[0].name).toBe("Shared Class");
    });

    it("returns empty array for user with no classrooms", async () => {
      const results = await listClassrooms(testDb, student.id);
      expect(results).toHaveLength(0);
    });
  });

  describe("getClassroom", () => {
    it("returns classroom by ID", async () => {
      const created = await createClassroom(testDb, {
        teacherId: teacher.id,
        name: "Find Me",
        gradeLevel: "9-12" as const,
        editorMode: "javascript" as const,
      });

      const found = await getClassroom(testDb, created.id);
      expect(found).not.toBeNull();
      expect(found!.name).toBe("Find Me");
      expect(found!.editorMode).toBe("javascript");
    });

    it("returns null for non-existent ID", async () => {
      const found = await getClassroom(
        testDb,
        "00000000-0000-0000-0000-000000000000"
      );
      expect(found).toBeNull();
    });
  });

  describe("getClassroomMembers", () => {
    it("returns all members of a classroom", async () => {
      const classroom = await createClassroom(testDb, {
        teacherId: teacher.id,
        name: "Full Class",
        gradeLevel: "6-8" as const,
        editorMode: "python" as const,
      });

      await testDb.insert(classroomMembers).values({
        classroomId: classroom.id,
        userId: student.id,
      });

      const members = await getClassroomMembers(testDb, classroom.id);
      expect(members).toHaveLength(1);
      expect(members[0].userId).toBe(student.id);
    });
  });
});

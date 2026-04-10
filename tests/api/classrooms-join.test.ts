import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser } from "../helpers";
import {
  createClassroom,
  getClassroomByJoinCode,
  joinClassroom,
  getClassroomMembers,
} from "@/lib/classrooms";

describe("join classroom by code", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;

  beforeEach(async () => {
    teacher = await createTestUser({ role: "teacher", email: "teacher@school.edu" });
    student = await createTestUser({ role: "student", email: "student@school.edu" });
  });

  it("finds classroom by join code", async () => {
    const classroom = await createClassroom(testDb, {
      teacherId: teacher.id,
      name: "Join Me",
      gradeLevel: "6-8" as const,
      editorMode: "python" as const,
    });

    const found = await getClassroomByJoinCode(testDb, classroom.joinCode);
    expect(found).not.toBeNull();
    expect(found!.id).toBe(classroom.id);
  });

  it("returns null for invalid join code", async () => {
    const found = await getClassroomByJoinCode(testDb, "INVALID1");
    expect(found).toBeNull();
  });

  it("adds student to classroom via join", async () => {
    const classroom = await createClassroom(testDb, {
      teacherId: teacher.id,
      name: "Join Me",
      gradeLevel: "6-8" as const,
      editorMode: "python" as const,
    });

    const member = await joinClassroom(testDb, classroom.id, student.id);
    expect(member).toBeDefined();

    const members = await getClassroomMembers(testDb, classroom.id);
    expect(members).toHaveLength(1);
    expect(members[0].userId).toBe(student.id);
  });

  it("does not duplicate membership on re-join", async () => {
    const classroom = await createClassroom(testDb, {
      teacherId: teacher.id,
      name: "Join Me",
      gradeLevel: "6-8" as const,
      editorMode: "python" as const,
    });

    await joinClassroom(testDb, classroom.id, student.id);
    await joinClassroom(testDb, classroom.id, student.id);

    const members = await getClassroomMembers(testDb, classroom.id);
    expect(members).toHaveLength(1);
  });
});

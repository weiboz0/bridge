import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, createTestOrg, createTestCourse, createTestClass, createTestAssignment } from "../helpers";
import { createAssignment, getAssignment, listAssignmentsByClass, updateAssignment, deleteAssignment } from "@/lib/assignments";
import { createClass } from "@/lib/classes";

describe("assignment operations", () => {
  let cls: Awaited<ReturnType<typeof createClass>>;

  beforeEach(async () => {
    const org = await createTestOrg();
    const teacher = await createTestUser({ email: "teacher@test.edu" });
    const course = await createTestCourse(org.id, teacher.id);
    cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Test", createdBy: teacher.id });
  });

  it("creates an assignment", async () => {
    const assignment = await createAssignment(testDb, {
      classId: cls.id,
      title: "Homework 1",
      description: "Write a loop",
    });
    expect(assignment.id).toBeDefined();
    expect(assignment.title).toBe("Homework 1");
  });

  it("gets assignment by ID", async () => {
    const assignment = await createTestAssignment(cls.id, { title: "Find Me" });
    const found = await getAssignment(testDb, assignment.id);
    expect(found).not.toBeNull();
    expect(found!.title).toBe("Find Me");
  });

  it("returns null for non-existent", async () => {
    const found = await getAssignment(testDb, "00000000-0000-0000-0000-000000000000");
    expect(found).toBeNull();
  });

  it("lists assignments by class", async () => {
    await createTestAssignment(cls.id, { title: "A" });
    await createTestAssignment(cls.id, { title: "B" });
    const list = await listAssignmentsByClass(testDb, cls.id);
    expect(list).toHaveLength(2);
  });

  it("updates an assignment", async () => {
    const assignment = await createTestAssignment(cls.id);
    const updated = await updateAssignment(testDb, assignment.id, { title: "Updated" });
    expect(updated!.title).toBe("Updated");
  });

  it("deletes an assignment", async () => {
    const assignment = await createTestAssignment(cls.id);
    const deleted = await deleteAssignment(testDb, assignment.id);
    expect(deleted).not.toBeNull();
    const remaining = await listAssignmentsByClass(testDb, cls.id);
    expect(remaining).toHaveLength(0);
  });
});

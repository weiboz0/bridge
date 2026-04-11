import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, createTestOrg, createTestCourse, createTestClass } from "../helpers";
import { classMemberships } from "@/lib/db/schema";
import { createClass } from "@/lib/classes";
import { getLinkedChildren } from "@/lib/parent-links";

describe("parent-links", () => {
  let org: Awaited<ReturnType<typeof createTestOrg>>;
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;
  let parent: Awaited<ReturnType<typeof createTestUser>>;
  let course: Awaited<ReturnType<typeof createTestCourse>>;

  beforeEach(async () => {
    org = await createTestOrg();
    teacher = await createTestUser({ email: "teacher@test.edu" });
    student = await createTestUser({ name: "Alice Student", email: "student@test.edu" });
    parent = await createTestUser({ email: "parent@test.edu" });
    course = await createTestCourse(org.id, teacher.id);
  });

  it("returns empty when parent has no linked children", async () => {
    const children = await getLinkedChildren(testDb, parent.id);
    expect(children).toHaveLength(0);
  });

  it("finds children linked via parent class membership", async () => {
    const cls = await createClass(testDb, {
      courseId: course.id,
      orgId: org.id,
      title: "Test",
      createdBy: teacher.id,
    });

    // Add student and parent to same class
    await testDb.insert(classMemberships).values({ classId: cls.id, userId: student.id, role: "student" });
    await testDb.insert(classMemberships).values({ classId: cls.id, userId: parent.id, role: "parent" });

    const children = await getLinkedChildren(testDb, parent.id);
    expect(children).toHaveLength(1);
    expect(children[0].name).toBe("Alice Student");
    expect(children[0].classCount).toBeGreaterThanOrEqual(1);
  });

  it("deduplicates children across multiple classes", async () => {
    const cls1 = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Class 1", createdBy: teacher.id });
    const cls2 = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Class 2", createdBy: teacher.id });

    // Student in both classes, parent in both
    await testDb.insert(classMemberships).values({ classId: cls1.id, userId: student.id, role: "student" });
    await testDb.insert(classMemberships).values({ classId: cls1.id, userId: parent.id, role: "parent" });
    await testDb.insert(classMemberships).values({ classId: cls2.id, userId: student.id, role: "student" });
    await testDb.insert(classMemberships).values({ classId: cls2.id, userId: parent.id, role: "parent" });

    const children = await getLinkedChildren(testDb, parent.id);
    expect(children).toHaveLength(1); // deduplicated
    expect(children[0].classCount).toBe(2);
  });

  it("finds multiple children", async () => {
    const student2 = await createTestUser({ name: "Bob Student", email: "student2@test.edu" });
    const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Test", createdBy: teacher.id });

    await testDb.insert(classMemberships).values({ classId: cls.id, userId: student.id, role: "student" });
    await testDb.insert(classMemberships).values({ classId: cls.id, userId: student2.id, role: "student" });
    await testDb.insert(classMemberships).values({ classId: cls.id, userId: parent.id, role: "parent" });

    const children = await getLinkedChildren(testDb, parent.id);
    expect(children).toHaveLength(2);
  });
});

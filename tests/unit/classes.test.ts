import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, createTestOrg, createTestCourse, createTestClass } from "../helpers";
import { createClass, getClass, listClassesByOrg, getClassByJoinCode, archiveClass, getClassSettings } from "@/lib/classes";
import { addClassMember, listClassMembers, joinClassByCode } from "@/lib/class-memberships";

describe("class operations", () => {
  let org: Awaited<ReturnType<typeof createTestOrg>>;
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let course: Awaited<ReturnType<typeof createTestCourse>>;

  beforeEach(async () => {
    org = await createTestOrg();
    teacher = await createTestUser({ email: "teacher@test.edu" });
    course = await createTestCourse(org.id, teacher.id);
  });

  it("creates a class with auto-created settings and instructor", async () => {
    const cls = await createClass(testDb, {
      courseId: course.id,
      orgId: org.id,
      title: "Fall 2026 Period 3",
      createdBy: teacher.id,
    });
    expect(cls.id).toBeDefined();
    expect(cls.joinCode).toHaveLength(8);

    const settings = await getClassSettings(testDb, cls.id);
    expect(settings).not.toBeNull();

    // Creator is instructor
    const members = await listClassMembers(testDb, cls.id);
    expect(members).toHaveLength(1);
    expect(members[0].role).toBe("instructor");
    expect(members[0].userId).toBe(teacher.id);
  });

  it("lists classes by org", async () => {
    await createClass(testDb, { courseId: course.id, orgId: org.id, title: "A", createdBy: teacher.id });
    await createClass(testDb, { courseId: course.id, orgId: org.id, title: "B", createdBy: teacher.id });

    const list = await listClassesByOrg(testDb, org.id);
    expect(list).toHaveLength(2);
  });

  it("finds class by join code", async () => {
    const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Join Me", createdBy: teacher.id });
    const found = await getClassByJoinCode(testDb, cls.joinCode);
    expect(found).not.toBeNull();
    expect(found!.id).toBe(cls.id);
  });

  it("archives a class", async () => {
    const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Archive Me", createdBy: teacher.id });
    const archived = await archiveClass(testDb, cls.id);
    expect(archived!.status).toBe("archived");
  });

  it("student joins class by code", async () => {
    const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Open Class", createdBy: teacher.id });
    const student = await createTestUser({ email: "student@test.edu" });

    const result = await joinClassByCode(testDb, cls.joinCode, student.id);
    expect(result).not.toBeNull();
    expect(result!.class.id).toBe(cls.id);

    const members = await listClassMembers(testDb, cls.id);
    expect(members).toHaveLength(2); // instructor + student
  });
});

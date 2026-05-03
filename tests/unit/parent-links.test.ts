import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, createTestOrg, createTestCourse } from "../helpers";
import { classMemberships, parentLinks } from "@/lib/db/schema";
import { createClass } from "@/lib/classes";
import { getLinkedChildren } from "@/lib/parent-links";

// Plan 064 — `getLinkedChildren` now queries the `parent_links`
// table directly. Pre-064 it derived children from
// `class_memberships role="parent"` (a privacy leak: parent in a
// class saw ALL students in that class).

describe("parent-links (plan 064 — explicit parent_links table)", () => {
  let org: Awaited<ReturnType<typeof createTestOrg>>;
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;
  let parent: Awaited<ReturnType<typeof createTestUser>>;
  let admin: Awaited<ReturnType<typeof createTestUser>>;
  let course: Awaited<ReturnType<typeof createTestCourse>>;

  beforeEach(async () => {
    org = await createTestOrg();
    teacher = await createTestUser({ email: "teacher@test.edu" });
    student = await createTestUser({ name: "Alice Student", email: "student@test.edu" });
    parent = await createTestUser({ email: "parent@test.edu" });
    admin = await createTestUser({ email: "admin@test.edu", isPlatformAdmin: true });
    course = await createTestCourse(org.id, teacher.id);
  });

  it("returns empty when parent has no linked children", async () => {
    const children = await getLinkedChildren(testDb, parent.id);
    expect(children).toHaveLength(0);
  });

  it("finds a child via an active parent_link row", async () => {
    await testDb.insert(parentLinks).values({
      parentUserId: parent.id,
      childUserId: student.id,
      createdBy: admin.id,
    });

    const children = await getLinkedChildren(testDb, parent.id);
    expect(children).toHaveLength(1);
    expect(children[0].name).toBe("Alice Student");
    expect(children[0].userId).toBe(student.id);
  });

  it("counts a child's classes via class_memberships role=student", async () => {
    const cls1 = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "C1", createdBy: teacher.id });
    const cls2 = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "C2", createdBy: teacher.id });
    await testDb.insert(classMemberships).values({ classId: cls1.id, userId: student.id, role: "student" });
    await testDb.insert(classMemberships).values({ classId: cls2.id, userId: student.id, role: "student" });

    await testDb.insert(parentLinks).values({
      parentUserId: parent.id,
      childUserId: student.id,
      createdBy: admin.id,
    });

    const children = await getLinkedChildren(testDb, parent.id);
    expect(children).toHaveLength(1);
    expect(children[0].classCount).toBe(2);
  });

  it("returns multiple linked children, sorted by name", async () => {
    const child2 = await createTestUser({ name: "Charlie Student", email: "charlie@test.edu" });
    const child3 = await createTestUser({ name: "Bob Student", email: "bob@test.edu" });

    await testDb.insert(parentLinks).values([
      { parentUserId: parent.id, childUserId: student.id, createdBy: admin.id }, // Alice
      { parentUserId: parent.id, childUserId: child2.id, createdBy: admin.id }, // Charlie
      { parentUserId: parent.id, childUserId: child3.id, createdBy: admin.id }, // Bob
    ]);

    const children = await getLinkedChildren(testDb, parent.id);
    expect(children).toHaveLength(3);
    expect(children.map((c) => c.name)).toEqual(["Alice Student", "Bob Student", "Charlie Student"]);
  });

  it("REVOKED links do NOT show up", async () => {
    await testDb.insert(parentLinks).values({
      parentUserId: parent.id,
      childUserId: student.id,
      createdBy: admin.id,
      status: "revoked",
      revokedAt: new Date(),
    });

    const children = await getLinkedChildren(testDb, parent.id);
    expect(children).toHaveLength(0);
  });

  it("the OLD `class_memberships role='parent'` model NO LONGER grants access (plan 064 privacy fix)", async () => {
    // Pre-064 this would have returned the student. Post-064 the
    // class_memberships row is irrelevant — only parent_links
    // grants access.
    const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "C", createdBy: teacher.id });
    await testDb.insert(classMemberships).values({ classId: cls.id, userId: student.id, role: "student" });
    await testDb.insert(classMemberships).values({ classId: cls.id, userId: parent.id, role: "parent" });

    const children = await getLinkedChildren(testDb, parent.id);
    expect(children).toHaveLength(0);
  });
});

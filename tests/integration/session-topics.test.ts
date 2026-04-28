import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  createTestUser,
  createTestOrg,
  createTestCourse,
  createTestTopic,
  createTestClass,
  createTestTeachingUnit,
} from "../helpers";
import {
  getSessionTopics,
  getTopicLinkedUnit,
  listLinkedUnitsByTopicIds,
  linkSessionTopic,
} from "@/lib/session-topics";
import * as schema from "@/lib/db/schema";

/**
 * Plan 044 phase 1: read paths surface the linked teaching_unit
 * alongside legacy topic content. Tests cover:
 *  - LEFT JOIN returns null Unit fields when no Unit is linked.
 *  - LEFT JOIN populates Unit fields when one is linked.
 *  - Cross-org leak guard: a Unit whose scope_id mismatches the
 *    topic's course org_id must NOT surface.
 */

async function makeSession(courseId: string, orgId: string, teacherId: string) {
  const cls = await createTestClass(courseId, orgId);
  const [s] = await testDb
    .insert(schema.sessions)
    .values({
      classId: cls.id,
      teacherId,
      title: "Session",
      status: "live",
    })
    .returning();
  return s;
}

describe("getSessionTopics + Unit linkage (plan 044)", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let org: Awaited<ReturnType<typeof createTestOrg>>;

  beforeEach(async () => {
    teacher = await createTestUser({ name: "Teacher", email: "t@example.com" });
    org = await createTestOrg();
  });

  it("returns null Unit fields when no Unit is linked", async () => {
    const course = await createTestCourse(org.id, teacher.id);
    const topic = await createTestTopic(course.id);
    const session = await makeSession(course.id, org.id, teacher.id);
    await linkSessionTopic(testDb, session.id, topic.id);

    const rows = await getSessionTopics(testDb, session.id);
    expect(rows).toHaveLength(1);
    expect(rows[0].topicId).toBe(topic.id);
    expect(rows[0].unitId).toBeNull();
    expect(rows[0].unitTitle).toBeNull();
    expect(rows[0].unitMaterialType).toBeNull();
  });

  it("populates Unit fields when an in-org Unit is linked to the topic", async () => {
    const course = await createTestCourse(org.id, teacher.id);
    const topic = await createTestTopic(course.id);
    const unit = await createTestTeachingUnit(org.id, teacher.id, {
      title: "Loops",
      materialType: "notes",
      topicId: topic.id,
    });
    const session = await makeSession(course.id, org.id, teacher.id);
    await linkSessionTopic(testDb, session.id, topic.id);

    const rows = await getSessionTopics(testDb, session.id);
    expect(rows).toHaveLength(1);
    expect(rows[0].unitId).toBe(unit.id);
    expect(rows[0].unitTitle).toBe("Loops");
    expect(rows[0].unitMaterialType).toBe("notes");
  });

  it("does NOT surface a Unit whose scope_id mismatches the topic's course org_id", async () => {
    // Course is in org A; the Unit's scope_id points at org B (a
    // misalignment that shouldn't happen in practice but the read-path
    // join must defend against per Codex correction #3).
    const orgB = await createTestOrg({ slug: "org-b", contactEmail: "b@e.com" });
    const course = await createTestCourse(org.id, teacher.id);
    const topic = await createTestTopic(course.id);
    await createTestTeachingUnit(orgB.id, teacher.id, { topicId: topic.id });
    const session = await makeSession(course.id, org.id, teacher.id);
    await linkSessionTopic(testDb, session.id, topic.id);

    const rows = await getSessionTopics(testDb, session.id);
    expect(rows).toHaveLength(1);
    expect(rows[0].unitId).toBeNull();
  });

  it("surfaces a platform-scope Unit linked to any topic", async () => {
    const course = await createTestCourse(org.id, teacher.id);
    const topic = await createTestTopic(course.id);
    const unit = await createTestTeachingUnit(org.id, teacher.id, {
      scope: "platform",
      scopeId: null,
      topicId: topic.id,
    });
    const session = await makeSession(course.id, org.id, teacher.id);
    await linkSessionTopic(testDb, session.id, topic.id);

    const rows = await getSessionTopics(testDb, session.id);
    expect(rows[0].unitId).toBe(unit.id);
  });
});

describe("getTopicLinkedUnit (plan 044)", () => {
  it("returns null when no Unit is linked", async () => {
    const teacher = await createTestUser({ name: "T", email: "t@e.com" });
    const org = await createTestOrg();
    const course = await createTestCourse(org.id, teacher.id);
    const topic = await createTestTopic(course.id);

    const got = await getTopicLinkedUnit(testDb, topic.id);
    expect(got).toBeNull();
  });

  it("returns the linked Unit's identity", async () => {
    const teacher = await createTestUser({ name: "T", email: "t@e.com" });
    const org = await createTestOrg();
    const course = await createTestCourse(org.id, teacher.id);
    const topic = await createTestTopic(course.id);
    const unit = await createTestTeachingUnit(org.id, teacher.id, {
      title: "Loops",
      materialType: "slides",
      topicId: topic.id,
    });

    const got = await getTopicLinkedUnit(testDb, topic.id);
    expect(got).not.toBeNull();
    expect(got!.unitId).toBe(unit.id);
    expect(got!.unitTitle).toBe("Loops");
    expect(got!.unitMaterialType).toBe("slides");
  });

  it("rejects mismatched-org Unit (cross-org leak guard)", async () => {
    const teacher = await createTestUser({ name: "T", email: "t@e.com" });
    const orgA = await createTestOrg();
    const orgB = await createTestOrg({ slug: "b", contactEmail: "b@e.com" });
    const course = await createTestCourse(orgA.id, teacher.id);
    const topic = await createTestTopic(course.id);
    await createTestTeachingUnit(orgB.id, teacher.id, { topicId: topic.id });

    const got = await getTopicLinkedUnit(testDb, topic.id);
    expect(got).toBeNull();
  });
});

describe("listLinkedUnitsByTopicIds (plan 044)", () => {
  it("returns an empty map for an empty input", async () => {
    expect(await listLinkedUnitsByTopicIds(testDb, [])).toEqual({});
  });

  it("returns linked Units keyed by topicId, omitting topics without Units", async () => {
    const teacher = await createTestUser({ name: "T", email: "t@e.com" });
    const org = await createTestOrg();
    const course = await createTestCourse(org.id, teacher.id);
    const topicWithUnit = await createTestTopic(course.id, { title: "T1" });
    const topicWithout = await createTestTopic(course.id, { title: "T2", sortOrder: 1 });
    const unit = await createTestTeachingUnit(org.id, teacher.id, {
      topicId: topicWithUnit.id,
    });

    const got = await listLinkedUnitsByTopicIds(testDb, [
      topicWithUnit.id,
      topicWithout.id,
    ]);
    expect(Object.keys(got)).toHaveLength(1);
    expect(got[topicWithUnit.id].unitId).toBe(unit.id);
    expect(got[topicWithout.id]).toBeUndefined();
  });
});

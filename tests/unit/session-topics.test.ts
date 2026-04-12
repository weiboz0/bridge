import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, createTestOrg, createTestCourse, createTestTopic, createTestClassroom, createTestSession } from "../helpers";
import { linkSessionTopic, unlinkSessionTopic, getSessionTopics } from "@/lib/session-topics";

describe("session-topics operations", () => {
  let session: Awaited<ReturnType<typeof createTestSession>>;
  let topic1: Awaited<ReturnType<typeof createTestTopic>>;
  let topic2: Awaited<ReturnType<typeof createTestTopic>>;

  beforeEach(async () => {
    const org = await createTestOrg();
    const teacher = await createTestUser({ email: "teacher@test.edu" });
    const course = await createTestCourse(org.id, teacher.id);
    topic1 = await createTestTopic(course.id, { title: "Topic 1", sortOrder: 0 });
    topic2 = await createTestTopic(course.id, { title: "Topic 2", sortOrder: 1 });
    const classroom = await createTestClassroom(teacher.id);
    session = await createTestSession(classroom.id, teacher.id);
  });

  it("links a topic to a session", async () => {
    const link = await linkSessionTopic(testDb, session.id, topic1.id);
    expect(link).not.toBeNull();
  });

  it("does not duplicate links", async () => {
    await linkSessionTopic(testDb, session.id, topic1.id);
    const dup = await linkSessionTopic(testDb, session.id, topic1.id);
    expect(dup).toBeNull();
  });

  it("gets session topics ordered by sortOrder", async () => {
    await linkSessionTopic(testDb, session.id, topic2.id);
    await linkSessionTopic(testDb, session.id, topic1.id);

    const topics = await getSessionTopics(testDb, session.id);
    expect(topics).toHaveLength(2);
    expect(topics[0].title).toBe("Topic 1"); // sortOrder 0
    expect(topics[1].title).toBe("Topic 2"); // sortOrder 1
  });

  it("unlinks a topic from a session", async () => {
    await linkSessionTopic(testDb, session.id, topic1.id);
    const removed = await unlinkSessionTopic(testDb, session.id, topic1.id);
    expect(removed).not.toBeNull();

    const remaining = await getSessionTopics(testDb, session.id);
    expect(remaining).toHaveLength(0);
  });

  it("returns empty for session with no topics", async () => {
    const topics = await getSessionTopics(testDb, session.id);
    expect(topics).toHaveLength(0);
  });
});

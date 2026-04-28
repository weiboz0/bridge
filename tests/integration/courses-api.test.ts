import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, createTestOrg, createTestOrgMembership, createTestCourse, createTestTopic } from "../helpers";
import { setMockUser, createRequest, parseResponse } from "../api-helpers";
import { POST, GET } from "@/app/api/courses/route";
import { GET as GET_COURSE, PATCH, DELETE } from "@/app/api/courses/[id]/route";
import { POST as CLONE } from "@/app/api/courses/[id]/clone/route";
import { POST as CREATE_TOPIC, GET as LIST_TOPICS } from "@/app/api/courses/[id]/topics/route";
import { GET as GET_TOPIC, PATCH as UPDATE_TOPIC, DELETE as DELETE_TOPIC } from "@/app/api/courses/[id]/topics/[topicId]/route";
import { PATCH as REORDER } from "@/app/api/courses/[id]/topics/reorder/route";

describe("Courses API", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let otherUser: Awaited<ReturnType<typeof createTestUser>>;
  let org: Awaited<ReturnType<typeof createTestOrg>>;

  beforeEach(async () => {
    org = await createTestOrg();
    teacher = await createTestUser({ name: "Teacher", email: "teacher@test.edu" });
    otherUser = await createTestUser({ name: "Other", email: "other@test.edu" });
    await createTestOrgMembership(org.id, teacher.id, { role: "teacher" });
  });

  describe("POST /api/courses", () => {
    it("teacher creates a course", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });

      const req = createRequest("/api/courses", {
        method: "POST",
        body: { orgId: org.id, title: "Python 101", gradeLevel: "6-8" },
      });
      const { status, body } = await parseResponse(await POST(req));
      expect(status).toBe(201);
      expect(body).toHaveProperty("title", "Python 101");
    });

    it("non-member cannot create course", async () => {
      setMockUser({ id: otherUser.id, name: otherUser.name, email: otherUser.email });

      const req = createRequest("/api/courses", {
        method: "POST",
        body: { orgId: org.id, title: "Nope", gradeLevel: "6-8" },
      });
      const { status } = await parseResponse(await POST(req));
      expect(status).toBe(403);
    });

    it("rejects invalid input", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });

      const req = createRequest("/api/courses", {
        method: "POST",
        body: { orgId: org.id },
      });
      const { status } = await parseResponse(await POST(req));
      expect(status).toBe(400);
    });
  });

  describe("GET /api/courses", () => {
    it("lists courses by org", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      await createTestCourse(org.id, teacher.id, { title: "Course A" });
      await createTestCourse(org.id, teacher.id, { title: "Course B" });

      const req = createRequest("/api/courses", { searchParams: { orgId: org.id } });
      const { status, body } = await parseResponse<any[]>(await GET(req));
      expect(status).toBe(200);
      expect(body).toHaveLength(2);
    });

    it("non-member cannot list courses", async () => {
      setMockUser({ id: otherUser.id, name: otherUser.name, email: otherUser.email });

      const req = createRequest("/api/courses", { searchParams: { orgId: org.id } });
      const { status } = await parseResponse(await GET(req));
      expect(status).toBe(403);
    });
  });

  describe("GET/PATCH/DELETE /api/courses/[id]", () => {
    it("gets a course", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const course = await createTestCourse(org.id, teacher.id, { title: "Find Me" });

      const res = await GET_COURSE(
        createRequest(`/api/courses/${course.id}`),
        { params: Promise.resolve({ id: course.id }) }
      );
      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("title", "Find Me");
    });

    it("creator can update course", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const course = await createTestCourse(org.id, teacher.id);

      const res = await PATCH(
        createRequest(`/api/courses/${course.id}`, {
          method: "PATCH",
          body: { title: "Updated", isPublished: true },
        }),
        { params: Promise.resolve({ id: course.id }) }
      );
      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("title", "Updated");
      expect(body).toHaveProperty("isPublished", true);
    });

    it("non-creator cannot update", async () => {
      setMockUser({ id: otherUser.id, name: otherUser.name, email: otherUser.email });
      const course = await createTestCourse(org.id, teacher.id);

      const res = await PATCH(
        createRequest(`/api/courses/${course.id}`, {
          method: "PATCH",
          body: { title: "Hacked" },
        }),
        { params: Promise.resolve({ id: course.id }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(403);
    });

    it("creator can delete course", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const course = await createTestCourse(org.id, teacher.id);

      const res = await DELETE(
        createRequest(`/api/courses/${course.id}`, { method: "DELETE" }),
        { params: Promise.resolve({ id: course.id }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(200);
    });
  });

  describe("POST /api/courses/[id]/clone", () => {
    it("clones a course", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const course = await createTestCourse(org.id, teacher.id, { title: "Original" });
      await createTestTopic(course.id, { title: "Topic 1" });

      const res = await CLONE(
        createRequest(`/api/courses/${course.id}/clone`, { method: "POST" }),
        { params: Promise.resolve({ id: course.id }) }
      );
      const { status, body } = await parseResponse(res);
      expect(status).toBe(201);
      expect(body).toHaveProperty("title", "Original (Copy)");
    });
  });

  describe("Topics API", () => {
    it("creates and lists topics", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const course = await createTestCourse(org.id, teacher.id);

      // Create
      const createRes = await CREATE_TOPIC(
        createRequest(`/api/courses/${course.id}/topics`, {
          method: "POST",
          body: { title: "Variables" },
        }),
        { params: Promise.resolve({ id: course.id }) }
      );
      const { status: createStatus } = await parseResponse(createRes);
      expect(createStatus).toBe(201);

      // List
      const listRes = await LIST_TOPICS(
        createRequest(`/api/courses/${course.id}/topics`),
        { params: Promise.resolve({ id: course.id }) }
      );
      const { status: listStatus, body: topics } = await parseResponse<any[]>(listRes);
      expect(listStatus).toBe(200);
      expect(topics).toHaveLength(1);
      expect(topics[0]).toHaveProperty("title", "Variables");
    });

    it("updates a topic", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const course = await createTestCourse(org.id, teacher.id);
      const topic = await createTestTopic(course.id);

      const res = await UPDATE_TOPIC(
        createRequest(`/api/courses/${course.id}/topics/${topic.id}`, {
          method: "PATCH",
          body: { title: "Updated Topic" },
        }),
        { params: Promise.resolve({ id: course.id, topicId: topic.id }) }
      );
      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("title", "Updated Topic");
    });

    // Plan 046: the lessonContent and starterCode columns are gone, but
    // strict zod still rejects ANY unknown field on topic POST/PATCH.
    // These tests use lessonContent as a canary so future readers
    // grepping for the deprecated name see the contract is enforced.
    it("rejects POST with unknown field (lessonContent)", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const course = await createTestCourse(org.id, teacher.id);
      const res = await CREATE_TOPIC(
        createRequest(`/api/courses/${course.id}/topics`, {
          method: "POST",
          body: { title: "X", lessonContent: { blocks: [] } },
        }),
        { params: Promise.resolve({ id: course.id }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(400);
    });

    it("rejects PATCH with unknown field (lessonContent)", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const course = await createTestCourse(org.id, teacher.id);
      const topic = await createTestTopic(course.id);
      const res = await UPDATE_TOPIC(
        createRequest(`/api/courses/${course.id}/topics/${topic.id}`, {
          method: "PATCH",
          body: { lessonContent: { blocks: [{ type: "p" }] } },
        }),
        { params: Promise.resolve({ id: course.id, topicId: topic.id }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(400);
    });

    it("deletes a topic", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const course = await createTestCourse(org.id, teacher.id);
      const topic = await createTestTopic(course.id);

      const res = await DELETE_TOPIC(
        createRequest(`/api/courses/${course.id}/topics/${topic.id}`, { method: "DELETE" }),
        { params: Promise.resolve({ id: course.id, topicId: topic.id }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(200);
    });

    it("reorders topics", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email });
      const course = await createTestCourse(org.id, teacher.id);
      const t1 = await createTestTopic(course.id, { title: "First", sortOrder: 0 });
      const t2 = await createTestTopic(course.id, { title: "Second", sortOrder: 1 });

      const res = await REORDER(
        createRequest(`/api/courses/${course.id}/topics/reorder`, {
          method: "PATCH",
          body: { topicIds: [t2.id, t1.id] },
        }),
        { params: Promise.resolve({ id: course.id }) }
      );
      const { status } = await parseResponse(res);
      expect(status).toBe(200);
    });
  });
});

import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser } from "../helpers";
import { setMockUser, createRequest, parseResponse } from "../api-helpers";
import { POST as CREATE, GET as LIST } from "@/app/api/annotations/route";
import { DELETE, PATCH } from "@/app/api/annotations/[id]/route";
import { createAnnotation } from "@/lib/annotations";

describe("Annotations API", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;

  beforeEach(async () => {
    teacher = await createTestUser({ name: "Teacher", role: "teacher", email: "teacher@test.edu" });
  });

  describe("POST /api/annotations", () => {
    it("creates an annotation", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });

      const req = createRequest("/api/annotations", {
        method: "POST",
        body: {
          documentId: "session:abc:user:xyz",
          lineStart: "5",
          lineEnd: "7",
          content: "Good work on this loop!",
        },
      });

      const { status, body } = await parseResponse(await CREATE(req));
      expect(status).toBe(201);
      expect(body).toHaveProperty("content", "Good work on this loop!");
      expect(body).toHaveProperty("lineStart", "5");
      expect(body).toHaveProperty("lineEnd", "7");
      expect(body).toHaveProperty("authorType", "teacher");
    });

    it("rejects unauthenticated request", async () => {
      setMockUser(null);

      const req = createRequest("/api/annotations", {
        method: "POST",
        body: {
          documentId: "session:abc:user:xyz",
          lineStart: "1",
          lineEnd: "1",
          content: "test",
        },
      });

      const { status } = await parseResponse(await CREATE(req));
      expect(status).toBe(401);
    });

    it("rejects missing content", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });

      const req = createRequest("/api/annotations", {
        method: "POST",
        body: {
          documentId: "session:abc:user:xyz",
          lineStart: "1",
          lineEnd: "1",
        },
      });

      const { status } = await parseResponse(await CREATE(req));
      expect(status).toBe(400);
    });
  });

  describe("GET /api/annotations", () => {
    it("lists annotations by document", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });
      const docId = "session:abc:user:xyz";

      await createAnnotation(testDb, {
        documentId: docId,
        authorId: teacher.id,
        authorType: "teacher",
        lineStart: "1",
        lineEnd: "1",
        content: "First",
      });
      await createAnnotation(testDb, {
        documentId: docId,
        authorId: teacher.id,
        authorType: "teacher",
        lineStart: "5",
        lineEnd: "5",
        content: "Second",
      });

      const req = createRequest("/api/annotations", {
        searchParams: { documentId: docId },
      });

      const { status, body } = await parseResponse<any[]>(await LIST(req));
      expect(status).toBe(200);
      expect(body).toHaveLength(2);
    });

    it("requires documentId param", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });

      const req = createRequest("/api/annotations");
      const { status } = await parseResponse(await LIST(req));
      expect(status).toBe(400);
    });
  });

  describe("DELETE /api/annotations/[id]", () => {
    it("deletes an annotation", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });

      const annotation = await createAnnotation(testDb, {
        documentId: "session:abc:user:xyz",
        authorId: teacher.id,
        authorType: "teacher",
        lineStart: "1",
        lineEnd: "1",
        content: "Delete me",
      });

      const res = await DELETE(
        createRequest(`/api/annotations/${annotation.id}`, { method: "DELETE" }),
        { params: Promise.resolve({ id: annotation.id }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(200);
    });

    it("returns 404 for non-existent annotation", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });

      const res = await DELETE(
        createRequest("/api/annotations/00000000-0000-0000-0000-000000000000", { method: "DELETE" }),
        { params: Promise.resolve({ id: "00000000-0000-0000-0000-000000000000" }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(404);
    });
  });

  describe("PATCH /api/annotations/[id] (resolve)", () => {
    it("resolves an annotation", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });

      const annotation = await createAnnotation(testDb, {
        documentId: "session:abc:user:xyz",
        authorId: teacher.id,
        authorType: "teacher",
        lineStart: "1",
        lineEnd: "1",
        content: "Resolve me",
      });

      const res = await PATCH(
        createRequest(`/api/annotations/${annotation.id}`, { method: "PATCH" }),
        { params: Promise.resolve({ id: annotation.id }) }
      );

      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("resolved");
      expect((body as any).resolved).not.toBeNull();
    });
  });
});

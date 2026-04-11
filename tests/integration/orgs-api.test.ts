import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, createTestOrg, createTestOrgMembership } from "../helpers";
import { setMockUser, createRequest, parseResponse } from "../api-helpers";
import { POST, GET } from "@/app/api/orgs/route";
import { GET as GET_ORG } from "@/app/api/orgs/[id]/route";
import { POST as ADD_MEMBER, GET as LIST_MEMBERS } from "@/app/api/orgs/[id]/members/route";
import { DELETE as REMOVE_MEMBER } from "@/app/api/orgs/[id]/members/[memberId]/route";

describe("Organization API", () => {
  let user: Awaited<ReturnType<typeof createTestUser>>;

  beforeEach(async () => {
    user = await createTestUser({ name: "Admin", email: "admin@test.edu" });
  });

  describe("POST /api/orgs", () => {
    it("creates a pending org and assigns creator as org_admin", async () => {
      setMockUser({ id: user.id, name: user.name, email: user.email });

      const req = createRequest("/api/orgs", {
        method: "POST",
        body: {
          name: "Lincoln High",
          slug: "lincoln-high",
          type: "school",
          contactEmail: "admin@lincoln.edu",
          contactName: "Admin User",
        },
      });

      const { status, body } = await parseResponse(await POST(req));
      expect(status).toBe(201);
      expect(body).toHaveProperty("name", "Lincoln High");
      expect(body).toHaveProperty("status", "pending");
    });

    it("rejects unauthenticated request", async () => {
      setMockUser(null);

      const req = createRequest("/api/orgs", {
        method: "POST",
        body: {
          name: "Test",
          slug: "test",
          type: "school",
          contactEmail: "a@b.com",
          contactName: "A",
        },
      });

      const { status } = await parseResponse(await POST(req));
      expect(status).toBe(401);
    });

    it("rejects invalid slug", async () => {
      setMockUser({ id: user.id, name: user.name, email: user.email });

      const req = createRequest("/api/orgs", {
        method: "POST",
        body: {
          name: "Test",
          slug: "Invalid Slug!",
          type: "school",
          contactEmail: "a@b.com",
          contactName: "A",
        },
      });

      const { status } = await parseResponse(await POST(req));
      expect(status).toBe(400);
    });
  });

  describe("GET /api/orgs", () => {
    it("lists user's org memberships", async () => {
      setMockUser({ id: user.id, name: user.name, email: user.email });
      const org = await createTestOrg();
      await createTestOrgMembership(org.id, user.id, { role: "teacher" });

      const { status, body } = await parseResponse<any[]>(await GET());
      expect(status).toBe(200);
      expect(body).toHaveLength(1);
    });
  });

  describe("GET /api/orgs/[id]", () => {
    it("returns org for member", async () => {
      setMockUser({ id: user.id, name: user.name, email: user.email });
      const org = await createTestOrg({ name: "My Org" });
      await createTestOrgMembership(org.id, user.id);

      const res = await GET_ORG(
        createRequest(`/api/orgs/${org.id}`),
        { params: Promise.resolve({ id: org.id }) }
      );

      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("name", "My Org");
    });

    it("rejects non-member access", async () => {
      setMockUser({ id: user.id, name: user.name, email: user.email });
      const org = await createTestOrg();

      const res = await GET_ORG(
        createRequest(`/api/orgs/${org.id}`),
        { params: Promise.resolve({ id: org.id }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(403);
    });

    it("allows platform admin access", async () => {
      setMockUser({ id: user.id, name: user.name, email: user.email, isPlatformAdmin: true });
      const org = await createTestOrg({ name: "Any Org" });

      const res = await GET_ORG(
        createRequest(`/api/orgs/${org.id}`),
        { params: Promise.resolve({ id: org.id }) }
      );

      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("name", "Any Org");
    });
  });

  describe("Members API", () => {
    it("org admin adds a member by email", async () => {
      const org = await createTestOrg();
      await createTestOrgMembership(org.id, user.id, { role: "org_admin" });
      const teacher = await createTestUser({ name: "Teacher", email: "teacher@test.edu" });

      setMockUser({ id: user.id, name: user.name, email: user.email });

      const res = await ADD_MEMBER(
        createRequest(`/api/orgs/${org.id}/members`, {
          method: "POST",
          body: { email: "teacher@test.edu", role: "teacher" },
        }),
        { params: Promise.resolve({ id: org.id }) }
      );

      const { status, body } = await parseResponse(res);
      expect(status).toBe(201);
      expect(body).toHaveProperty("role", "teacher");
    });

    it("non-admin cannot add members", async () => {
      const org = await createTestOrg();
      await createTestOrgMembership(org.id, user.id, { role: "teacher" });

      setMockUser({ id: user.id, name: user.name, email: user.email });

      const res = await ADD_MEMBER(
        createRequest(`/api/orgs/${org.id}/members`, {
          method: "POST",
          body: { email: "someone@test.edu", role: "teacher" },
        }),
        { params: Promise.resolve({ id: org.id }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(403);
    });

    it("lists org members", async () => {
      const org = await createTestOrg();
      await createTestOrgMembership(org.id, user.id, { role: "org_admin" });

      setMockUser({ id: user.id, name: user.name, email: user.email });

      const res = await LIST_MEMBERS(
        createRequest(`/api/orgs/${org.id}/members`),
        { params: Promise.resolve({ id: org.id }) }
      );

      const { status, body } = await parseResponse<any[]>(res);
      expect(status).toBe(200);
      expect(body).toHaveLength(1);
    });
  });
});

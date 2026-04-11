import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, createTestOrg } from "../helpers";
import { setMockUser, createRequest, parseResponse } from "../api-helpers";
import { GET } from "@/app/api/admin/orgs/route";
import { PATCH } from "@/app/api/admin/orgs/[id]/route";

describe("Platform Admin API", () => {
  let admin: Awaited<ReturnType<typeof createTestUser>>;
  let regularUser: Awaited<ReturnType<typeof createTestUser>>;

  beforeEach(async () => {
    admin = await createTestUser({ name: "Admin", email: "admin@bridge.app", isPlatformAdmin: true });
    regularUser = await createTestUser({ name: "Regular", email: "user@test.edu" });
  });

  describe("GET /api/admin/orgs", () => {
    it("platform admin lists pending orgs", async () => {
      setMockUser({ id: admin.id, name: admin.name, email: admin.email, isPlatformAdmin: true });
      await createTestOrg({ status: "pending" });
      await createTestOrg({ status: "active" });

      const req = createRequest("/api/admin/orgs", {
        searchParams: { status: "pending" },
      });

      const { status, body } = await parseResponse<any[]>(await GET(req));
      expect(status).toBe(200);
      expect(body).toHaveLength(1);
    });

    it("rejects non-admin", async () => {
      setMockUser({ id: regularUser.id, name: regularUser.name, email: regularUser.email });

      const req = createRequest("/api/admin/orgs");
      const { status } = await parseResponse(await GET(req));
      expect(status).toBe(403);
    });
  });

  describe("PATCH /api/admin/orgs/[id]", () => {
    it("approves a pending org", async () => {
      setMockUser({ id: admin.id, name: admin.name, email: admin.email, isPlatformAdmin: true });
      const org = await createTestOrg({ status: "pending" });

      const res = await PATCH(
        createRequest(`/api/admin/orgs/${org.id}`, {
          method: "PATCH",
          body: { status: "active" },
        }),
        { params: Promise.resolve({ id: org.id }) }
      );

      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("status", "active");
      expect(body).toHaveProperty("verifiedAt");
    });

    it("suspends an org", async () => {
      setMockUser({ id: admin.id, name: admin.name, email: admin.email, isPlatformAdmin: true });
      const org = await createTestOrg({ status: "active" });

      const res = await PATCH(
        createRequest(`/api/admin/orgs/${org.id}`, {
          method: "PATCH",
          body: { status: "suspended" },
        }),
        { params: Promise.resolve({ id: org.id }) }
      );

      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("status", "suspended");
    });

    it("rejects non-admin", async () => {
      setMockUser({ id: regularUser.id, name: regularUser.name, email: regularUser.email });
      const org = await createTestOrg({ status: "pending" });

      const res = await PATCH(
        createRequest(`/api/admin/orgs/${org.id}`, {
          method: "PATCH",
          body: { status: "active" },
        }),
        { params: Promise.resolve({ id: org.id }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(403);
    });
  });
});

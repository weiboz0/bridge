import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, createTestOrg, createTestOrgMembership } from "../helpers";
import { setMockUser, createRequest, parseResponse } from "../api-helpers";
import { PATCH, DELETE } from "@/app/api/orgs/[id]/members/[memberId]/route";
import { addOrgMember } from "@/lib/org-memberships";

describe("Org Member Management API", () => {
  let admin: Awaited<ReturnType<typeof createTestUser>>;
  let member: Awaited<ReturnType<typeof createTestUser>>;
  let nonAdmin: Awaited<ReturnType<typeof createTestUser>>;
  let org: Awaited<ReturnType<typeof createTestOrg>>;

  beforeEach(async () => {
    org = await createTestOrg();
    admin = await createTestUser({ name: "Admin", email: "admin@test.edu" });
    member = await createTestUser({ name: "Member", email: "member@test.edu" });
    nonAdmin = await createTestUser({ name: "NonAdmin", email: "nonadmin@test.edu" });
    await createTestOrgMembership(org.id, admin.id, { role: "org_admin" });
  });

  describe("PATCH /api/orgs/[id]/members/[memberId]", () => {
    it("org admin updates member status", async () => {
      setMockUser({ id: admin.id, name: admin.name, email: admin.email });
      const membership = await addOrgMember(testDb, { orgId: org.id, userId: member.id, role: "teacher" });

      const res = await PATCH(
        createRequest(`/api/orgs/${org.id}/members/${membership!.id}`, {
          method: "PATCH",
          body: { status: "suspended" },
        }),
        { params: Promise.resolve({ id: org.id, memberId: membership!.id }) }
      );

      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect(body).toHaveProperty("status", "suspended");
    });

    it("non-admin cannot update member", async () => {
      setMockUser({ id: nonAdmin.id, name: nonAdmin.name, email: nonAdmin.email });
      const membership = await addOrgMember(testDb, { orgId: org.id, userId: member.id, role: "teacher" });

      const res = await PATCH(
        createRequest(`/api/orgs/${org.id}/members/${membership!.id}`, {
          method: "PATCH",
          body: { status: "suspended" },
        }),
        { params: Promise.resolve({ id: org.id, memberId: membership!.id }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(403);
    });

    it("rejects invalid status", async () => {
      setMockUser({ id: admin.id, name: admin.name, email: admin.email });
      const membership = await addOrgMember(testDb, { orgId: org.id, userId: member.id, role: "teacher" });

      const res = await PATCH(
        createRequest(`/api/orgs/${org.id}/members/${membership!.id}`, {
          method: "PATCH",
          body: { status: "invalid" },
        }),
        { params: Promise.resolve({ id: org.id, memberId: membership!.id }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(400);
    });

    it("returns 404 for membership from different org", async () => {
      setMockUser({ id: admin.id, name: admin.name, email: admin.email });
      const otherOrg = await createTestOrg();
      const otherMembership = await addOrgMember(testDb, { orgId: otherOrg.id, userId: member.id, role: "teacher" });

      const res = await PATCH(
        createRequest(`/api/orgs/${org.id}/members/${otherMembership!.id}`, {
          method: "PATCH",
          body: { status: "suspended" },
        }),
        { params: Promise.resolve({ id: org.id, memberId: otherMembership!.id }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(404);
    });
  });

  describe("DELETE /api/orgs/[id]/members/[memberId]", () => {
    it("org admin removes member", async () => {
      setMockUser({ id: admin.id, name: admin.name, email: admin.email });
      const membership = await addOrgMember(testDb, { orgId: org.id, userId: member.id, role: "teacher" });

      const res = await DELETE(
        createRequest(`/api/orgs/${org.id}/members/${membership!.id}`, { method: "DELETE" }),
        { params: Promise.resolve({ id: org.id, memberId: membership!.id }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(200);
    });

    it("non-admin cannot remove member", async () => {
      setMockUser({ id: nonAdmin.id, name: nonAdmin.name, email: nonAdmin.email });
      const membership = await addOrgMember(testDb, { orgId: org.id, userId: member.id, role: "teacher" });

      const res = await DELETE(
        createRequest(`/api/orgs/${org.id}/members/${membership!.id}`, { method: "DELETE" }),
        { params: Promise.resolve({ id: org.id, memberId: membership!.id }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(403);
    });
  });
});

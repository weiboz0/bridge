import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, createTestOrg, createTestOrgMembership } from "../helpers";
import {
  addOrgMember,
  listOrgMembers,
  getUserMemberships,
  getUserRoleInOrg,
  updateMemberStatus,
  removeOrgMember,
} from "@/lib/org-memberships";

describe("org membership operations", () => {
  let org: Awaited<ReturnType<typeof createTestOrg>>;
  let user: Awaited<ReturnType<typeof createTestUser>>;

  beforeEach(async () => {
    org = await createTestOrg();
    user = await createTestUser({ email: "member@test.edu" });
  });

  it("adds a member to an org", async () => {
    const membership = await addOrgMember(testDb, {
      orgId: org.id,
      userId: user.id,
      role: "teacher",
    });
    expect(membership).toBeDefined();
    expect(membership!.role).toBe("teacher");
    expect(membership!.status).toBe("active");
  });

  it("does not duplicate membership for same org/user/role", async () => {
    await addOrgMember(testDb, { orgId: org.id, userId: user.id, role: "teacher" });
    const dup = await addOrgMember(testDb, { orgId: org.id, userId: user.id, role: "teacher" });
    expect(dup).toBeUndefined();
  });

  it("allows same user with different roles in same org", async () => {
    await addOrgMember(testDb, { orgId: org.id, userId: user.id, role: "teacher" });
    const parentMembership = await addOrgMember(testDb, { orgId: org.id, userId: user.id, role: "parent" });
    expect(parentMembership).toBeDefined();
    expect(parentMembership!.role).toBe("parent");
  });

  it("lists org members with user info", async () => {
    await addOrgMember(testDb, { orgId: org.id, userId: user.id, role: "teacher" });
    const members = await listOrgMembers(testDb, org.id);
    expect(members).toHaveLength(1);
    expect(members[0].name).toBe(user.name);
    expect(members[0].email).toBe("member@test.edu");
  });

  it("gets user memberships across orgs", async () => {
    const org2 = await createTestOrg();
    await addOrgMember(testDb, { orgId: org.id, userId: user.id, role: "teacher" });
    await addOrgMember(testDb, { orgId: org2.id, userId: user.id, role: "student" });

    const memberships = await getUserMemberships(testDb, user.id);
    expect(memberships).toHaveLength(2);
  });

  it("gets user role in specific org", async () => {
    await addOrgMember(testDb, { orgId: org.id, userId: user.id, role: "teacher" });
    const roles = await getUserRoleInOrg(testDb, org.id, user.id);
    expect(roles).toHaveLength(1);
    expect(roles[0].role).toBe("teacher");
  });

  it("returns empty for user not in org", async () => {
    const roles = await getUserRoleInOrg(testDb, org.id, user.id);
    expect(roles).toHaveLength(0);
  });

  it("updates member status", async () => {
    const membership = await addOrgMember(testDb, { orgId: org.id, userId: user.id, role: "teacher" });
    const updated = await updateMemberStatus(testDb, membership!.id, "suspended");
    expect(updated!.status).toBe("suspended");
  });

  it("removes a member", async () => {
    const membership = await addOrgMember(testDb, { orgId: org.id, userId: user.id, role: "teacher" });
    const removed = await removeOrgMember(testDb, membership!.id);
    expect(removed).not.toBeNull();

    const remaining = await listOrgMembers(testDb, org.id);
    expect(remaining).toHaveLength(0);
  });
});

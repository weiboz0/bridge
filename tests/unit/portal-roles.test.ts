import { describe, it, expect } from "vitest";
import {
  buildUserRoles,
  getPrimaryRole,
  getPrimaryPortalPath,
  isAuthorizedForPortal,
  getPortalPath,
} from "@/lib/portal/roles";

describe("buildUserRoles", () => {
  it("includes admin role when isPlatformAdmin", () => {
    const roles = buildUserRoles(true, []);
    expect(roles).toHaveLength(1);
    expect(roles[0].role).toBe("admin");
  });

  it("builds roles from active memberships in active orgs", () => {
    const roles = buildUserRoles(false, [
      { role: "teacher", status: "active", orgId: "org1", orgName: "School A", orgStatus: "active" },
      { role: "student", status: "active", orgId: "org2", orgName: "School B", orgStatus: "active" },
    ]);
    expect(roles).toHaveLength(2);
    expect(roles[0].role).toBe("teacher");
    expect(roles[1].role).toBe("student");
  });

  it("filters out pending memberships", () => {
    const roles = buildUserRoles(false, [
      { role: "teacher", status: "pending", orgId: "org1", orgName: "School A", orgStatus: "active" },
    ]);
    expect(roles).toHaveLength(0);
  });

  it("filters out suspended org memberships", () => {
    const roles = buildUserRoles(false, [
      { role: "teacher", status: "active", orgId: "org1", orgName: "School A", orgStatus: "suspended" },
    ]);
    expect(roles).toHaveLength(0);
  });

  it("deduplicates same role in same org", () => {
    const roles = buildUserRoles(false, [
      { role: "teacher", status: "active", orgId: "org1", orgName: "School A", orgStatus: "active" },
      { role: "teacher", status: "active", orgId: "org1", orgName: "School A", orgStatus: "active" },
    ]);
    expect(roles).toHaveLength(1);
  });

  it("allows same role in different orgs", () => {
    const roles = buildUserRoles(false, [
      { role: "teacher", status: "active", orgId: "org1", orgName: "School A", orgStatus: "active" },
      { role: "teacher", status: "active", orgId: "org2", orgName: "School B", orgStatus: "active" },
    ]);
    expect(roles).toHaveLength(2);
  });
});

describe("getPrimaryRole", () => {
  it("returns null for empty roles", () => {
    expect(getPrimaryRole([])).toBeNull();
  });

  it("prioritizes admin over teacher", () => {
    const primary = getPrimaryRole([
      { role: "teacher", orgId: "org1" },
      { role: "admin" },
    ]);
    expect(primary!.role).toBe("admin");
  });

  it("prioritizes teacher over student", () => {
    const primary = getPrimaryRole([
      { role: "student", orgId: "org1" },
      { role: "teacher", orgId: "org1" },
    ]);
    expect(primary!.role).toBe("teacher");
  });
});

describe("getPrimaryPortalPath", () => {
  it("returns /onboarding for no roles", () => {
    expect(getPrimaryPortalPath([])).toBe("/onboarding");
  });

  it("returns /admin for admin", () => {
    expect(getPrimaryPortalPath([{ role: "admin" }])).toBe("/admin");
  });

  it("returns /teacher for teacher", () => {
    expect(getPrimaryPortalPath([{ role: "teacher", orgId: "org1" }])).toBe("/teacher");
  });
});

describe("isAuthorizedForPortal", () => {
  it("returns true when user has the role", () => {
    expect(isAuthorizedForPortal([{ role: "teacher", orgId: "org1" }], "teacher")).toBe(true);
  });

  it("returns false when user lacks the role", () => {
    expect(isAuthorizedForPortal([{ role: "student", orgId: "org1" }], "teacher")).toBe(false);
  });
});

describe("getPortalPath", () => {
  it("returns correct paths", () => {
    expect(getPortalPath("admin")).toBe("/admin");
    expect(getPortalPath("org_admin")).toBe("/org");
    expect(getPortalPath("teacher")).toBe("/teacher");
    expect(getPortalPath("student")).toBe("/student");
    expect(getPortalPath("parent")).toBe("/parent");
  });
});

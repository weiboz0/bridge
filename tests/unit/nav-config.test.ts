import { describe, it, expect } from "vitest";
import { portalConfigs, getPortalConfig } from "@/lib/portal/nav-config";

describe("nav-config", () => {
  it("has configs for all 5 portals", () => {
    expect(Object.keys(portalConfigs)).toHaveLength(5);
    expect(portalConfigs.admin).toBeDefined();
    expect(portalConfigs.org_admin).toBeDefined();
    expect(portalConfigs.teacher).toBeDefined();
    expect(portalConfigs.student).toBeDefined();
    expect(portalConfigs.parent).toBeDefined();
  });

  it("every nav item has a valid href starting with /", () => {
    for (const config of Object.values(portalConfigs)) {
      for (const item of config.navItems) {
        expect(item.href).toMatch(/^\//);
      }
    }
  });

  it("every nav item has a non-empty icon", () => {
    for (const config of Object.values(portalConfigs)) {
      for (const item of config.navItems) {
        expect(item.icon.length).toBeGreaterThan(0);
      }
    }
  });

  it("every nav item has a non-empty label", () => {
    for (const config of Object.values(portalConfigs)) {
      for (const item of config.navItems) {
        expect(item.label.length).toBeGreaterThan(0);
      }
    }
  });

  it("getPortalConfig returns config for valid role", () => {
    const config = getPortalConfig("teacher");
    expect(config).not.toBeNull();
    expect(config!.role).toBe("teacher");
  });

  it("getPortalConfig returns null for invalid role", () => {
    expect(getPortalConfig("invalid")).toBeNull();
  });

  // Review-002 P1 #5: org-admin nav linked into /teacher/* which redirected
  // back to /org for org admins without the teacher role. Lock the rule:
  // each portal's nav stays inside its own basePath. The single intentional
  // cross-portal link is org_admin → /teacher/* (allowed if a user holds
  // both roles), which we now want to forbid by default and only re-add
  // through an explicit allow-list when org-scoped views ship.
  it("no nav item points outside its own portal's basePath", () => {
    // Other-portal prefixes that any given config is forbidden from linking to.
    const allBasePaths: Record<string, string[]> = {
      admin: ["/org", "/teacher", "/student", "/parent"],
      org_admin: ["/admin", "/teacher", "/student", "/parent"],
      teacher: ["/admin", "/org", "/student", "/parent"],
      student: ["/admin", "/org", "/teacher", "/parent"],
      parent: ["/admin", "/org", "/teacher", "/student"],
    };
    for (const [role, config] of Object.entries(portalConfigs)) {
      const forbidden = allBasePaths[role] ?? [];
      for (const item of config.navItems) {
        for (const pfx of forbidden) {
          expect(
            item.href.startsWith(pfx + "/") || item.href === pfx,
            `${role} nav item "${item.label}" → ${item.href} crosses into ${pfx}`
          ).toBe(false);
        }
      }
    }
  });
});

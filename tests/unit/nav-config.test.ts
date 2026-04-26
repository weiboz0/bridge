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
});

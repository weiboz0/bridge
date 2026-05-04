import { describe, it, expect } from "vitest";
import { findActiveIndex } from "../../src/lib/portal/active-match";

describe("findActiveIndex (longest-match wins)", () => {
  const TEACHER_ITEMS = [
    { href: "/teacher" },
    { href: "/teacher/units" },
    { href: "/teacher/problems" },
  ];

  it("highlights the deepest match — /teacher/units picks Units, not Dashboard", () => {
    // Codex review of plan 067 phases 2+3 caught this: the naive
    // pathname.startsWith(itemPath + "/") check would match BOTH
    // "/teacher" and "/teacher/units" on /teacher/units. Longest
    // match wins.
    expect(findActiveIndex("/teacher/units", TEACHER_ITEMS)).toBe(1);
  });

  it("nested route under a sub-item picks the sub-item — /teacher/units/abc → Units", () => {
    expect(findActiveIndex("/teacher/units/abc", TEACHER_ITEMS)).toBe(1);
  });

  it("exact match on the dashboard href picks Dashboard", () => {
    expect(findActiveIndex("/teacher", TEACHER_ITEMS)).toBe(0);
  });

  it("returns -1 when no item matches", () => {
    expect(findActiveIndex("/parent/children", TEACHER_ITEMS)).toBe(-1);
  });

  it("strips query strings from item hrefs before matching", () => {
    // Org-scoped items carry ?orgId=... appended by SidebarSection.
    const orgItems = [
      { href: "/org?orgId=org-a" },
      { href: "/org/teachers?orgId=org-a" },
    ];
    expect(findActiveIndex("/org/teachers", orgItems)).toBe(1);
    expect(findActiveIndex("/org", orgItems)).toBe(0);
  });

  it("is robust against a partial-segment match (no `/path` matching `/pathological`)", () => {
    const items = [{ href: "/admin" }, { href: "/admins" }];
    // Pathname /admins should match the second item, not the first;
    // the prefix-with-slash boundary prevents the false match.
    expect(findActiveIndex("/admins", items)).toBe(1);
    expect(findActiveIndex("/admin", items)).toBe(0);
  });
});

// @vitest-environment jsdom
//
// Plan 089 phase 2 — sidebar href-dedupe tests (Decision #7).
//
// 4 cases:
//   1. Single-role: all nav items render, no dedupe.
//   2. Multi-role: second occurrence of a shared href is suppressed
//      (only one <a href="/library"> in the desktop sidebar when both
//      sections are expanded).
//   3. Dedupe key ignores `?orgId=` suffix — raw nav-config href is the key.
//   4. Empty-after-dedupe section still renders its header but not the nav list.
//
// Queries are scoped to the <aside> (desktop sidebar) to avoid false
// duplicates from the mobile bottom-nav, which renders independently.
//
// Section-expansion behaviour: `useSidebarSections` only auto-expands the
// active section (computed from the pathname). Tests that need multiple
// sections expanded seed localStorage before render.

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, within } from "@testing-library/react";

vi.mock("next/navigation", () => ({
  usePathname: () => "/teacher",
  useSearchParams: () => new URLSearchParams(),
}));

// nav-config returns controlled navItems — mutable per test via `configs`.
vi.mock("@/lib/portal/nav-config", () => ({
  getPortalConfig: (role: string) => configs[role] ?? null,
}));

// Sidebar hook — never collapsed so all labels render.
vi.mock("@/lib/hooks/use-sidebar", () => ({
  useSidebar: () => ({ collapsed: false, toggle: () => {} }),
}));

import { Sidebar } from "@/components/portal/sidebar";
import { SIDEBAR_EXPANDED_STORAGE_KEY, sectionKey } from "@/lib/hooks/use-sidebar-sections";
import type { UserRole } from "@/lib/portal/types";

// Mutable config map overridden per test.
const configs: Record<string, {
  role: string;
  label: string;
  basePath: string;
  navItems: { label: string; href: string; icon: string }[];
}> = {};

function adminConfig(navItems?: { label: string; href: string; icon: string }[]) {
  return {
    role: "admin",
    label: "Platform Admin",
    basePath: "/admin",
    navItems: navItems ?? [
      { label: "Organizations", href: "/admin/orgs", icon: "building-2" },
      { label: "Users", href: "/admin/users", icon: "users" },
      { label: "Library", href: "/library", icon: "library" },
    ],
  };
}

function teacherConfig(navItems?: { label: string; href: string; icon: string }[]) {
  return {
    role: "teacher",
    label: "Teacher",
    basePath: "/teacher",
    navItems: navItems ?? [
      { label: "Dashboard", href: "/teacher", icon: "layout-dashboard" },
      { label: "Library", href: "/library", icon: "library" },
      { label: "Problems", href: "/teacher/problems", icon: "puzzle" },
    ],
  };
}

function orgAdminConfig(navItems?: { label: string; href: string; icon: string }[]) {
  return {
    role: "org_admin",
    label: "Organization",
    basePath: "/org",
    navItems: navItems ?? [
      { label: "Dashboard", href: "/org", icon: "layout-dashboard" },
      { label: "Library", href: "/library", icon: "library" },
      { label: "Settings", href: "/org/settings", icon: "settings" },
    ],
  };
}

// Seed localStorage so the given section keys are expanded.
function expandSections(...keys: string[]) {
  const overrides: Record<string, boolean> = {};
  for (const k of keys) overrides[k] = true;
  localStorage.setItem(SIDEBAR_EXPANDED_STORAGE_KEY, JSON.stringify(overrides));
}

// Helper: get the desktop <aside> element.
function getAside(): HTMLElement {
  const aside = document.querySelector("aside");
  if (!aside) throw new Error("Desktop sidebar <aside> not found");
  return aside as HTMLElement;
}

beforeEach(() => {
  localStorage.clear();
  Object.keys(configs).forEach((k) => delete configs[k]);
  configs.admin = adminConfig();
  configs.teacher = teacherConfig();
  configs.org_admin = orgAdminConfig();
});

// ── Case 1: single-role unchanged ─────────────────────────────────────────

describe("Case 1 — single-role: all nav items render without dedupe", () => {
  it("teacher single-role: all teacher nav items present in desktop sidebar", () => {
    const roles: UserRole[] = [{ role: "teacher" }];
    render(<Sidebar userName="Test" roles={roles} currentRole="teacher" />);

    const aside = getAside();
    const hrefs = within(aside).getAllByRole("link").map((l) => l.getAttribute("href"));

    // Single-role: no header/expand mechanic, all items render directly.
    expect(hrefs).toContain("/teacher");
    expect(hrefs).toContain("/library");
    expect(hrefs).toContain("/teacher/problems");
  });

  it("admin single-role: all admin nav items present in desktop sidebar", () => {
    const roles: UserRole[] = [{ role: "admin" }];
    render(<Sidebar userName="Test" roles={roles} currentRole="admin" />);

    const aside = getAside();
    const hrefs = within(aside).getAllByRole("link").map((l) => l.getAttribute("href"));

    expect(hrefs).toContain("/admin/orgs");
    expect(hrefs).toContain("/admin/users");
    expect(hrefs).toContain("/library");
  });
});

// ── Case 2: multi-role dedupes by href ────────────────────────────────────

describe("Case 2 — multi-role: second /library occurrence is suppressed", () => {
  it("admin + teacher: only one /library link across both expanded sections", () => {
    // admin renders first in roles order → claims /library.
    // teacher's /library is removed by dedupe.
    // Seed both sections as expanded so both render their items.
    expandSections(sectionKey("admin"), sectionKey("teacher"));

    const roles: UserRole[] = [{ role: "admin" }, { role: "teacher" }];
    render(<Sidebar userName="Test" roles={roles} currentRole="admin" />);

    const aside = getAside();
    const libraryLinks = aside.querySelectorAll('a[href="/library"]');
    expect(libraryLinks).toHaveLength(1);
  });

  it("admin + teacher: both section headers visible; role-unique items all present", () => {
    expandSections(sectionKey("admin"), sectionKey("teacher"));

    const roles: UserRole[] = [{ role: "admin" }, { role: "teacher" }];
    render(<Sidebar userName="Test" roles={roles} currentRole="admin" />);

    expect(screen.getByRole("button", { name: /Platform Admin/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Teacher/ })).toBeInTheDocument();

    const aside = getAside();
    const hrefs = within(aside).getAllByRole("link").map((l) => l.getAttribute("href"));

    // Admin's unique items.
    expect(hrefs).toContain("/admin/orgs");
    expect(hrefs).toContain("/admin/users");

    // Teacher's unique items (Library is deduped; Dashboard + Problems survive).
    expect(hrefs).toContain("/teacher");
    expect(hrefs).toContain("/teacher/problems");

    // Exactly one /library link.
    const libraryCount = hrefs.filter((h) => h === "/library").length;
    expect(libraryCount).toBe(1);
  });

  it("teacher + admin order: teacher claims Library; admin's is suppressed", () => {
    // teacher first → teacher claims /library; admin loses it.
    expandSections(sectionKey("teacher"), sectionKey("admin"));

    const roles: UserRole[] = [{ role: "teacher" }, { role: "admin" }];
    render(<Sidebar userName="Test" roles={roles} currentRole="teacher" />);

    const aside = getAside();
    const libraryLinks = aside.querySelectorAll('a[href="/library"]');
    expect(libraryLinks).toHaveLength(1);

    // The single Library link lives inside the first expanded nav (teacher's).
    const navs = aside.querySelectorAll("nav");
    expect(navs.length).toBeGreaterThan(0);
    const firstNav = navs[0];
    expect(firstNav.querySelector('a[href="/library"]')).not.toBeNull();
  });
});

// ── Case 3: dedupe key ignores ?orgId= suffix ─────────────────────────────

describe("Case 3 — dedupe key is raw nav-config href, ignores ?orgId= suffix", () => {
  it("org_admin + teacher: raw /library claimed by org_admin; teacher's suppressed even though org_admin's link renders as /library?orgId=org-x", () => {
    // org_admin section expanded (seeds localStorage with its key).
    expandSections(sectionKey("org_admin", "org-x"), sectionKey("teacher"));

    const roles: UserRole[] = [
      { role: "org_admin", orgId: "org-x", orgName: "Test Org" },
      { role: "teacher" },
    ];
    render(<Sidebar userName="Test" roles={roles} currentRole="org_admin" />);

    const aside = getAside();

    // org_admin's Library link has ?orgId=org-x appended by SidebarSection.
    const orgLibraryLink = aside.querySelector('a[href="/library?orgId=org-x"]');
    expect(orgLibraryLink).not.toBeNull();

    // Teacher's raw /library link is absent — raw href "/library" was already seen.
    const rawLibraryLink = aside.querySelector('a[href="/library"]');
    expect(rawLibraryLink).toBeNull();

    // Total: exactly one /library-prefixed link.
    const allLibraryLinks = aside.querySelectorAll('a[href^="/library"]');
    expect(allLibraryLinks).toHaveLength(1);
  });
});

// ── Case 4: empty-after-dedupe section renders header without nav list ─────

describe("Case 4 — empty-after-dedupe section: header visible, no nav list", () => {
  it("section fully deduped away still shows its collapsible header, no nav", () => {
    // Teacher config has only /library — deduped by admin which renders first.
    configs.teacher = teacherConfig([
      { label: "Library", href: "/library", icon: "library" },
    ]);

    expandSections(sectionKey("admin"), sectionKey("teacher"));

    const roles: UserRole[] = [{ role: "admin" }, { role: "teacher" }];
    render(<Sidebar userName="Test" roles={roles} currentRole="admin" />);

    // Both section headers render.
    const adminHeader = screen.getByRole("button", { name: /Platform Admin/ });
    const teacherHeader = screen.getByRole("button", { name: /Teacher/ });
    expect(adminHeader).toBeInTheDocument();
    expect(teacherHeader).toBeInTheDocument();

    const aside = getAside();

    // Only one Library link (admin's; teacher's was deduped).
    const libraryLinks = aside.querySelectorAll('a[href="/library"]');
    expect(libraryLinks).toHaveLength(1);

    // The Teacher section's container has no <nav> inside (empty items list
    // causes SidebarSection to render the header but skip the <nav>).
    const teacherSection = teacherHeader.closest("div");
    expect(teacherSection).not.toBeNull();
    expect(teacherSection!.querySelector("nav")).toBeNull();
  });
});

// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

vi.mock("next/navigation", () => ({
  usePathname: () => "/teacher",
  useSearchParams: () => new URLSearchParams(),
}));

import { SidebarSection } from "@/components/portal/sidebar-section";
import { Sidebar } from "@/components/portal/sidebar";
import type { UserRole, NavItem } from "@/lib/portal/types";
import { sectionKey, SIDEBAR_EXPANDED_STORAGE_KEY } from "@/lib/hooks/use-sidebar-sections";

const TEACHER_ITEMS: NavItem[] = [
  { label: "Dashboard", href: "/teacher", icon: "layout-dashboard" },
  { label: "Problems", href: "/teacher/problems", icon: "puzzle" },
];

beforeEach(() => {
  localStorage.clear();
});

describe("SidebarSection (plan 067)", () => {
  it("single-role mode hides the header (preserves pre-067 look)", () => {
    const { container } = render(
      <SidebarSection
        role={{ role: "teacher" }}
        label="Teacher"
        navItems={TEACHER_ITEMS}
        collapsed={false}
        expanded={true}
        multiRole={false}
        onToggle={() => {}}
      />
    );
    // No section header button; just the nav items.
    expect(container.querySelector("button[aria-expanded]")).toBeNull();
    expect(screen.getByText("Dashboard")).toBeInTheDocument();
    expect(screen.getByText("Problems")).toBeInTheDocument();
  });

  it("multi-role mode renders a header and toggles on click", () => {
    const onToggle = vi.fn();
    render(
      <SidebarSection
        role={{ role: "teacher" }}
        label="Teacher"
        navItems={TEACHER_ITEMS}
        collapsed={false}
        expanded={true}
        multiRole={true}
        onToggle={onToggle}
      />
    );
    const header = screen.getByRole("button", { expanded: true });
    expect(header).toBeInTheDocument();
    expect(header.textContent).toContain("Teacher");
    fireEvent.click(header);
    expect(onToggle).toHaveBeenCalledTimes(1);
  });

  it("collapsed (icon-only) section: items render without text labels", () => {
    render(
      <SidebarSection
        role={{ role: "teacher" }}
        label="Teacher"
        navItems={TEACHER_ITEMS}
        collapsed={true}
        expanded={true}
        multiRole={true}
        onToggle={() => {}}
      />
    );
    // Item label text should NOT render visibly when collapsed.
    expect(screen.queryByText("Dashboard")).toBeNull();
    expect(screen.queryByText("Problems")).toBeNull();
    // But the title attribute should carry the label for tooltip access.
    const links = document.querySelectorAll("a[title]");
    expect(links.length).toBeGreaterThanOrEqual(2);
  });

  it("collapsed: section is not shown when expanded={false} (multiRole)", () => {
    render(
      <SidebarSection
        role={{ role: "teacher" }}
        label="Teacher"
        navItems={TEACHER_ITEMS}
        collapsed={false}
        expanded={false}
        multiRole={true}
        onToggle={() => {}}
      />
    );
    expect(screen.queryByText("Dashboard")).toBeNull();
    expect(screen.queryByText("Problems")).toBeNull();
  });

  it("org-scoped role appends ?orgId= to every link href", () => {
    const ORG_ITEMS: NavItem[] = [
      { label: "Dashboard", href: "/org", icon: "layout-dashboard" },
      { label: "Teachers", href: "/org/teachers", icon: "graduation-cap" },
    ];
    render(
      <SidebarSection
        role={{ role: "org_admin", orgId: "org-a", orgName: "Org A" }}
        label="Organization"
        navItems={ORG_ITEMS}
        collapsed={false}
        expanded={true}
        multiRole={true}
        onToggle={() => {}}
      />
    );
    const dashLink = screen.getByText("Dashboard").closest("a");
    const teachLink = screen.getByText("Teachers").closest("a");
    expect(dashLink?.getAttribute("href")).toBe("/org?orgId=org-a");
    expect(teachLink?.getAttribute("href")).toBe("/org/teachers?orgId=org-a");
  });

  it("multi-role header includes the orgName for disambiguation", () => {
    render(
      <SidebarSection
        role={{ role: "org_admin", orgId: "org-a", orgName: "Bridge HQ" }}
        label="Organization"
        navItems={[]}
        collapsed={false}
        expanded={true}
        multiRole={true}
        onToggle={() => {}}
      />
    );
    expect(screen.getByText(/Bridge HQ/)).toBeInTheDocument();
  });
});

describe("Sidebar (plan 067) — multi-section integration", () => {
  it("renders one section per role for a multi-role user", () => {
    const roles: UserRole[] = [
      { role: "teacher" },
      { role: "admin" },
    ];
    render(<Sidebar userName="Test" roles={roles} currentRole="teacher" />);
    // Both section headers visible.
    expect(screen.getByText(/Teacher/)).toBeInTheDocument();
    expect(screen.getByText(/Platform Admin/)).toBeInTheDocument();
  });

  it("two org_admin sections at different orgs both render with disambiguating org names", () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const roles: UserRole[] = [
      { role: "org_admin", orgId: "org-a", orgName: "Org A" },
      { role: "org_admin", orgId: "org-b", orgName: "Org B" },
    ];
    render(<Sidebar userName="Test" roles={roles} currentRole="org_admin" />);
    expect(screen.getByText(/Org A/)).toBeInTheDocument();
    expect(screen.getByText(/Org B/)).toBeInTheDocument();
    // No duplicate-key React warnings (composite key (role, orgId) prevents this).
    const dupKey = errorSpy.mock.calls.find((c) =>
      String(c[0]).includes("Encountered two children with the same key")
    );
    expect(dupKey).toBeUndefined();
    errorSpy.mockRestore();
  });
});

describe("sectionKey helper", () => {
  it("composite key includes orgId for org-scoped roles", () => {
    expect(sectionKey("org_admin", "org-a")).toBe("org_admin:org-a");
    expect(sectionKey("teacher", undefined)).toBe("teacher:none");
    expect(sectionKey("teacher", null)).toBe("teacher:none");
  });
});

describe("localStorage persistence", () => {
  it("uses the canonical key", () => {
    expect(SIDEBAR_EXPANDED_STORAGE_KEY).toBe("bridge.sidebar.expanded");
  });

  it("toggling a section writes the override to localStorage and survives re-render", async () => {
    // Multi-role user, two sections; toggle the (non-active) admin
    // section and confirm the override persists.
    const roles: UserRole[] = [
      { role: "teacher" },
      { role: "admin" },
    ];
    const { unmount } = render(<Sidebar userName="Test" roles={roles} currentRole="teacher" />);

    const adminHeader = screen.getByRole("button", { name: /Platform Admin/ });
    expect(adminHeader.getAttribute("aria-expanded")).toBe("false");

    fireEvent.click(adminHeader);

    const stored = localStorage.getItem(SIDEBAR_EXPANDED_STORAGE_KEY);
    expect(stored).toBeTruthy();
    const parsed = JSON.parse(stored!) as Record<string, boolean>;
    expect(parsed[sectionKey("admin")]).toBe(true);

    // Re-mount and verify the override loads from localStorage on init.
    unmount();
    render(<Sidebar userName="Test" roles={roles} currentRole="teacher" />);
    const reMountedAdminHeader = screen.getByRole("button", { name: /Platform Admin/ });
    expect(reMountedAdminHeader.getAttribute("aria-expanded")).toBe("true");
  });
});

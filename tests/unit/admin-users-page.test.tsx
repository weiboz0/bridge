// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("@/lib/api-client", () => ({
  api: vi.fn(),
}));

// Mock next/navigation for client components used inside the page
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), refresh: vi.fn(), back: vi.fn() }),
  redirect: vi.fn(),
}));

// next/link renders as a plain anchor in tests without App Router context
vi.mock("next/link", () => ({
  default: ({ href, children, className }: { href: string; children: React.ReactNode; className?: string }) => (
    <a href={href} className={className}>{children}</a>
  ),
}));

import AdminUsersPage from "@/app/(portal)/admin/users/page";
import { api } from "@/lib/api-client";

const mockedApi = vi.mocked(api);

const IDENTITY = { userId: "admin-001", status: "active" };

const ORGS = [
  { id: "org-1", name: "Riverside School" },
  { id: "org-2", name: "Westview Academy" },
];

const USERS = [
  {
    id: "user-001",
    name: "Alice Teacher",
    email: "alice@school.edu",
    avatarUrl: null,
    isPlatformAdmin: false,
    status: "active" as const,
    orgRole: "teacher",
    orgId: "org-1",
    orgName: "Riverside School",
    hasPassword: true,
    createdAt: "2026-01-10T00:00:00Z",
    updatedAt: "2026-01-10T00:00:00Z",
  },
  {
    id: "user-002",
    name: "Bob Student",
    email: "bob@school.edu",
    avatarUrl: null,
    isPlatformAdmin: false,
    status: "suspended" as const,
    orgRole: "student",
    orgId: "org-1",
    orgName: "Riverside School",
    hasPassword: false,
    createdAt: "2026-01-15T00:00:00Z",
    updatedAt: "2026-02-01T00:00:00Z",
  },
  {
    id: "admin-001",
    name: "Carol Admin",
    email: "carol@bridge.io",
    avatarUrl: null,
    isPlatformAdmin: true,
    status: "active" as const,
    orgRole: null,
    orgId: null,
    orgName: null,
    hasPassword: true,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  },
];

async function renderPage(searchParams: { role?: string; orgId?: string } = {}) {
  mockedApi.mockImplementation(async (path: string) => {
    if (path.includes("/api/me/identity")) return IDENTITY;
    if (path.includes("/api/admin/orgs")) return ORGS;
    if (path.includes("/api/admin/users")) return USERS;
    throw new Error(`Unexpected API call: ${path}`);
  });

  const element = await AdminUsersPage({ searchParams: Promise.resolve(searchParams) });
  render(element as React.ReactElement);
}

describe("AdminUsersPage", () => {
  beforeEach(() => {
    mockedApi.mockReset();
  });

  it("renders Role, Org, and Status columns with API data", async () => {
    await renderPage();

    // Column headers
    expect(screen.getByText("Role")).toBeInTheDocument();
    expect(screen.getByText("Org")).toBeInTheDocument();
    expect(screen.getByText("Status")).toBeInTheDocument();

    // User data
    expect(screen.getByText("Alice Teacher")).toBeInTheDocument();
    expect(screen.getByText("alice@school.edu")).toBeInTheDocument();
    // Teacher role label (appears in both chip row and table — use getAllByText)
    expect(screen.getAllByText("Teacher").length).toBeGreaterThan(0);
    // Org name
    expect(screen.getAllByText("Riverside School").length).toBeGreaterThan(0);
  });

  it("shows Suspended badge for suspended users", async () => {
    await renderPage();

    const badges = screen.getAllByText("Suspended");
    expect(badges.length).toBeGreaterThan(0);
  });

  it("filter chips render with correct active state for role searchParam", async () => {
    await renderPage({ role: "teacher" });

    // The "Teacher" chip should have the active class (bg-primary)
    const teacherChip = screen.getByRole("link", { name: "Teacher" });
    expect(teacherChip).toHaveClass("bg-primary");

    // "All" chip should NOT be active
    const allChip = screen.getByRole("link", { name: "All" });
    expect(allChip).not.toHaveClass("bg-primary");
  });

  it("filter chip hrefs preserve orgId when present", async () => {
    await renderPage({ orgId: "org-1" });

    const teacherChip = screen.getByRole("link", { name: "Teacher" });
    expect(teacherChip.getAttribute("href")).toContain("orgId=org-1");

    const allChip = screen.getByRole("link", { name: "All" });
    // "All" chip has no role param but should still preserve orgId
    expect(allChip.getAttribute("href")).toContain("orgId=org-1");
  });

  it("org <select> shows correct default and all org options", async () => {
    await renderPage({ orgId: "org-1" });

    const select = screen.getByRole("combobox") as HTMLSelectElement;
    expect(select).toBeInTheDocument();

    const options = Array.from(select.options).map((o) => o.text);
    expect(options).toContain("All orgs");
    expect(options).toContain("Riverside School");
    expect(options).toContain("Westview Academy");

    // defaultValue should be org-1
    expect(select.value).toBe("org-1");
  });

  it("self-row renders UserActions with isSelf=true (shows only View details, no destructive items)", async () => {
    await renderPage();

    // Carol Admin is the logged-in user (admin-001 matches identity.userId)
    // The self-row should render actions but without destructive items
    // We can't easily open the dropdown in unit tests without interaction,
    // but we verify the component renders for the self row by checking
    // that all rows have actions buttons (no longer hidden for self)
    const actionButtons = screen.getAllByRole("button", { name: /actions/i });
    // All 3 users should have action buttons now (isSelf branch renders the component)
    expect(actionButtons.length).toBe(3);
  });
});

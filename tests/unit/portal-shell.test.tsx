// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";

// --- mocks ---------------------------------------------------------------

vi.mock("@/lib/api-client", () => ({ api: vi.fn() }));

// In Next.js, redirect() throws a special NEXT_REDIRECT error internally to
// halt rendering. Simulate that here so tests that assert redirect() was called
// don't need to worry about code running past the redirect call.
class RedirectError extends Error {
  constructor(public destination: string) {
    super(`NEXT_REDIRECT: ${destination}`);
    this.name = "RedirectError";
  }
}
vi.mock("next/navigation", () => ({
  redirect: vi.fn((url: string) => {
    throw new RedirectError(url);
  }),
}));

// Sidebar just needs to render something so we can assert children reached it.
vi.mock("@/components/portal/sidebar", () => ({
  Sidebar: ({ userName }: { userName: string }) => (
    <nav data-testid="sidebar">{userName}</nav>
  ),
}));

vi.mock("@/components/portal/identity-drift-banner", () => ({
  IdentityDriftBanner: () => null,
}));

// getPortalConfig must return something truthy for role-specific gates to pass.
vi.mock("@/lib/portal/nav-config", () => ({
  getPortalConfig: (role: string) =>
    role === "admin"
      ? { role: "admin", label: "Admin", basePath: "/admin", navItems: [] }
      : null,
}));

// -------------------------------------------------------------------------

import { PortalShell } from "@/components/portal/portal-shell";
import { ApiError } from "@/lib/api-error";
import { api } from "@/lib/api-client";
import { redirect } from "next/navigation";

const mockApi = vi.mocked(api);
const mockRedirect = vi.mocked(redirect);

const ADMIN_RESPONSE = {
  authorized: true,
  userName: "Admin User",
  roles: [{ role: "admin" }],
};

const TEACHER_RESPONSE = {
  authorized: true,
  userName: "Teacher User",
  roles: [{ role: "teacher", orgId: "org-1", orgName: "School A" }],
};

const NO_ROLES_RESPONSE = {
  authorized: true,
  userName: "No-Role User",
  roles: [],
};

beforeEach(() => {
  mockApi.mockReset();
  mockRedirect.mockReset();
});

// -------------------------------------------------------------------------
// Role-specific gate (pre-existing behaviour)
// -------------------------------------------------------------------------

describe("PortalShell — role-specific gate", () => {
  it("renders children and sidebar when user holds the required role", async () => {
    mockApi.mockResolvedValue(ADMIN_RESPONSE);
    const result = await PortalShell({
      portalRole: "admin",
      children: <p>admin content</p>,
    });
    render(result as React.ReactElement);
    expect(screen.getByTestId("sidebar")).toBeInTheDocument();
    expect(screen.getByText("admin content")).toBeInTheDocument();
  });

  it("redirects to / when user lacks the required role", async () => {
    mockApi.mockResolvedValue(TEACHER_RESPONSE);
    await expect(
      PortalShell({ portalRole: "admin", children: <p>admin content</p> })
    ).rejects.toThrow("NEXT_REDIRECT: /");
    expect(mockRedirect).toHaveBeenCalledWith("/");
  });

  it("redirects to /login on 401", async () => {
    mockApi.mockRejectedValue(new ApiError(401, "Unauthorized"));
    await expect(
      PortalShell({ portalRole: "admin", children: <p>content</p> })
    ).rejects.toThrow("NEXT_REDIRECT: /login");
    expect(mockRedirect).toHaveBeenCalledWith("/login");
  });

  it("redirects to /login when authorized=false", async () => {
    mockApi.mockResolvedValue({ authorized: false, userName: "", roles: [] });
    await expect(
      PortalShell({ portalRole: "admin", children: <p>content</p> })
    ).rejects.toThrow("NEXT_REDIRECT: /login");
    expect(mockRedirect).toHaveBeenCalledWith("/login");
  });
});

// -------------------------------------------------------------------------
// Null-role gate (plan 089 phase 1 — Decision #13)
// -------------------------------------------------------------------------

describe("PortalShell — null-role gate (role-neutral /library shell)", () => {
  it("renders children and sidebar when user has ≥1 role", async () => {
    mockApi.mockResolvedValue(ADMIN_RESPONSE);
    const result = await PortalShell({
      portalRole: null,
      children: <p>library content</p>,
    });
    render(result as React.ReactElement);
    expect(screen.getByTestId("sidebar")).toBeInTheDocument();
    expect(screen.getByText("library content")).toBeInTheDocument();
    expect(mockRedirect).not.toHaveBeenCalled();
  });

  it("renders for any role, not just admin", async () => {
    mockApi.mockResolvedValue(TEACHER_RESPONSE);
    const result = await PortalShell({
      portalRole: null,
      children: <p>teacher library</p>,
    });
    render(result as React.ReactElement);
    expect(screen.getByText("teacher library")).toBeInTheDocument();
    expect(mockRedirect).not.toHaveBeenCalled();
  });

  it("redirects to / when user has 0 roles", async () => {
    mockApi.mockResolvedValue(NO_ROLES_RESPONSE);
    await expect(
      PortalShell({ portalRole: null, children: <p>library content</p> })
    ).rejects.toThrow("NEXT_REDIRECT: /");
    expect(mockRedirect).toHaveBeenCalledWith("/");
  });

  it("redirects to /login when unauthenticated (401)", async () => {
    mockApi.mockRejectedValue(new ApiError(401, "Unauthorized"));
    await expect(
      PortalShell({ portalRole: null, children: <p>library content</p> })
    ).rejects.toThrow("NEXT_REDIRECT: /login");
    expect(mockRedirect).toHaveBeenCalledWith("/login");
  });

  it("redirects to /login when authorized=false", async () => {
    mockApi.mockResolvedValue({ authorized: false, userName: "", roles: [] });
    await expect(
      PortalShell({ portalRole: null, children: <p>library content</p> })
    ).rejects.toThrow("NEXT_REDIRECT: /login");
    expect(mockRedirect).toHaveBeenCalledWith("/login");
  });
});

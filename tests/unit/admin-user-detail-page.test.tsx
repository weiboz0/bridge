// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("@/lib/api-client", () => ({
  api: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  redirect: vi.fn(),
}));

// next/link renders as a plain anchor in tests without App Router context
vi.mock("next/link", () => ({
  default: ({ href, children, className }: { href: string; children: React.ReactNode; className?: string }) => (
    <a href={href} className={className}>{children}</a>
  ),
}));

import AdminUserDetailPage from "@/app/(portal)/admin/users/[id]/page";
import { api } from "@/lib/api-client";
import { ApiError } from "@/lib/api-error";

const VALID_UUID = "f47ac10b-58cc-4372-a567-0e02b2c3d479";
const mockedApi = vi.mocked(api);

async function renderPage(id: string) {
  const element = await AdminUserDetailPage({ params: Promise.resolve({ id }) });
  render(element as React.ReactElement);
}

const SAMPLE_USER = {
  id: VALID_UUID,
  name: "Alice Teacher",
  email: "alice@school.edu",
  avatarUrl: null,
  isPlatformAdmin: false,
  status: "active" as const,
  orgRole: "teacher",
  orgId: "org-1",
  orgName: "Riverside School",
  hasPassword: true,
  createdAt: "2026-01-10T10:00:00Z",
  updatedAt: "2026-03-15T14:30:00Z",
};

describe("AdminUserDetailPage", () => {
  beforeEach(() => {
    mockedApi.mockReset();
  });

  it("renders metadata card with user data on happy path", async () => {
    mockedApi.mockResolvedValueOnce(SAMPLE_USER);
    await renderPage(VALID_UUID);

    // Heading
    expect(screen.getByRole("heading", { name: "Alice Teacher" })).toBeInTheDocument();
    // Status badge
    expect(screen.getByText("Active")).toBeInTheDocument();
    // Metadata fields
    expect(screen.getByText("alice@school.edu")).toBeInTheDocument();
    expect(screen.getByText("Teacher")).toBeInTheDocument();
    expect(screen.getByText("Riverside School")).toBeInTheDocument();
    expect(screen.getByText("No")).toBeInTheDocument(); // Platform admin: No
    // Back link
    expect(screen.getByRole("link", { name: /back to users/i })).toHaveAttribute(
      "href",
      "/admin/users"
    );
  });

  it("renders Activity placeholder card", async () => {
    mockedApi.mockResolvedValueOnce(SAMPLE_USER);
    await renderPage(VALID_UUID);

    expect(screen.getByText("Activity")).toBeInTheDocument();
    expect(
      screen.getByText(/Session history, audit log, and per-user metrics will appear here/i)
    ).toBeInTheDocument();
  });

  it("renders 400 card on malformed UUID without calling API", async () => {
    await renderPage("not-a-uuid");

    expect(screen.getByRole("heading", { name: "User not found" })).toBeInTheDocument();
    expect(screen.getByText(/not-a-uuid/)).toBeInTheDocument();
    expect(mockedApi).not.toHaveBeenCalled();
  });

  it("renders 404 card when API returns 404", async () => {
    mockedApi.mockRejectedValueOnce(new ApiError(404, "Not Found"));
    await renderPage(VALID_UUID);

    expect(screen.getByRole("heading", { name: "User not found" })).toBeInTheDocument();
    expect(screen.getByText(/No user with id/i)).toBeInTheDocument();
    expect(screen.getByText(VALID_UUID)).toBeInTheDocument();
  });

  it("renders 403 panel when API returns 403", async () => {
    mockedApi.mockRejectedValueOnce(new ApiError(403, "Forbidden"));
    await renderPage(VALID_UUID);

    expect(screen.getByText(/Platform admin access required/i)).toBeInTheDocument();
    expect(screen.getByText(/platform-admin privileges/i)).toBeInTheDocument();
  });
});

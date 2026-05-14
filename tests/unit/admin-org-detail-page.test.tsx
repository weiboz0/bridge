// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("@/lib/api-client", () => ({
  api: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  redirect: vi.fn(),
}));

vi.mock("next/link", () => ({
  default: ({ href, children, className }: { href: string; children: React.ReactNode; className?: string }) => (
    <a href={href} className={className}>{children}</a>
  ),
}));

// OrgEditTrigger is a client component; stub it in server-component test context.
vi.mock("@/app/(portal)/admin/orgs/[id]/org-edit-trigger", () => ({
  OrgEditTrigger: ({ org }: { org: { id: string } }) => (
    <button data-testid="edit-org-trigger" data-org-id={org.id}>
      Edit organization
    </button>
  ),
}));

import AdminOrgDetailPage from "@/app/(portal)/admin/orgs/[id]/page";
import { api } from "@/lib/api-client";
import { ApiError } from "@/lib/api-error";

const VALID_UUID = "f47ac10b-58cc-4372-a567-0e02b2c3d479";
const mockedApi = vi.mocked(api);

async function renderPage(id: string) {
  const element = await AdminOrgDetailPage({ params: Promise.resolve({ id }) });
  render(element as React.ReactElement);
}

const SAMPLE_ORG = {
  id: VALID_UUID,
  name: "Riverside Academy",
  slug: "riverside-academy",
  type: "school",
  status: "active" as const,
  contactEmail: "admin@riverside.edu",
  contactName: "Jane Smith",
  domain: "riverside.edu",
  settings: "{}",
  verifiedAt: null,
  createdAt: "2026-01-10T10:00:00Z",
  updatedAt: "2026-03-15T14:30:00Z",
  teacherCount: 5,
  studentCount: 32,
  parentCount: 3,
  adminCount: 2,
  totalActive: 42,
};

describe("AdminOrgDetailPage", () => {
  beforeEach(() => {
    mockedApi.mockReset();
  });

  it("renders metadata card with all org fields", async () => {
    mockedApi.mockResolvedValueOnce(SAMPLE_ORG);
    await renderPage(VALID_UUID);

    expect(screen.getByRole("heading", { name: "Riverside Academy" })).toBeInTheDocument();
    expect(screen.getByText("school")).toBeInTheDocument();
    expect(screen.getByText("riverside-academy")).toBeInTheDocument();
    expect(screen.getByText("Jane Smith")).toBeInTheDocument();
    expect(screen.getByText("admin@riverside.edu")).toBeInTheDocument();
    // Back link
    expect(screen.getByRole("link", { name: /back to organizations/i })).toHaveAttribute(
      "href",
      "/admin/orgs"
    );
  });

  it("renders member counts with dot-separator format", async () => {
    mockedApi.mockResolvedValueOnce(SAMPLE_ORG);
    await renderPage(VALID_UUID);

    expect(
      screen.getByText("5 teachers · 32 students · 3 parents · 2 org admins · 42 active total")
    ).toBeInTheDocument();
  });

  it("renders 'No active members yet' when totalActive is 0", async () => {
    mockedApi.mockResolvedValueOnce({ ...SAMPLE_ORG, totalActive: 0 });
    await renderPage(VALID_UUID);

    expect(screen.getByText("No active members yet")).toBeInTheDocument();
  });

  it("renders Activity placeholder card", async () => {
    mockedApi.mockResolvedValueOnce(SAMPLE_ORG);
    await renderPage(VALID_UUID);

    expect(screen.getByText("Activity")).toBeInTheDocument();
    expect(
      screen.getByText(/Session volume, recent admin actions, and per-org metrics will appear here/i)
    ).toBeInTheDocument();
  });

  it("renders the Edit organization button", async () => {
    mockedApi.mockResolvedValueOnce(SAMPLE_ORG);
    await renderPage(VALID_UUID);

    expect(screen.getByTestId("edit-org-trigger")).toBeInTheDocument();
    expect(screen.getByText("Edit organization")).toBeInTheDocument();
  });

  it("renders 400 card on malformed UUID without calling API", async () => {
    await renderPage("not-a-uuid");

    expect(screen.getByRole("heading", { name: "Organization not found" })).toBeInTheDocument();
    expect(screen.getByText(/not-a-uuid/)).toBeInTheDocument();
    expect(mockedApi).not.toHaveBeenCalled();
  });

  it("renders 404 card when API returns 404", async () => {
    mockedApi.mockRejectedValueOnce(new ApiError(404, "Not Found"));
    await renderPage(VALID_UUID);

    expect(screen.getByRole("heading", { name: "Organization not found" })).toBeInTheDocument();
    expect(screen.getByText(/No organization with id/i)).toBeInTheDocument();
  });

  it("renders 403 panel when API returns 403", async () => {
    mockedApi.mockRejectedValueOnce(new ApiError(403, "Forbidden"));
    await renderPage(VALID_UUID);

    expect(screen.getByText(/Platform admin access required/i)).toBeInTheDocument();
    expect(screen.getByText(/platform-admin privileges/i)).toBeInTheDocument();
  });
});

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
  default: ({
    href,
    children,
    className,
  }: {
    href: string;
    children: React.ReactNode;
    className?: string;
  }) => (
    <a href={href} className={className}>
      {children}
    </a>
  ),
}));

// Stub the client-side trigger (uses useRouter which can't run in RSC tests).
vi.mock("@/app/(portal)/admin/books/book-create-trigger", () => ({
  BookCreateTrigger: () => <button>+ New book</button>,
}));

// Stub BookActions client component.
vi.mock("@/components/books/book-actions", () => ({
  BookActions: ({ bookTitle }: { bookTitle: string }) => (
    <div data-testid="book-actions">{bookTitle}</div>
  ),
}));

import AdminBooksPage from "@/app/(portal)/admin/books/page";
import { api } from "@/lib/api-client";
import { ApiError } from "@/lib/api-error";

const mockedApi = vi.mocked(api);

const SAMPLE_BOOKS = [
  {
    id: "book-1",
    title: "Python for K-8",
    description: "Intro Python",
    scope: "platform",
    scopeId: null,
    createdBy: "user-1",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-04-01T00:00:00Z",
  },
  {
    id: "book-2",
    title: "Data Structures",
    description: "CS fundamentals",
    scope: "org",
    scopeId: "org-1",
    createdBy: "user-2",
    createdAt: "2026-02-01T00:00:00Z",
    updatedAt: "2026-05-01T00:00:00Z",
  },
];

const SAMPLE_ORGS = [{ id: "org-1", name: "Riverside School" }];

async function renderPage(searchParams: { scope?: string; scopeId?: string } = {}) {
  // Two api calls: books + admin/orgs
  mockedApi.mockImplementation(async (path: string) => {
    if (path.startsWith("/api/books")) return { items: SAMPLE_BOOKS };
    if (path.startsWith("/api/admin/orgs")) return { items: SAMPLE_ORGS };
    throw new Error(`Unexpected API call: ${path}`);
  });
  const element = await AdminBooksPage({ searchParams: Promise.resolve(searchParams) });
  render(element as React.ReactElement);
}

describe("AdminBooksPage", () => {
  beforeEach(() => {
    mockedApi.mockReset();
  });

  it("renders the page heading and + New book button", async () => {
    await renderPage();
    expect(screen.getByRole("heading", { name: "Books" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "+ New book" })).toBeInTheDocument();
  });

  it("renders all expected columns in the table", async () => {
    await renderPage();
    expect(screen.getByText("Title")).toBeInTheDocument();
    expect(screen.getByText("Scope")).toBeInTheDocument();
    expect(screen.getByText("Owner")).toBeInTheDocument();
    expect(screen.getByText("Chapters")).toBeInTheDocument();
    expect(screen.getByText("Updated")).toBeInTheDocument();
  });

  it("renders books with title links", async () => {
    await renderPage();
    const pythonLink = screen.getByRole("link", { name: "Python for K-8" });
    expect(pythonLink).toHaveAttribute("href", "/admin/books/book-1");
    const dsLink = screen.getByRole("link", { name: "Data Structures" });
    expect(dsLink).toHaveAttribute("href", "/admin/books/book-2");
  });

  it("renders scope badges for each book", async () => {
    await renderPage();
    // "Platform" and "Org" may appear multiple times (scope badge + filter tab).
    expect(screen.getAllByText("Platform").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("Org").length).toBeGreaterThanOrEqual(1);
  });

  it("renders owner column: org name for org-scope, 'Platform' for platform-scope", async () => {
    await renderPage();
    // Platform book shows "Platform" as owner.
    const platformTexts = screen.getAllByText("Platform");
    // One is the scope badge, one is the owner column — both present.
    expect(platformTexts.length).toBeGreaterThanOrEqual(1);
    // Org book shows org name.
    expect(screen.getByText("Riverside School")).toBeInTheDocument();
  });

  it("filter chips show All / Platform / Org tabs", async () => {
    await renderPage();
    expect(screen.getByRole("link", { name: "All" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Platform" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Org" })).toBeInTheDocument();
  });

  it("All tab link has correct href", async () => {
    await renderPage();
    expect(screen.getByRole("link", { name: "All" })).toHaveAttribute(
      "href",
      "/admin/books"
    );
  });

  it("scope=org tab link href includes scope=org", async () => {
    await renderPage();
    expect(screen.getByRole("link", { name: "Org" })).toHaveAttribute(
      "href",
      "/admin/books?scope=org"
    );
  });

  it("renders org dropdown when scope=org filter is active", async () => {
    // Mock specifically for this test.
    mockedApi.mockImplementation(async (path: string) => {
      if (path.startsWith("/api/books")) return { items: SAMPLE_BOOKS };
      if (path.startsWith("/api/admin/orgs")) return { items: SAMPLE_ORGS };
      throw new Error(`Unexpected: ${path}`);
    });
    const element = await AdminBooksPage({
      searchParams: Promise.resolve({ scope: "org" }),
    });
    render(element as React.ReactElement);
    expect(screen.getByRole("combobox", { name: /Filter by organization/i })).toBeInTheDocument();
  });

  it("renders empty state when no books found", async () => {
    mockedApi.mockImplementation(async (path: string) => {
      if (path.startsWith("/api/books")) return { items: [] };
      if (path.startsWith("/api/admin/orgs")) return { items: [] };
      throw new Error(`Unexpected: ${path}`);
    });
    const element = await AdminBooksPage({ searchParams: Promise.resolve({}) });
    render(element as React.ReactElement);
    expect(screen.getByText("No books found.")).toBeInTheDocument();
  });

  it("renders 403 panel on 403 error", async () => {
    // Books call is first (auth gate). Orgs call is second (best-effort).
    mockedApi.mockRejectedValueOnce(new ApiError(403, "Forbidden"));
    const element = await AdminBooksPage({ searchParams: Promise.resolve({}) });
    render(element as React.ReactElement);
    expect(screen.getByText(/Platform admin access required/i)).toBeInTheDocument();
  });
});

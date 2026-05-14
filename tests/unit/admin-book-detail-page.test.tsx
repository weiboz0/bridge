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

// Stub the client-side edit trigger (uses useRouter / useState).
vi.mock("@/app/(portal)/admin/books/[id]/book-edit-trigger", () => ({
  BookEditTrigger: ({ book }: { book: { id: string } }) => (
    <button data-testid="edit-book-trigger" data-book-id={book.id}>
      Edit book
    </button>
  ),
}));

import AdminBookDetailPage from "@/app/(portal)/admin/books/[id]/page";
import { api } from "@/lib/api-client";
import { ApiError } from "@/lib/api-error";

const VALID_UUID = "f47ac10b-58cc-4372-a567-0e02b2c3d479";
const mockedApi = vi.mocked(api);

const SAMPLE_BOOK = {
  id: VALID_UUID,
  title: "Python for K-8",
  description: "Intro Python curriculum",
  scope: "org",
  scopeId: "aaaaaaaa-0000-0000-0000-000000000001",
  createdBy: "user-001",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-04-01T00:00:00Z",
};

const SAMPLE_ORG = {
  id: "aaaaaaaa-0000-0000-0000-000000000001",
  name: "Riverside School",
};

async function renderPage(id: string) {
  const element = await AdminBookDetailPage({ params: Promise.resolve({ id }) });
  render(element as React.ReactElement);
}

describe("AdminBookDetailPage", () => {
  beforeEach(() => {
    mockedApi.mockReset();
  });

  it("renders book metadata card on happy path", async () => {
    mockedApi
      .mockResolvedValueOnce(SAMPLE_BOOK) // GET /api/books/{id}
      .mockResolvedValueOnce(SAMPLE_ORG); // GET /api/admin/orgs/{scopeId}

    await renderPage(VALID_UUID);

    expect(screen.getByRole("heading", { name: "Python for K-8" })).toBeInTheDocument();
    expect(screen.getByText("Intro Python curriculum")).toBeInTheDocument();
    // "Org" appears in scope badge + metadata field — at least once.
    expect(screen.getAllByText("Org").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("Riverside School")).toBeInTheDocument();
  });

  it("renders all metadata fields", async () => {
    mockedApi
      .mockResolvedValueOnce(SAMPLE_BOOK)
      .mockResolvedValueOnce(SAMPLE_ORG);

    await renderPage(VALID_UUID);

    expect(screen.getByText("Scope")).toBeInTheDocument();
    expect(screen.getByText("Owner")).toBeInTheDocument();
    expect(screen.getByText("Created")).toBeInTheDocument();
    expect(screen.getByText("Last updated")).toBeInTheDocument();
  });

  it("renders the 'Chapters in this book' card with empty state", async () => {
    mockedApi
      .mockResolvedValueOnce(SAMPLE_BOOK)
      .mockResolvedValueOnce(SAMPLE_ORG);

    await renderPage(VALID_UUID);

    expect(screen.getByText("Chapters in this book")).toBeInTheDocument();
    expect(screen.getByText(/No chapters assigned yet/i)).toBeInTheDocument();
  });

  it("renders the Edit book button", async () => {
    mockedApi
      .mockResolvedValueOnce(SAMPLE_BOOK)
      .mockResolvedValueOnce(SAMPLE_ORG);

    await renderPage(VALID_UUID);

    expect(screen.getByTestId("edit-book-trigger")).toBeInTheDocument();
    expect(screen.getByTestId("edit-book-trigger")).toHaveAttribute(
      "data-book-id",
      VALID_UUID
    );
  });

  it("renders Back to books link", async () => {
    mockedApi
      .mockResolvedValueOnce(SAMPLE_BOOK)
      .mockResolvedValueOnce(SAMPLE_ORG);

    await renderPage(VALID_UUID);

    expect(screen.getByRole("link", { name: /Back to books/i })).toHaveAttribute(
      "href",
      "/admin/books"
    );
  });

  it("renders 'Book not found' for malformed UUID without calling API", async () => {
    await renderPage("not-a-uuid");

    expect(screen.getByRole("heading", { name: "Book not found" })).toBeInTheDocument();
    expect(screen.getByText(/not-a-uuid/)).toBeInTheDocument();
    expect(mockedApi).not.toHaveBeenCalled();
  });

  it("renders 404 card when API returns 404", async () => {
    mockedApi.mockRejectedValueOnce(new ApiError(404, "Not Found"));
    await renderPage(VALID_UUID);

    expect(screen.getByRole("heading", { name: "Book not found" })).toBeInTheDocument();
    expect(screen.getByText(/No book with id/i)).toBeInTheDocument();
  });

  it("renders 403 panel when API returns 403", async () => {
    mockedApi.mockRejectedValueOnce(new ApiError(403, "Forbidden"));
    await renderPage(VALID_UUID);

    expect(screen.getByText(/Platform admin access required/i)).toBeInTheDocument();
  });

  it("shows 'Platform' as owner for platform-scope books", async () => {
    const platformBook = { ...SAMPLE_BOOK, scope: "platform", scopeId: null };
    mockedApi.mockResolvedValueOnce(platformBook);
    // No org fetch for platform scope.

    await renderPage(VALID_UUID);

    // Owner field should show "Platform"
    const ownerLabel = screen.getByText("Owner");
    // The field value should be "Platform"
    expect(ownerLabel.parentElement?.textContent).toContain("Platform");
  });
});

// @vitest-environment jsdom
//
// Plan 089 phase 3 — tests for /library/[id] detail page (LibraryBookDetailPage).
//
// Role matrices:
//   1. admin viewing platform book — metadata card + chapters list rendered; edit button visible.
//   2. teacher viewing own-org book — canEdit gate passes; edit button visible.
//   3. teacher viewing other-org book (404 from backend) — 404 fallback rendered.
//   4. student — no canEdit; 404 fallback when backend hides the book (404).
//   5. Invalid UUID in URL — 404 fallback without calling API.

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("@/lib/api-client", () => ({
  api: vi.fn(),
}));

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

// Stub the client-side edit trigger.
vi.mock("@/app/(portal)/library/[id]/library-book-edit-trigger", () => ({
  LibraryBookEditTrigger: ({ book }: { book: { id: string } }) => (
    <button data-testid="edit-book-trigger" data-book-id={book.id}>
      Edit book
    </button>
  ),
}));

import LibraryBookDetailPage from "@/app/(portal)/library/[id]/page";
import { api } from "@/lib/api-client";
import { ApiError } from "@/lib/api-error";
import { redirect } from "next/navigation";

const mockedApi = vi.mocked(api);
const mockedRedirect = vi.mocked(redirect);

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const VALID_UUID = "f47ac10b-58cc-4372-a567-0e02b2c3d479";
const ORG_ID = "aaaaaaaa-0000-0000-0000-000000000001";
const OTHER_ORG_ID = "bbbbbbbb-0000-0000-0000-000000000002";

const PLATFORM_BOOK = {
  id: VALID_UUID,
  title: "Python for K-8",
  description: "Intro Python curriculum",
  scope: "platform",
  scopeId: null,
  createdBy: "user-1",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-04-01T00:00:00Z",
};

const ORG_BOOK = {
  id: VALID_UUID,
  title: "Data Structures",
  description: "CS fundamentals",
  scope: "org",
  scopeId: ORG_ID,
  createdBy: "user-2",
  createdAt: "2026-02-01T00:00:00Z",
  updatedAt: "2026-05-01T00:00:00Z",
};

const SAMPLE_CHAPTERS = [
  { id: "ch-1", title: "Chapter One", status: "draft", scope: "org", bookId: VALID_UUID },
  { id: "ch-2", title: "Chapter Two", status: "classroom_ready", scope: "org", bookId: VALID_UUID },
];

const ADMIN_PORTAL_ACCESS = {
  authorized: true,
  userName: "Admin User",
  roles: [{ role: "admin" }],
};

const TEACHER_OWN_ORG_PORTAL_ACCESS = {
  authorized: true,
  userName: "Teacher User",
  roles: [{ role: "teacher", orgId: ORG_ID, orgName: "Riverside School" }],
};

const TEACHER_OTHER_ORG_PORTAL_ACCESS = {
  authorized: true,
  userName: "Teacher User",
  roles: [{ role: "teacher", orgId: OTHER_ORG_ID, orgName: "Other School" }],
};

const STUDENT_PORTAL_ACCESS = {
  authorized: true,
  userName: "Student User",
  roles: [{ role: "student" }],
};

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

async function renderPage(id: string) {
  const element = await LibraryBookDetailPage({ params: Promise.resolve({ id }) });
  render(element as React.ReactElement);
}

beforeEach(() => {
  mockedApi.mockReset();
  mockedRedirect.mockReset();
});

// ---------------------------------------------------------------------------
// 1. Admin viewing platform book
// ---------------------------------------------------------------------------

describe("LibraryBookDetailPage — admin viewing platform book", () => {
  beforeEach(() => {
    mockedApi.mockImplementation(async (path: string) => {
      if (path === "/api/me/portal-access") return ADMIN_PORTAL_ACCESS;
      if (path === `/api/books/${VALID_UUID}`) return PLATFORM_BOOK;
      // Chapters: admin path uses /admin/chapters base; chapters fetched with bookId param.
      if (path.startsWith("/api/chapters")) return { items: SAMPLE_CHAPTERS };
      throw new Error(`Unexpected API call: ${path}`);
    });
  });

  it("renders the book title as heading", async () => {
    await renderPage(VALID_UUID);
    expect(screen.getByRole("heading", { name: "Python for K-8" })).toBeInTheDocument();
  });

  it("renders the book description", async () => {
    await renderPage(VALID_UUID);
    expect(screen.getByText("Intro Python curriculum")).toBeInTheDocument();
  });

  it("renders the Metadata card with all fields", async () => {
    await renderPage(VALID_UUID);
    expect(screen.getByText("Scope")).toBeInTheDocument();
    expect(screen.getByText("Owner")).toBeInTheDocument();
    expect(screen.getByText("Created")).toBeInTheDocument();
    expect(screen.getByText("Last updated")).toBeInTheDocument();
  });

  it("shows 'Platform' in Metadata card (scope + owner)", async () => {
    await renderPage(VALID_UUID);
    const platformTexts = screen.getAllByText("Platform");
    expect(platformTexts.length).toBeGreaterThanOrEqual(1);
  });

  it("renders 'Chapters in this book' section with chapter rows", async () => {
    await renderPage(VALID_UUID);
    expect(screen.getByText("Chapters in this book")).toBeInTheDocument();
    expect(screen.getByText("Chapter One")).toBeInTheDocument();
    expect(screen.getByText("Chapter Two")).toBeInTheDocument();
  });

  it("renders chapter status labels correctly", async () => {
    await renderPage(VALID_UUID);
    expect(screen.getByText("Draft")).toBeInTheDocument();
    expect(screen.getByText("Classroom Ready")).toBeInTheDocument();
  });

  it("renders edit button (admin can always edit)", async () => {
    await renderPage(VALID_UUID);
    expect(screen.getByTestId("edit-book-trigger")).toBeInTheDocument();
  });

  it("edit button references the correct book id", async () => {
    await renderPage(VALID_UUID);
    expect(screen.getByTestId("edit-book-trigger")).toHaveAttribute(
      "data-book-id",
      VALID_UUID
    );
  });

  it("renders Back to library link pointing at /library", async () => {
    await renderPage(VALID_UUID);
    expect(screen.getByRole("link", { name: /Back to library/i })).toHaveAttribute(
      "href",
      "/library"
    );
  });
});

// ---------------------------------------------------------------------------
// 2. Teacher viewing their own-org book
// ---------------------------------------------------------------------------

describe("LibraryBookDetailPage — teacher viewing own-org book", () => {
  beforeEach(() => {
    mockedApi.mockImplementation(async (path: string) => {
      if (path === "/api/me/portal-access") return TEACHER_OWN_ORG_PORTAL_ACCESS;
      if (path === `/api/books/${VALID_UUID}`) return ORG_BOOK;
      if (path.startsWith("/api/chapters")) return { items: SAMPLE_CHAPTERS };
      throw new Error(`Unexpected API call: ${path}`);
    });
  });

  it("renders the book title", async () => {
    await renderPage(VALID_UUID);
    expect(screen.getByRole("heading", { name: "Data Structures" })).toBeInTheDocument();
  });

  it("shows the edit button (teacher is member of the book's org)", async () => {
    await renderPage(VALID_UUID);
    expect(screen.getByTestId("edit-book-trigger")).toBeInTheDocument();
  });

  it("resolves org name from roles[] — shows Riverside School", async () => {
    await renderPage(VALID_UUID);
    expect(screen.getByText("Riverside School")).toBeInTheDocument();
  });

  it("renders chapter list from backend", async () => {
    await renderPage(VALID_UUID);
    expect(screen.getByText("Chapter One")).toBeInTheDocument();
    expect(screen.getByText("Chapter Two")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 3. Teacher viewing other-org book (404 from backend — no-leak semantics)
// ---------------------------------------------------------------------------

describe("LibraryBookDetailPage — teacher viewing other-org book (404)", () => {
  it("renders 404 fallback when book API returns 404", async () => {
    // Promise.all rejects when either call rejects; 404 on book fetch → fallback.
    mockedApi.mockImplementation(async (path: string) => {
      if (path === "/api/me/portal-access") return TEACHER_OTHER_ORG_PORTAL_ACCESS;
      if (path === `/api/books/${VALID_UUID}`) throw new ApiError(404, "Not Found");
      throw new Error(`Unexpected API call: ${path}`);
    });

    await renderPage(VALID_UUID);

    expect(screen.getByRole("heading", { name: "Book not found" })).toBeInTheDocument();
    expect(screen.getByText(/No book with id/i)).toBeInTheDocument();
  });

  it("renders 404 fallback when book API returns 403", async () => {
    mockedApi.mockImplementation(async (path: string) => {
      if (path === "/api/me/portal-access") return TEACHER_OTHER_ORG_PORTAL_ACCESS;
      if (path === `/api/books/${VALID_UUID}`) throw new ApiError(403, "Forbidden");
      throw new Error(`Unexpected API call: ${path}`);
    });

    await renderPage(VALID_UUID);

    expect(screen.getByRole("heading", { name: "Book not found" })).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 4. Student — backend hides the book (404)
// ---------------------------------------------------------------------------

describe("LibraryBookDetailPage — student (book hidden by backend)", () => {
  it("renders 404 fallback when backend returns 404 for student", async () => {
    mockedApi.mockImplementation(async (path: string) => {
      if (path === "/api/me/portal-access") return STUDENT_PORTAL_ACCESS;
      if (path === `/api/books/${VALID_UUID}`) throw new ApiError(404, "Not Found");
      throw new Error(`Unexpected API call: ${path}`);
    });

    await renderPage(VALID_UUID);

    expect(screen.getByRole("heading", { name: "Book not found" })).toBeInTheDocument();
    expect(screen.getByText(/No book with id/i)).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 5. Invalid UUID in URL — 404 fallback without calling API
// ---------------------------------------------------------------------------

describe("LibraryBookDetailPage — invalid UUID", () => {
  it("renders 'Book not found' immediately without calling the API", async () => {
    await renderPage("not-a-valid-uuid");

    expect(screen.getByRole("heading", { name: "Book not found" })).toBeInTheDocument();
    expect(screen.getByText(/not-a-valid-uuid/)).toBeInTheDocument();
    expect(mockedApi).not.toHaveBeenCalled();
  });

  it("handles a UUID-like but invalid string", async () => {
    await renderPage("00000000-0000-0000-0000-ZZZZZZZZZZZZ");
    expect(screen.getByRole("heading", { name: "Book not found" })).toBeInTheDocument();
    expect(mockedApi).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// 6. Unauthorized — redirect to /login
// ---------------------------------------------------------------------------

describe("LibraryBookDetailPage — unauthorized", () => {
  it("redirects to /login when portal-access returns 401", async () => {
    mockedApi.mockRejectedValue(new ApiError(401, "Unauthorized"));
    await expect(
      LibraryBookDetailPage({ params: Promise.resolve({ id: VALID_UUID }) })
    ).rejects.toThrow("NEXT_REDIRECT: /login");
    expect(mockedRedirect).toHaveBeenCalledWith("/login");
  });
});

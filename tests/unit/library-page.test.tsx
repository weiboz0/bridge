// @vitest-environment jsdom
//
// Plan 089 phase 3 — tests for /library list page (LibraryPage).
//
// Role matrices exercised:
//   1. admin — 1 platform book + 1 org book; create button visible; org name from /api/admin/orgs.
//   2. teacher — 1 platform book + 1 own-org book; create button visible.
//   3. org_admin — same as teacher; org name from roles[].orgName.
//   4. student — no create button (backend returns empty list; no role qualifies).
//   5. unauthorized (401 from /api/me/portal-access) — redirects to /login.

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("@/lib/api-client", () => ({
  api: vi.fn(),
}));

// redirect() in Next.js throws internally; simulate so tests that expect
// redirect don't see code running past the call.
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

// Stub the client-side create trigger (uses useRouter / useState).
vi.mock("@/app/(portal)/library/library-book-create-trigger", () => ({
  LibraryBookCreateTrigger: ({ availableOrgs }: { availableOrgs: { id: string; name: string }[] }) => (
    <button data-testid="create-trigger" data-org-count={availableOrgs.length}>
      + New book
    </button>
  ),
}));

// Stub BookActions client component.
vi.mock("@/components/books/book-actions", () => ({
  BookActions: ({ bookTitle }: { bookTitle: string }) => (
    <div data-testid="book-actions">{bookTitle}</div>
  ),
}));

import LibraryPage from "@/app/(portal)/library/page";
import { api } from "@/lib/api-client";
import { ApiError } from "@/lib/api-error";
import { redirect } from "next/navigation";

const mockedApi = vi.mocked(api);
const mockedRedirect = vi.mocked(redirect);

// ---------------------------------------------------------------------------
// Shared fixtures
// ---------------------------------------------------------------------------

const ORG_ID = "aaaaaaaa-0000-0000-0000-000000000001";

const PLATFORM_BOOK = {
  id: "book-platform-1",
  title: "Python for K-8",
  description: "Intro Python",
  scope: "platform",
  scopeId: null,
  createdBy: "user-1",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-04-01T00:00:00Z",
};

const ORG_BOOK = {
  id: "book-org-1",
  title: "Data Structures",
  description: "CS fundamentals",
  scope: "org",
  scopeId: ORG_ID,
  createdBy: "user-2",
  createdAt: "2026-02-01T00:00:00Z",
  updatedAt: "2026-05-01T00:00:00Z",
};

const ADMIN_PORTAL_ACCESS = {
  authorized: true,
  userName: "Admin User",
  roles: [{ role: "admin" }],
};

const TEACHER_PORTAL_ACCESS = {
  authorized: true,
  userName: "Teacher User",
  roles: [{ role: "teacher", orgId: ORG_ID, orgName: "Riverside School" }],
};

const ORG_ADMIN_PORTAL_ACCESS = {
  authorized: true,
  userName: "OrgAdmin User",
  roles: [{ role: "org_admin", orgId: ORG_ID, orgName: "Riverside School" }],
};

const STUDENT_PORTAL_ACCESS = {
  authorized: true,
  userName: "Student User",
  roles: [{ role: "student" }],
};

const SAMPLE_ORGS = [{ id: ORG_ID, name: "Riverside School" }];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

async function renderPage() {
  const element = await LibraryPage();
  render(element as React.ReactElement);
}

beforeEach(() => {
  mockedApi.mockReset();
  mockedRedirect.mockReset();
});

// ---------------------------------------------------------------------------
// 1. Render-as-admin
// ---------------------------------------------------------------------------

describe("LibraryPage — render as admin", () => {
  beforeEach(() => {
    mockedApi.mockImplementation(async (path: string) => {
      if (path === "/api/me/portal-access") return ADMIN_PORTAL_ACCESS;
      if (path === "/api/books") return { items: [PLATFORM_BOOK, ORG_BOOK] };
      if (path === "/api/admin/orgs") return { items: SAMPLE_ORGS };
      throw new Error(`Unexpected API call: ${path}`);
    });
  });

  it("renders the Library heading", async () => {
    await renderPage();
    expect(screen.getByRole("heading", { name: "Library" })).toBeInTheDocument();
  });

  it("renders 2 book rows in the table (1 platform + 1 org)", async () => {
    await renderPage();
    const rows = screen.getAllByTestId("book-actions");
    expect(rows).toHaveLength(2);
  });

  it("shows the create trigger button", async () => {
    await renderPage();
    expect(screen.getByTestId("create-trigger")).toBeInTheDocument();
  });

  it("resolves org name from /api/admin/orgs — shows Riverside School", async () => {
    await renderPage();
    expect(screen.getByText("Riverside School")).toBeInTheDocument();
  });

  it("shows 'Platform' as owner for platform-scope book", async () => {
    await renderPage();
    // "Platform" appears in both the scope badge and owner column.
    const platformTexts = screen.getAllByText("Platform");
    expect(platformTexts.length).toBeGreaterThanOrEqual(1);
  });

  it("title links point to /library/{id}", async () => {
    await renderPage();
    const platformLink = screen.getByRole("link", { name: "Python for K-8" });
    expect(platformLink).toHaveAttribute("href", `/library/${PLATFORM_BOOK.id}`);
    const orgLink = screen.getByRole("link", { name: "Data Structures" });
    expect(orgLink).toHaveAttribute("href", `/library/${ORG_BOOK.id}`);
  });

  it("renders expected table column headers", async () => {
    await renderPage();
    expect(screen.getByText("Title")).toBeInTheDocument();
    expect(screen.getByText("Scope")).toBeInTheDocument();
    expect(screen.getByText("Owner")).toBeInTheDocument();
    expect(screen.getByText("Chapters")).toBeInTheDocument();
    expect(screen.getByText("Updated")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 2. Render-as-teacher
// ---------------------------------------------------------------------------

describe("LibraryPage — render as teacher", () => {
  beforeEach(() => {
    mockedApi.mockImplementation(async (path: string) => {
      if (path === "/api/me/portal-access") return TEACHER_PORTAL_ACCESS;
      if (path === "/api/books") return { items: [PLATFORM_BOOK, ORG_BOOK] };
      // Teacher should NOT call /api/admin/orgs — throw if it does.
      throw new Error(`Unexpected API call: ${path}`);
    });
  });

  it("renders the Library heading", async () => {
    await renderPage();
    expect(screen.getByRole("heading", { name: "Library" })).toBeInTheDocument();
  });

  it("renders 2 rows (1 platform + 1 org)", async () => {
    await renderPage();
    expect(screen.getAllByTestId("book-actions")).toHaveLength(2);
  });

  it("shows create trigger (teacher has org membership)", async () => {
    await renderPage();
    expect(screen.getByTestId("create-trigger")).toBeInTheDocument();
  });

  it("resolves org name from roles[] not from /api/admin/orgs", async () => {
    await renderPage();
    // orgName comes from TEACHER_PORTAL_ACCESS.roles[0].orgName.
    expect(screen.getByText("Riverside School")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 3. Render-as-org_admin
// ---------------------------------------------------------------------------

describe("LibraryPage — render as org_admin", () => {
  beforeEach(() => {
    mockedApi.mockImplementation(async (path: string) => {
      if (path === "/api/me/portal-access") return ORG_ADMIN_PORTAL_ACCESS;
      if (path === "/api/books") return { items: [PLATFORM_BOOK, ORG_BOOK] };
      throw new Error(`Unexpected API call: ${path}`);
    });
  });

  it("renders 2 rows", async () => {
    await renderPage();
    expect(screen.getAllByTestId("book-actions")).toHaveLength(2);
  });

  it("shows create trigger (org_admin has org membership)", async () => {
    await renderPage();
    expect(screen.getByTestId("create-trigger")).toBeInTheDocument();
  });

  it("resolves org name from roles[].orgName", async () => {
    await renderPage();
    expect(screen.getByText("Riverside School")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 4. Render-as-student
// ---------------------------------------------------------------------------

describe("LibraryPage — render as student", () => {
  beforeEach(() => {
    mockedApi.mockImplementation(async (path: string) => {
      if (path === "/api/me/portal-access") return STUDENT_PORTAL_ACCESS;
      // Backend returns empty list for students.
      if (path === "/api/books") return { items: [] };
      throw new Error(`Unexpected API call: ${path}`);
    });
  });

  it("renders Library heading", async () => {
    await renderPage();
    expect(screen.getByRole("heading", { name: "Library" })).toBeInTheDocument();
  });

  it("shows empty state — no books available", async () => {
    await renderPage();
    expect(screen.getByText("No books available.")).toBeInTheDocument();
  });

  it("does NOT render a create trigger", async () => {
    await renderPage();
    expect(screen.queryByTestId("create-trigger")).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// 5. Unauthorized (401) — redirects to /login
// ---------------------------------------------------------------------------

describe("LibraryPage — unauthorized", () => {
  it("redirects to /login when /api/me/portal-access returns 401", async () => {
    mockedApi.mockRejectedValueOnce(new ApiError(401, "Unauthorized"));
    await expect(LibraryPage()).rejects.toThrow("NEXT_REDIRECT: /login");
    expect(mockedRedirect).toHaveBeenCalledWith("/login");
  });

  it("redirects to /login when authorized=false", async () => {
    mockedApi.mockResolvedValueOnce({ authorized: false, userName: "", roles: [] });
    await expect(LibraryPage()).rejects.toThrow("NEXT_REDIRECT: /login");
    expect(mockedRedirect).toHaveBeenCalledWith("/login");
  });
});

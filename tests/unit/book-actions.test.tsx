// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

const mockRefresh = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: mockRefresh }),
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

beforeEach(() => {
  (globalThis as unknown as { fetch: typeof fetch | undefined }).fetch = undefined;
  mockRefresh.mockReset();
});

function mockFetch(status: number, body?: unknown) {
  (globalThis as unknown as { fetch: typeof fetch }).fetch = vi.fn(async () =>
    new Response(body == null ? null : JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    })
  ) as unknown as typeof fetch;
  return (globalThis as unknown as { fetch: ReturnType<typeof vi.fn> }).fetch;
}

import { BookActions } from "@/components/books/book-actions";

const DEFAULT_PROPS = {
  bookId: "book-abc",
  bookTitle: "Python for K-8",
  bookScope: "org" as const,
  bookScopeId: "org-1",
  bookDescription: "Intro to Python",
};

function openMenu() {
  const btn = screen.getByRole("button", { name: /actions/i });
  fireEvent.click(btn);
}

describe("BookActions", () => {
  it("renders the actions dropdown trigger button", () => {
    render(<BookActions {...DEFAULT_PROPS} />);
    expect(screen.getByRole("button", { name: /actions/i })).toBeInTheDocument();
  });

  it("dropdown shows View details, Edit book, and Delete book options", () => {
    render(<BookActions {...DEFAULT_PROPS} />);
    openMenu();
    expect(screen.getByText("View details")).toBeInTheDocument();
    expect(screen.getByText("Edit book…")).toBeInTheDocument();
    expect(screen.getByText("Delete book…")).toBeInTheDocument();
  });

  it("View details links to /admin/books/{bookId}", () => {
    render(<BookActions {...DEFAULT_PROPS} />);
    openMenu();
    expect(screen.getByRole("link", { name: "View details" })).toHaveAttribute(
      "href",
      `/admin/books/${DEFAULT_PROPS.bookId}`
    );
  });

  it("clicking Edit book opens BookEditDialog", () => {
    render(<BookActions {...DEFAULT_PROPS} />);
    openMenu();
    fireEvent.click(screen.getByText("Edit book…"));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Edit book")).toBeInTheDocument();
  });

  it("clicking Delete book opens ConfirmDialog with typeToConfirm set to the book title", () => {
    render(<BookActions {...DEFAULT_PROPS} />);
    openMenu();
    fireEvent.click(screen.getByText("Delete book…"));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Delete book")).toBeInTheDocument();
    // The typeToConfirm input placeholder should be the book title.
    expect(
      screen.getByPlaceholderText(DEFAULT_PROPS.bookTitle)
    ).toBeInTheDocument();
  });

  it("delete confirm button is disabled until the book title is typed", () => {
    render(<BookActions {...DEFAULT_PROPS} />);
    openMenu();
    fireEvent.click(screen.getByText("Delete book…"));
    const confirmBtn = screen.getByRole("button", { name: /^Delete$/ });
    expect(confirmBtn).toBeDisabled();

    fireEvent.change(screen.getByPlaceholderText(DEFAULT_PROPS.bookTitle), {
      target: { value: DEFAULT_PROPS.bookTitle },
    });
    expect(confirmBtn).not.toBeDisabled();
  });

  it("confirmed delete fires DELETE /api/books/{id}", async () => {
    const fetchMock = mockFetch(204);
    render(<BookActions {...DEFAULT_PROPS} />);
    openMenu();
    fireEvent.click(screen.getByText("Delete book…"));

    fireEvent.change(screen.getByPlaceholderText(DEFAULT_PROPS.bookTitle), {
      target: { value: DEFAULT_PROPS.bookTitle },
    });
    fireEvent.click(screen.getByRole("button", { name: /^Delete$/ }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        `/api/books/${DEFAULT_PROPS.bookId}`,
        expect.objectContaining({ method: "DELETE" })
      );
    });
    await waitFor(() => expect(mockRefresh).toHaveBeenCalledTimes(1));
  });
});

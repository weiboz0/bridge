// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { BookPickerDialog } from "@/components/books/book-picker-dialog";

const PLATFORM_BOOKS = [
  {
    id: "book-1",
    title: "Python for K-8",
    scope: "platform" as const,
    scopeId: null,
  },
  {
    id: "book-2",
    title: "Intro to Scratch",
    scope: "platform" as const,
    scopeId: null,
  },
];

function noop() {}

describe("BookPickerDialog", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns null when open=false", () => {
    const { container } = render(
      <BookPickerDialog open={false} onClose={noop} onPicked={noop} scope="platform" />
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders the 'Unfiled' option always at the top", async () => {
    vi.mocked(fetch).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ items: PLATFORM_BOOKS }),
    } as Response);

    render(
      <BookPickerDialog open onClose={noop} onPicked={noop} scope="platform" />
    );

    expect(screen.getByText(/Remove from book/i)).toBeInTheDocument();
  });

  it("fetches books filtered by scope=platform", async () => {
    vi.mocked(fetch).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ items: PLATFORM_BOOKS }),
    } as Response);

    render(
      <BookPickerDialog open onClose={noop} onPicked={noop} scope="platform" />
    );

    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith(
        expect.stringContaining("scope=platform"),
        expect.objectContaining({ signal: expect.any(AbortSignal) })
      );
    });
    await waitFor(() => {
      expect(screen.getByText("Python for K-8")).toBeInTheDocument();
      expect(screen.getByText("Intro to Scratch")).toBeInTheDocument();
    });
  });

  it("fetches books filtered by scope=org and scopeId when provided", async () => {
    vi.mocked(fetch).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ items: [] }),
    } as Response);

    render(
      <BookPickerDialog
        open
        onClose={noop}
        onPicked={noop}
        scope="org"
        scopeId="org-abc"
      />
    );

    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith(
        expect.stringContaining("scopeId=org-abc"),
        expect.objectContaining({ signal: expect.any(AbortSignal) })
      );
    });
  });

  it("search input filters the book list by title", async () => {
    vi.mocked(fetch).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ items: PLATFORM_BOOKS }),
    } as Response);

    render(
      <BookPickerDialog open onClose={noop} onPicked={noop} scope="platform" />
    );

    await waitFor(() => {
      expect(screen.getByText("Python for K-8")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByPlaceholderText(/Search books/i), {
      target: { value: "scratch" },
    });

    expect(screen.queryByText("Python for K-8")).not.toBeInTheDocument();
    expect(screen.getByText("Intro to Scratch")).toBeInTheDocument();
  });

  it("clicking Unfiled calls onPicked(null) and onClose", async () => {
    vi.mocked(fetch).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ items: PLATFORM_BOOKS }),
    } as Response);

    const onPicked = vi.fn();
    const onClose = vi.fn();
    render(
      <BookPickerDialog open onClose={onClose} onPicked={onPicked} scope="platform" />
    );

    fireEvent.click(screen.getByText(/Remove from book/i));
    expect(onPicked).toHaveBeenCalledWith(null);
    expect(onClose).toHaveBeenCalled();
  });

  it("clicking a book calls onPicked(book.id) and onClose", async () => {
    vi.mocked(fetch).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ items: PLATFORM_BOOKS }),
    } as Response);

    const onPicked = vi.fn();
    const onClose = vi.fn();
    render(
      <BookPickerDialog open onClose={onClose} onPicked={onPicked} scope="platform" />
    );

    await waitFor(() => {
      expect(screen.getByText("Python for K-8")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Python for K-8"));
    expect(onPicked).toHaveBeenCalledWith("book-1");
    expect(onClose).toHaveBeenCalled();
  });

  it("shows 'No books found' when no results match the search", async () => {
    vi.mocked(fetch).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ items: PLATFORM_BOOKS }),
    } as Response);

    render(
      <BookPickerDialog open onClose={noop} onPicked={noop} scope="platform" />
    );

    await waitFor(() => {
      expect(screen.getByText("Python for K-8")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByPlaceholderText(/Search books/i), {
      target: { value: "xyznotexist" },
    });

    expect(screen.getByText("No books found.")).toBeInTheDocument();
  });
});

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { BookEditDialog } from "@/components/books/book-edit-dialog";

const AVAILABLE_ORGS = [
  { id: "org-1", name: "Riverside School" },
  { id: "org-2", name: "Westview Academy" },
];

const EXISTING_BOOK = {
  id: "book-001",
  title: "Python for K-8",
  description: "Intro to Python for kids",
  scope: "org" as const,
  scopeId: "org-1",
};

function noop() {}

describe("BookEditDialog", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns null when open=false", () => {
    const { container } = render(
      <BookEditDialog open={false} onClose={noop} onSaved={noop} />
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders create form when no book prop provided", () => {
    render(
      <BookEditDialog
        open
        onClose={noop}
        onSaved={noop}
        availableOrgs={AVAILABLE_ORGS}
      />
    );
    expect(screen.getByText("New book")).toBeInTheDocument();
    expect(screen.getByLabelText("Title")).toBeInTheDocument();
    expect(screen.getByLabelText("Description")).toBeInTheDocument();
  });

  it("save button disabled when title is empty", () => {
    render(
      <BookEditDialog
        open
        onClose={noop}
        onSaved={noop}
        availableOrgs={AVAILABLE_ORGS}
      />
    );
    const saveBtn = screen.getByRole("button", { name: "Save" });
    expect(saveBtn).toBeDisabled();
  });

  it("save button enabled when title has text and org is selected", () => {
    render(
      <BookEditDialog
        open
        onClose={noop}
        onSaved={noop}
        availableOrgs={AVAILABLE_ORGS}
      />
    );
    fireEvent.change(screen.getByLabelText("Title"), {
      target: { value: "My Book" },
    });
    // org scope is default; org-1 is pre-selected via availableOrgs[0]
    expect(screen.getByRole("button", { name: "Save" })).not.toBeDisabled();
  });

  it("save disabled in create mode with org scope when no org selected", () => {
    render(
      <BookEditDialog
        open
        onClose={noop}
        onSaved={noop}
        availableOrgs={[]}
      />
    );
    fireEvent.change(screen.getByLabelText("Title"), {
      target: { value: "My Book" },
    });
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("save disabled in edit mode when all fields unchanged", () => {
    render(
      <BookEditDialog
        book={EXISTING_BOOK}
        open
        onClose={noop}
        onSaved={noop}
      />
    );
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("save enabled in edit mode when title changes", () => {
    render(
      <BookEditDialog
        book={EXISTING_BOOK}
        open
        onClose={noop}
        onSaved={noop}
      />
    );
    const titleInput = screen.getByLabelText("Title");
    fireEvent.change(titleInput, { target: { value: "Python for K-8 (Updated)" } });
    expect(screen.getByRole("button", { name: "Save" })).not.toBeDisabled();
  });

  it("calls POST /api/books on successful create and invokes onSaved + onClose", async () => {
    const savedBook = {
      id: "new-book",
      title: "My New Book",
      description: "",
      scope: "org",
      scopeId: "org-1",
      createdBy: "user-1",
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-01T00:00:00Z",
    };
    vi.mocked(fetch).mockResolvedValueOnce({
      ok: true,
      json: async () => savedBook,
    } as Response);

    const onSaved = vi.fn();
    const onClose = vi.fn();
    render(
      <BookEditDialog
        open
        onClose={onClose}
        onSaved={onSaved}
        availableOrgs={AVAILABLE_ORGS}
      />
    );

    fireEvent.change(screen.getByLabelText("Title"), {
      target: { value: "My New Book" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(onSaved).toHaveBeenCalledWith(savedBook);
      expect(onClose).toHaveBeenCalled();
    });
  });

  it("calls PATCH /api/books/{id} on successful edit and invokes onSaved + onClose", async () => {
    const updatedBook = {
      ...EXISTING_BOOK,
      title: "Python for K-8 Vol 2",
      createdBy: "user-1",
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-05-01T00:00:00Z",
    };
    vi.mocked(fetch).mockResolvedValueOnce({
      ok: true,
      json: async () => updatedBook,
    } as Response);

    const onSaved = vi.fn();
    const onClose = vi.fn();
    render(
      <BookEditDialog
        book={EXISTING_BOOK}
        open
        onClose={onClose}
        onSaved={onSaved}
      />
    );

    fireEvent.change(screen.getByLabelText("Title"), {
      target: { value: "Python for K-8 Vol 2" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith(
        `/api/books/${EXISTING_BOOK.id}`,
        expect.objectContaining({ method: "PATCH" })
      );
      expect(onSaved).toHaveBeenCalledWith(updatedBook);
      expect(onClose).toHaveBeenCalled();
    });
  });

  it("shows inline error on 4xx response", async () => {
    vi.mocked(fetch).mockResolvedValueOnce({
      ok: false,
      status: 422,
      json: async () => ({ error: "Title too long" }),
    } as Response);

    render(
      <BookEditDialog
        open
        onClose={noop}
        onSaved={noop}
        availableOrgs={AVAILABLE_ORGS}
      />
    );

    fireEvent.change(screen.getByLabelText("Title"), {
      target: { value: "Trigger Error" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent("Title too long");
    });
  });

  it("resets form fields on reopen", () => {
    const { rerender } = render(
      <BookEditDialog
        book={EXISTING_BOOK}
        open
        onClose={noop}
        onSaved={noop}
      />
    );
    const titleInput = screen.getByLabelText("Title") as HTMLInputElement;
    fireEvent.change(titleInput, { target: { value: "Changed Title" } });
    expect(titleInput.value).toBe("Changed Title");

    // Close and reopen — should reset.
    rerender(
      <BookEditDialog
        book={EXISTING_BOOK}
        open={false}
        onClose={noop}
        onSaved={noop}
      />
    );
    rerender(
      <BookEditDialog
        book={EXISTING_BOOK}
        open
        onClose={noop}
        onSaved={noop}
      />
    );
    expect((screen.getByLabelText("Title") as HTMLInputElement).value).toBe(
      EXISTING_BOOK.title
    );
  });
});

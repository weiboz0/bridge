// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

// Mock the chapter-search client BEFORE importing the dialog so that
// the dialog picks up the mock when it pulls searchChapters in. Using
// vi.hoisted because vi.mock factories are hoisted to the top of the
// file — a plain top-level const would not be initialized in time.
const { searchChaptersMock } = vi.hoisted(() => ({ searchChaptersMock: vi.fn() }));
vi.mock("@/lib/chapter-search", () => ({
  searchChapters: searchChaptersMock,
}));

import { ChapterPickerDialog } from "@/components/teacher/chapter-picker-dialog";

const COURSE_ID = "course-1";
const TOPIC_ID = "topic-1";

beforeEach(() => {
  searchChaptersMock.mockReset();
});

function makeItem(overrides: Partial<any> = {}) {
  return {
    id: "chapter-1",
    scope: "platform",
    scopeId: null,
    title: "Sample Chapter",
    slug: null,
    summary: "A short summary",
    gradeLevel: "K-5",
    subjectTags: [],
    standardsTags: [],
    estimatedMinutes: null,
    materialType: "notes",
    status: "classroom_ready",
    createdBy: "u-admin",
    createdAt: "2026-04-01T00:00:00Z",
    updatedAt: "2026-04-01T00:00:00Z",
    linkedTopicId: null,
    linkedTopicTitle: null,
    canLink: true,
    ...overrides,
  };
}

describe("ChapterPickerDialog", () => {
  it("does not render when open=false", () => {
    render(
      <ChapterPickerDialog
        open={false}
        onClose={() => {}}
        courseId={COURSE_ID}
        currentTopicId={TOPIC_ID}
        onPicked={async () => {}}
      />
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("opens, debounces a search call, and renders results", async () => {
    searchChaptersMock.mockResolvedValueOnce({
      items: [makeItem({ id: "u1", title: "Loops Intro" })],
      nextCursor: null,
      error: null,
    });
    render(
      <ChapterPickerDialog
        open
        onClose={() => {}}
        courseId={COURSE_ID}
        currentTopicId={TOPIC_ID}
        onPicked={async () => {}}
      />
    );

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    await waitFor(() => expect(searchChaptersMock).toHaveBeenCalledTimes(1));
    expect(searchChaptersMock).toHaveBeenCalledWith(
      expect.objectContaining({ linkableForCourse: COURSE_ID, limit: 20 })
    );

    await waitFor(() => expect(screen.getByText("Loops Intro")).toBeInTheDocument());
    expect(screen.getByRole("button", { name: /^pick$/i })).toBeEnabled();
  });

  it("typing in the search input fires a new debounced query", async () => {
    searchChaptersMock.mockResolvedValue({ items: [], nextCursor: null, error: null });
    render(
      <ChapterPickerDialog
        open
        onClose={() => {}}
        courseId={COURSE_ID}
        currentTopicId={TOPIC_ID}
        onPicked={async () => {}}
      />
    );

    await waitFor(() => expect(searchChaptersMock).toHaveBeenCalledTimes(1));

    const input = screen.getByLabelText("Search") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "python" } });

    await waitFor(() =>
      expect(searchChaptersMock).toHaveBeenCalledWith(
        expect.objectContaining({ q: "python" })
      )
    );
  });

  it("renders an error banner on server failure (not the empty state)", async () => {
    searchChaptersMock.mockResolvedValueOnce({
      items: [],
      nextCursor: null,
      error: "server",
    });
    render(
      <ChapterPickerDialog
        open
        onClose={() => {}}
        courseId={COURSE_ID}
        currentTopicId={TOPIC_ID}
        onPicked={async () => {}}
      />
    );

    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(/server error/i)
    );
    expect(screen.queryByText(/no matching chapters/i)).toBeNull();
  });

  it("renders the empty state when results are genuinely empty", async () => {
    searchChaptersMock.mockResolvedValueOnce({ items: [], nextCursor: null, error: null });
    render(
      <ChapterPickerDialog
        open
        onClose={() => {}}
        courseId={COURSE_ID}
        currentTopicId={TOPIC_ID}
        onPicked={async () => {}}
      />
    );

    await waitFor(() =>
      expect(screen.getByText(/no matching chapters/i)).toBeInTheDocument()
    );
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("disables Pick for already-linked-elsewhere rows and shows the topic title when present", async () => {
    searchChaptersMock.mockResolvedValueOnce({
      items: [
        makeItem({
          id: "u-linked",
          title: "Already Used",
          linkedTopicId: "other-topic",
          linkedTopicTitle: "Some Other Topic",
          canLink: true,
        }),
      ],
      nextCursor: null,
      error: null,
    });
    render(
      <ChapterPickerDialog
        open
        onClose={() => {}}
        courseId={COURSE_ID}
        currentTopicId={TOPIC_ID}
        onPicked={async () => {}}
      />
    );

    await waitFor(() => expect(screen.getByText("Already Used")).toBeInTheDocument());
    expect(screen.queryByRole("button", { name: /^pick$/i })).toBeNull();
    expect(screen.getByText(/already linked/i)).toBeInTheDocument();
    expect(screen.getByText(/Some Other Topic/)).toBeInTheDocument();
  });

  it("cross-org linked row shows 'Already linked' with no topic title leak", async () => {
    searchChaptersMock.mockResolvedValueOnce({
      items: [
        makeItem({
          id: "u-cross",
          title: "Cross Org Linked",
          linkedTopicId: "other-topic-id",
          linkedTopicTitle: null, // backend redacted
          canLink: true,
        }),
      ],
      nextCursor: null,
      error: null,
    });
    render(
      <ChapterPickerDialog
        open
        onClose={() => {}}
        courseId={COURSE_ID}
        currentTopicId={TOPIC_ID}
        onPicked={async () => {}}
      />
    );

    await waitFor(() => expect(screen.getByText("Cross Org Linked")).toBeInTheDocument());
    const badge = screen.getByText(/already linked/i);
    expect(badge).toBeInTheDocument();
    // No topic title rendered alongside the badge.
    expect(screen.queryByText(/other-topic/i)).toBeNull();
  });

  it("shows a 'Linked here' badge for the topic's own currently-linked Chapter", async () => {
    searchChaptersMock.mockResolvedValueOnce({
      items: [
        makeItem({
          id: "u-here",
          title: "Currently Mine",
          linkedTopicId: TOPIC_ID,
          linkedTopicTitle: "(this topic)",
          canLink: true,
        }),
      ],
      nextCursor: null,
      error: null,
    });
    render(
      <ChapterPickerDialog
        open
        onClose={() => {}}
        courseId={COURSE_ID}
        currentTopicId={TOPIC_ID}
        onPicked={async () => {}}
      />
    );

    await waitFor(() => expect(screen.getByText("Currently Mine")).toBeInTheDocument());
    expect(screen.getByText(/linked here/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^pick$/i })).toBeNull();
  });

  it("clicking Pick calls onPicked and closes the dialog", async () => {
    searchChaptersMock.mockResolvedValueOnce({
      items: [makeItem({ id: "u-pick", title: "To Pick" })],
      nextCursor: null,
      error: null,
    });
    const onPicked = vi.fn().mockResolvedValue(undefined);
    const onClose = vi.fn();
    render(
      <ChapterPickerDialog
        open
        onClose={onClose}
        courseId={COURSE_ID}
        currentTopicId={TOPIC_ID}
        onPicked={onPicked}
      />
    );

    await waitFor(() => expect(screen.getByText("To Pick")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: /^pick$/i }));

    await waitFor(() => expect(onPicked).toHaveBeenCalledWith("u-pick"));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it("Load more uses the cursor from page 1", async () => {
    searchChaptersMock
      .mockResolvedValueOnce({
        items: [makeItem({ id: "p1-a", title: "Page 1 A" })],
        nextCursor: "cursor-1",
        error: null,
      })
      .mockResolvedValueOnce({
        items: [makeItem({ id: "p2-a", title: "Page 2 A" })],
        nextCursor: null,
        error: null,
      });
    render(
      <ChapterPickerDialog
        open
        onClose={() => {}}
        courseId={COURSE_ID}
        currentTopicId={TOPIC_ID}
        onPicked={async () => {}}
      />
    );

    await waitFor(() => expect(screen.getByText("Page 1 A")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: /load more/i }));

    await waitFor(() =>
      expect(searchChaptersMock).toHaveBeenLastCalledWith(
        expect.objectContaining({ cursor: "cursor-1" })
      )
    );
    await waitFor(() => expect(screen.getByText("Page 2 A")).toBeInTheDocument());
    // Page 1 results stay rendered (append, not replace).
    expect(screen.getByText("Page 1 A")).toBeInTheDocument();
  });

  it("Escape key closes the dialog", async () => {
    searchChaptersMock.mockResolvedValueOnce({ items: [], nextCursor: null, error: null });
    const onClose = vi.fn();
    render(
      <ChapterPickerDialog
        open
        onClose={onClose}
        courseId={COURSE_ID}
        currentTopicId={TOPIC_ID}
        onPicked={async () => {}}
      />
    );

    fireEvent.keyDown(window, { key: "Escape" });
    expect(onClose).toHaveBeenCalled();
  });
});

// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";

// Mock next/navigation so the page can read params and call .push().
const pushMock = vi.fn();
let mockParams: { id: string; topicId: string } = {
  id: "11111111-2222-3333-4444-555555555555",
  topicId: "66666666-7777-8888-9999-aaaaaaaaaaaa",
};
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock }),
  useParams: () => mockParams,
}));

import TopicEditorPage from "@/app/(portal)/teacher/courses/[id]/topics/[topicId]/page";

beforeEach(() => {
  pushMock.mockReset();
  (globalThis as any).fetch = undefined;
  mockParams = {
    id: "11111111-2222-3333-4444-555555555555",
    topicId: "66666666-7777-8888-9999-aaaaaaaaaaaa",
  };
});

function mockFetch(handler: (url: string) => { status: number; body?: any }) {
  (globalThis as any).fetch = vi.fn(async (input: any) => {
    const url = typeof input === "string" ? input : input?.url ?? "";
    const r = handler(url);
    return new Response(r.body == null ? null : JSON.stringify(r.body), {
      status: r.status,
      headers: { "Content-Type": "application/json" },
    });
  });
}

describe("TopicEditorPage — Plan 048 phase 4 error states", () => {
  it("renders an explicit not-found card when the topic GET returns 404", async () => {
    mockFetch(() => ({ status: 404 }));
    render(<TopicEditorPage />);
    await waitFor(() =>
      expect(screen.getByText(/focus area not found/i)).toBeInTheDocument()
    );
    // Loading… must NOT linger.
    expect(screen.queryByText("Loading…")).toBeNull();
    // Back to Course link is present.
    expect(screen.getByRole("button", { name: /back to course/i })).toBeInTheDocument();
  });

  it("renders an access-denied card on 403", async () => {
    mockFetch(() => ({ status: 403 }));
    render(<TopicEditorPage />);
    await waitFor(() =>
      expect(screen.getByText(/access denied/i)).toBeInTheDocument()
    );
    expect(
      screen.getByText(/don.t have permission to edit this focus area/i)
    ).toBeInTheDocument();
  });

  it("renders a generic server-error card on 500", async () => {
    mockFetch(() => ({ status: 500 }));
    render(<TopicEditorPage />);
    await waitFor(() =>
      expect(screen.getByText(/couldn.t load focus area/i)).toBeInTheDocument()
    );
    expect(screen.getByText(/server returned an error/i)).toBeInTheDocument();
  });

  it("renders an invalid-UUID card when params are not UUIDs (no fetch)", async () => {
    mockParams = { id: "not-a-uuid", topicId: "also-bad" };
    const fetchMock = vi.fn();
    (globalThis as any).fetch = fetchMock;
    render(<TopicEditorPage />);
    await waitFor(() =>
      expect(screen.getByText(/invalid focus-area url/i)).toBeInTheDocument()
    );
    // No API call when UUIDs are invalid.
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("renders the editor when topic load succeeds, with new 'Edit Focus Area' header", async () => {
    mockFetch((url) => {
      if (url.includes("/topics/")) {
        return {
          status: 200,
          body: { id: "topic-1", title: "Loops", description: "d" },
        };
      }
      // Linked-unit lookup — empty.
      return { status: 404 };
    });
    render(<TopicEditorPage />);
    await waitFor(() =>
      expect(screen.getByRole("heading", { name: /edit focus area/i })).toBeInTheDocument()
    );
    expect(screen.getByText(/focus area details/i)).toBeInTheDocument();
  });
});

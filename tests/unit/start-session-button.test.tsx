// @vitest-environment jsdom
//
// Plan 090 phase 5 — StartSessionButton redirect branching (Codex finding,
// Decision 8/Phase 5). Pre-090 this always pushed to /teacher/sessions/{id}.
// Now: a class-less create response (no classId) routes the host to the
// role-neutral /sessions/{id} room (so a non-teacher host isn't bounced by
// the /teacher portal's role gate); a class-bound response keeps the
// existing /teacher/sessions/{id} route.

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

const pushMock = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock }),
}));

import { StartSessionButton } from "@/components/teacher/start-session-button";

describe("StartSessionButton — redirect branching (plan 090 phase 5)", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    pushMock.mockReset();
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("class-less create (classId: null in response) routes to /sessions/{id}", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ id: "sess-orphan-1", classId: null, title: "Ad-hoc" }),
        { status: 201 }
      )
    );

    render(<StartSessionButton mode="orphan" defaultTitle="Ad-hoc" />);
    // Orphan mode: first click just reveals the title form; the form's
    // own submit button (same label, prefilled with defaultTitle) fires
    // the actual create.
    fireEvent.click(screen.getByRole("button", { name: /Start Session/i }));
    fireEvent.click(screen.getByRole("button", { name: /Start Session/i }));

    await waitFor(() => expect(pushMock).toHaveBeenCalledTimes(1));
    expect(pushMock).toHaveBeenCalledWith("/sessions/sess-orphan-1");
  });

  it("class-bound create (classId present in response) routes to /teacher/sessions/{id}", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ id: "sess-class-1", classId: "class-42", title: "Period 3" }),
        { status: 201 }
      )
    );

    render(<StartSessionButton classId="class-42" defaultTitle="Period 3" />);
    fireEvent.click(screen.getByRole("button", { name: /Start Live Session/i }));

    await waitFor(() => expect(pushMock).toHaveBeenCalledTimes(1));
    expect(pushMock).toHaveBeenCalledWith("/teacher/sessions/sess-class-1");
  });

  it("preserves the 422 unlinked-topic confirmation before retrying the exact create payload", async () => {
    fetchMock
      .mockResolvedValueOnce(new Response(JSON.stringify({
        code: "some_topics_unlinked",
        error: "Some focus areas have no material",
        unlinkedTopicTitles: ["Functions"],
      }), { status: 422 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ id: "sess-class-1", classId: "class-42", title: "Period 3" }), { status: 201 }));

    render(<StartSessionButton classId="class-42" defaultTitle="Period 3" />);
    fireEvent.click(screen.getByRole("button", { name: /Start Live Session/i }));
    expect(await screen.findByRole("dialog", { name: /some focus areas have no material/i })).toBeInTheDocument();
    expect(pushMock).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Start anyway" }));
    await waitFor(() => expect(fetchMock).toHaveBeenLastCalledWith("/api/sessions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title: "Period 3", classId: "class-42", confirmUnlinkedTopics: true }),
    }));
    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/teacher/sessions/sess-class-1"));
  });

  it("acknowledges the replacement archive warning exactly once before navigating", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({
      id: "sess-class-1",
      classId: "class-42",
      replacedSessions: [{ id: "previous-session", whiteboardServerArchiveComplete: false }],
    }), { status: 201 }));
    render(<StartSessionButton classId="class-42" defaultTitle="Period 3" />);
    fireEvent.click(screen.getByRole("button", { name: /Start Live Session/i }));

    expect(await screen.findByRole("dialog", { name: "Previous session ended" })).toHaveTextContent(
      "Starting this session ended 1 other live session for this class. Its whiteboard archive may be incomplete.",
    );
    expect(pushMock).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    await waitFor(() => expect(pushMock).toHaveBeenCalledTimes(1));
    expect(pushMock).toHaveBeenCalledWith("/teacher/sessions/sess-class-1");
  });
});

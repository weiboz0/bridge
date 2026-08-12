// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

const pushMock = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: pushMock }) }));

import { ScheduledSessionList } from "@/components/teacher/scheduled-session-list";

const CLASS_ID = "11111111-1111-4111-8111-111111111111";
const SCHEDULE_ID = "22222222-2222-4222-8222-222222222222";

function json(payload: unknown, status = 200): Response {
  return new Response(JSON.stringify(payload), { status, headers: { "content-type": "application/json" } });
}

describe("ScheduledSessionList — plan 094 phase 11", () => {
  beforeEach(() => {
    pushMock.mockReset();
    vi.stubGlobal("fetch", vi.fn());
  });
  afterEach(() => vi.unstubAllGlobals());

  it("lists only planned sessions from the class schedule endpoint", async () => {
    const fetchMock = vi.mocked(fetch).mockResolvedValue(json([
      { id: SCHEDULE_ID, classId: CLASS_ID, title: "Tomorrow", scheduledStart: "2026-08-13T10:00:00Z", scheduledEnd: "2026-08-13T11:00:00Z", status: "planned" },
      { id: "33333333-3333-4333-8333-333333333333", classId: CLASS_ID, title: "Started", scheduledStart: "2026-08-13T12:00:00Z", scheduledEnd: "2026-08-13T13:00:00Z", status: "started" },
    ]));
    render(<ScheduledSessionList classId={CLASS_ID} />);
    expect(await screen.findByText("Tomorrow")).toBeInTheDocument();
    expect(screen.queryByText("Started")).not.toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(`/api/classes/${CLASS_ID}/schedule`);
  });

  it("does not expose a start control when schedule discovery is unauthorized or errors", async () => {
    const fetchMock = vi.mocked(fetch).mockResolvedValue(new Response(null, { status: 403 }));
    const { rerender } = render(<ScheduledSessionList classId={CLASS_ID} />);
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(screen.queryByRole("button", { name: "Start now" })).not.toBeInTheDocument();
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 500 }));
    rerender(<ScheduledSessionList classId="44444444-4444-4444-8444-444444444444" />);
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole("button", { name: "Start now" })).not.toBeInTheDocument();
  });

  it("shows the exact replacement warning once before class-session navigation", async () => {
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input.toString();
      if (url === `/api/classes/${CLASS_ID}/schedule`) {
        return Promise.resolve(json([{ id: SCHEDULE_ID, classId: CLASS_ID, title: "Tomorrow", scheduledStart: "2026-08-13T10:00:00Z", scheduledEnd: "2026-08-13T11:00:00Z", status: "planned" }]));
      }
      if (url === `/api/schedule/${SCHEDULE_ID}/start` && init?.method === "POST") {
        return Promise.resolve(json({ id: "session-1", classId: CLASS_ID, replacedSessions: [{ id: "previous", whiteboardServerArchiveComplete: false }] }));
      }
      throw new Error(`unexpected endpoint ${url}`);
    });
    render(<ScheduledSessionList classId={CLASS_ID} />);
    fireEvent.click(await screen.findByRole("button", { name: "Start now" }));
    expect(await screen.findByRole("dialog", { name: "Previous session ended" })).toHaveTextContent(
      "Starting this session ended 1 other live session for this class. Its whiteboard archive may be incomplete.",
    );
    expect(pushMock).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    expect(pushMock).toHaveBeenCalledTimes(1);
    expect(pushMock).toHaveBeenCalledWith("/teacher/sessions/session-1");
  });

  it("renders the start endpoint error and does not navigate", async () => {
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input.toString();
      if (url === `/api/classes/${CLASS_ID}/schedule`) {
        return Promise.resolve(json([{ id: SCHEDULE_ID, classId: CLASS_ID, title: "Tomorrow", scheduledStart: "2026-08-13T10:00:00Z", scheduledEnd: "2026-08-13T11:00:00Z", status: "planned" }]));
      }
      if (url === `/api/schedule/${SCHEDULE_ID}/start` && init?.method === "POST") {
        return Promise.resolve(json({ error: "Not authorized to start this session" }, 403));
      }
      throw new Error(`unexpected endpoint ${url}`);
    });
    render(<ScheduledSessionList classId={CLASS_ID} />);
    fireEvent.click(await screen.findByRole("button", { name: "Start now" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Not authorized to start this session");
    expect(pushMock).not.toHaveBeenCalled();
  });
});

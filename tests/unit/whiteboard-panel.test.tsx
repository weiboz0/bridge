// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";

const SESSION_ID = "11111111-1111-4111-8111-111111111111";

vi.mock("next-auth/react", () => ({
  useSession: () => ({ data: { user: { id: "teacher-id" } } }),
}));

vi.mock("@/lib/whiteboard/use-whiteboard", () => ({
  useWhiteboard: () => ({ scene: null, onChange: vi.fn(), connected: true, realtimeUnavailable: false }),
}));

vi.mock("next/dynamic", () => ({
  default: () => () => <div data-testid="board" />,
}));

import { WhiteboardPanel } from "@/components/session/whiteboard/whiteboard-panel";

function json(payload: unknown, status = 200): Response {
  return new Response(JSON.stringify(payload), { status, headers: { "content-type": "application/json" } });
}

function requestURL(input: RequestInfo | URL): string {
  return typeof input === "string" ? input : input.toString();
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

describe("WhiteboardPanel — plan 094 phase 9 settings cutover", () => {
  beforeEach(() => vi.stubGlobal("fetch", vi.fn()));
  afterEach(() => vi.unstubAllGlobals());

  it("loads the strict dedicated settings schema and renders the teacher floor control", async () => {
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockImplementation((input: RequestInfo | URL) => {
      const url = requestURL(input);
      if (url.endsWith("/canvases")) return Promise.resolve(json({ items: [] }));
      if (url.endsWith("/canvas-settings")) return Promise.resolve(json({ canvasFloor: "host", whiteboardServerArchiveComplete: true }));
      throw new Error(`unexpected endpoint ${url}`);
    });

    render(<WhiteboardPanel sessionId={SESSION_ID} teacherControls />);

    expect(await screen.findByLabelText("Canvas floor")).toHaveValue("host");
    expect(fetchMock).toHaveBeenCalledWith(`/api/sessions/${SESSION_ID}/canvas-settings`);
    expect(fetchMock.mock.calls.map(([url]) => url)).not.toContain(`/api/sessions/${SESSION_ID}/settings`);
  });

  it("updates only a strict canvasFloor payload through the dedicated PATCH route", async () => {
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestURL(input);
      if (url.endsWith("/canvases")) return Promise.resolve(json({ items: [] }));
      if (url.endsWith("/canvas-settings") && init?.method === "PATCH") return Promise.resolve(json({ canvasFloor: "participants" }));
      if (url.endsWith("/canvas-settings")) return Promise.resolve(json({ canvasFloor: "host" }));
      throw new Error(`unexpected endpoint ${url}`);
    });

    render(<WhiteboardPanel sessionId={SESSION_ID} teacherControls />);
    fireEvent.change(await screen.findByLabelText("Canvas floor"), { target: { value: "participants" } });

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      `/api/sessions/${SESSION_ID}/canvas-settings`,
      { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ canvasFloor: "participants" }) },
    ));
  });

  it("does not show teacher settings controls when archive or nonteacher settings receives 403", async () => {
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockImplementation((input: RequestInfo | URL) => {
      const url = requestURL(input);
      if (url.endsWith("/canvases")) return Promise.resolve(json({ items: [] }));
      if (url.endsWith("/canvas-settings")) return Promise.resolve(new Response(null, { status: 403 }));
      throw new Error(`unexpected endpoint ${url}`);
    });

    const { rerender } = render(<WhiteboardPanel sessionId={SESSION_ID} teacherControls />);
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(`/api/sessions/${SESSION_ID}/canvas-settings`));
    expect(screen.queryByLabelText("Canvas floor")).not.toBeInTheDocument();

    rerender(<WhiteboardPanel sessionId={SESSION_ID} archive teacherControls />);
    expect(screen.queryByLabelText("Canvas floor")).not.toBeInTheDocument();
  });

  it("reads true and false durable archive status while treating omission and 403 as no completeness claim", async () => {
    for (const [label, payload, expected] of [
      ["confirmed", { canvasFloor: "private", whiteboardServerArchiveComplete: true }, "Whiteboard archive confirmed"],
      ["degraded", { canvasFloor: "private", whiteboardServerArchiveComplete: false }, "Latest whiteboard changes may not have been archived"],
      ["legacy omitted", { canvasFloor: "private" }, null],
    ] as const) {
      const fetchMock = vi.mocked(fetch);
      fetchMock.mockReset();
      fetchMock.mockImplementation((input: RequestInfo | URL) => {
        const url = requestURL(input);
        if (url.endsWith("/canvases")) return Promise.resolve(json({ items: [] }));
        if (url.endsWith("/canvas-settings")) return Promise.resolve(json(payload));
        throw new Error(`unexpected endpoint ${url}`);
      });
      const { unmount } = render(<WhiteboardPanel sessionId={SESSION_ID} archive />);
      await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(`/api/sessions/${SESSION_ID}/canvas-settings`));
      if (expected) expect(await screen.findByText(expected)).toBeInTheDocument();
      else expect(screen.queryByText(/archive confirmed|latest whiteboard changes/i)).not.toBeInTheDocument();
      unmount();
      // Labels make all three cases individually legible in a RED report.
      expect(label).toBeTruthy();
    }
  });

  it("immediately hides session A's floor while session B settings are loading", async () => {
    const sessionB = "22222222-2222-4222-8222-222222222222";
    const settingsB = deferred<Response>();
    const canvasesB = deferred<Response>();
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockImplementation((input: RequestInfo | URL) => {
      const url = requestURL(input);
      if (url === `/api/sessions/${SESSION_ID}/canvases`) return Promise.resolve(json({ items: [] }));
      if (url === `/api/sessions/${sessionB}/canvases`) return canvasesB.promise;
      if (url === `/api/sessions/${SESSION_ID}/canvas-settings`) return Promise.resolve(json({ canvasFloor: "host" }));
      if (url === `/api/sessions/${sessionB}/canvas-settings`) return settingsB.promise;
      throw new Error(`unexpected endpoint ${url}`);
    });

    const { rerender } = render(<WhiteboardPanel sessionId={SESSION_ID} teacherControls />);
    expect(await screen.findByLabelText("Canvas floor")).toHaveValue("host");

    rerender(<WhiteboardPanel sessionId={sessionB} teacherControls />);

    expect(screen.queryByLabelText("Canvas floor")).not.toBeInTheDocument();
  });

  it("immediately hides session A's archive warning while session B settings are loading", async () => {
    const sessionB = "22222222-2222-4222-8222-222222222222";
    const settingsB = deferred<Response>();
    const canvasesB = deferred<Response>();
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockImplementation((input: RequestInfo | URL) => {
      const url = requestURL(input);
      if (url === `/api/sessions/${SESSION_ID}/canvases`) return Promise.resolve(json({ items: [] }));
      if (url === `/api/sessions/${sessionB}/canvases`) return canvasesB.promise;
      if (url === `/api/sessions/${SESSION_ID}/canvas-settings`) {
        return Promise.resolve(json({ canvasFloor: "private", whiteboardServerArchiveComplete: false }));
      }
      if (url === `/api/sessions/${sessionB}/canvas-settings`) return settingsB.promise;
      throw new Error(`unexpected endpoint ${url}`);
    });

    const { rerender } = render(<WhiteboardPanel sessionId={SESSION_ID} archive />);
    expect(await screen.findByText("Latest whiteboard changes may not have been archived")).toBeInTheDocument();

    rerender(<WhiteboardPanel sessionId={sessionB} archive />);

    expect(screen.queryByText("Latest whiteboard changes may not have been archived")).not.toBeInTheDocument();
  });

  it("does not allow a reverse-order session A response to overwrite session B settings", async () => {
    const sessionB = "22222222-2222-4222-8222-222222222222";
    const settingsA = deferred<Response>();
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockImplementation((input: RequestInfo | URL) => {
      const url = requestURL(input);
      if (url.endsWith("/canvases")) return Promise.resolve(json({ items: [] }));
      if (url === `/api/sessions/${SESSION_ID}/canvas-settings`) return settingsA.promise;
      if (url === `/api/sessions/${sessionB}/canvas-settings`) return Promise.resolve(json({ canvasFloor: "participants" }));
      throw new Error(`unexpected endpoint ${url}`);
    });

    const { rerender } = render(<WhiteboardPanel sessionId={SESSION_ID} teacherControls />);
    rerender(<WhiteboardPanel sessionId={sessionB} teacherControls />);

    expect(await screen.findByLabelText("Canvas floor")).toHaveValue("participants");
    await act(async () => {
      settingsA.resolve(json({ canvasFloor: "host" }));
      await settingsA.promise;
    });
    expect(screen.getByLabelText("Canvas floor")).toHaveValue("participants");
  });

  it.each([
    ["fails", json({}, 500)],
    ["is forbidden", new Response(null, { status: 403 })],
  ] as const)("does not retain session A settings when session B settings %s", async (_outcome, settingsB) => {
    const sessionBId = "22222222-2222-4222-8222-222222222222";
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockImplementation((input: RequestInfo | URL) => {
      const url = requestURL(input);
      if (url.endsWith("/canvases")) return Promise.resolve(json({ items: [] }));
      if (url === `/api/sessions/${SESSION_ID}/canvas-settings`) return Promise.resolve(json({ canvasFloor: "host" }));
      if (url === `/api/sessions/${sessionBId}/canvas-settings`) return Promise.resolve(settingsB);
      throw new Error(`unexpected endpoint ${url}`);
    });

    const { rerender } = render(<WhiteboardPanel sessionId={SESSION_ID} teacherControls />);
    expect(await screen.findByLabelText("Canvas floor")).toHaveValue("host");

    rerender(<WhiteboardPanel sessionId={sessionBId} teacherControls />);
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(`/api/sessions/${sessionBId}/canvas-settings`));

    expect(screen.queryByLabelText("Canvas floor")).not.toBeInTheDocument();
  });

  it("cannot PATCH session B with the stale session A floor while B settings are loading", async () => {
    const sessionB = "22222222-2222-4222-8222-222222222222";
    const settingsB = deferred<Response>();
    const canvasesB = deferred<Response>();
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestURL(input);
      if (url === `/api/sessions/${SESSION_ID}/canvases`) return Promise.resolve(json({ items: [] }));
      if (url === `/api/sessions/${sessionB}/canvases`) return canvasesB.promise;
      if (url === `/api/sessions/${SESSION_ID}/canvas-settings`) return Promise.resolve(json({ canvasFloor: "host" }));
      if (url === `/api/sessions/${sessionB}/canvas-settings` && init?.method === "PATCH") {
        return Promise.resolve(json({ canvasFloor: "participants" }));
      }
      if (url === `/api/sessions/${sessionB}/canvas-settings`) return settingsB.promise;
      throw new Error(`unexpected endpoint ${url}`);
    });

    const { rerender } = render(<WhiteboardPanel sessionId={SESSION_ID} teacherControls />);
    expect(await screen.findByLabelText("Canvas floor")).toHaveValue("host");

    rerender(<WhiteboardPanel sessionId={sessionB} teacherControls />);

    expect(screen.queryByLabelText("Canvas floor")).not.toBeInTheDocument();
    expect(fetchMock.mock.calls.filter(([, init]) => init?.method === "PATCH")).toHaveLength(0);
  });

  it.each([
    ["GET-only archive status", { canvasFloor: "participants", whiteboardServerArchiveComplete: true }],
    ["an unknown field", { canvasFloor: "participants", unexpected: true }],
  ])("rejects a PATCH success response with %s and preserves the safe floor", async (_label, patchResponse) => {
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestURL(input);
      if (url.endsWith("/canvases")) return Promise.resolve(json({ items: [] }));
      if (url.endsWith("/canvas-settings") && init?.method === "PATCH") return Promise.resolve(json(patchResponse));
      if (url.endsWith("/canvas-settings")) return Promise.resolve(json({ canvasFloor: "host" }));
      throw new Error(`unexpected endpoint ${url}`);
    });

    render(<WhiteboardPanel sessionId={SESSION_ID} teacherControls />);
    const floor = await screen.findByLabelText("Canvas floor");
    fireEvent.change(floor, { target: { value: "participants" } });

    expect(await screen.findByRole("alert")).toHaveTextContent("Unable to update the canvas floor");
    expect(screen.getByLabelText("Canvas floor")).toHaveValue("host");
  });

  it.each([
    ["success", json({ canvasFloor: "participants" })],
    ["failure", json({}, 500)],
    ["malformed success", json({ canvasFloor: "participants", unexpected: true })],
  ] as const)("keeps session B's settings state after an in-flight session A PATCH %s", async (_outcome, patchAResult) => {
    const sessionB = "22222222-2222-4222-8222-222222222222";
    const patchA = deferred<Response>();
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestURL(input);
      if (url.endsWith("/canvases")) return Promise.resolve(json({ items: [] }));
      if (url === `/api/sessions/${SESSION_ID}/canvas-settings` && init?.method === "PATCH") return patchA.promise;
      if (url === `/api/sessions/${SESSION_ID}/canvas-settings`) return Promise.resolve(json({ canvasFloor: "host" }));
      if (url === `/api/sessions/${sessionB}/canvas-settings`) {
        return Promise.resolve(json({ canvasFloor: "participants", whiteboardServerArchiveComplete: true }));
      }
      throw new Error(`unexpected endpoint ${url}`);
    });

    const { rerender } = render(<WhiteboardPanel sessionId={SESSION_ID} teacherControls />);
    fireEvent.change(await screen.findByLabelText("Canvas floor"), { target: { value: "participants" } });
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      `/api/sessions/${SESSION_ID}/canvas-settings`,
      { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ canvasFloor: "participants" }) },
    ));

    rerender(<WhiteboardPanel sessionId={sessionB} teacherControls />);
    expect(await screen.findByLabelText("Canvas floor")).toHaveValue("participants");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();

    await act(async () => {
      patchA.resolve(patchAResult);
      await patchA.promise;
    });

    expect(screen.getByLabelText("Canvas floor")).toHaveValue("participants");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});

// @vitest-environment jsdom
//
// Plan 094 phase 3 — the neutral archive may only discover visible canvas
// metadata until a reader explicitly selects one.  The selected document is
// always passed to the board as read-only; the board's change callback is
// deliberately inert so a direct archive visit cannot create a local Yjs
// write even while the session remains live.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

const whiteboardOptions: Array<{ canvasId: string | null; readOnly: boolean }> = [];
const bindingWritePath = vi.fn();
const boardProps: Array<{
  readOnly: boolean;
  onChange: (scene: unknown) => void;
}> = [];

vi.mock("next-auth/react", () => ({
  useSession: () => ({ data: { user: { id: "archive-reader" } } }),
}));

vi.mock("@/lib/whiteboard/use-whiteboard", () => ({
  useWhiteboard: vi.fn((options: { canvasId: string | null; readOnly: boolean }) => {
    whiteboardOptions.push(options);
    return {
      scene: null,
      connected: false,
      realtimeUnavailable: false,
      // This stands in for the binding's actual Yjs write path.  If archive
      // mode ever stops reaching the hook with readOnly=true, a board change
      // reaches this spy and the regression fails.
      onChange: options.readOnly ? () => {} : bindingWritePath,
    };
  }),
}));

vi.mock("next/dynamic", () => ({
  default: () => (props: {
    readOnly: boolean;
    onChange: (scene: unknown) => void;
  }) => {
    boardProps.push(props);
    return (
      <button
        data-testid="board-change-attempt"
        onClick={() => props.onChange({ elements: [{ id: "attempt" }], appState: {} })}
      >
        Attempt local scene change
      </button>
    );
  },
}));

import { WhiteboardArchive } from "@/components/session/whiteboard/whiteboard-archive";

const SESSION_ID = "11111111-1111-4111-8111-111111111111";
const CANVAS_ID = "22222222-2222-4222-8222-222222222222";

function canvasList(items: unknown[]): Response {
  return new Response(JSON.stringify({ items }), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

describe("WhiteboardArchive — plan 094 phase 3", () => {
  beforeEach(() => {
    whiteboardOptions.length = 0;
    boardProps.length = 0;
    bindingWritePath.mockReset();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("initially fetches only visible canvas metadata and never mints a document token", async () => {
    const fetchMock = vi.fn().mockResolvedValue(canvasList([]));
    vi.stubGlobal("fetch", fetchMock);

    render(<WhiteboardArchive sessionId={SESSION_ID} />);

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(`/api/sessions/${SESSION_ID}/canvases`);
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(whiteboardOptions.filter(({ canvasId }) => canvasId !== null)).toEqual([]);
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      `/api/sessions/${SESSION_ID}/canvases`,
    ]);
  });

  it("shows a generic empty state without archive mutation controls or a token mint", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(canvasList([])));

    render(<WhiteboardArchive sessionId={SESSION_ID} />);

    expect(await screen.findByText(/no whiteboards are available/i)).toBeInTheDocument();
    expect(whiteboardOptions.filter(({ canvasId }) => canvasId !== null)).toEqual([]);
    expect(screen.queryByRole("button", { name: /create|new whiteboard|visibility/i })).not.toBeInTheDocument();
  });

  it("mints one selected canvas token and keeps the archive board read-only with an inert change callback", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        canvasList([
          {
            id: CANVAS_ID,
            sessionId: SESSION_ID,
            ownerId: "33333333-3333-4333-8333-333333333333",
            title: "Visible board",
            visibility: "participants",
          },
        ]),
      ),
    );

    render(<WhiteboardArchive sessionId={SESSION_ID} />);

    fireEvent.click(await screen.findByRole("button", { name: /visible board/i }));

    await waitFor(() => {
      expect(whiteboardOptions.filter(({ canvasId }) => canvasId !== null)).toEqual([
        { canvasId: CANVAS_ID, readOnly: true },
      ]);
    });
    const latestBoard = boardProps.at(-1);
    expect(latestBoard).toMatchObject({
      readOnly: true,
    });

    fireEvent.click(screen.getByTestId("board-change-attempt"));
    expect(bindingWritePath).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: /create|new whiteboard|visibility/i })).not.toBeInTheDocument();
  });
});

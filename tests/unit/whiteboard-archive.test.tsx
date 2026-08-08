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
const routerPush = vi.fn();

type SessionEventListener = (event: { data?: string }) => void;
class TestEventSource {
  static instances: TestEventSource[] = [];
  readonly listeners = new Map<string, SessionEventListener>();
  readonly close = vi.fn();

  constructor(_url: string) {
    TestEventSource.instances.push(this);
  }

  addEventListener(name: string, listener: SessionEventListener) {
    this.listeners.set(name, listener);
  }

  emit(name: string, data?: unknown) {
    this.listeners.get(name)?.({ data: data === undefined ? undefined : JSON.stringify(data) });
  }
}

vi.mock("next-auth/react", () => ({
  useSession: () => ({ data: { user: { id: "archive-reader" } } }),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: routerPush }),
}));

vi.mock("@/lib/hooks/use-panel-layout", () => ({
  usePanelLayout: () => ({
    layout: { leftVisible: false, rightVisible: false, leftWidth: 20, rightWidth: 25 },
    toggleLeft: vi.fn(),
    toggleRight: vi.fn(),
  }),
}));

vi.mock("@/lib/hooks/use-student-layout", () => ({
  useStudentLayout: () => ({ mode: "side-by-side", toggle: vi.fn() }),
}));

vi.mock("@/lib/realtime/use-realtime-token", () => ({
  useRealtimeToken: () => ({ token: "", unavailable: false }),
}));

vi.mock("@/lib/yjs/use-yjs-provider", () => ({
  useYjsProvider: () => ({ yText: null, provider: null, connected: false }),
}));

vi.mock("@/components/realtime/realtime-config-banner", () => ({
  RealtimeConfigBanner: () => null,
}));

vi.mock("@/components/session/teacher/teacher-header", () => ({
  TeacherHeader: ({ onEndSession }: { onEndSession: () => Promise<void> }) => (
    <button onClick={() => void onEndSession()}>End session</button>
  ),
}));

vi.mock("@/components/session/teacher/student-list-panel", () => ({ StudentListPanel: () => null }));
vi.mock("@/components/session/teacher/mode-toolbar", () => ({ ModeToolbar: () => null }));
vi.mock("@/components/session/teacher/ai-assistant-panel", () => ({ AiAssistantPanel: () => null }));
vi.mock("@/components/session/student-grid", () => ({ StudentGrid: () => null }));
vi.mock("@/components/annotations/annotation-form", () => ({ AnnotationForm: () => null }));
vi.mock("@/components/annotations/annotation-list", () => ({ AnnotationList: () => null }));
vi.mock("@/components/editor/editor-switcher", () => ({ EditorSwitcher: () => null }));
vi.mock("@/components/editor/code-editor", () => ({ CodeEditor: () => null }));
vi.mock("@/components/ai/ai-chat-panel", () => ({ AiChatPanel: () => null }));
vi.mock("@/components/help-queue/raise-hand-button", () => ({ RaiseHandButton: () => null }));

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
import { TeacherDashboard } from "@/components/session/teacher/teacher-dashboard";
import { StudentSession } from "@/components/session/student/student-session";

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
    routerPush.mockReset();
    TestEventSource.instances = [];
    vi.stubGlobal("EventSource", TestEventSource);
  });

  function renderTeacherDashboard() {
    return render(
      <TeacherDashboard
        sessionId={SESSION_ID}
        classId={null}
        editorMode="python"
        courseTopics={[]}
      />,
    );
  }

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

  it("keeps the teacher on the live dashboard when ending the session is not successful", async () => {
    const fetchMock = vi.fn().mockImplementation((url: string) => {
      if (url === `/api/sessions/${SESSION_ID}/end`) return Promise.resolve(new Response(null, { status: 500 }));
      return Promise.resolve(canvasList([]));
    });
    vi.stubGlobal("fetch", fetchMock);

    renderTeacherDashboard();
    fireEvent.click(screen.getByRole("button", { name: "End session" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(`/api/sessions/${SESSION_ID}/end`, { method: "POST" });
    });
    expect(routerPush).not.toHaveBeenCalled();
  });

  it("redirects the teacher to the archive only after a successful end response", async () => {
    const fetchMock = vi.fn().mockImplementation((url: string) => {
      if (url === `/api/sessions/${SESSION_ID}/end`) return Promise.resolve(new Response(null, { status: 204 }));
      return Promise.resolve(canvasList([]));
    });
    vi.stubGlobal("fetch", fetchMock);

    renderTeacherDashboard();
    fireEvent.click(screen.getByRole("button", { name: "End session" }));

    await waitFor(() => {
      expect(routerPush).toHaveBeenCalledWith(`/sessions/${SESSION_ID}/whiteboards`);
    });
  });

  it("redirects a student to the archive when the live session emits session_ended", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(canvasList([])));

    render(<StudentSession sessionId={SESSION_ID} classId={null} editorMode="python" />);
    await waitFor(() => expect(TestEventSource.instances).toHaveLength(1));

    const archiveLocation = { href: "" };
    vi.stubGlobal("window", { location: archiveLocation });
    TestEventSource.instances[0].emit("session_ended");

    expect(archiveLocation.href).toBe(`/sessions/${SESSION_ID}/whiteboards`);
  });
});

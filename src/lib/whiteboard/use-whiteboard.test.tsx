// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import * as Y from "yjs";

const state = vi.hoisted(() => ({
  yDoc: null as unknown,
  observeScene: vi.fn(() => () => {}),
  readScene: vi.fn(() => null),
  writeScene: vi.fn(),
}));

vi.mock("@/lib/realtime/use-realtime-token", () => ({
  useRealtimeToken: () => ({ token: "canvas-token", unavailable: false }),
}));

vi.mock("@/lib/yjs/use-yjs-provider", () => ({
  useYjsProvider: () => ({
    yDoc: state.yDoc,
    connected: true,
  }),
}));

vi.mock("./excalidraw-yjs", () => ({
  observeExcalidrawScene: state.observeScene,
  readExcalidrawScene: state.readScene,
  writeExcalidrawScene: state.writeScene,
}));

import { useWhiteboard } from "./use-whiteboard";

const scene = {
  elements: [{ id: "rectangle-1", type: "rectangle" }],
  appState: { viewBackgroundColor: "#ffffff" },
};

describe("useWhiteboard", () => {
  beforeEach(() => {
    state.yDoc = new Y.Doc();
    state.observeScene.mockClear();
    state.readScene.mockClear();
    state.writeScene.mockClear();
  });

  afterEach(() => {
    (state.yDoc as Y.Doc).destroy();
    state.yDoc = null;
  });

  it("does not enqueue a Yjs scene write from an archive read-only onChange", async () => {
    const { result } = renderHook(() => useWhiteboard({
      canvasId: "22222222-2222-4222-8222-222222222222",
      readOnly: true,
    }));

    await waitFor(() => expect(state.readScene).toHaveBeenCalledTimes(1));
    act(() => result.current.onChange(scene));

    expect(state.writeScene).not.toHaveBeenCalled();
  });

  it("writes a scene through the custom Yjs binding when the board is writable", async () => {
    const { result } = renderHook(() => useWhiteboard({
      canvasId: "22222222-2222-4222-8222-222222222222",
      readOnly: false,
    }));

    await waitFor(() => expect(state.readScene).toHaveBeenCalledTimes(1));
    act(() => result.current.onChange(scene));

    expect(state.writeScene).toHaveBeenCalledTimes(1);
    expect(state.writeScene).toHaveBeenCalledWith(
      (state.yDoc as Y.Doc).getMap("excalidraw-scene"),
      scene,
      expect.any(Symbol),
    );
  });
});

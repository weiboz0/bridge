// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import * as Y from "yjs";
import { z } from "zod";
import { ZodError } from "zod/v4";

const state = vi.hoisted(() => ({
  yDoc: null as unknown,
  observeScene: vi.fn(() => () => {}),
  readScene: vi.fn(() => null),
  writeScene: vi.fn(),
  realtimeCalls: [] as unknown[][],
  providerCalls: [] as unknown[],
  tokenBySessionId: new Map<string | undefined, string>(),
}));

vi.mock("@/lib/realtime/use-realtime-token", () => ({
  useRealtimeToken: (...args: unknown[]) => {
    state.realtimeCalls.push(args);
    return { token: state.tokenBySessionId.get(args[1] as string | undefined) ?? "canvas-token", unavailable: false };
  },
}));

vi.mock("@/lib/yjs/use-yjs-provider", () => ({
  useYjsProvider: (args: unknown) => {
    state.providerCalls.push(args);
    return { yDoc: state.yDoc, connected: true };
  },
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
    state.realtimeCalls.length = 0;
    state.providerCalls.length = 0;
    state.tokenBySessionId.clear();
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

  it("passes the selected canvas sessionId to the real token producer before mounting a provider", async () => {
    const canvasId = "22222222-2222-4222-8222-222222222222";
    const firstSessionId = "11111111-1111-4111-8111-111111111111";
    const secondSessionId = "33333333-3333-4333-8333-333333333333";
    const opts = (sessionId: string) => ({ canvasId, sessionId, readOnly: false }) as unknown as Parameters<typeof useWhiteboard>[0];
    state.tokenBySessionId.set(firstSessionId, "canvas-A");
    state.tokenBySessionId.set(secondSessionId, "");
    const { rerender } = renderHook(({ sessionId }) => useWhiteboard(opts(sessionId)), { initialProps: { sessionId: firstSessionId } });
    expect(state.realtimeCalls.at(-1)).toEqual([`canvas:${canvasId}`, firstSessionId]);
    expect(state.providerCalls.at(-1)).toMatchObject({ documentName: `canvas:${canvasId}`, token: "canvas-A" });
    rerender({ sessionId: secondSessionId });
    expect(state.realtimeCalls.at(-1)).toEqual([`canvas:${canvasId}`, secondSessionId]);
    expect(state.providerCalls.at(-1)).toMatchObject({ documentName: `canvas:${canvasId}`, token: "" });
  });

  it("keeps Zod root validation errors compatible with zod/v4", () => {
    expect(() => z.string().parse(42)).toThrow(ZodError);
  });
});

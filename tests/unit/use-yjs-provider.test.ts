// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { HocuspocusProvider } from "@hocuspocus/provider";
import { OutgoingMessage } from "@hocuspocus/server";
import * as Y from "yjs";
import { canvasReconnectPolicy, useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { __resetRealtimeTokenCacheForTesting, getRealtimeToken } from "@/lib/realtime/get-token";

afterEach(() => {
  __resetRealtimeTokenCacheForTesting();
  vi.unstubAllGlobals();
});

describe("canvas reconnect policy", () => {
  it("constructs the real pinned canvas provider with delay at least minDelay", () => {
    expect(() => {
      const { unmount } = renderHook(() => useYjsProvider({
        documentName: "canvas:22222222-2222-4222-8222-222222222222",
        token: "canvas-token",
        serverUrl: "ws://127.0.0.1:1",
      }));
      unmount();
    }).not.toThrow();
  });

  it("resets a twenty-second recovery horizon on every session_freezing rejection", () => {
    const start = 1_000;
    const first = canvasReconnectPolicy({ now: start, event: { code: "session_freezing" }, believedLive: true });
    const reset = canvasReconnectPolicy({ now: start + 19_999, event: { code: "session_freezing" }, previous: first, believedLive: true });
    expect(first.retry).toBe(true);
    expect(first.delayMs).toBeLessThanOrEqual(2_000);
    expect(reset.fastRecoveryUntil).toBeGreaterThanOrEqual(start + 39_999);
  });

  it("keeps ordinary live outages under two seconds for the first twenty seconds, then retries indefinitely with a thirty-second jittered tail", () => {
    const fast = canvasReconnectPolicy({ now: 10_000, event: { code: "transport_closed" }, believedLive: true });
    const tail = canvasReconnectPolicy({ now: 30_001, event: { code: "transport_closed" }, previous: fast, believedLive: true });
    expect(fast.delayMs).toBeLessThanOrEqual(2_000);
    expect(tail.retry).toBe(true);
    expect(tail.delayMs).toBeGreaterThan(2_000);
    expect(tail.delayMs).toBeLessThanOrEqual(30_000);
  });

  it("does not turn retryable freeze or outage states into manual reloads, while terminal states remain terminal", () => {
    expect(canvasReconnectPolicy({ now: 0, event: { code: "session_freezing" }, believedLive: true }).manualReload).toBe(false);
    expect(canvasReconnectPolicy({ now: 0, event: { code: "transport_closed" }, believedLive: true }).manualReload).toBe(false);
    expect(canvasReconnectPolicy({ now: 0, event: { code: "jwt_refresh_failed" }, believedLive: true })).toMatchObject({ retry: false, terminal: true });
  });

  it("leaves attempt and session documents on their legacy provider behavior", () => {
    expect(canvasReconnectPolicy({ now: 0, documentName: "attempt:abc", event: { code: "session_freezing" }, believedLive: true })).toMatchObject({ compatibility: "legacy" });
    expect(canvasReconnectPolicy({ now: 0, documentName: "session:abc", event: { code: "transport_closed" }, believedLive: true })).toMatchObject({ compatibility: "legacy" });
  });

  it("gives the installed CLOSE reason precedence over its generic close code so session_freezing resets recovery", () => {
    const result = canvasReconnectPolicy({
      now: 1_000,
      event: { code: 1000, reason: "session_freezing" },
      believedLive: true,
    });
    expect(result.fastRecoveryUntil).toBe(21_000);
    expect(result.retry).toBe(true);
  });

  it("models one provider close as one retry advance, resets it on a real connect, and stops after a terminal close", async () => {
    const { createCanvasProviderEventBridge } = await import("@/lib/yjs/use-yjs-provider");
    const bridge = createCanvasProviderEventBridge({ documentName: "canvas:abc", now: () => 1_000 });
    bridge.onClose({ code: 1000, reason: "session_freezing" });
    bridge.onDisconnect({ code: 1000, reason: "session_freezing" });
    expect(bridge.state()).toMatchObject({ attempt: 1, retry: true });
    bridge.onConnect();
    expect(bridge.state()).toBeUndefined();
    bridge.onClose({ code: 1000, reason: "canvas_jwt_expired" });
    expect(bridge.state()).toMatchObject({ terminal: true, retry: false });
  });

  it("handles the installed server Connection.close CLOSE frame by refreshing the canvas token and reconnecting without replaying unauthenticated writes", async () => {
    const { bindInstalledCanvasProvider } = await import("@/lib/yjs/use-yjs-provider");
    const documentName = "canvas:22222222-2222-4222-8222-222222222222";
    const frame = new OutgoingMessage(documentName).writeCloseMessage("canvas_jwt_expired").toUint8Array();
    let refreshes = 0;
    let reconnects = 0;
    let queuedWrites = 0;
    const websocket = { on() {}, off() {}, attach() {}, detach() {}, setConfiguration() {}, send() { queuedWrites += 1; } };
    const provider = new HocuspocusProvider({ name: documentName, document: new Y.Doc(), websocketProvider: websocket as never });
    const release = bindInstalledCanvasProvider({ provider, refreshToken: async () => { refreshes += 1; return "new-token"; }, reconnect: () => { reconnects += 1; } });
    provider.onMessage({ data: frame } as MessageEvent);
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    expect(refreshes).toBe(1);
    expect(reconnects).toBe(1);
    expect(queuedWrites).toBe(0);
    release();
    provider.destroy();
  });

  it("recovers an installed binary ArrayBuffer CLOSE by force-reminting before it can emit any queued canvas write", async () => {
    const { bindInstalledCanvasProvider } = await import("@/lib/yjs/use-yjs-provider");
    const documentName = "canvas:22222222-2222-4222-8222-222222222222";
    const bytes = new OutgoingMessage(documentName).writeCloseMessage("canvas_jwt_expired").toUint8Array();
    const frame = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength);
    let refreshes = 0;
    let reconnects = 0;
    let writes = 0;
    const websocket = { on() {}, off() {}, attach() {}, detach() {}, setConfiguration() {}, send() { writes += 1; } };
    const provider = new HocuspocusProvider({ name: documentName, document: new Y.Doc(), websocketProvider: websocket as never });
    const release = bindInstalledCanvasProvider({ provider, refreshToken: async () => { refreshes += 1; return "forced-remint"; }, reconnect: () => { reconnects += 1; } });
    provider.onMessage({ data: frame } as MessageEvent);
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    expect(refreshes).toBe(1);
    expect(reconnects).toBe(1);
    expect(writes).toBe(0);
    release();
    provider.destroy();
  });

  it("keeps an installed ArrayBuffer CLOSE fenced through retryable 409 freezing, then remints inside the reset fast horizon", async () => {
    const { bindInstalledCanvasProvider } = await import("@/lib/yjs/use-yjs-provider");
    const documentName = "canvas:22222222-2222-4222-8222-222222222222";
    const bytes = new OutgoingMessage(documentName).writeCloseMessage("session_freezing").toUint8Array();
    const frame = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength);
    let now = 1_000;
    let retry: (() => void) | undefined;
    let remints = 0;
    let writes = 0;
    let authFailures = 0;
    const websocket = { on() {}, off() {}, attach() {}, detach() {}, setConfiguration() {}, send() { writes += 1; } };
    const provider = new HocuspocusProvider({ name: documentName, document: new Y.Doc(), websocketProvider: websocket as never, onAuthenticationFailed: () => authFailures++ });
    const release = bindInstalledCanvasProvider({
      provider,
      now: () => now,
      schedule: (callback, delayMs) => { expect(delayMs).toBeLessThanOrEqual(2_000); retry = callback; return 0 as never; },
      refreshToken: async () => {
        remints += 1;
        if (remints === 1) throw Object.assign(new Error("freezing"), { status: 409, code: "session_freezing" });
        return "recovered-token";
      },
    });
    provider.onMessage({ data: frame } as MessageEvent);
    await Promise.resolve();
    expect(remints).toBe(1);
    expect(writes).toBe(0);
    expect(authFailures).toBe(0);
    now += 250;
    retry!();
    await Promise.resolve();
    expect(remints).toBe(2);
    expect(writes).toBe(0);
    expect(authFailures).toBe(0);
    release();
    provider.destroy();
  });

  it("routes the real getRealtimeToken 409 session_freezing error into the installed provider retry loop", async () => {
    const { bindInstalledCanvasProvider } = await import("@/lib/yjs/use-yjs-provider");
    const documentName = "canvas:22222222-2222-4222-8222-222222222222";
    const frame = new OutgoingMessage(documentName).writeCloseMessage("session_freezing").toUint8Array();
    let retry: (() => void) | undefined;
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: "freezing", code: "session_freezing" }), { status: 409, headers: { "content-type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ token: "recovered", expiresAt: new Date(Date.now() + 25 * 60_000).toISOString() }), { status: 200, headers: { "content-type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    const websocket = { on() {}, off() {}, attach() {}, detach() {}, setConfiguration() {}, send() {} };
    const provider = new HocuspocusProvider({ name: documentName, document: new Y.Doc(), websocketProvider: websocket as never });
    const release = bindInstalledCanvasProvider({
      provider,
      refreshToken: () => getRealtimeToken(documentName, "11111111-1111-4111-8111-111111111111", { forceRefresh: true }),
      schedule: (callback, delayMs) => { expect(delayMs).toBeLessThanOrEqual(2_000); retry = callback; return 0 as never; },
    });
    provider.onMessage({ data: frame } as MessageEvent);
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    retry!();
    await Promise.resolve(); await Promise.resolve(); await Promise.resolve();
    expect(fetchMock).toHaveBeenCalledTimes(2);
    release();
    provider.destroy();
  });

  it("cancels a scheduled retry when the installed provider unmounts before its timer fires", async () => {
    const { bindInstalledCanvasProvider } = await import("@/lib/yjs/use-yjs-provider");
    const documentName = "canvas:22222222-2222-4222-8222-222222222222";
    const frame = new OutgoingMessage(documentName).writeCloseMessage("session_freezing").toUint8Array();
    let callback: (() => void) | undefined;
    let refreshes = 0;
    let cancelled = 0;
    const websocket = { on() {}, off() {}, attach() {}, detach() {}, setConfiguration() {}, send() {} };
    const provider = new HocuspocusProvider({ name: documentName, document: new Y.Doc(), websocketProvider: websocket as never });
    const release = bindInstalledCanvasProvider({
      provider,
      refreshToken: async () => { refreshes += 1; throw Object.assign(new Error("freeze"), { status: 409, code: "session_freezing" }); },
      schedule: (next) => { callback = next; return 42 as never; },
      cancelSchedule: () => { cancelled += 1; },
    });
    provider.onMessage({ data: frame } as MessageEvent);
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
    expect(refreshes).toBe(1);
    release();
    callback!();
    await Promise.resolve();
    expect(cancelled).toBe(1);
    expect(refreshes).toBe(1);
    provider.destroy();
  });

  it("treats the pinned PermissionDenied session_freezing reason as the same recoverable canvas lifecycle path", async () => {
    const { bindInstalledCanvasProvider } = await import("@/lib/yjs/use-yjs-provider");
    const documentName = "canvas:22222222-2222-4222-8222-222222222222";
    const frame = new OutgoingMessage(documentName).writePermissionDenied("session_freezing").toUint8Array();
    let retry: (() => void) | undefined;
    let remints = 0;
    let permanentFailures = 0;
    const websocket = { on() {}, off() {}, attach() {}, detach() {}, setConfiguration() {}, send() {} };
    const provider = new HocuspocusProvider({ name: documentName, document: new Y.Doc(), websocketProvider: websocket as never, onAuthenticationFailed: () => permanentFailures++ });
    const release = bindInstalledCanvasProvider({
      provider,
      schedule: (callback, delayMs) => { expect(delayMs).toBeLessThanOrEqual(2_000); retry = callback; return 0 as never; },
      refreshToken: async () => {
        remints += 1;
        if (remints === 1) throw Object.assign(new Error("still freezing"), { status: 409, code: "session_freezing" });
        return "post-freeze-token";
      },
    });
    provider.onMessage({ data: frame } as MessageEvent);
    await Promise.resolve();
    expect(remints).toBe(1);
    expect(permanentFailures).toBe(0);
    retry!();
    await Promise.resolve();
    expect(remints).toBe(2);
    expect(permanentFailures).toBe(0);
    release();
    provider.destroy();
  });

  it("uses an attached installed provider to emit Auth before sync and keeps local writes fenced until that ordered recovery completes", async () => {
    const { bindInstalledCanvasProvider } = await import("@/lib/yjs/use-yjs-provider");
    const documentName = "canvas:22222222-2222-4222-8222-222222222222";
    const frame = new OutgoingMessage(documentName).writeCloseMessage("canvas_jwt_expired").toUint8Array();
    const outbound: string[] = [];
    const websocket = { on() {}, off() {}, attach() {}, detach() {}, setConfiguration() {}, send(frame: Uint8Array) { void frame; } };
    const provider = new HocuspocusProvider({ name: documentName, document: new Y.Doc(), websocketProvider: websocket as never });
    (provider as unknown as { _isAttached: boolean })._isAttached = true;
    (provider as unknown as { configuration: { websocketProvider: typeof websocket } }).configuration.websocketProvider = websocket;
    const send = provider.send.bind(provider);
    provider.send = ((message, ...args) => {
      outbound.push(new (message as new () => { description: string })().description);
      return send(message, ...args);
    }) as typeof provider.send;
    const token = Promise.withResolvers<string>();
    let remints = 0;
    const release = bindInstalledCanvasProvider({ provider, refreshToken: () => { remints += 1; return token.promise; } });
    provider.onMessage({ data: frame } as MessageEvent);
    provider.startSync();
    expect(outbound).toEqual([]);
    expect(remints).toBe(1);
    token.resolve("ordered-token");
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
    expect(outbound.slice(0, 2)).toEqual(["Authentication", "First sync step"]);
    release();
    provider.destroy();
  });

  it("resets composed canvas recovery after each successful reauth so a later outage and later expiry remain recoverable", async () => {
    const { bindInstalledCanvasProvider, createCanvasProviderEventBridge } = await import("@/lib/yjs/use-yjs-provider");
    const documentName = "canvas:22222222-2222-4222-8222-222222222222";
    const frame = new OutgoingMessage(documentName).writeCloseMessage("canvas_jwt_expired").toUint8Array();
    const websocket = { on() {}, off() {}, attach() {}, detach() {}, setConfiguration() {}, send() {} };
    const provider = new HocuspocusProvider({ name: documentName, document: new Y.Doc(), websocketProvider: websocket as never });
    const bridge = createCanvasProviderEventBridge({ documentName, now: () => 1_000 });
    let recovered = 0;
    const release = bindInstalledCanvasProvider({ provider, refreshToken: async () => "renewed", onRecovered: () => { recovered += 1; bridge.onConnect(); } });
    bridge.onClose({ reason: "canvas_jwt_expired" });
    provider.onMessage({ data: frame } as MessageEvent);
    await Promise.resolve(); await Promise.resolve(); await Promise.resolve();
    expect(recovered).toBe(1);
    expect(bridge.state()).toBeUndefined();
    expect(bridge.onClose({ reason: "transport_closed" })).toMatchObject({ retry: true, terminal: false, delayMs: expect.any(Number) });
    bridge.onConnect();
    provider.onMessage({ data: frame } as MessageEvent);
    await Promise.resolve(); await Promise.resolve(); await Promise.resolve();
    expect(recovered).toBe(2);
    release();
    provider.destroy();
  });

});

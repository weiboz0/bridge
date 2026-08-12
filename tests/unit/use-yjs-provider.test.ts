// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { canvasReconnectPolicy } from "@/lib/yjs/use-yjs-provider";

describe("canvas reconnect policy", () => {
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
});

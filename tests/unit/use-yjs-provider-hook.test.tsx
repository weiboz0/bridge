// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { OutgoingMessage } from "@hocuspocus/server";

const state = vi.hoisted(() => ({ providers: [] as Array<Record<string, unknown>> }));

vi.mock("@hocuspocus/provider", () => {
  class Provider {
    _isAttached = true;
    destroyed = false;
    readonly listeners = new Map<string, Set<(input: Record<string, unknown>) => void>>();
    readonly configurations: Array<Record<string, unknown>> = [];
    readonly configuration: Record<string, unknown>;
    constructor(configuration: Record<string, unknown>) {
      this.configuration = {
        ...configuration,
        websocketProvider: { setConfiguration: (next: Record<string, unknown>) => this.configurations.push(next) },
      };
      this.on("authenticationFailed", configuration.onAuthenticationFailed as (input: Record<string, unknown>) => void);
      state.providers.push(this as unknown as Record<string, unknown>);
    }
    get isAttached() { return this._isAttached; }
    on(name: string, listener: (input: Record<string, unknown>) => void) { (this.listeners.get(name) ?? this.listeners.set(name, new Set()).get(name)!).add(listener); }
    off(name: string, listener: (input: Record<string, unknown>) => void) { this.listeners.get(name)?.delete(listener); }
    setConfiguration(next: Record<string, unknown>) { Object.assign(this.configuration, next); this.configurations.push(next); }
    send() {}
    async sendToken() {}
    startSync() {}
    onMessage() {}
    destroy() { this.destroyed = true; this._isAttached = false; this.listeners.clear(); }
  }
  return { HocuspocusProvider: Provider };
});

import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";

afterEach(() => { state.providers.length = 0; });

describe("useYjsProvider installed canvas recovery wiring", () => {
  it("registers recovery on the managed provider, resets live retry after reauth, and cleans it up", async () => {
    const documentName = "canvas:22222222-2222-4222-8222-222222222222";
    let remints = 0;
    const refreshToken = vi.fn(async () => `token-${++remints}`);
    const { result, unmount } = renderHook(() => useYjsProvider({
      documentName,
      token: "initial",
      refreshToken,
    }));
    await waitFor(() => expect(result.current.provider).not.toBeNull());
    const provider = state.providers[0] as unknown as {
      onMessage(event: MessageEvent): void;
      configuration: { onClose(input: { event: { code: number; reason: string } }): void };
      configurations: Array<Record<string, unknown>>;
      destroyed: boolean;
      isAttached: boolean;
      listeners: Map<string, Set<unknown>>;
    };
    expect(provider.isAttached).toBe(true);

    const expiry = new OutgoingMessage(documentName).writeCloseMessage("canvas_jwt_expired").toUint8Array();
    act(() => provider.onMessage({ data: expiry } as MessageEvent));
    await waitFor(() => expect(remints).toBe(1));
    expect(provider.configurations).toContainEqual({ token: "token-1" });

    (provider.configuration.onClose as (input: { event: { code: number; reason: string } }) => void)({ event: { code: 1006, reason: "transport_closed" } });
    expect(provider.configurations).toContainEqual(expect.objectContaining({ maxAttempts: 0 }));

    act(() => provider.onMessage({ data: expiry } as MessageEvent));
    await waitFor(() => expect(remints).toBe(2));
    unmount();
    expect(provider.destroyed).toBe(true);
    expect(provider.isAttached).toBe(false);
    expect(provider.listeners.get("authenticationFailed")?.size ?? 0).toBe(0);
  });
});

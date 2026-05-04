// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, render } from "@testing-library/react";
import { __resetRealtimeTokenCacheForTesting } from "../../src/lib/realtime/get-token";
import { useRealtimeToken } from "../../src/lib/realtime/use-realtime-token";

function mintResponse(token: string, expiresInMs = 25 * 60 * 1000): Response {
  return new Response(
    JSON.stringify({ token, expiresAt: new Date(Date.now() + expiresInMs).toISOString() }),
    { status: 200, headers: { "content-type": "application/json" } },
  );
}

function HookHarness({ docName, onToken, onUnavailable }: { docName: string; onToken: (t: string) => void; onUnavailable?: (u: boolean) => void }) {
  const { token, unavailable } = useRealtimeToken(docName);
  onToken(token);
  onUnavailable?.(unavailable);
  return null;
}

describe("useRealtimeToken", () => {
  beforeEach(() => {
    __resetRealtimeTokenCacheForTesting();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns empty string until the mint resolves, then the token", async () => {
    let resolveFetch: (r: Response) => void = () => {};
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation(
        () => new Promise<Response>((r) => { resolveFetch = r; }),
      ),
    );

    const tokens: string[] = [];
    render(<HookHarness docName="unit:abc" onToken={(t) => tokens.push(t)} />);

    // First render: empty.
    expect(tokens.at(-1)).toBe("");

    // Resolve the mint — flush.
    await act(async () => {
      resolveFetch(mintResponse("ey.tok"));
    });
    expect(tokens.at(-1)).toBe("ey.tok");
  });

  it("returns empty string for noop or empty docName", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    const tokens: string[] = [];
    render(<HookHarness docName="noop" onToken={(t) => tokens.push(t)} />);
    expect(tokens.at(-1)).toBe("");
    expect(fetchMock).not.toHaveBeenCalled();

    const tokens2: string[] = [];
    render(<HookHarness docName="" onToken={(t) => tokens2.push(t)} />);
    expect(tokens2.at(-1)).toBe("");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("CLEARS the previous token when docName changes (A→B race fix)", async () => {
    // First mint resolves quickly, second is slow → during the second
    // mint window we want to see "" not the stale A token.
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mintResponse("tok-A"))
      .mockImplementationOnce(
        () => new Promise<Response>(() => { /* never resolves */ }),
      );
    vi.stubGlobal("fetch", fetchMock);

    const tokens: string[] = [];
    const { rerender } = render(
      <HookHarness docName="unit:A" onToken={(t) => tokens.push(t)} />,
    );

    // Wait for tok-A to land.
    await act(async () => { await Promise.resolve(); });
    await act(async () => { await Promise.resolve(); });
    expect(tokens.at(-1)).toBe("tok-A");

    // Switch to docName=B. The mint for B never resolves, so we
    // expect the hook to flip back to "" synchronously — NOT keep
    // returning tok-A.
    await act(async () => {
      rerender(<HookHarness docName="unit:B" onToken={(t) => tokens.push(t)} />);
    });
    expect(tokens.at(-1), "stale tok-A must be cleared during B's mint window").toBe("");
  });

  it("plan 068 phase 4: surfaces unavailable=true when mint returns 503", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValueOnce(
        new Response("Realtime tokens not configured", { status: 503 })
      ),
    );

    const tokens: string[] = [];
    const unavailables: boolean[] = [];
    render(
      <HookHarness
        docName="unit:not-configured"
        onToken={(t) => tokens.push(t)}
        onUnavailable={(u) => unavailables.push(u)}
      />,
    );

    // Suppress the expected console.error from the catch handler.
    const errSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    await act(async () => { await Promise.resolve(); });
    await act(async () => { await Promise.resolve(); });
    errSpy.mockRestore();

    expect(tokens.at(-1)).toBe("");
    expect(unavailables.at(-1)).toBe(true);
  });

  it.each([
    ["500 server error", () => Promise.resolve(new Response("Server error", { status: 500 }))],
    ["401 unauthorized", () => Promise.resolve(new Response("Unauthorized", { status: 401 }))],
    ["network error (fetch throws)", () => Promise.reject(new TypeError("Network error"))],
  ])(
    "plan 068 phase 4: unavailable stays false on non-503 (%s)",
    async (_label, fetchImpl) => {
      vi.stubGlobal("fetch", vi.fn().mockImplementationOnce(fetchImpl));

      const tokens: string[] = [];
      const unavailables: boolean[] = [];
      render(
        <HookHarness
          docName="unit:non-503"
          onToken={(t) => tokens.push(t)}
          onUnavailable={(u) => unavailables.push(u)}
        />,
      );

      const errSpy = vi.spyOn(console, "error").mockImplementation(() => {});
      await act(async () => { await Promise.resolve(); });
      await act(async () => { await Promise.resolve(); });
      errSpy.mockRestore();

      expect(tokens.at(-1)).toBe("");
      expect(unavailables.at(-1)).toBe(false);
    }
  );

  it("does not call setToken after unmount (cancelled flag)", async () => {
    let resolveFetch: (r: Response) => void = () => {};
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation(
        () => new Promise<Response>((r) => { resolveFetch = r; }),
      ),
    );
    const tokens: string[] = [];
    const { unmount } = render(
      <HookHarness docName="unit:unmount" onToken={(t) => tokens.push(t)} />,
    );

    unmount();
    // Resolve AFTER unmount — should NOT trigger any further onToken callbacks.
    const beforeResolve = tokens.length;
    await act(async () => {
      resolveFetch(mintResponse("after-unmount"));
    });
    expect(tokens.length).toBe(beforeResolve);
  });
});

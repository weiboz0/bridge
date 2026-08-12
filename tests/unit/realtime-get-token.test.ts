import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  __resetRealtimeTokenCacheForTesting,
  getRealtimeToken,
  RealtimeMintError,
} from "../../src/lib/realtime/get-token";

function mintResponse(token: string, expiresInMs: number): Response {
  return new Response(
    JSON.stringify({
      token,
      expiresAt: new Date(Date.now() + expiresInMs).toISOString(),
    }),
    { status: 200, headers: { "content-type": "application/json" } },
  );
}

describe("getRealtimeToken", () => {
  beforeEach(() => {
    __resetRealtimeTokenCacheForTesting();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it("rejects empty / noop documentName without calling fetch", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    await expect(getRealtimeToken("")).rejects.toThrow(/documentName is required/);
    await expect(getRealtimeToken("noop")).rejects.toThrow(/documentName is required/);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("posts to /api/realtime/token with credentials and returns the token", async () => {
    const fetchMock = vi.fn().mockResolvedValue(mintResponse("ey.fake.tok", 25 * 60 * 1000));
    vi.stubGlobal("fetch", fetchMock);

    const tok = await getRealtimeToken("chapter:abc");
    expect(tok).toBe("ey.fake.tok");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/realtime/token");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body)).toEqual({ documentName: "chapter:abc" });
  });

  it("caches by documentName — second call within TTL doesn't refetch", async () => {
    const fetchMock = vi.fn().mockResolvedValue(mintResponse("ey.tok-1", 25 * 60 * 1000));
    vi.stubGlobal("fetch", fetchMock);

    await getRealtimeToken("chapter:abc");
    await getRealtimeToken("chapter:abc");
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("does NOT confuse different doc-names — separate fetches", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mintResponse("tok-A", 25 * 60 * 1000))
      .mockResolvedValueOnce(mintResponse("tok-B", 25 * 60 * 1000));
    vi.stubGlobal("fetch", fetchMock);

    const [a, b] = await Promise.all([
      getRealtimeToken("chapter:A"),
      getRealtimeToken("chapter:B"),
    ]);
    expect(a).toBe("tok-A");
    expect(b).toBe("tok-B");
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("uses the canvas sessionId in both the mint body and cache identity", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mintResponse("canvas-session-A", 25 * 60 * 1000))
      .mockResolvedValueOnce(mintResponse("canvas-session-B", 25 * 60 * 1000));
    vi.stubGlobal("fetch", fetchMock);
    const getCanvasToken = getRealtimeToken as (documentName: string, sessionId?: string) => Promise<string>;
    const documentName = "canvas:22222222-2222-4222-8222-222222222222";
    await expect(getCanvasToken(documentName, "11111111-1111-4111-8111-111111111111")).resolves.toBe("canvas-session-A");
    await expect(getCanvasToken(documentName, "33333333-3333-4333-8333-333333333333")).resolves.toBe("canvas-session-B");
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ documentName, sessionId: "11111111-1111-4111-8111-111111111111" });
  });

  it("does not de-dupe in-flight canvas mints across different sessionIds", async () => {
    const resolvers: Array<(response: Response) => void> = [];
    const fetchMock = vi.fn().mockImplementation(() => new Promise<Response>((resolve) => resolvers.push(resolve)));
    vi.stubGlobal("fetch", fetchMock);
    const getCanvasToken = getRealtimeToken as (documentName: string, sessionId?: string) => Promise<string>;
    const documentName = "canvas:22222222-2222-4222-8222-222222222222";
    const first = getCanvasToken(documentName, "11111111-1111-4111-8111-111111111111");
    const second = getCanvasToken(documentName, "33333333-3333-4333-8333-333333333333");
    expect(fetchMock).toHaveBeenCalledTimes(2);
    resolvers[0](mintResponse("first", 25 * 60 * 1000));
    resolvers[1](mintResponse("second", 25 * 60 * 1000));
    await expect(first).resolves.toBe("first");
    await expect(second).resolves.toBe("second");
  });

  it("dedupes in-flight requests for the same doc-name", async () => {
    let resolveFetch: (r: Response) => void = () => {};
    const fetchMock = vi.fn().mockImplementation(
      () =>
        new Promise<Response>((resolve) => {
          resolveFetch = resolve;
        }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const p1 = getRealtimeToken("chapter:race");
    const p2 = getRealtimeToken("chapter:race");
    const p3 = getRealtimeToken("chapter:race");
    resolveFetch(mintResponse("ey.shared", 25 * 60 * 1000));
    const [a, b, c] = await Promise.all([p1, p2, p3]);
    expect([a, b, c]).toEqual(["ey.shared", "ey.shared", "ey.shared"]);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("refreshes when the cached token is within the leeway window", async () => {
    // First mint expires in 30s — INSIDE the 60s leeway → should
    // be considered stale on the next call.
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mintResponse("first", 30 * 1000))
      .mockResolvedValueOnce(mintResponse("second", 25 * 60 * 1000));
    vi.stubGlobal("fetch", fetchMock);

    await getRealtimeToken("chapter:short");
    const next = await getRealtimeToken("chapter:short");
    expect(next).toBe("second");
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("translates 503 into a RealtimeMintError with status 503", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "Realtime tokens not configured" }), {
          status: 503,
          headers: { "content-type": "application/json" },
        }),
      ),
    );
    await expect(getRealtimeToken("chapter:abc")).rejects.toMatchObject({
      name: "RealtimeMintError",
      status: 503,
      message: expect.stringContaining("not configured"),
    });
  });

  it("translates 403 into a RealtimeMintError with status 403", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "Not authorized" }), {
          status: 403,
          headers: { "content-type": "application/json" },
        }),
      ),
    );
    await expect(getRealtimeToken("attempt:foo")).rejects.toMatchObject({
      status: 403,
      message: expect.stringContaining("403 Not authorized"),
    });
  });

  it("translates network errors into RealtimeMintError", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockRejectedValue(new TypeError("connection refused")),
    );
    await expect(getRealtimeToken("chapter:abc")).rejects.toThrow(/network error.*connection refused/);
  });

  it("clears in-flight on failure so the next caller can retry", async () => {
    let attempt = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation(() => {
        attempt += 1;
        if (attempt === 1) {
          return Promise.resolve(
            new Response(JSON.stringify({ error: "transient" }), { status: 500 }),
          );
        }
        return Promise.resolve(mintResponse("recovered", 25 * 60 * 1000));
      }),
    );
    await expect(getRealtimeToken("chapter:abc")).rejects.toThrow();
    // Second call must NOT see the cached failure — should re-fetch.
    const tok = await getRealtimeToken("chapter:abc");
    expect(tok).toBe("recovered");
  });

  it("rejects responses missing token or expiresAt", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ token: "abc" /* no expiresAt */ }), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      ),
    );
    await expect(getRealtimeToken("chapter:abc")).rejects.toThrow(/missing fields/);
  });

  it("rejects responses with unparseable expiresAt", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ token: "abc", expiresAt: "not-a-date" }), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      ),
    );
    await expect(getRealtimeToken("chapter:abc")).rejects.toThrow(/unparseable/);
  });

  it("RealtimeMintError class is exported and named", () => {
    const err = new RealtimeMintError("test", 400);
    expect(err.name).toBe("RealtimeMintError");
    expect(err.status).toBe(400);
  });
});

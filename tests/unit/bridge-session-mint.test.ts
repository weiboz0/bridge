import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  bridgeSessionExpiringSoon,
  mintBridgeSession,
} from "@/lib/bridge-session-mint";

// Helper: build a fake JWT with a given exp (in unix seconds). We
// don't need a real signature — the Edge helper trusts our own
// cookie's payload format and never verifies.
function fakeJwt(payload: Record<string, unknown>): string {
  const header = base64Url(JSON.stringify({ alg: "HS256", typ: "JWT" }));
  const body = base64Url(JSON.stringify(payload));
  const sig = base64Url("fake-signature");
  return `${header}.${body}.${sig}`;
}

function base64Url(s: string): string {
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

describe("bridgeSessionExpiringSoon", () => {
  it("returns true when token is undefined", () => {
    expect(bridgeSessionExpiringSoon(undefined)).toBe(true);
  });

  it("returns true when token is empty string", () => {
    expect(bridgeSessionExpiringSoon("")).toBe(true);
  });

  it("returns true when token has wrong number of segments", () => {
    expect(bridgeSessionExpiringSoon("just.two")).toBe(true);
    expect(bridgeSessionExpiringSoon("one")).toBe(true);
    expect(bridgeSessionExpiringSoon("a.b.c.d")).toBe(true);
  });

  it("returns true when payload is not valid JSON", () => {
    const tok = `${base64Url(`{"alg":"HS256"}`)}.${base64Url("not-json")}.${base64Url("sig")}`;
    expect(bridgeSessionExpiringSoon(tok)).toBe(true);
  });

  it("returns true when payload has no exp claim", () => {
    const tok = fakeJwt({ sub: "u1" });
    expect(bridgeSessionExpiringSoon(tok)).toBe(true);
  });

  it("returns true when exp is in the past", () => {
    const exp = Math.floor(Date.now() / 1000) - 60;
    const tok = fakeJwt({ exp });
    expect(bridgeSessionExpiringSoon(tok)).toBe(true);
  });

  it("returns true when exp is within 24h of now (approaching threshold)", () => {
    const exp = Math.floor(Date.now() / 1000) + 12 * 60 * 60; // 12h from now
    const tok = fakeJwt({ exp });
    expect(bridgeSessionExpiringSoon(tok)).toBe(true);
  });

  it("returns false when exp is well beyond the 24h threshold", () => {
    const exp = Math.floor(Date.now() / 1000) + 6 * 24 * 60 * 60; // 6 days
    const tok = fakeJwt({ exp });
    expect(bridgeSessionExpiringSoon(tok)).toBe(false);
  });

  it("returns true at exactly the 24h threshold (boundary case)", () => {
    // Exactly at the boundary should re-mint (we'd rather over-mint
    // by a hair than serve a token that flips to expired between
    // the check and the request reaching Go).
    const exp = Math.floor(Date.now() / 1000) + 24 * 60 * 60;
    const tok = fakeJwt({ exp });
    expect(bridgeSessionExpiringSoon(tok)).toBe(true);
  });

  it("returns false when exp is far in the future (7 days)", () => {
    const exp = Math.floor(Date.now() / 1000) + 7 * 24 * 60 * 60;
    const tok = fakeJwt({ exp });
    expect(bridgeSessionExpiringSoon(tok)).toBe(false);
  });
});

describe("mintBridgeSession", () => {
  const originalFetch = globalThis.fetch;
  const originalEnv = { ...process.env };

  beforeEach(() => {
    process.env.BRIDGE_INTERNAL_SECRET = "test-bearer-secret";
    process.env.GO_INTERNAL_API_URL = "http://localhost:8002";
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    process.env = { ...originalEnv };
    vi.restoreAllMocks();
  });

  it("returns null when BRIDGE_INTERNAL_SECRET is unset (skips network)", async () => {
    delete process.env.BRIDGE_INTERNAL_SECRET;
    const fetchSpy = vi.fn();
    globalThis.fetch = fetchSpy as unknown as typeof fetch;

    const result = await mintBridgeSession({ email: "u@example.com", name: "User" });
    expect(result).toBeNull();
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("posts to /api/internal/sessions with bearer + JSON body", async () => {
    const expiresAt = new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString();
    const fetchSpy = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify({ token: "minted.jwt.here", expiresAt }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
    );
    globalThis.fetch = fetchSpy as unknown as typeof fetch;

    const result = await mintBridgeSession({ email: "u@example.com", name: "Test User" });

    expect(result).not.toBeNull();
    expect(result!.token).toBe("minted.jwt.here");
    expect(result!.expiresAt).toBeInstanceOf(Date);

    expect(fetchSpy).toHaveBeenCalledTimes(1);
    const call = fetchSpy.mock.calls[0]!;
    const url = call[0];
    const init = call[1]!;
    expect(url).toBe("http://localhost:8002/api/internal/sessions");
    expect(init.method).toBe("POST");
    const headers = init.headers as Record<string, string>;
    expect(headers["Authorization"]).toBe("Bearer test-bearer-secret");
    expect(headers["Content-Type"]).toBe("application/json");
    const body = JSON.parse(init.body as string);
    expect(body).toEqual({ email: "u@example.com", name: "Test User" });
  });

  it("returns null when Go responds non-2xx", async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response("server error", { status: 500 })
    ) as unknown as typeof fetch;

    const result = await mintBridgeSession({ email: "u@example.com", name: "User" });
    expect(result).toBeNull();
  });

  it("returns null when response body is missing token or expiresAt", async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ token: "x" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    ) as unknown as typeof fetch;

    const result = await mintBridgeSession({ email: "u@example.com", name: "User" });
    expect(result).toBeNull();
  });

  it("returns null when expiresAt is unparseable", async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ token: "x", expiresAt: "not-a-date" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    ) as unknown as typeof fetch;

    const result = await mintBridgeSession({ email: "u@example.com", name: "User" });
    expect(result).toBeNull();
  });

  it("returns null when fetch throws (network error)", async () => {
    globalThis.fetch = vi.fn(async () => {
      throw new Error("ECONNREFUSED");
    }) as unknown as typeof fetch;

    const result = await mintBridgeSession({ email: "u@example.com", name: "User" });
    expect(result).toBeNull();
  });

  it("uses GO_INTERNAL_API_URL override when set", async () => {
    process.env.GO_INTERNAL_API_URL = "http://prod-go.internal:9090";
    const fetchSpy = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(
          JSON.stringify({
            token: "x",
            expiresAt: new Date(Date.now() + 86400000).toISOString(),
          }),
          { status: 200 }
        )
    );
    globalThis.fetch = fetchSpy as unknown as typeof fetch;

    await mintBridgeSession({ email: "u@example.com", name: "User" });
    const call = fetchSpy.mock.calls[0]!;
    expect(call[0]).toBe("http://prod-go.internal:9090/api/internal/sessions");
  });
});

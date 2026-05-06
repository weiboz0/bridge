import { test, expect } from "@playwright/test";
import { createHmac } from "node:crypto";
import { HocuspocusProvider, HocuspocusProviderWebsocket } from "@hocuspocus/provider";
// `ws` package has no shipped types; cast at the import site.
// eslint-disable-next-line @typescript-eslint/no-require-imports
const WebSocket: typeof globalThis.WebSocket = require("ws");
import { ACCOUNTS, loginWithCredentials } from "./helpers";

/**
 * Plan 053 phase 3 — Hocuspocus signed-token e2e ratchet.
 *
 *   HTTP-side
 *     1. Authenticated user can mint a JWT for a unit-scope doc.
 *     2. Unauthenticated POST returns 401 (or 503 if secret unset).
 *     3. Unauthorized doc-name returns 403 / 404.
 *
 *   WS-side (the round-trip path the plan promised)
 *     4. A valid JWT (minted by the Go API) gets the connection
 *        accepted — exercise the full mint → verify → DB-recheck path.
 *     5. A forged JWT (signed with the WRONG secret) is rejected.
 *     6. An expired JWT (signed correctly, exp in the past) is
 *        rejected.
 *
 * The WS tests run in the Playwright Node runner using `@hocuspocus/provider`
 * directly (with the `ws` package as the WebSocket implementation). They
 * skip gracefully when HOCUSPOCUS_TOKEN_SECRET is unavailable to the
 * runner.
 */

const HOCUSPOCUS_URL = process.env.NEXT_PUBLIC_HOCUSPOCUS_URL ?? "ws://localhost:4000";
const TOKEN_SECRET = process.env.HOCUSPOCUS_TOKEN_SECRET ?? "";
const REALTIME_ISSUER = "bridge-platform";

function b64url(s: string): string {
  return Buffer.from(s).toString("base64url");
}

function mintRealtimeJwt(args: {
  secret: string;
  sub: string;
  role: string;
  scope: string;
  ttlSeconds: number;
  iatOffsetSeconds?: number;
}): string {
  const header = b64url(JSON.stringify({ alg: "HS256", typ: "JWT" }));
  const now = Math.floor(Date.now() / 1000) + (args.iatOffsetSeconds ?? 0);
  const payload = b64url(
    JSON.stringify({
      iss: REALTIME_ISSUER,
      sub: args.sub,
      role: args.role,
      scope: args.scope,
      iat: now,
      exp: now + args.ttlSeconds,
    }),
  );
  const sig = createHmac("sha256", args.secret).update(`${header}.${payload}`).digest("base64url");
  return `${header}.${payload}.${sig}`;
}

interface ConnectOutcome {
  outcome: "connected" | "authFailed" | "closed" | "timeout";
  reason?: string;
}

// Try to open a Hocuspocus connection in the Playwright Node runner.
// Returns the first lifecycle event observed within `timeoutMs`.
async function tryConnect(
  url: string,
  token: string,
  documentName: string,
  timeoutMs = 4000,
): Promise<ConnectOutcome> {
  return await new Promise<ConnectOutcome>((resolve) => {
    let settled = false;
    const settle = (o: ConnectOutcome) => {
      if (settled) return;
      settled = true;
      try { provider.destroy(); } catch { /* noop */ }
      resolve(o);
    };
    // HocuspocusProvider can take either a `url` (creates the WS
    // internally with `globalThis.WebSocket`) or a pre-built
    // `websocketProvider` where you can plug in a polyfill — needed
    // here because the Playwright Node runner has no native
    // `WebSocket`. The `ws` package fills it in.
    const websocketProvider = new HocuspocusProviderWebsocket({
      url,
      WebSocketPolyfill: WebSocket,
    });
    // IMPORTANT: settle the SUCCESS path on `onAuthenticated`, NOT
    // `onConnect`. The provider fires `onConnect` on the FIRST
    // server message, BEFORE the server has accepted or rejected
    // the auth token (Hocuspocus sends a permission-denied message
    // for failed auth, but `onConnect` has already fired by then).
    // Resolving on `onConnect` would let forged/expired-token tests
    // spuriously settle as "connected" before the auth failure
    // event lands.
    const provider = new HocuspocusProvider({
      websocketProvider,
      name: documentName,
      token,
      onAuthenticated: () => settle({ outcome: "connected" }),
      onAuthenticationFailed: ({ reason }: { reason: string }) =>
        settle({ outcome: "authFailed", reason }),
      onClose: () => settle({ outcome: "closed" }),
    });
    setTimeout(() => settle({ outcome: "timeout" }), timeoutMs);
  });
}

test.describe("Plan 053 phase 3 — realtime token mint (HTTP)", () => {
  test("authenticated user can mint a JWT for a unit doc-name", async ({ page }) => {
    await loginWithCredentials(page, ACCOUNTS.teacher.email, ACCOUNTS.teacher.password);

    const unitsRes = await page.request.get("/api/me/units");
    if (!unitsRes.ok()) {
      test.skip(true, "teacher has no units accessible — skipping mint spec");
    }
    const unitsBody = (await unitsRes.json()) as { units?: Array<{ id: string }> };
    const unitId = unitsBody.units?.[0]?.id;
    test.skip(!unitId, "teacher has no units — skipping mint spec");

    const documentName = `unit:${unitId}`;
    const res = await page.request.post("/api/realtime/token", {
      data: { documentName },
      headers: { "Content-Type": "application/json" },
    });

    if (res.status() === 503) {
      throw new Error(
        "Phase 1 of plan 072 made HOCUSPOCUS_TOKEN_SECRET required-in-production. " +
          "CI must provision this secret. A 503 from the mint endpoint is now a " +
          "configuration failure, not a skip condition.",
      );
    }

    expect(res.status(), `mint should return 200, got ${res.status()} (${await res.text()})`).toBe(200);
    const body = (await res.json()) as { token: string; expiresAt: string };

    expect(body.token).toMatch(/^ey[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/);
    const exp = new Date(body.expiresAt).getTime();
    expect(Number.isNaN(exp)).toBe(false);
    expect(exp - Date.now()).toBeGreaterThan(60_000);
    expect(exp - Date.now()).toBeLessThan(40 * 60_000);
  });

  test("unauthenticated request to mint endpoint returns 401", async ({ request }) => {
    const res = await request.post("/api/realtime/token", {
      data: { documentName: "unit:any" },
      headers: { "Content-Type": "application/json" },
    });
    expect(res.status()).toBe(401);
  });

  test("authenticated request for an unauthorized doc-name returns 403/404", async ({ page }) => {
    await loginWithCredentials(page, ACCOUNTS.student.email, ACCOUNTS.student.password);
    const res = await page.request.post("/api/realtime/token", {
      data: { documentName: "unit:00000000-0000-0000-0000-000000000000" },
      headers: { "Content-Type": "application/json" },
    });
    if (res.status() === 503) {
      throw new Error(
        "Phase 1 of plan 072 made HOCUSPOCUS_TOKEN_SECRET required-in-production. " +
          "CI must provision this secret. A 503 from the mint endpoint is now a " +
          "configuration failure, not a skip condition.",
      );
    }
    expect([403, 404]).toContain(res.status());
  });
});

test.describe("Plan 053 phase 3 — Hocuspocus WebSocket auth", () => {
  test.beforeAll(async () => {
    test.skip(!TOKEN_SECRET, "HOCUSPOCUS_TOKEN_SECRET unset — skipping WS auth ratchet");
  });

  test("VALID JWT — connection accepted", async ({ page }) => {
    await loginWithCredentials(page, ACCOUNTS.teacher.email, ACCOUNTS.teacher.password);

    // Resolve a unit the teacher can edit.
    const unitsRes = await page.request.get("/api/me/units");
    test.skip(!unitsRes.ok(), "/api/me/units unavailable");
    const { units } = (await unitsRes.json()) as { units?: Array<{ id: string }> };
    const unitId = units?.[0]?.id;
    test.skip(!unitId, "no units to test against");

    const documentName = `unit:${unitId}`;

    // Mint via the real Go API so the JWT round-trips through the
    // production code path. Then drop into Node-side provider.
    const mintRes = await page.request.post("/api/realtime/token", {
      data: { documentName },
      headers: { "Content-Type": "application/json" },
    });
    expect(mintRes.status()).toBe(200);
    const { token } = (await mintRes.json()) as { token: string };

    const result = await tryConnect(HOCUSPOCUS_URL, token, documentName);
    expect(result.outcome, `valid JWT should connect, got: ${JSON.stringify(result)}`).toBe(
      "connected",
    );
  });

  test("FORGED JWT — wrong secret signs → auth fails", async () => {
    const documentName = "unit:00000000-0000-0000-0000-000000000099";
    const forged = mintRealtimeJwt({
      secret: "WRONG-SECRET-attacker-does-not-have-the-real-one",
      sub: "any-user",
      role: "teacher",
      scope: documentName,
      ttlSeconds: 60,
    });

    const result = await tryConnect(HOCUSPOCUS_URL, forged, documentName);
    expect(
      result.outcome,
      `forged JWT must NOT connect, got: ${JSON.stringify(result)}`,
    ).not.toBe("connected");
  });

  test("EXPIRED JWT — correct signature but exp in the past → auth fails", async () => {
    const documentName = "unit:00000000-0000-0000-0000-000000000099";
    const expired = mintRealtimeJwt({
      secret: TOKEN_SECRET,
      sub: "any-user",
      role: "teacher",
      scope: documentName,
      ttlSeconds: -3600, // exp in the past
      iatOffsetSeconds: -7200,
    });

    const result = await tryConnect(HOCUSPOCUS_URL, expired, documentName);
    expect(
      result.outcome,
      `expired JWT must NOT connect, got: ${JSON.stringify(result)}`,
    ).not.toBe("connected");
  });
});

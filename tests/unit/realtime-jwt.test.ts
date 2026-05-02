import { createHmac } from "node:crypto";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  isLikelyJwt,
  JwtVerifyError,
  REALTIME_ISSUER,
  rechckDocumentAccess,
  verifyRealtimeJwt,
} from "../../server/realtime-jwt";

const SECRET = "phase2-test-secret-do-not-leak-into-production";

function b64url(buf: Buffer | string): string {
  const b = typeof buf === "string" ? Buffer.from(buf) : buf;
  return b.toString("base64url");
}

interface MintArgs {
  alg?: string;
  typ?: string;
  iss?: string;
  sub?: string;
  role?: string;
  scope?: string;
  iat?: number;
  exp?: number;
  signWith?: string;
  signingPayload?: string; // override the bytes the HMAC covers (for tamper tests)
}

function mint(args: MintArgs = {}): string {
  const header = {
    alg: args.alg ?? "HS256",
    typ: args.typ ?? "JWT",
  };
  const now = Math.floor(Date.now() / 1000);
  const payload: Record<string, unknown> = {
    iss: args.iss ?? REALTIME_ISSUER,
    iat: args.iat ?? now,
    exp: args.exp ?? now + 25 * 60,
  };
  if (args.sub !== undefined) payload.sub = args.sub;
  else payload.sub = "u-1";
  if (args.role !== undefined) payload.role = args.role;
  else payload.role = "user";
  if (args.scope !== undefined) payload.scope = args.scope;
  else payload.scope = "unit:abc";

  const encHeader = b64url(JSON.stringify(header));
  const encPayload = b64url(JSON.stringify(payload));
  const signedBase = args.signingPayload ?? `${encHeader}.${encPayload}`;
  const sig = createHmac("sha256", args.signWith ?? SECRET)
    .update(signedBase)
    .digest("base64url");
  return `${encHeader}.${encPayload}.${sig}`;
}

describe("isLikelyJwt", () => {
  it("returns true for a real-shape JWT", () => {
    expect(isLikelyJwt(mint())).toBe(true);
  });

  it("returns false for the legacy `userId:role` shape", () => {
    expect(isLikelyJwt("u-123:teacher")).toBe(false);
  });

  it("returns false for empty / single-token / unprefixed strings", () => {
    expect(isLikelyJwt("")).toBe(false);
    expect(isLikelyJwt("notajwt")).toBe(false);
    expect(isLikelyJwt("ey.bogus")).toBe(false); // only 2 parts
    expect(isLikelyJwt("xx.yy.zz")).toBe(false); // doesn't start with `ey`
  });
});

describe("verifyRealtimeJwt", () => {
  it("returns claims for a freshly-minted token", () => {
    const tok = mint({ sub: "u-7", role: "teacher", scope: "unit:abc-123" });
    const claims = verifyRealtimeJwt(tok, SECRET);
    expect(claims.sub).toBe("u-7");
    expect(claims.role).toBe("teacher");
    expect(claims.scope).toBe("unit:abc-123");
    expect(claims.iss).toBe(REALTIME_ISSUER);
  });

  it("rejects an empty secret on the verify side", () => {
    const tok = mint();
    expect(() => verifyRealtimeJwt(tok, "")).toThrow(/HOCUSPOCUS_TOKEN_SECRET is empty/);
  });

  it("rejects a token signed with the wrong secret", () => {
    const tok = mint({ signWith: "some-other-secret" });
    expect(() => verifyRealtimeJwt(tok, SECRET)).toThrow(/signature is invalid/);
  });

  it("rejects a tampered payload (signature still verifies the original bytes)", () => {
    // Sign over the ORIGINAL payload, then swap the encoded payload for a
    // different one — exactly the bit-flip / claim-substitution attack.
    const header = { alg: "HS256", typ: "JWT" };
    const original = { sub: "u-1", role: "user", scope: "unit:abc", iss: REALTIME_ISSUER, iat: 0, exp: 9999999999 };
    const tampered = { ...original, role: "teacher", scope: "broadcast:secret" };
    const encH = b64url(JSON.stringify(header));
    const encP = b64url(JSON.stringify(tampered));
    const sig = createHmac("sha256", SECRET).update(`${encH}.${b64url(JSON.stringify(original))}`).digest("base64url");
    const forged = `${encH}.${encP}.${sig}`;
    expect(() => verifyRealtimeJwt(forged, SECRET)).toThrow(/signature is invalid/);
  });

  it("rejects alg=none (algorithm-confusion guard)", () => {
    // Build a deliberately-broken token: header says alg=none, no
    // signature. We sign the body with HS256 anyway just so the
    // signature byte-compare doesn't short-circuit — the post-verify
    // header check should reject.
    const tok = mint({ alg: "none" });
    expect(() => verifyRealtimeJwt(tok, SECRET)).toThrow(/Unexpected JWT alg/);
  });

  it("rejects wrong issuer", () => {
    const tok = mint({ iss: "evil-platform" });
    expect(() => verifyRealtimeJwt(tok, SECRET)).toThrow(/wrong issuer/);
  });

  it("rejects an expired token", () => {
    const tok = mint({ iat: Math.floor(Date.now() / 1000) - 3600, exp: Math.floor(Date.now() / 1000) - 60 });
    expect(() => verifyRealtimeJwt(tok, SECRET)).toThrow(/expired/);
  });

  it("rejects iat far in the future (clock-skew defense)", () => {
    const tok = mint({ iat: Math.floor(Date.now() / 1000) + 3600 });
    expect(() => verifyRealtimeJwt(tok, SECRET)).toThrow(/iat in the future/);
  });

  it("rejects missing required claims", () => {
    expect(() => verifyRealtimeJwt(mint({ sub: "" }), SECRET)).toThrow(/missing sub/);
    expect(() => verifyRealtimeJwt(mint({ role: "" }), SECRET)).toThrow(/missing role/);
    expect(() => verifyRealtimeJwt(mint({ scope: "" }), SECRET)).toThrow(/missing scope/);
  });

  it("rejects a malformed JWT (non-3-part)", () => {
    expect(() => verifyRealtimeJwt("not.ajwt", SECRET)).toThrow(/Invalid JWT format/);
  });

  it("rejects garbage body even when sig length matches", () => {
    // Forge a token whose body is non-JSON but whose signature still
    // verifies the encoded bytes. Should fail at parse-time.
    const encH = b64url(JSON.stringify({ alg: "HS256", typ: "JWT" }));
    const encP = b64url("not valid json at all");
    const sig = createHmac("sha256", SECRET).update(`${encH}.${encP}`).digest("base64url");
    const tok = `${encH}.${encP}.${sig}`;
    expect(() => verifyRealtimeJwt(tok, SECRET)).toThrow(JwtVerifyError);
  });
});

describe("rechckDocumentAccess", () => {
  const baseArgs = {
    apiBaseUrl: "http://localhost:8002",
    secret: SECRET,
    documentName: "session:abc:user:def",
    sub: "u-7",
  };

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns allowed:true when the API responds 200 + allowed:true", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ allowed: true }), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    await expect(rechckDocumentAccess(baseArgs)).resolves.toEqual({ allowed: true, reason: undefined });

    // POST to /api/internal/realtime/auth with bearer + JSON body.
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("http://localhost:8002/api/internal/realtime/auth");
    expect(init.method).toBe("POST");
    expect(init.headers.Authorization).toBe(`Bearer ${SECRET}`);
    expect(JSON.parse(init.body)).toEqual({
      documentName: "session:abc:user:def",
      sub: "u-7",
    });
  });

  it("returns allowed:false with reason when the API responds 200 + allowed:false", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ allowed: false, reason: "Not authorized" }), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      ),
    );
    await expect(rechckDocumentAccess(baseArgs)).resolves.toEqual({
      allowed: false,
      reason: "Not authorized",
    });
  });

  it("throws on 4xx (malformed input from upstream)", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "invalid documentName" }), {
          status: 400,
          headers: { "content-type": "application/json" },
        }),
      ),
    );
    await expect(rechckDocumentAccess(baseArgs)).rejects.toThrow(/internal recheck failed: 400 invalid documentName/);
  });

  it("throws on 404 (missing user/resource)", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "User not found" }), {
          status: 404,
          headers: { "content-type": "application/json" },
        }),
      ),
    );
    await expect(rechckDocumentAccess(baseArgs)).rejects.toThrow(/internal recheck failed: 404 User not found/);
  });

  it("throws on 500 (DB outage / store unavailable)", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "Database error" }), {
          status: 500,
          headers: { "content-type": "application/json" },
        }),
      ),
    );
    await expect(rechckDocumentAccess(baseArgs)).rejects.toThrow(/internal recheck failed: 500 Database error/);
  });

  it("propagates fetch network errors as-is (so Hocuspocus closes the connection)", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockRejectedValue(new TypeError("network: connection refused")),
    );
    await expect(rechckDocumentAccess(baseArgs)).rejects.toThrow(/network: connection refused/);
  });
});

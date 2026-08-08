import { afterEach, describe, expect, test } from "bun:test";
import { createHmac } from "node:crypto";

import { JwtVerifyError, rechckDocumentAccess, verifyRealtimeJwt } from "./realtime-jwt";

const secret = "canvas-test-secret";

function signedClaims(claims: Record<string, unknown>): string {
  const header = Buffer.from(JSON.stringify({ alg: "HS256", typ: "JWT" })).toString("base64url");
  const payload = Buffer.from(JSON.stringify({
    sub: "11111111-1111-4111-8111-111111111111",
    role: "user",
    scope: "canvas:22222222-2222-4222-8222-222222222222",
    iss: "bridge-platform",
    iat: Math.floor(Date.now() / 1000),
    exp: Math.floor(Date.now() / 1000) + 60,
    ...claims,
  })).toString("base64url");
  const signature = createHmac("sha256", secret).update(`${header}.${payload}`).digest("base64url");
  return `${header}.${payload}.${signature}`;
}

afterEach(() => {
  globalThis.fetch = originalFetch;
});

const originalFetch = globalThis.fetch;

describe("canvas realtime JWT compatibility", () => {
  test("defaults a missing readOnly claim to false", () => {
    expect(verifyRealtimeJwt(signedClaims({}), secret).readOnly).toBe(false);
  });

  test("rejects a present non-boolean readOnly claim", () => {
    expect(() => verifyRealtimeJwt(signedClaims({ readOnly: "false" }), secret)).toThrow(JwtVerifyError);
  });

  test("fails closed when the internal auth 200 body is malformed", async () => {
    globalThis.fetch = async () => new Response(JSON.stringify({ allowed: true, readOnly: "false" }), { status: 200 });
    await expect(rechckDocumentAccess({ apiBaseUrl: "http://api.example", secret, documentName: "canvas:22222222-2222-4222-8222-222222222222", sub: "11111111-1111-4111-8111-111111111111" })).rejects.toThrow(JwtVerifyError);
  });

  test("keeps non-canvas internal responses without readOnly compatible", async () => {
    globalThis.fetch = async () => new Response(JSON.stringify({ allowed: true }), { status: 200 });
    await expect(rechckDocumentAccess({ apiBaseUrl: "http://api.example", secret, documentName: "session:22222222-2222-4222-8222-222222222222:user:11111111-1111-4111-8111-111111111111", sub: "11111111-1111-4111-8111-111111111111" })).resolves.toEqual({ allowed: true, readOnly: false });
  });

  test("fails closed when allowed is not a boolean", async () => {
    globalThis.fetch = async () => new Response(JSON.stringify({ allowed: "true", readOnly: false }), { status: 200 });
    await expect(rechckDocumentAccess({ apiBaseUrl: "http://api.example", secret, documentName: "session:22222222-2222-4222-8222-222222222222:user:11111111-1111-4111-8111-111111111111", sub: "11111111-1111-4111-8111-111111111111" })).rejects.toThrow(JwtVerifyError);
  });
});

import { describe, it, expect } from "vitest";
import { POST } from "@/app/api/auth/signup-intent/route";

function makeRequest(body: unknown): Request {
  return new Request("http://localhost/api/auth/signup-intent", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

describe("POST /api/auth/signup-intent", () => {
  it("sets the cookie on a valid teacher intent", async () => {
    const res = await POST(makeRequest({ role: "teacher" }) as never);
    expect(res.status).toBe(200);
    const setCookie = res.headers.getSetCookie();
    expect(setCookie.length).toBe(1);
    expect(setCookie[0]).toMatch(/^bridge-signup-intent=/);
    expect(setCookie[0]).toMatch(/Path=\//);
    expect(setCookie[0]).toMatch(/Max-Age=300/);
    expect(setCookie[0]).toMatch(/HttpOnly/i);
    expect(setCookie[0]).toMatch(/SameSite=lax/i);
  });

  it("sets the cookie on a valid student intent with invite", async () => {
    const res = await POST(
      makeRequest({ role: "student", inviteCode: "ABCD1234" }) as never
    );
    expect(res.status).toBe(200);
    const cookieValue = decodeURIComponent(
      res.headers.getSetCookie()[0].split(";")[0].split("=")[1]
    );
    const parsed = JSON.parse(cookieValue);
    expect(parsed).toEqual({ role: "student", inviteCode: "ABCD1234" });
  });

  it("rejects invalid role", async () => {
    const res = await POST(makeRequest({ role: "admin" }) as never);
    expect(res.status).toBe(400);
  });

  it("rejects missing role", async () => {
    const res = await POST(makeRequest({ inviteCode: "X" }) as never);
    expect(res.status).toBe(400);
  });

  it("rejects malformed JSON", async () => {
    const req = new Request("http://localhost/api/auth/signup-intent", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "not-json",
    });
    const res = await POST(req as never);
    expect(res.status).toBe(400);
  });
});

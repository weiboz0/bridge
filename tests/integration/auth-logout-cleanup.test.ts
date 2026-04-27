import { describe, it, expect } from "vitest";
import { POST } from "@/app/api/auth/logout-cleanup/route";

describe("POST /api/auth/logout-cleanup", () => {
  it("returns the cleared cookie names and emits explicit Set-Cookie for each variant", async () => {
    const res = await POST();
    expect(res.status).toBe(200);

    const body = await res.json();
    expect(body.cleared).toEqual([
      "__Secure-authjs.session-token",
      "authjs.session-token",
    ]);

    const setCookieRaw = res.headers.getSetCookie();
    expect(setCookieRaw.length).toBe(2);

    const secure = setCookieRaw.find((s) => s.startsWith("__Secure-authjs.session-token="));
    const insecure = setCookieRaw.find((s) => s.startsWith("authjs.session-token="));
    expect(secure).toBeDefined();
    expect(insecure).toBeDefined();

    // Both must use Path=/ and Max-Age=0 to match Auth.js's host-only cookie
    // attributes — anything else and the browser ignores the deletion.
    expect(secure).toMatch(/Path=\//i);
    expect(secure).toMatch(/Max-Age=0/i);
    expect(secure).toMatch(/HttpOnly/i);
    expect(secure).toMatch(/SameSite=Lax/i);
    // The __Secure- prefix variant requires the Secure attribute.
    expect(secure).toMatch(/Secure/);

    expect(insecure).toMatch(/Path=\//i);
    expect(insecure).toMatch(/Max-Age=0/i);
    expect(insecure).toMatch(/HttpOnly/i);
    expect(insecure).toMatch(/SameSite=Lax/i);
    expect(insecure).not.toMatch(/Secure/);
  });
});

import { test, expect } from "@playwright/test";
import { ACCOUNTS, loginWithCredentials } from "./helpers";

/**
 * Plan 053 phase 3 — Hocuspocus signed-token e2e ratchet.
 *
 * Asserts that the new client-mint flow works end-to-end:
 *
 *   1. Student signs in.
 *   2. Their browser POSTs /api/realtime/token with a unit-scope
 *      doc-name (we use a known seeded teaching unit) and gets a JWT
 *      back, OR the endpoint is correctly 503ing if the secret isn't
 *      set in the test environment (skip the rest of the spec then).
 *   3. The mint response carries `token` (string, "ey..."-prefixed,
 *      3 dot-parts) and `expiresAt` (ISO timestamp in the future).
 *
 * The full WebSocket handshake against Hocuspocus is exercised by
 * the existing collaborative-edit specs that run with all three
 * services up; that path validates the mint-then-connect chain end
 * to end.
 *
 * Phase 4 will extend this spec with forged + expired token cases
 * (HOCUSPOCUS_REQUIRE_SIGNED_TOKEN=1) once the flag flips.
 */
test.describe("Plan 053 phase 3 — realtime token mint", () => {
  test("authenticated user can mint a JWT for a unit doc-name", async ({ page }) => {
    await loginWithCredentials(page, ACCOUNTS.teacher.email, ACCOUNTS.teacher.password);

    // Use a well-known seeded teaching unit. The teacher account
    // should be able to edit at least one unit. Resolve via the
    // teacher's own units list — defensive against test seed drift.
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
      test.skip(true, "HOCUSPOCUS_TOKEN_SECRET not set in this environment");
    }

    expect(res.status(), `mint should return 200, got ${res.status()} (${await res.text()})`).toBe(200);
    const body = (await res.json()) as { token: string; expiresAt: string };

    expect(body.token).toBeTruthy();
    expect(body.token).toMatch(/^ey[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/);
    const exp = new Date(body.expiresAt).getTime();
    expect(Number.isNaN(exp)).toBe(false);
    expect(exp - Date.now()).toBeGreaterThan(60_000); // at least 1 min in the future
    expect(exp - Date.now()).toBeLessThan(40 * 60_000); // less than 40 min (Go clamps to 30)
  });

  test("unauthenticated request to mint endpoint returns 401", async ({ request }) => {
    const res = await request.post("/api/realtime/token", {
      data: { documentName: "unit:any" },
      headers: { "Content-Type": "application/json" },
    });
    // The Go API returns 401 (no Bridge session). The Next.js proxy
    // forwards as-is. If the secret isn't configured the endpoint
    // returns 503 BEFORE the auth check, so accept either.
    expect([401, 503]).toContain(res.status());
  });

  test("authenticated request for a doc-name the user can't access returns 403", async ({ page }) => {
    await loginWithCredentials(page, ACCOUNTS.student.email, ACCOUNTS.student.password);

    // Students cannot edit unit docs (canEditUnit gate denies). So
    // minting a unit-scope token should 403 (or 503 if secret unset).
    // Use a syntactically valid uuid that the student certainly does
    // not own.
    const res = await page.request.post("/api/realtime/token", {
      data: { documentName: "unit:00000000-0000-0000-0000-000000000000" },
      headers: { "Content-Type": "application/json" },
    });
    if (res.status() === 503) {
      test.skip(true, "HOCUSPOCUS_TOKEN_SECRET not set in this environment");
    }
    // 404 if unit doesn't exist; 403 if it does but student isn't
    // authorized. Either is the correct deny.
    expect([403, 404]).toContain(res.status());
  });
});

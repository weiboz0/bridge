import { test, expect } from "@playwright/test";
import { ACCOUNTS, loginWithCredentials, logout } from "./helpers";

/**
 * Auth identity drift regression — review 002.
 *
 * Two scenarios:
 *  - 5.1a: Sequential sign-in / sign-out as different users. After each
 *    sign-in, /api/me/memberships and /api/me/identity must reflect the
 *    current user, not the previous one. After sign-out, both cookie
 *    variants must be gone in the browser context (the logout-cleanup
 *    endpoint does this explicitly — Auth.js's own signOut clears only
 *    the variant it currently uses).
 *  - 5.1b: Stale cookie seeding. Plant a stale __Secure- cookie value
 *    in the jar before signing in as a different user. The api-client
 *    canonical-name policy must ignore the stale variant; identity
 *    must reflect the freshly signed-in user.
 */

const COOKIE_NAMES = ["authjs.session-token", "__Secure-authjs.session-token"];

async function expectDiagnosticMatch(page: import("@playwright/test").Page) {
  const diag = await page.request.get("/api/auth/debug");
  expect(diag.ok()).toBeTruthy();
  const body = await diag.json();
  expect(body.match, `auth diagnostic should report match: ${JSON.stringify(body)}`).toBe(true);
}

test.describe("Auth identity drift (review 002 regression)", () => {
  test("5.1a — sequential sign-in/sign-out as different roles never leaks identity", async ({ browser }) => {
    const context = await browser.newContext();
    const page = await context.newPage();

    // Teacher
    await loginWithCredentials(page, ACCOUNTS.teacher.email, ACCOUNTS.teacher.password);
    await expectDiagnosticMatch(page);
    const teacherIdentity = await (await page.request.get("/api/me/identity")).json();
    expect(teacherIdentity.email).toBe(ACCOUNTS.teacher.email);

    await logout(page);

    // After logout, both cookie variants must be gone in this context.
    const cookiesAfterLogout = (await context.cookies()).map((c) => c.name);
    for (const name of COOKIE_NAMES) {
      expect(cookiesAfterLogout, `cookie ${name} should be cleared`).not.toContain(name);
    }

    // Student
    await loginWithCredentials(page, ACCOUNTS.student.email, ACCOUNTS.student.password);
    await expectDiagnosticMatch(page);
    const studentIdentity = await (await page.request.get("/api/me/identity")).json();
    expect(studentIdentity.email).toBe(ACCOUNTS.student.email);
    expect(studentIdentity.userId).not.toBe(teacherIdentity.userId);

    await logout(page);

    // Admin
    await loginWithCredentials(page, ACCOUNTS.admin.email, ACCOUNTS.admin.password);
    await expectDiagnosticMatch(page);
    const sessionRes = await page.request.get("/api/auth/session");
    expect(sessionRes.ok()).toBeTruthy();
    const session = await sessionRes.json();
    expect(session.user.isPlatformAdmin).toBe(true);
    const adminStats = await page.request.get("/api/admin/stats");
    expect(adminStats.ok()).toBeTruthy();

    await context.close();
  });

  test("5.1b — stale __Secure- cookie from a prior user is ignored on next sign-in", async ({ browser }) => {
    const context = await browser.newContext();

    // Plant a stale __Secure-authjs.session-token before any sign-in.
    //
    // Scope note: the planted value is opaque garbage, not a valid signed
    // token from a real different user (crafting one requires the dev
    // AUTH_SECRET and Auth.js's JWE encoder, which the E2E setup doesn't
    // have wired). The architectural property we lock in here is "the api
    // client never *forwards* the stale variant when canonical is missing"
    // — Phase 1's getSessionCookieName + Go's canonical cookie selection
    // both refuse the planted cookie regardless of its content.
    //
    // Stronger version (TODO plan 040): obtain a valid stale token at
    // setup time, plant that, and assert Go returns the *new* user's
    // identity rather than the stale-token user. Today's test still
    // catches the "any leak path" regression.
    await context.addCookies([
      {
        name: "__Secure-authjs.session-token",
        value: "stale-from-different-user-aaaa-bbbb-cccc",
        domain: "localhost",
        path: "/",
        httpOnly: true,
        secure: false,
        sameSite: "Lax",
      },
    ]);

    const page = await context.newPage();
    await loginWithCredentials(page, ACCOUNTS.student.email, ACCOUNTS.student.password);

    // Diagnostic must report the live student, not the planted cookie's
    // (invalid) identity. Both layers must agree.
    await expectDiagnosticMatch(page);

    // Memberships must be the student's, not somehow the planted cookie's.
    const identity = await (await page.request.get("/api/me/identity")).json();
    expect(identity.email).toBe(ACCOUNTS.student.email);

    await context.close();
  });
});

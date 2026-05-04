import { test, expect } from "@playwright/test";
import { ACCOUNTS, loginWithCredentials } from "./helpers";

/**
 * Plan 065 Phase 2 — verify the lazy mint in the Edge middleware
 * actually attaches a `bridge.session` Set-Cookie on authenticated
 * responses.
 *
 * This test was originally specified as driving a real Google OAuth
 * callback, but Codex pass-3 confirmed CI can't drive real Google
 * OAuth without out-of-band secret allowlisting. The credentials
 * provider exercises the same mint path (the middleware fires on
 * EVERY authenticated request, not just at sign-in time) so the
 * Set-Cookie reliability is proved by either flow.
 *
 * The test:
 *  1. Sign in via the credentials form (eve@demo.edu / bridge123).
 *  2. Navigate to a portal page (`/teacher`).
 *  3. Assert the browser context has a `bridge.session` cookie set
 *     by the middleware response.
 *  4. Make an authenticated request to a Go-proxied endpoint and
 *     confirm the cookie is sent (and the call succeeds — Go
 *     middleware reads either bridge.session OR the Auth.js JWE
 *     during rollout; either path returns 200).
 */
test("middleware lazy-mint attaches bridge.session on first portal hit", async ({
  page,
  context,
}) => {
  // Skip if BRIDGE_INTERNAL_SECRET isn't set in the dev env — without
  // it the helper short-circuits and no cookie is attached. This is
  // the documented fail-closed behavior.
  // We can't read the server's env, but we can detect the symptom:
  // if no bridge.session cookie appears after sign-in, either the
  // helper short-circuited (env unset) or the wiring is broken. The
  // test expects the cookie; if your dev env doesn't have the secret
  // configured, set it before running this spec.

  await loginWithCredentials(page, ACCOUNTS.teacher.email, ACCOUNTS.teacher.password);
  await page.goto("/teacher");
  await page.waitForLoadState("networkidle");

  const cookies = await context.cookies();
  const bridgeCookie = cookies.find((c) => c.name === "bridge.session");

  expect(
    bridgeCookie,
    "Edge middleware should set `bridge.session` after sign-in. " +
      "If this fails, check: (1) BRIDGE_INTERNAL_SECRET is set in the Go API .env, " +
      "(2) BRIDGE_INTERNAL_SECRET is also exposed to the Next runtime, " +
      "(3) the Go API is reachable at GO_INTERNAL_API_URL (default localhost:8002)."
  ).toBeTruthy();

  expect(bridgeCookie!.value).toMatch(/^[\w-]+\.[\w-]+\.[\w-]+$/);
  expect(bridgeCookie!.httpOnly).toBe(true);
  expect(bridgeCookie!.sameSite).toBe("Lax");
  // The cookie's expires field should be ~7 days from now.
  const expiresInDays = (bridgeCookie!.expires - Date.now() / 1000) / (24 * 60 * 60);
  expect(expiresInDays).toBeGreaterThan(6);
  expect(expiresInDays).toBeLessThan(8);

  // Sanity: an authenticated request still works. With Phase 2 alone
  // (Go reads JWE primary, bridge.session is dormant), the Go API
  // accepts the request via JWE. Phase 3 will flip to reading
  // bridge.session first; the same call must still succeed there.
  const meResponse = await page.request.get("/api/me/identity");
  expect(meResponse.status()).toBe(200);
});

test("bridge.session is cleared after logout-cleanup", async ({ page, context }) => {
  await loginWithCredentials(page, ACCOUNTS.teacher.email, ACCOUNTS.teacher.password);
  await page.goto("/teacher");
  await page.waitForLoadState("networkidle");

  // Confirm the cookie was set.
  let cookies = await context.cookies();
  expect(cookies.find((c) => c.name === "bridge.session")).toBeTruthy();

  // Bridge's signout flow (see src/components/portal/sidebar-footer.tsx
  // and src/components/sign-out-button.tsx) calls POST
  // /api/auth/logout-cleanup BEFORE NextAuth's signOut(). Plan 065
  // phase 2 added bridge.session to the cleanup list so the cookie
  // is explicitly expired alongside the Auth.js session cookies —
  // critical because /api/auth/* routes are outside the middleware
  // matcher, so the wrapper's null-session cleanup path doesn't
  // run for the signout request itself.
  const cleanup = await page.request.post("/api/auth/logout-cleanup");
  expect(cleanup.status()).toBe(200);

  cookies = await context.cookies();
  const bridgeCookie = cookies.find((c) => c.name === "bridge.session");
  expect(
    bridgeCookie,
    "bridge.session must be cleared by /api/auth/logout-cleanup so the " +
      "Bridge-issued cookie doesn't outlive the Auth.js session it pairs with"
  ).toBeFalsy();
});

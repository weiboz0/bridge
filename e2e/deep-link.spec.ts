import { test, expect } from "@playwright/test";
import { ACCOUNTS } from "./helpers";

/**
 * Deep-link callbackUrl preservation — review 002 P2 #7 regression.
 *
 * Before plan 040, hitting a portal URL while signed out lost the path:
 * the redirect to /login carried no callbackUrl because the previous
 * portal-shell.tsx implementation tried to read `x-invoke-path` /
 * `x-url` headers that aren't actually populated by Next.js.
 *
 * After plan 040 phase 5, src/middleware.ts runs on portal trees and
 * the `authorized` callback in src/lib/auth.ts redirects to
 * /login?callbackUrl=<original> using request.nextUrl directly.
 */

test.describe("Deep-link preservation", () => {
  test("unauthenticated portal request redirects to /login with callbackUrl", async ({ browser }) => {
    const context = await browser.newContext();
    const page = await context.newPage();

    const target = "/teacher/classes/00000000-0000-0000-0000-000000000abc";
    await page.goto(target);

    // Middleware redirects before the page renders. Final URL is
    // /login with the original path encoded in callbackUrl.
    await page.waitForURL(/\/login\?callbackUrl=/);
    const url = new URL(page.url());
    expect(url.pathname).toBe("/login");
    expect(url.searchParams.get("callbackUrl")).toBe(target);

    await context.close();
  });

  test("after sign-in, user lands on the original deep-linked path", async ({ browser }) => {
    const context = await browser.newContext();
    const page = await context.newPage();

    // Use a path the teacher account is allowed to render so the post-
    // login redirect actually loads — even an unknown class id resolves
    // to a notFound() page rather than a 401, which is what we want here.
    const target = "/teacher/sessions";
    await page.goto(target);
    await page.waitForURL(/\/login\?callbackUrl=/);

    await page.fill('input[id="email"]', ACCOUNTS.teacher.email);
    await page.fill('input[id="password"]', ACCOUNTS.teacher.password);
    await page.click('button[type="submit"]');

    // After sign-in the login form pushes to callbackUrl on success.
    await page.waitForURL(target, { timeout: 10000 });

    await context.close();
  });
});

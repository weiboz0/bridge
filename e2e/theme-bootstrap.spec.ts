import { test, expect } from "@playwright/test";

/**
 * Theme bootstrap — review 002 P2 #12 regression.
 *
 * Plans 040 and 048 each tried client-script approaches to FOUC
 * prevention; both kept tripping React 19 + Next 16's "Encountered
 * a script tag while rendering React component" dev warning. The
 * current approach (post-049 hotfix) reads the theme from a
 * `bridge-theme` cookie server-side in `src/app/layout.tsx` and
 * sets the `dark` class on `<html>` directly — no inline script.
 *
 * This spec asserts:
 *   1. With `bridge-theme=dark` set as a cookie, the `<html>`
 *      element has the `dark` class on first paint (server-rendered).
 *   2. No "Encountered a script tag while rendering React component"
 *      message appears in the console during initial load.
 */

test.describe("Theme bootstrap", () => {
  test("dark class applied without FOUC + no script-tag dev overlay", async ({ page, context, baseURL }) => {
    const consoleErrors: string[] = [];
    page.on("console", (msg) => {
      if (msg.type() === "error") consoleErrors.push(msg.text());
    });

    // Pre-seed the cookie + localStorage before the page renders.
    // The cookie drives the SSR render; localStorage is mirrored for
    // backwards compat with sessions that pre-date the cookie.
    const url = new URL(baseURL ?? "http://localhost:3003");
    await context.addCookies([
      {
        name: "bridge-theme",
        value: "dark",
        url: url.origin,
        sameSite: "Lax",
      },
    ]);
    await page.addInitScript(() => {
      localStorage.setItem("bridge-theme", "dark");
    });

    await page.goto("/");

    // <html> should have the dark class.
    const htmlClass = await page.evaluate(() => document.documentElement.className);
    expect(htmlClass).toContain("dark");

    // The specific React/Next dev-overlay sentinel must not appear.
    const offending = consoleErrors.find((m) =>
      m.includes("Encountered a script tag while rendering React component")
    );
    expect(offending, `unexpected script-tag warning: ${offending}`).toBeUndefined();
  });
});

import { test, expect } from "@playwright/test";

/**
 * Theme bootstrap — review 002 P2 #12 regression.
 *
 * Plan 040 phase 8 moved the theme-init script from
 * `<script dangerouslySetInnerHTML>` (which triggered the dev-overlay
 * "Encountered a script tag while rendering React component" error on
 * every route) to next/script with `beforeInteractive`.
 *
 * This spec asserts:
 *   1. With `bridge-theme=dark` pre-seeded, the `<html>` element has
 *      the `dark` class on first paint.
 *   2. No "Encountered a script tag while rendering React component"
 *      message appears in the console during initial load.
 */

test.describe("Theme bootstrap", () => {
  test("dark class applied without FOUC + no script-tag dev overlay", async ({ page }) => {
    const consoleErrors: string[] = [];
    page.on("console", (msg) => {
      if (msg.type() === "error") consoleErrors.push(msg.text());
    });

    // Pre-seed the dark preference before the page renders.
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

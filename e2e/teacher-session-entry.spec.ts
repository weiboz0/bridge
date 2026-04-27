import { test, expect } from "@playwright/test";

/**
 * Teacher session entry — review 002 P0 #2 regression.
 *
 * The dashboard listed live sessions but opening one returned a Next 404,
 * because the page combined NextAuth viewer identity with Go session data
 * and the boundary drifted. After Phase 2, the consolidated /teacher-page
 * Go endpoint is the single authorization point, so a session listed on
 * the teacher dashboard MUST open without a 404.
 */

test.use({ storageState: "e2e/.auth/teacher.json" });

test.describe("Teacher session entry", () => {
  test("dashboard live-session link opens the workspace", async ({ page }) => {
    await page.goto("/teacher/sessions");
    await expect(page.getByRole("heading", { name: /Sessions/i })).toBeVisible();

    // Find any session card link. If none exists, this run can't exercise
    // the regression — skip rather than false-pass.
    const firstLink = page.locator('a[href*="/teacher/sessions/"]').first();
    if (!(await firstLink.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip(true, "no listed sessions to open");
      return;
    }

    const href = await firstLink.getAttribute("href");
    expect(href).toBeTruthy();

    await firstLink.click();

    // The workspace should render — anything but a 404.
    // The dashboard varies by session shape (class-bound vs orphan), so we
    // assert two things: URL navigated to the session page, AND something
    // with `data-testid="teacher-dashboard"` rendered (set in the dashboard
    // root). A blank page or 404 fails both checks.
    await page.waitForURL(/\/teacher\/sessions\//);
    const notFound = page.locator("text=/404|not found/i");
    await expect(notFound).toHaveCount(0);
    const dashboardRoot = page.locator('[data-testid="teacher-dashboard"]');
    await expect(dashboardRoot).toBeVisible({ timeout: 10000 });
  });
});

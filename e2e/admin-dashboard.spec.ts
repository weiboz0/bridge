import { test, expect } from "@playwright/test";

/**
 * Platform admin dashboard smoke — review 002 P0 #3 regression.
 *
 * /admin used to render blank because /api/admin/stats returned 403 on
 * a Go-authorized identity that disagreed with the NextAuth session. After
 * Phase 1 (canonical token forwarding) and Phase 2 (defensive error state),
 * the page must either show the stats or surface an explicit error card —
 * never a blank page or 404.
 */

test.use({ storageState: "e2e/.auth/admin.json" });

test.describe("Admin dashboard smoke", () => {
  test("/admin renders without errors", async ({ page }) => {
    await page.goto("/admin");
    await expect(page.getByRole("heading", { name: "Platform Admin" })).toBeVisible();

    // Either the stats grid renders or the defensive error card does. Both
    // are acceptable; a blank page is not.
    const statsHeader = page.getByText(/Pending Organizations/i);
    const errorHeader = page.getByText(/Couldn.t load platform stats/i);
    await expect(statsHeader.or(errorHeader)).toBeVisible({ timeout: 10000 });
  });

  test("/admin/orgs renders without errors", async ({ page }) => {
    await page.goto("/admin/orgs");
    await expect(page.locator("body")).not.toContainText("404");
  });
});

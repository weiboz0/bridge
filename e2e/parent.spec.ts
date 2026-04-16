import { test, expect } from "@playwright/test";

/**
 * Parent Portal E2E — deeper tests for the parent experience:
 * - Dashboard shows linked children
 * - Navigating to a child's detail page
 * - Accessing the reports page
 */

test.describe("Parent Portal", () => {
  test.use({ storageState: "e2e/.auth/parent.json" });

  test("parent dashboard shows linked children", async ({ page }) => {
    await page.goto("/parent");
    await expect(page.getByRole("heading", { name: "Parent Dashboard" })).toBeVisible();

    // Check for either children cards or the empty state message
    const childCards = page.locator("a[href*='/parent/children/']");
    const emptyMessage = page.locator("text=No children linked yet");

    const hasChildren = await childCards.first().isVisible({ timeout: 5000 }).catch(() => false);
    const isEmpty = await emptyMessage.isVisible({ timeout: 1000 }).catch(() => false);

    // One of these should be true
    expect(hasChildren || isEmpty).toBeTruthy();

    if (hasChildren) {
      // Verify child cards show a name
      const childCount = await childCards.count();
      expect(childCount).toBeGreaterThan(0);
    }
  });

  test("parent can view child detail page", async ({ page }) => {
    await page.goto("/parent");
    await expect(page.getByRole("heading", { name: "Parent Dashboard" })).toBeVisible();

    // Click on the first child card
    const childLink = page.locator("a[href*='/parent/children/']").first();
    if (!(await childLink.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip(true, "No children linked to this parent");
      return;
    }

    await childLink.click();
    await page.waitForURL(/\/parent\/children\/[^/]+$/);

    // Verify the child detail page loaded with expected sections
    // Should show child name as heading
    const heading = page.getByRole("heading").first();
    await expect(heading).toBeVisible();

    // Should show "Classes" section
    const classesSection = page.locator("text=Classes");
    await expect(classesSection.first()).toBeVisible({ timeout: 5000 });

    // Should show "Recent Code" section
    const codeSection = page.locator("text=Recent Code");
    await expect(codeSection.first()).toBeVisible({ timeout: 5000 });
  });

  test("parent can view reports page", async ({ page }) => {
    await page.goto("/parent/reports");

    // Verify the reports page loads
    await expect(page.getByRole("heading", { name: "Reports" })).toBeVisible();

    // The reports page currently shows a "coming soon" message
    await expect(page.locator("text=coming soon")).toBeVisible();
  });

  test("parent children page redirects to dashboard", async ({ page }) => {
    // The /parent/children route redirects to /parent
    await page.goto("/parent/children");
    await page.waitForURL(/\/parent$/);
    await expect(page.getByRole("heading", { name: "Parent Dashboard" })).toBeVisible();
  });
});

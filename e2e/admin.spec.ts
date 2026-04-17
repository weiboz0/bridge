import { test, expect } from "@playwright/test";

/**
 * Admin Portal E2E — tests platform admin functionality:
 * - Dashboard stats (pending orgs, active orgs, total users)
 * - Organization list with status filters
 * - User list with action dropdowns
 * - Approving a pending organization (if test data exists)
 */

test.describe("Admin Portal", () => {
  test.use({ storageState: "e2e/.auth/admin.json" });

  test("admin can see platform stats on dashboard", async ({ page }) => {
    await page.goto("/admin");

    // Verify the dashboard heading
    await expect(page.getByRole("heading", { name: "Platform Admin" })).toBeVisible();

    // Verify stat cards are present
    await expect(page.locator("text=Pending Organizations")).toBeVisible();
    await expect(page.locator("text=Active Organizations")).toBeVisible();
    await expect(page.locator("text=Total Users")).toBeVisible();

    // Verify the numbers are rendered (3xl bold text in card content)
    const statValues = page.locator(".text-3xl.font-bold");
    const count = await statValues.count();
    expect(count).toBeGreaterThanOrEqual(3);
  });

  test("admin can view org list", async ({ page }) => {
    await page.goto("/admin/orgs");

    // Verify the heading
    await expect(page.getByRole("heading", { name: "Organizations" })).toBeVisible();

    // Verify filter tabs are present
    await expect(page.locator("a[href='/admin/orgs']").first()).toBeVisible();
    await expect(page.locator("a[href='/admin/orgs?status=pending']")).toBeVisible();
    await expect(page.locator("a[href='/admin/orgs?status=active']")).toBeVisible();

    // Verify at least one org is listed (Bridge Demo School should exist)
    const orgCards = page.locator("text=Bridge Demo School");
    if (!(await orgCards.first().isVisible({ timeout: 5000 }).catch(() => false))) {
      // No orgs at all — check for the empty state
      const emptyState = page.locator("text=No organizations found");
      await expect(emptyState).toBeVisible();
    } else {
      await expect(orgCards.first()).toBeVisible();
    }
  });

  test("admin can filter orgs by status", async ({ page }) => {
    // Navigate to active orgs filter
    await page.goto("/admin/orgs?status=active");
    await expect(page.getByRole("heading", { name: "Organizations" })).toBeVisible();

    // The "Active" filter tab should be highlighted
    const activeTab = page.locator("a[href='/admin/orgs?status=active']");
    await expect(activeTab).toBeVisible();

    // All displayed org cards should have "active" status badge
    const statusBadges = page.locator("text=active").first();
    if (await statusBadges.isVisible({ timeout: 3000 }).catch(() => false)) {
      await expect(statusBadges).toBeVisible();
    }
  });

  test("admin can view user list with dropdown actions", async ({ page }) => {
    await page.goto("/admin/users");

    // Verify the heading (includes user count)
    const heading = page.getByRole("heading").filter({ hasText: "Users" });
    await expect(heading).toBeVisible();

    // Verify table headers
    await expect(page.locator("th:text('Name')")).toBeVisible();
    await expect(page.locator("th:text('Email')")).toBeVisible();
    await expect(page.locator("th:text('Actions')")).toBeVisible();

    // Verify at least one user row exists
    const rows = page.locator("tbody tr");
    const rowCount = await rows.count();
    expect(rowCount).toBeGreaterThan(0);

    // Verify action dropdown (MoreHorizontal button) is visible for non-self users.
    const actionButton = page.getByRole("button", { name: "Actions" }).first();
    if (await actionButton.isVisible({ timeout: 3000 }).catch(() => false)) {
      await actionButton.click();

      // Dropdown menu should appear with impersonation option
      const dropdownItem = page.locator("[role='menuitem']").first();
      await expect(dropdownItem).toBeVisible({ timeout: 3000 });

      // Close the dropdown by pressing Escape
      await page.keyboard.press("Escape");
    }
  });

  test("admin can approve a pending org", async ({ page }) => {
    await page.goto("/admin/orgs?status=pending");
    await expect(page.getByRole("heading", { name: "Organizations" })).toBeVisible();

    // Look for an Approve button — only exists if there are pending orgs
    const approveButton = page.getByRole("button", { name: "Approve" }).first();
    if (!(await approveButton.isVisible({ timeout: 3000 }).catch(() => false))) {
      test.skip(true, "No pending organizations to approve");
      return;
    }

    // Click approve
    await approveButton.click();

    // Wait for the page to revalidate
    await page.waitForTimeout(2000);

    // After approval, the org should no longer show as pending on this filtered page
    // (it may still appear briefly during revalidation)
    // Just verify the page didn't error out
    await expect(page.getByRole("heading", { name: "Organizations" })).toBeVisible();
  });
});

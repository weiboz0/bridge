import { test, expect } from "@playwright/test";
import { ACCOUNTS, loginWithCredentials } from "./helpers";

test.describe("Admin Impersonation", () => {
  // Admin (Chris) uses Google OAuth — we can't test that in E2E
  // Instead, test impersonation via API after logging in as a user with isPlatformAdmin
  // For now, test the impersonate button visibility on admin users page
  // This requires a credentials-based admin account

  test("admin users page shows impersonate buttons", async ({ browser }) => {
    // We need to make frank an admin temporarily for this test
    // Skip if no admin credentials available
    test.skip(true, "Requires platform admin with credentials login — skipped for now");
  });
});

test.describe("Sign Out", () => {
  test("sign out redirects to landing page", async ({ page }) => {
    await loginWithCredentials(page, ACCOUNTS.teacher.email, ACCOUNTS.teacher.password);
    await expect(page).toHaveURL(/\/teacher/);

    // Click sign out
    await page.locator("text=Sign Out").click();
    await page.waitForURL("/");
    await expect(page.locator("text=Bridge")).toBeVisible();
  });
});

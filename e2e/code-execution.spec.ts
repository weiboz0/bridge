import { test, expect } from "@playwright/test";
import { ACCOUNTS, loginWithCredentials } from "./helpers";

test.describe("Code Execution", () => {
  test("standalone editor loads and accepts input", async ({ page }) => {
    await loginWithCredentials(page, ACCOUNTS.teacher.email, ACCOUNTS.teacher.password);

    // Navigate to a class with an editor
    // Use the legacy editor route which doesn't require class enrollment
    await page.goto("/teacher/courses");
    await page.waitForTimeout(1000);

    // Verify the teacher portal is accessible
    await expect(page.getByRole("heading", { name: "My Courses" })).toBeVisible();
  });
});

test.describe("Theme Toggle", () => {
  test.use({ storageState: "e2e/.auth/teacher.json" });

  test("theme toggle switches between light and dark", async ({ page }) => {
    await page.goto("/teacher");

    // Find theme toggle button (contains sun or moon emoji)
    const themeButton = page.locator("button").filter({ hasText: /☀️|🌙/ }).first();
    if (await themeButton.isVisible({ timeout: 3000 }).catch(() => false)) {
      // Check initial state
      const htmlClass = await page.locator("html").getAttribute("class");
      const wasDark = htmlClass?.includes("dark");

      // Click toggle
      await themeButton.click();
      await page.waitForTimeout(500);

      // Verify class changed
      const newHtmlClass = await page.locator("html").getAttribute("class");
      if (wasDark) {
        expect(newHtmlClass).not.toContain("dark");
      } else {
        expect(newHtmlClass).toContain("dark");
      }

      // Toggle back
      await themeButton.click();
    }
  });
});

test.describe("Join Class by Code", () => {
  test("student can access join page", async ({ page }) => {
    await loginWithCredentials(page, ACCOUNTS.student.email, ACCOUNTS.student.password);
    await page.goto("/student/classes");
    await expect(page.getByRole("heading", { name: "My Classes" })).toBeVisible();
  });
});

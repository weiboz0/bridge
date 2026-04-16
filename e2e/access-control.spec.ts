import { test, expect } from "@playwright/test";

/**
 * Cross-Role Access Control E2E — verifies that:
 * - Students cannot access the teacher portal
 * - Teachers cannot access the admin portal
 * - Unauthenticated users are redirected to the login page
 *
 * The PortalShell component checks roles via /api/me/portal-access and
 * redirects unauthorized users to "/" (home) or "/login" depending on
 * whether they are authenticated.
 */

test.describe("Cross-Role Access Control", () => {
  test.describe("student cannot access teacher portal", () => {
    test.use({ storageState: "e2e/.auth/student.json" });

    test("student is redirected away from /teacher", async ({ page }) => {
      await page.goto("/teacher");

      // Student should be redirected to "/" (the landing page) since they
      // are authenticated but lack the teacher role
      await page.waitForURL((url) => !url.pathname.startsWith("/teacher"), {
        timeout: 10000,
      });

      // Should NOT be on a teacher page
      const currentUrl = page.url();
      expect(currentUrl).not.toContain("/teacher");
    });

    test("student is redirected away from /teacher/classes", async ({ page }) => {
      await page.goto("/teacher/classes");

      await page.waitForURL((url) => !url.pathname.startsWith("/teacher"), {
        timeout: 10000,
      });

      const currentUrl = page.url();
      expect(currentUrl).not.toContain("/teacher");
    });
  });

  test.describe("teacher cannot access admin portal", () => {
    test.use({ storageState: "e2e/.auth/teacher.json" });

    test("teacher is redirected away from /admin", async ({ page }) => {
      await page.goto("/admin");

      // Teacher should be redirected to "/" since they lack the admin role
      await page.waitForURL((url) => !url.pathname.startsWith("/admin"), {
        timeout: 10000,
      });

      const currentUrl = page.url();
      expect(currentUrl).not.toContain("/admin");
    });

    test("teacher is redirected away from /admin/users", async ({ page }) => {
      await page.goto("/admin/users");

      await page.waitForURL((url) => !url.pathname.startsWith("/admin"), {
        timeout: 10000,
      });

      const currentUrl = page.url();
      expect(currentUrl).not.toContain("/admin");
    });
  });

  test.describe("student cannot access admin portal", () => {
    test.use({ storageState: "e2e/.auth/student.json" });

    test("student is redirected away from /admin", async ({ page }) => {
      await page.goto("/admin");

      await page.waitForURL((url) => !url.pathname.startsWith("/admin"), {
        timeout: 10000,
      });

      const currentUrl = page.url();
      expect(currentUrl).not.toContain("/admin");
    });
  });

  test.describe("unauthenticated user redirected to login", () => {
    // Use empty storage state (no cookies)
    test.use({ storageState: { cookies: [], origins: [] } });

    test("unauthenticated user is redirected to /login from /teacher", async ({ page }) => {
      await page.goto("/teacher");

      // Should redirect to /login for unauthenticated users
      await page.waitForURL(/\/login/, { timeout: 10000 });
      await expect(page.locator("text=Log In to Bridge")).toBeVisible();
    });

    test("unauthenticated user is redirected to /login from /student", async ({ page }) => {
      await page.goto("/student");

      await page.waitForURL(/\/login/, { timeout: 10000 });
      await expect(page.locator("text=Log In to Bridge")).toBeVisible();
    });

    test("unauthenticated user is redirected to /login from /admin", async ({ page }) => {
      await page.goto("/admin");

      await page.waitForURL(/\/login/, { timeout: 10000 });
      await expect(page.locator("text=Log In to Bridge")).toBeVisible();
    });

    test("unauthenticated user is redirected to /login from /parent", async ({ page }) => {
      await page.goto("/parent");

      await page.waitForURL(/\/login/, { timeout: 10000 });
      await expect(page.locator("text=Log In to Bridge")).toBeVisible();
    });
  });
});

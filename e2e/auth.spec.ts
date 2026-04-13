import { test, expect } from "@playwright/test";
import { ACCOUNTS, loginWithCredentials, logout } from "./helpers";

test.describe("Authentication", () => {
  test("login page renders", async ({ page }) => {
    await page.goto("/login");
    await expect(page.locator("text=Log In to Bridge")).toBeVisible();
    await expect(page.locator("text=Continue with Google")).toBeVisible();
    await expect(page.locator('input[id="email"]')).toBeVisible();
    await expect(page.locator('input[id="password"]')).toBeVisible();
  });

  test("login as teacher redirects to teacher portal", async ({ page }) => {
    await loginWithCredentials(page, ACCOUNTS.teacher.email, ACCOUNTS.teacher.password);
    await expect(page).toHaveURL(/\/teacher/);
    await expect(page.getByRole("heading", { name: "Teacher Dashboard" })).toBeVisible();
  });

  test("login as student redirects to student portal", async ({ page }) => {
    await loginWithCredentials(page, ACCOUNTS.student.email, ACCOUNTS.student.password);
    await expect(page).toHaveURL(/\/student/);
  });

  test("login as org admin redirects to org portal", async ({ page }) => {
    await loginWithCredentials(page, ACCOUNTS.orgAdmin.email, ACCOUNTS.orgAdmin.password);
    await expect(page).toHaveURL(/\/org/);
  });

  test("login as parent redirects to parent portal", async ({ page }) => {
    await loginWithCredentials(page, ACCOUNTS.parent.email, ACCOUNTS.parent.password);
    await expect(page).toHaveURL(/\/parent/);
  });

  test("invalid credentials shows error", async ({ page }) => {
    await page.goto("/login");
    await page.fill('input[id="email"]', "wrong@test.edu");
    await page.fill('input[id="password"]', "wrongpassword");
    await page.click('button[type="submit"]');
    await expect(page.locator("text=Invalid email or password")).toBeVisible();
  });

  test("register page renders", async ({ page }) => {
    await page.goto("/register");
    await expect(page.locator("text=Create an Account")).toBeVisible();
  });
});

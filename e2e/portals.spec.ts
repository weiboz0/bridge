import { test, expect } from "@playwright/test";

test.describe("Teacher Portal Navigation", () => {
  test.use({ storageState: "e2e/.auth/teacher.json" });

  test("shows sidebar with nav items", async ({ page }) => {
    await page.goto("/teacher");
    await expect(page.locator("aside")).toBeVisible();
    await expect(page.getByRole("link", { name: "Courses", exact: true })).toBeVisible();
    await expect(page.getByRole("link", { name: "Classes", exact: true })).toBeVisible();
  });

  test("navigates to courses page", async ({ page }) => {
    await page.goto("/teacher");
    await page.getByRole("link", { name: "Courses", exact: true }).click();
    await expect(page).toHaveURL(/\/teacher\/courses/);
  });

  test("navigates to classes page", async ({ page }) => {
    await page.goto("/teacher");
    await page.getByRole("link", { name: "Classes", exact: true }).click();
    await expect(page).toHaveURL(/\/teacher\/classes/);
  });
});

test.describe("Student Portal Navigation", () => {
  test.use({ storageState: "e2e/.auth/student.json" });

  test("shows sidebar with nav items", async ({ page }) => {
    await page.goto("/student");
    await expect(page.locator("aside")).toBeVisible();
    await expect(page.getByRole("link", { name: /My Classes/ })).toBeVisible();
    await expect(page.getByRole("link", { name: "My Work", exact: true })).toBeVisible();
  });

  test("navigates to classes page", async ({ page }) => {
    await page.goto("/student");
    await page.getByRole("link", { name: /My Classes/ }).click();
    await expect(page).toHaveURL(/\/student\/classes/);
  });

  test("navigates to code page", async ({ page }) => {
    await page.goto("/student");
    await page.getByRole("link", { name: "My Work", exact: true }).click();
    await expect(page).toHaveURL(/\/student\/code/);
  });
});

test.describe("Org Admin Portal Navigation", () => {
  test.use({ storageState: "e2e/.auth/org-admin.json" });

  test("shows sidebar with nav items", async ({ page }) => {
    await page.goto("/org");
    await expect(page.locator("aside")).toBeVisible();
    await expect(page.getByRole("link", { name: /Teachers/ })).toBeVisible();
    await expect(page.getByRole("link", { name: /Students/ })).toBeVisible();
  });

  test("shows org dashboard with stats", async ({ page }) => {
    await page.goto("/org");
    await expect(page.locator("text=Bridge Demo School")).toBeVisible();
  });
});

test.describe("Parent Portal Navigation", () => {
  test.use({ storageState: "e2e/.auth/parent.json" });

  test("shows sidebar with nav items", async ({ page }) => {
    await page.goto("/parent");
    await expect(page.locator("aside")).toBeVisible();
    await expect(page.getByRole("link", { name: "Dashboard", exact: true })).toBeVisible();
    await expect(page.getByRole("link", { name: "Sessions", exact: true })).toBeVisible();
  });

  test("shows parent dashboard", async ({ page }) => {
    await page.goto("/parent");
    await expect(page.getByRole("heading", { name: "Parent Dashboard" })).toBeVisible();
  });
});

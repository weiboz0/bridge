import { test, expect } from "@playwright/test";

test.describe("Code Editor", () => {
  test.use({ storageState: "e2e/.auth/student.json" });

  test("student code page loads", async ({ page }) => {
    await page.goto("/student/code");
    await expect(page.getByRole("heading", { name: "My Code" })).toBeVisible();
  });

  test("student classes page loads", async ({ page }) => {
    await page.goto("/student/classes");
    await expect(page.getByRole("heading", { name: "My Classes" })).toBeVisible();
  });
});

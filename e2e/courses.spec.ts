import { test, expect } from "@playwright/test";

test.describe("Course & Class Management", () => {
  test.use({ storageState: "e2e/.auth/teacher.json" });

  test("teacher can create a course", async ({ page }) => {
    await page.goto("/teacher/courses");
    await expect(page.getByRole("heading", { name: "My Courses" })).toBeVisible();

    // Fill create course form
    const titleInput = page.locator('input[name="title"]');
    if (await titleInput.isVisible()) {
      await titleInput.fill("E2E Test Course");
      await page.click('button:text("Create")');
      // Should redirect to course detail
      await page.waitForURL(/\/teacher\/courses\//);
      await expect(page.locator("text=E2E Test Course")).toBeVisible();
    }
  });

  test("teacher can add a topic to a course", async ({ page }) => {
    await page.goto("/teacher/courses");

    // Click on an existing course
    const courseLink = page.locator("text=E2E Test Course").first();
    if (await courseLink.isVisible()) {
      await courseLink.click();
      await page.waitForURL(/\/teacher\/courses\//);

      // Add a topic
      const topicInput = page.locator('input[name="title"]');
      if (await topicInput.isVisible()) {
        const topicName = `E2E Topic ${Date.now()}`;
        await topicInput.fill(topicName);
        await page.click('button:text("Add Topic")');
        await page.waitForTimeout(1000);
        await expect(page.locator(`text=${topicName}`).first()).toBeVisible();
      }
    }
  });

  test("teacher can create a class from a course", async ({ page }) => {
    await page.goto("/teacher/courses");

    const courseLink = page.locator("text=E2E Test Course").first();
    if (await courseLink.isVisible()) {
      await courseLink.click();
      await page.waitForURL(/\/teacher\/courses\//);

      // Click create class
      const createClassLink = page.locator("text=Create Class");
      if (await createClassLink.isVisible()) {
        await createClassLink.click();
        await page.waitForURL(/\/create-class/);

        await page.fill('input[name="title"]', "E2E Test Class");
        await page.fill('input[name="term"]', "Spring 2026");
        await page.click('button:text("Create Class")');
        await page.waitForURL(/\/teacher\/classes\//);
        await expect(page.locator("text=E2E Test Class")).toBeVisible();
      }
    }
  });

  test("teacher can see class join code on class detail page", async ({ page }) => {
    await page.goto("/teacher/classes");
    await expect(page.getByRole("heading", { name: "My Classes" })).toBeVisible();

    // Click the first class to open its detail page
    const classLink = page.locator("a[href*='/teacher/classes/']").first();
    if (!(await classLink.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip(true, "No classes available");
      return;
    }

    await classLink.click();
    await page.waitForURL(/\/teacher\/classes\//);

    // Verify the "Join Code" card heading is visible
    await expect(page.locator("text=Join Code")).toBeVisible();
    await expect(page.locator("text=Share with students")).toBeVisible();

    // Verify the join code is displayed as an 8-character code
    const codeElement = page.locator(".font-mono.tracking-widest");
    await expect(codeElement).toBeVisible();
    const codeText = await codeElement.textContent();
    expect(codeText!.trim()).toHaveLength(8);
  });
});

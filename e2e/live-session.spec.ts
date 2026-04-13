import { test, expect, type BrowserContext } from "@playwright/test";
import { ACCOUNTS, loginWithCredentials } from "./helpers";

test.describe("Live Sessions (multi-browser)", () => {
  let teacherContext: BrowserContext;
  let studentContext: BrowserContext;

  test.beforeAll(async ({ browser }) => {
    // Create separate browser contexts for teacher and student
    teacherContext = await browser.newContext();
    studentContext = await browser.newContext();

    // Login as teacher
    const teacherPage = await teacherContext.newPage();
    await loginWithCredentials(teacherPage, ACCOUNTS.teacher.email, ACCOUNTS.teacher.password);
    await teacherPage.close();

    // Login as student
    const studentPage = await studentContext.newPage();
    await loginWithCredentials(studentPage, ACCOUNTS.student.email, ACCOUNTS.student.password);
    await studentPage.close();
  });

  test.afterAll(async () => {
    await teacherContext?.close();
    await studentContext?.close();
  });

  test("teacher can view classes page", async () => {
    const page = await teacherContext.newPage();
    await page.goto("/teacher/classes");
    await expect(page.getByRole("heading", { name: "My Classes" })).toBeVisible();
    await page.close();
  });

  test("student can view classes page", async () => {
    const page = await studentContext.newPage();
    await page.goto("/student/classes");
    await expect(page.getByRole("heading", { name: "My Classes" })).toBeVisible();
    await page.close();
  });

  // Note: Full live session tests (start session, join, real-time sync)
  // require both the Next.js server AND Hocuspocus to be running,
  // plus a class with the student enrolled. These are integration-level
  // E2E tests that need specific data setup.

  test("teacher can access course detail", async () => {
    const page = await teacherContext.newPage();
    await page.goto("/teacher/courses");

    // Click first course if exists
    const courseLink = page.locator("a[href*='/teacher/courses/']").first();
    if (await courseLink.isVisible({ timeout: 3000 }).catch(() => false)) {
      await courseLink.click();
      await page.waitForURL(/\/teacher\/courses\//);
      // Should see topics section
      await expect(page.locator("text=Topics")).toBeVisible();
    }
    await page.close();
  });
});

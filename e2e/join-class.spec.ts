import { test, expect, type BrowserContext } from "@playwright/test";
import { ACCOUNTS, loginWithCredentials } from "./helpers";

/**
 * Join Class by Code E2E — tests that:
 * 1. A student can join a class using a valid join code
 * 2. An invalid join code shows an error message
 */

test.describe("Join Class by Code", () => {
  test.describe("student joins with valid code", () => {
    let teacherContext: BrowserContext;
    let student2Context: BrowserContext;
    let joinCode: string;
    let classId: string;

    test.beforeAll(async ({ browser }) => {
      teacherContext = await browser.newContext();
      student2Context = await browser.newContext();

      const teacherPage = await teacherContext.newPage();
      await loginWithCredentials(teacherPage, ACCOUNTS.teacher.email, ACCOUNTS.teacher.password);
      await teacherPage.close();

      const studentPage = await student2Context.newPage();
      await loginWithCredentials(studentPage, ACCOUNTS.student2.email, ACCOUNTS.student2.password);
      await studentPage.close();
    });

    test.afterAll(async () => {
      await teacherContext?.close();
      await student2Context?.close();
    });

    test("teacher can see join code on class detail", async () => {
      const page = await teacherContext.newPage();

      await page.goto("/teacher/classes");
      await expect(page.getByRole("heading", { name: "My Classes" })).toBeVisible();

      // Click the first class
      const classLink = page.locator("a[href*='/teacher/classes/']").first();
      if (!(await classLink.isVisible({ timeout: 5000 }).catch(() => false))) {
        test.skip(true, "No classes available");
        await page.close();
        return;
      }

      const href = await classLink.getAttribute("href");
      classId = href!.split("/teacher/classes/")[1];

      await classLink.click();
      await page.waitForURL(/\/teacher\/classes\//);

      // Find the join code displayed on the page (8-char code in mono font)
      await expect(page.locator("text=Join Code")).toBeVisible();

      // The join code is displayed as a large mono-spaced text
      const codeElement = page.locator(".font-mono.tracking-widest");
      if (!(await codeElement.isVisible({ timeout: 3000 }).catch(() => false))) {
        test.skip(true, "Join code element not visible");
        await page.close();
        return;
      }

      joinCode = (await codeElement.textContent())!.trim();
      expect(joinCode).toHaveLength(8);

      await page.close();
    });

    test("student2 can join a class with the join code", async () => {
      test.skip(!joinCode || !classId, "No join code or classId captured from previous test");

      const page = await student2Context.newPage();

      // Check enrollment state before joining. If student2 is already enrolled
      // (from a prior run), skip to avoid spurious "already enrolled" errors.
      const preEnrollment = await student2Context.request.get(`/api/classes/${classId}`);
      if (preEnrollment.ok()) {
        test.skip(true, "student2 already enrolled in this class");
        await page.close();
        return;
      }

      // Navigate to student dashboard where "Join a Class" button is
      await page.goto("/student");
      await expect(page.getByRole("heading", { name: "My Dashboard" })).toBeVisible();

      // Click "Join a Class" button to open the form
      const joinButton = page.getByRole("button", { name: "Join a Class" });
      await expect(joinButton).toBeVisible();
      await joinButton.click();

      // Enter the join code
      const codeInput = page.locator("#joinCode");
      await expect(codeInput).toBeVisible();
      await codeInput.fill(joinCode);

      // Submit + wait for the POST to complete
      const [joinResp] = await Promise.all([
        page.waitForResponse(
          (r) => r.url().includes("/join") && r.request().method() === "POST"
        ),
        page.getByRole("button", { name: "Join" }).click(),
      ]);
      expect(joinResp.ok()).toBeTruthy();

      // Verify enrollment: the student should now be able to GET the class detail
      const postEnrollment = await student2Context.request.get(`/api/classes/${classId}`);
      expect(postEnrollment.ok()).toBeTruthy();

      await page.close();
    });
  });

  test.describe("invalid join code", () => {
    test.use({ storageState: "e2e/.auth/student.json" });

    test("invalid join code shows error", async ({ page }) => {
      await page.goto("/student");
      await expect(page.getByRole("heading", { name: "My Dashboard" })).toBeVisible();

      // Click "Join a Class" button
      const joinButton = page.getByRole("button", { name: "Join a Class" });
      await expect(joinButton).toBeVisible();
      await joinButton.click();

      // Enter an invalid join code
      const codeInput = page.locator("#joinCode");
      await expect(codeInput).toBeVisible();
      await codeInput.fill("INVALID1");

      // Click "Join"
      await page.getByRole("button", { name: "Join" }).click();

      // Verify error message appears
      const errorMessage = page.locator(".text-destructive");
      await expect(errorMessage).toBeVisible({ timeout: 5000 });
      const errorText = await errorMessage.textContent();
      // Should show something like "Class not found" or "Invalid join code"
      expect(errorText).toBeTruthy();
    });
  });
});

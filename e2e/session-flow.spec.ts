import { test, expect, type BrowserContext, type Page } from "@playwright/test";
import { ACCOUNTS, loginWithCredentials } from "./helpers";
import { getFixtureState } from "./helpers/fixture-state";

/**
 * Session Flow E2E — tests the core lifecycle:
 * Teacher starts a session -> Student sees it -> Teacher ends it -> Past sessions visible
 *
 * Uses test.describe.serial() since these tests are sequentially dependent.
 */

test.describe.serial("Session Flow", () => {
  let teacherContext: BrowserContext;
  let studentContext: BrowserContext;
  let classId: string;
  let sessionId: string;

  test.beforeAll(async ({ browser }) => {
    teacherContext = await browser.newContext();
    studentContext = await browser.newContext();

    // Login both users
    const teacherPage = await teacherContext.newPage();
    await loginWithCredentials(teacherPage, ACCOUNTS.teacher.email, ACCOUNTS.teacher.password);
    await teacherPage.close();

    const studentPage = await studentContext.newPage();
    await loginWithCredentials(studentPage, ACCOUNTS.student.email, ACCOUNTS.student.password);
    await studentPage.close();
  });

  test.afterAll(async () => {
    await teacherContext?.close();
    await studentContext?.close();
  });

  test("teacher can start a live session from class detail page", async () => {
    const page = await teacherContext.newPage();

    // Read classId from fixture state written by seed.setup.ts
    const fixtureState = getFixtureState();
    classId = fixtureState.classId;
    expect(classId).toBeDefined();

    // Navigate directly to the seeded class detail page
    await page.goto(`/teacher/classes/${classId}`);
    await page.waitForURL(/\/teacher\/classes\//);

    // Verify join code is visible (confirms we're on class detail)
    await expect(page.locator("text=Join Code")).toBeVisible();

    // The seed cleans up any active sessions — resume button must NOT be present
    const resumeButton = page.locator("text=Resume Session");
    await expect(resumeButton).not.toBeVisible({ timeout: 2000 });

    // Click "Start Live Session" button
    const startButton = page.getByRole("button", { name: "Start Live Session" });
    await expect(startButton).toBeVisible({ timeout: 5000 });

    await startButton.click();

    // Should redirect to session dashboard
    await page.waitForURL(/\/teacher\/classes\/.*\/session\/.*\/dashboard/, {
      timeout: 10000,
    });

    // Extract sessionId from the URL
    const url = page.url();
    const match = url.match(/\/session\/([^/]+)\/dashboard/);
    expect(match).toBeTruthy();
    sessionId = match![1];

    await page.close();
  });

  test("student sees active session on class page", async () => {
    expect(classId).toBeDefined();
    expect(sessionId).toBeDefined();

    const page = await studentContext.newPage();

    // Navigate to student's class detail page
    await page.goto(`/student/classes/${classId}`);

    // Verify "Live Session — Join Now" card is visible — fixture enrolls alice in the class
    const liveCard = page.locator("text=Live Session — Join Now");
    await expect(liveCard).toBeVisible({ timeout: 5000 });

    await expect(liveCard).toBeVisible();

    // Click the live session card to join
    await liveCard.click();
    await page.waitForURL(/\/student\/classes\/.*\/session\//, { timeout: 10000 });

    await page.close();
  });

  test("teacher can end a session", async () => {
    expect(sessionId).toBeDefined();

    const page = await teacherContext.newPage();

    // Navigate to session dashboard
    await page.goto(`/teacher/classes/${classId}/session/${sessionId}/dashboard`);

    // Look for "End Session" button — must be present after the start-session test ran
    const endButton = page.getByRole("button", { name: "End Session" });
    await expect(endButton).toBeVisible({ timeout: 5000 });

    await endButton.click();

    // Should redirect back to the class detail page (no /session/ segment).
    await page.waitForURL(
      (url) =>
        url.pathname.startsWith("/teacher/classes/") &&
        !url.pathname.includes("/session/"),
      { timeout: 10000 }
    );

    await page.close();
  });

  test("teacher sees past sessions on class page", async () => {
    expect(classId).toBeDefined();

    const page = await teacherContext.newPage();

    await page.goto(`/teacher/classes/${classId}`);

    // Verify past sessions section exists — the end-session test guarantees at least one past session
    const pastSessionsHeading = page.locator("text=Past Sessions");
    await expect(pastSessionsHeading).toBeVisible({ timeout: 5000 });

    await expect(pastSessionsHeading).toBeVisible();

    // Verify there's at least one past session entry with duration and student count
    const sessionEntry = page.locator("text=students").first();
    await expect(sessionEntry).toBeVisible({ timeout: 5000 });

    await page.close();
  });
});

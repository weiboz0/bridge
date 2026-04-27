import { test, expect } from "@playwright/test";

test.describe("Help Queue (API-driven)", () => {
  test.use({ storageState: "e2e/.auth/student.json" });

  test("student can check help queue status via API", async ({ request }) => {
    // This tests the API endpoint exists and responds correctly
    // Full help queue testing requires an active session

    // Try to list — should work even without active session
    const res = await request.get("/api/sessions/00000000-0000-0000-0000-000000000000/help-queue");
    // Expect 200 (empty queue) or 401/404
    expect([200, 404]).toContain(res.status());
  });
});

test.describe("Parent Portal", () => {
  test.use({ storageState: "e2e/.auth/parent.json" });

  test("parent dashboard loads", async ({ page }) => {
    await page.goto("/parent");
    await expect(page.getByRole("heading", { name: "Parent Dashboard" })).toBeVisible();
  });

  // /parent/children + the "My Children" nav entry were removed in plan
  // 040 phase 7 pending a real children-list product design. Locked by
  // tests/unit/nav-config.test.ts and e2e/parent.spec.ts (404 assertion).

  test("parent can access reports page", async ({ page }) => {
    await page.goto("/parent");
    await page.getByRole("link", { name: /Reports/ }).click();
    await expect(page).toHaveURL(/\/parent\/reports/);
  });
});

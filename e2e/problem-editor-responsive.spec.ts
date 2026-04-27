import { test, expect, type Page } from "@playwright/test";

/**
 * Problem editor responsive layout — review-002 P2 #11 regression.
 *
 * The shell at `src/components/problem/problem-shell.tsx` used to be a
 * fixed three-pane layout (`min-w-[360px]` left + `min-w-[320px]` right
 * inside `overflow-hidden`) with no responsive fallback. Plan 042 added
 * Tailwind `lg:` breakpoint logic and a tab bar that shows below the
 * breakpoint.
 *
 * This spec asserts: at wide widths, all three panes visible and no tab
 * bar. At narrow widths, tab bar visible and only the active pane
 * visible. Switching tabs swaps the visible pane. No horizontal overflow
 * at any tested width.
 */

test.use({ storageState: "e2e/.auth/student.json" });

async function findProblemUrl(page: Page): Promise<string | null> {
  // The student dashboard surfaces enrolled classes; navigate into one
  // and find a problem if any exists. If not, the spec skips — the
  // responsive shape is what we're testing, not the data flow.
  await page.goto("/student/classes");
  const classLink = page.locator('a[href*="/student/classes/"]').first();
  if (!(await classLink.isVisible({ timeout: 5000 }).catch(() => false))) {
    return null;
  }
  await classLink.click();
  await page.waitForURL(/\/student\/classes\//);

  const problemLink = page.locator('a[href*="/problems/"]').first();
  if (!(await problemLink.isVisible({ timeout: 5000 }).catch(() => false))) {
    return null;
  }
  return problemLink.getAttribute("href");
}

test.describe("Problem editor responsive layout", () => {
  test("wide viewport (1440px): all 3 panes visible, no tab bar", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    const url = await findProblemUrl(page);
    test.skip(!url, "no problem available to test against");
    await page.goto(url!);

    // Tab bar is hidden via lg:hidden — present in DOM but not visible.
    const tablist = page.locator('[role="tablist"]');
    await expect(tablist).toBeHidden();

    // All three panes visible at lg+.
    for (const id of ["problem", "code", "io"]) {
      await expect(page.locator(`#problem-pane-${id}`)).toBeVisible();
    }
  });

  test("boundary viewport (1024px): all 3 panes visible (lg is inclusive)", async ({ page }) => {
    await page.setViewportSize({ width: 1024, height: 768 });
    const url = await findProblemUrl(page);
    test.skip(!url, "no problem available to test against");
    await page.goto(url!);

    for (const id of ["problem", "code", "io"]) {
      await expect(page.locator(`#problem-pane-${id}`)).toBeVisible();
    }

    // No horizontal overflow at the tightest wide-layout width
    // (this is the failure mode the original review-002 issue described).
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - window.innerWidth
    );
    expect(overflow).toBeLessThanOrEqual(0);
  });

  test("narrow viewport (800px): tab bar visible, one pane at a time, switching swaps", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 800, height: 1024 });
    const url = await findProblemUrl(page);
    test.skip(!url, "no problem available to test against");
    await page.goto(url!);

    // Tab bar visible + 3 tab buttons.
    const tablist = page.locator('[role="tablist"]');
    await expect(tablist).toBeVisible();
    await expect(tablist.getByRole("tab")).toHaveCount(3);

    // No horizontal overflow at narrow.
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - window.innerWidth
    );
    expect(overflow).toBeLessThanOrEqual(0);

    // Initially: code visible, others hidden.
    await expect(page.locator("#problem-pane-code")).toBeVisible();
    await expect(page.locator("#problem-pane-problem")).toBeHidden();
    await expect(page.locator("#problem-pane-io")).toBeHidden();

    // Active pane has non-zero size — catches a Monaco-rendered-at-zero
    // failure mode that visibility-only assertions miss.
    const codeBox = await page.locator("#problem-pane-code").boundingBox();
    expect(codeBox?.width ?? 0).toBeGreaterThan(0);
    expect(codeBox?.height ?? 0).toBeGreaterThan(0);

    // Click "Problem" → swap.
    await page.getByRole("tab", { name: "Problem" }).click();
    await expect(page.locator("#problem-pane-problem")).toBeVisible();
    await expect(page.locator("#problem-pane-code")).toBeHidden();
    await expect(page.locator("#problem-pane-io")).toBeHidden();

    // Click "I/O" → swap.
    await page.getByRole("tab", { name: "I/O" }).click();
    await expect(page.locator("#problem-pane-io")).toBeVisible();
    await expect(page.locator("#problem-pane-problem")).toBeHidden();

    // Back to Code: editor should still respond (state preserved).
    await page.getByRole("tab", { name: "Code" }).click();
    await expect(page.locator("#problem-pane-code")).toBeVisible();
  });
});

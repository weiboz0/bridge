import { test, expect, type Page } from "@playwright/test";

/**
 * Problem editor responsive layout — review-002 P2 #11 + plan 043 P1.
 *
 * The shell at `src/components/problem/problem-shell.tsx` used to be a
 * fixed three-pane layout with no responsive fallback. Plan 042 added
 * a viewport `lg:` breakpoint + tab bar. Plan 043 phase 4 switched
 * those rules to `@container/shell` (Tailwind v4 container queries) so
 * the breakpoint reacts to the actual pane width, not the window width
 * — closing the sidebar-squeeze bug from review 005 (1024px viewport
 * with the desktop sidebar visible left ~800px for the editor and the
 * old `lg:` rule still triggered the three-pane layout in ~120px of
 * editor space).
 *
 * This spec asserts: wide-enough container → all 3 panes visible AND
 * the code pane has a usable width. Narrow container → tab bar
 * visible, one pane at a time, switching swaps. No horizontal overflow
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
  test("wide viewport (1440px): all 3 panes visible, code pane ≥ 360px", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    const url = await findProblemUrl(page);
    test.skip(!url, "no problem available to test against");
    await page.goto(url!);

    // Tab bar hidden — container is wide enough that the @3xl/shell rules apply.
    const tablist = page.locator('[role="tablist"]');
    await expect(tablist).toBeHidden();

    for (const id of ["problem", "code", "io"]) {
      await expect(page.locator(`#problem-pane-${id}`)).toBeVisible();
    }

    // Plan 043 Codex correction #4: assert the code pane has a usable
    // width. Visibility alone passes when Monaco renders in 80px.
    const codeBox = await page.locator("#problem-pane-code").boundingBox();
    expect(codeBox?.width ?? 0).toBeGreaterThanOrEqual(360);
  });

  test("desktop with sidebar (1280×800): wide layout + usable code pane", async ({ page }) => {
    // 1280 viewport → ~1056px content area after the desktop sidebar.
    // Container is well above the @3xl/shell (768px) breakpoint.
    await page.setViewportSize({ width: 1280, height: 800 });
    const url = await findProblemUrl(page);
    test.skip(!url, "no problem available to test against");
    await page.goto(url!);

    for (const id of ["problem", "code", "io"]) {
      await expect(page.locator(`#problem-pane-${id}`)).toBeVisible();
    }

    const codeBox = await page.locator("#problem-pane-code").boundingBox();
    expect(codeBox?.width ?? 0).toBeGreaterThanOrEqual(360);

    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - window.innerWidth
    );
    expect(overflow).toBeLessThanOrEqual(0);
  });

  test("squeezed viewport (1024×768 with sidebar): no horizontal overflow", async ({ page }) => {
    // 1024 viewport - sidebar(~224) ≈ 800px container, just above @3xl
    // (768px). Either layout is acceptable; what we assert is no
    // horizontal scroll and Monaco having non-zero size — the
    // pre-043 failure mode was a 120px-wide editor. Plan 043's
    // container query produces a usable layout regardless of which
    // branch wins, so we assert the bug is gone, not which side won.
    await page.setViewportSize({ width: 1024, height: 768 });
    const url = await findProblemUrl(page);
    test.skip(!url, "no problem available to test against");
    await page.goto(url!);

    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - window.innerWidth
    );
    expect(overflow).toBeLessThanOrEqual(0);

    const codeBox = await page.locator("#problem-pane-code").boundingBox();
    expect(codeBox?.width ?? 0).toBeGreaterThan(0);
    expect(codeBox?.height ?? 0).toBeGreaterThan(0);
  });

  test("narrow viewport (800px): tab bar visible, one pane at a time", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 800, height: 1024 });
    const url = await findProblemUrl(page);
    test.skip(!url, "no problem available to test against");
    await page.goto(url!);

    // 800 viewport - sidebar(~224) ≈ 576px container, below @3xl (768px).
    // Tab bar should be visible.
    const tablist = page.locator('[role="tablist"]');
    await expect(tablist).toBeVisible();
    await expect(tablist.getByRole("tab")).toHaveCount(3);

    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - window.innerWidth
    );
    expect(overflow).toBeLessThanOrEqual(0);

    // Initially: code visible, others hidden.
    await expect(page.locator("#problem-pane-code")).toBeVisible();
    await expect(page.locator("#problem-pane-problem")).toBeHidden();
    await expect(page.locator("#problem-pane-io")).toBeHidden();

    const codeBox = await page.locator("#problem-pane-code").boundingBox();
    expect(codeBox?.width ?? 0).toBeGreaterThan(0);
    expect(codeBox?.height ?? 0).toBeGreaterThan(0);

    await page.getByRole("tab", { name: "Problem" }).click();
    await expect(page.locator("#problem-pane-problem")).toBeVisible();
    await expect(page.locator("#problem-pane-code")).toBeHidden();
    await expect(page.locator("#problem-pane-io")).toBeHidden();

    await page.getByRole("tab", { name: "I/O" }).click();
    await expect(page.locator("#problem-pane-io")).toBeVisible();
    await expect(page.locator("#problem-pane-problem")).toBeHidden();

    await page.getByRole("tab", { name: "Code" }).click();
    await expect(page.locator("#problem-pane-code")).toBeVisible();
  });
});

import { test, expect } from "@playwright/test";
import { ACCOUNTS, loginWithCredentials } from "./helpers";

// Plan 067 phase 3 — sectioned-nav e2e smoke.
//
// The contract this test enforces is "the legacy role-switcher
// button is gone and the new sectioned sidebar renders". Codex's
// post-impl review of phases 2+3 caught that admin@e2e.test is
// documented as is_platform_admin only with no org memberships
// (helpers.ts:3-11), so a "must show >1 section" assertion would
// be flaky. We assert ≥ 1 section instead — multi-section coverage
// lives in tests/unit/sidebar-section.test.tsx where seed data
// is controlled.

test("admin sidebar renders sectioned nav, no role-switcher button", async ({
  page,
}) => {
  await loginWithCredentials(page, ACCOUNTS.admin.email, ACCOUNTS.admin.password);

  await page.goto("/admin");
  await page.waitForLoadState("networkidle");

  // Desktop sidebar items should render under at least one
  // SidebarSection — even single-role users see their nav. (Single-
  // role users render items inline without a header chevron, so we
  // assert on the nav links rather than aria-expanded buttons.)
  const sidebarLinks = page.locator("aside a");
  expect(await sidebarLinks.count()).toBeGreaterThan(0);

  // The legacy role-switcher rendered a button labeled "Switch role"
  // or similar. Sectioned nav has no such button — its absence is
  // the substantive contract this PR enforces.
  await expect(page.locator("text=Switch role")).toHaveCount(0);
});

test("clicking a sidebar item navigates within the portal", async ({ page }) => {
  await loginWithCredentials(page, ACCOUNTS.admin.email, ACCOUNTS.admin.password);
  await page.goto("/admin");
  await page.waitForLoadState("networkidle");

  // Find the first sidebar link with an href under one of the role
  // base paths. We don't depend on a specific item label since seed
  // navigation can evolve; we just verify the click navigates to a
  // real portal route.
  const firstNavLink = page.locator("aside a").first();
  await firstNavLink.waitFor({ state: "visible" });
  const href = await firstNavLink.getAttribute("href");
  expect(href).toBeTruthy();

  await firstNavLink.click();
  await page.waitForLoadState("networkidle");

  expect(page.url()).toMatch(/\/(admin|teacher|student|parent|org)\b/);
});

import { test, expect } from "@playwright/test";
import { ACCOUNTS, loginWithCredentials } from "./helpers";

// Plan 067 phase 3 — sectioned-nav e2e smoke.
//
// The platform-admin account (`admin@e2e.test`) has multiple roles
// in the seed data, so the sidebar should render multiple
// SidebarSection groups simultaneously rather than a role-switcher
// strip. This test verifies the basic shape: more than one section
// header is visible on a viewport wide enough for the desktop
// sidebar.

test("platform admin sees multi-section sidebar (no role switcher)", async ({
  page,
}) => {
  await loginWithCredentials(page, ACCOUNTS.admin.email, ACCOUNTS.admin.password);

  // Land somewhere with the portal shell; /admin is the platform-admin
  // landing page which the seeded admin user always has access to.
  await page.goto("/admin");
  await page.waitForLoadState("networkidle");

  // The desktop sidebar groups its items under collapsible section
  // headers (each is a `<button aria-expanded=...>`). Multiple
  // headers means multiple sections rendered concurrently — i.e.,
  // we are NOT showing a single-role view.
  const sectionHeaders = page.locator('button[aria-expanded]').filter({
    hasText: /Platform Admin|Teacher|Organization|Student|Parent/i,
  });
  await expect(sectionHeaders.first()).toBeVisible();
  expect(await sectionHeaders.count()).toBeGreaterThan(1);

  // The legacy role-switcher rendered a button with text "Switch role"
  // or showed each role as a sibling. Sectioned nav has no such
  // button — its absence is part of the contract.
  await expect(page.locator("text=Switch role")).toHaveCount(0);
});

test("clicking into a section's nav item navigates to that portal", async ({
  page,
}) => {
  await loginWithCredentials(page, ACCOUNTS.admin.email, ACCOUNTS.admin.password);
  await page.goto("/admin");
  await page.waitForLoadState("networkidle");

  // Expand the Platform Admin section if it isn't already, then click
  // the first nav item under it. The exact label varies with seed
  // data so we just assert that we end up on a /admin/* path after
  // the click.
  const adminHeader = page
    .locator('button[aria-expanded]')
    .filter({ hasText: /Platform Admin/i })
    .first();
  if ((await adminHeader.getAttribute("aria-expanded")) === "false") {
    await adminHeader.click();
  }

  // First item in the admin section's nav. The `<aside>` wraps the
  // sidebar; scope the link query to it so we don't pick up content
  // links in the main view.
  const firstNavLink = page.locator("aside a").first();
  await firstNavLink.waitFor({ state: "visible" });
  const href = await firstNavLink.getAttribute("href");
  expect(href).toBeTruthy();
  await firstNavLink.click();
  await page.waitForLoadState("networkidle");

  // The href could be /admin, /admin/users, etc. — assert we're on
  // some portal path (one of the role base paths).
  expect(page.url()).toMatch(/\/(admin|teacher|student|parent|org)\b/);
});

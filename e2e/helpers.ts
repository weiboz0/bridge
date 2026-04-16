import { type Page, expect } from "@playwright/test";

export const ACCOUNTS = {
  teacher: { email: "eve@demo.edu", password: "bridge123" },
  student: { email: "alice@demo.edu", password: "bridge123" },
  orgAdmin: { email: "frank@demo.edu", password: "bridge123" },
  parent: { email: "diana@demo.edu", password: "bridge123" },
  student2: { email: "bob@demo.edu", password: "bridge123" },
  admin: { email: "admin@e2e.test", password: "bridge123" },
};

export async function loginWithCredentials(
  page: Page,
  email: string,
  password: string
) {
  await page.goto("/login");
  await page.fill('input[id="email"]', email);
  await page.fill('input[id="password"]', password);
  await page.click('button[type="submit"]');
  // Wait for redirect away from login
  await page.waitForURL((url) => !url.pathname.includes("/login"), {
    timeout: 10000,
  });
}

export async function logout(page: Page) {
  // Find and click sign out button
  const signOutButton = page.locator("text=Sign Out");
  if (await signOutButton.isVisible()) {
    await signOutButton.click();
    await page.waitForURL("/");
  }
}

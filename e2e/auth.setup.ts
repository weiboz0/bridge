import { test as setup } from "@playwright/test";
import { ACCOUNTS, loginWithCredentials } from "./helpers";

// Save auth state for each role so tests don't need to login each time

setup("authenticate teacher", async ({ page }) => {
  await loginWithCredentials(page, ACCOUNTS.teacher.email, ACCOUNTS.teacher.password);
  await page.context().storageState({ path: "e2e/.auth/teacher.json" });
});

setup("authenticate student", async ({ page }) => {
  await loginWithCredentials(page, ACCOUNTS.student.email, ACCOUNTS.student.password);
  await page.context().storageState({ path: "e2e/.auth/student.json" });
});

setup("authenticate org admin", async ({ page }) => {
  await loginWithCredentials(page, ACCOUNTS.orgAdmin.email, ACCOUNTS.orgAdmin.password);
  await page.context().storageState({ path: "e2e/.auth/org-admin.json" });
});

setup("authenticate parent", async ({ page }) => {
  await loginWithCredentials(page, ACCOUNTS.parent.email, ACCOUNTS.parent.password);
  await page.context().storageState({ path: "e2e/.auth/parent.json" });
});

setup("authenticate admin", async ({ page }) => {
  await loginWithCredentials(page, ACCOUNTS.admin.email, ACCOUNTS.admin.password);
  await page.context().storageState({ path: "e2e/.auth/admin.json" });
});

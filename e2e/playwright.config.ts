import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: ".",
  testMatch: "**/*.spec.ts",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: "list",
  timeout: 30000,

  use: {
    baseURL: process.env.E2E_BASE_URL || "http://localhost:3003",
    trace: "on-first-retry",
    screenshot: "only-on-failure",
  },

  projects: [
    // Auth setup — runs first, saves storage state
    {
      name: "auth-setup",
      testMatch: "auth.setup.ts",
    },
    // Main tests — use saved auth state
    {
      name: "tests",
      dependencies: ["auth-setup"],
      use: {
        ...devices["Desktop Chrome"],
      },
    },
  ],
});

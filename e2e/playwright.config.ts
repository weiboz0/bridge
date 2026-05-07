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
    // Seed — runs first, ensures fixture data exists and writes e2e/.fixture/state.json
    {
      name: "seed",
      testMatch: "seed.setup.ts",
    },
    // Auth setup — runs after seed, saves storage state for each role
    {
      name: "auth-setup",
      testMatch: "auth.setup.ts",
      dependencies: ["seed"],
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

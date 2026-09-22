import { defineConfig } from "vitest/config";

// This configuration intentionally omits the repository test setup, whose
// database cleanup is inappropriate while the separately provisioned E2E
// stack is running. It exercises only Playwright configuration loading.
export default defineConfig({
  test: {
    include: ["tests/unit/playwright-config.test.ts"],
  },
});

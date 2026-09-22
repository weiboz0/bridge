import { defineConfig } from "vitest/config";
import path from "path";

export default defineConfig({
  // Keep Zod named exports and errors in one module instance across test runners.
  ssr: {
    noExternal: [/^zod(?:\/.*)?$/],
  },
  test: {
    globals: true,
    environment: "node",
    setupFiles: ["./tests/setup.ts", "./tests/setup-dom.ts"],
    include: ["tests/**/*.test.ts", "tests/**/*.test.tsx"],
    fileParallelism: false,
  },
  resolve: {
    noExternal: [/[/]node_modules[/]zod(?:[/]|$)/],
    dedupe: ["zod"],
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
});

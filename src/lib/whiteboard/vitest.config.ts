import { defineConfig } from "vitest/config";
import path from "path";

const repositoryRoot = path.resolve(__dirname, "../../..");

/** Runs the source-local hook regression that the root Vitest glob excludes. */
export default defineConfig({
  test: {
    globals: true,
    environment: "node",
    setupFiles: [
      path.join(repositoryRoot, "tests/setup.ts"),
      path.join(repositoryRoot, "tests/setup-dom.ts"),
    ],
    include: ["src/lib/whiteboard/use-whiteboard.test.tsx"],
    fileParallelism: false,
  },
  resolve: {
    alias: {
      "@": path.join(repositoryRoot, "src"),
    },
  },
});

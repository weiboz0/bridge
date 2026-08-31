import { afterEach, describe, expect, it } from "vitest";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import { spawnSync } from "node:child_process";

const configSourcePath = join(process.cwd(), "e2e/playwright.config.ts");
const tempDirectories: string[] = [];

afterEach(async () => {
  await Promise.all(tempDirectories.splice(0).map((directory) => rm(directory, { recursive: true, force: true })));
});

function loadConfigFrom(directory: string, overrides: Record<string, string> = {}) {
  const configURL = pathToFileURL(join(directory, "playwright.config.mjs")).href;
  const result = spawnSync("/usr/bin/node", ["-e", `import(${JSON.stringify(configURL)}).then((module) => console.log(JSON.stringify({ baseURL: module.default.use?.baseURL, failureFlag: process.env.BRIDGE_E2E_CANVAS_CONTROL_FAILURE })))`], {
    cwd: directory,
    env: { PATH: process.env.PATH ?? "", NODE_ENV: "test", ...overrides },
    encoding: "utf8",
  });
  expect(result.status, result.stderr).toBe(0);
  return JSON.parse(result.stdout) as { baseURL: string; failureFlag?: string };
}

describe("Playwright environment configuration", () => {
  it("loads E2E settings from the working-directory .env without overriding explicit shell values", async () => {
    const directory = await mkdtemp(join(process.cwd(), ".playwright-config-test-"));
    tempDirectories.push(directory);
    await writeFile(join(directory, ".env"), "E2E_BASE_URL=http://dotenv.test:3999\nBRIDGE_E2E_CANVAS_CONTROL_FAILURE=1\n");
    await writeFile(join(directory, "playwright.config.mjs"), await readFile(configSourcePath));

    expect(loadConfigFrom(directory)).toEqual({
      baseURL: "http://dotenv.test:3999",
      failureFlag: "1",
    });
    expect(loadConfigFrom(directory, {
      E2E_BASE_URL: "http://shell.test:4888",
      BRIDGE_E2E_CANVAS_CONTROL_FAILURE: "0",
    })).toEqual({
      baseURL: "http://shell.test:4888",
      failureFlag: "0",
    });

    const emptyDirectory = await mkdtemp(join(process.cwd(), ".playwright-config-test-"));
    tempDirectories.push(emptyDirectory);
    await writeFile(join(emptyDirectory, "playwright.config.mjs"), await readFile(configSourcePath));
    expect(loadConfigFrom(emptyDirectory)).toEqual({ baseURL: "http://localhost:3003" });
  });
});

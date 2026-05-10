import { existsSync } from "node:fs";
import { describe, expect, it } from "vitest";

describe("teacher placeholder routes", () => {
  it("does not ship direct-access schedule or reports placeholders", () => {
    expect(existsSync("src/app/(portal)/teacher/schedule/page.tsx")).toBe(false);
    expect(existsSync("src/app/(portal)/teacher/reports/page.tsx")).toBe(false);
  });
});

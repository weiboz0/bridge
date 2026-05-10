import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

function source(path: string): string {
  return readFileSync(path, "utf8");
}

describe("Plan 081 admin realtime health indicator", () => {
  const adminPage = "src/app/(portal)/admin/page.tsx";

  it("fetches the realtime health endpoint from the admin dashboard", () => {
    expect(source(adminPage)).toMatch(/api<[^>]*>\(["']\/api\/health\/realtime["']/);
  });

  it("shows healthy and degraded realtime labels", () => {
    const text = source(adminPage);
    expect(text).toMatch(/Realtime health/);
    expect(text).toMatch(/Token minting ready/);
    expect(text).toMatch(/Needs configuration/);
  });

  it("tells operators to set HOCUSPOCUS_TOKEN_SECRET when degraded", () => {
    expect(source(adminPage)).toMatch(/HOCUSPOCUS_TOKEN_SECRET/);
  });
});

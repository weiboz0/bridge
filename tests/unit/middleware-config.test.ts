import { describe, it, expect } from "vitest";
import { middlewareMatcher } from "@/lib/portal/middleware-matcher";

describe("middleware matcher", () => {
  // Regression guard for the bug Codex caught in plan 040's pre-impl
  // review: an earlier draft would have replaced src/middleware.ts and
  // dropped the API auth-guard contract. Lock the matcher so any future
  // edit that removes either entry fails CI loudly.
  it("matches the API auth-guard paths from before plan 040", () => {
    expect(middlewareMatcher).toContain("/api/orgs/:path*");
    expect(middlewareMatcher).toContain("/api/admin/:path*");
  });

  it("matches every portal tree for deep-link preservation", () => {
    expect(middlewareMatcher).toContain("/teacher/:path*");
    expect(middlewareMatcher).toContain("/student/:path*");
    expect(middlewareMatcher).toContain("/parent/:path*");
    expect(middlewareMatcher).toContain("/org/:path*");
    expect(middlewareMatcher).toContain("/admin/:path*");
  });
});

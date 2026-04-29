import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";
import { middlewareMatcher } from "@/lib/portal/middleware-matcher";

/**
 * Plan 047 phase 1: the literal matcher in `src/middleware.ts` must
 * stay in lockstep with the canonical list in
 * `src/lib/portal/middleware-matcher.ts`.
 *
 * Why this test exists: Next.js 16 / Turbopack rejects imported
 * references in the middleware `config.matcher` field — it must be a
 * literal in `src/middleware.ts`. We can't import `@/middleware` here
 * because the file pulls in Auth.js, which doesn't initialize outside
 * the Next runtime.
 *
 * Strategy: read `src/middleware.ts` as text, extract every quoted
 * path-matcher string starting with one of Bridge's actual matcher
 * prefixes, and assert the set equals `middlewareMatcher`. Loud-fail
 * on length mismatch — if a future matcher prefix is added that
 * doesn't match the regex, the test forces an update.
 */
describe("middleware.ts matcher parity with middleware-matcher.ts", () => {
  it("inline matcher in middleware.ts equals middlewareMatcher", () => {
    const source = readFileSync("src/middleware.ts", "utf8");
    const matches = source.matchAll(
      /"(\/(?:api|teacher|student|parent|org|admin)\/[^"]+)"/g
    );
    const inline = [...matches].map((m) => m[1]).sort();
    const expected = [...middlewareMatcher].sort();
    expect(inline).toEqual(expected);
  });

  it("middlewareMatcher only contains paths covered by the test regex", () => {
    // If a new prefix ever lands in the canonical list, this test
    // fails first and the parity test above is updated alongside.
    const allowedPrefixes = ["/api/", "/teacher/", "/student/", "/parent/", "/org/", "/admin/"];
    for (const path of middlewareMatcher) {
      expect(
        allowedPrefixes.some((p) => path.startsWith(p)),
        `matcher path "${path}" doesn't match any test-regex prefix; update both this file and the parity test above`
      ).toBe(true);
    }
  });
});

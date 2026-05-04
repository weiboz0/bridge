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
    // Plan 065 Phase 2 expanded the matcher to cover every API path
    // that proxies to Go. The regex now matches any quoted path
    // beginning with "/" — broad enough to catch the API list, the
    // portal trees, AND non-:path* paths like "/api/auth/register".
    // The `inMatcherBlock` filter scopes the search to the matcher
    // array so the comment block at the top of middleware.ts (which
    // also contains slash-paths) doesn't pollute the comparison.
    const matcherStart = source.indexOf("matcher: [");
    const matcherEnd = source.indexOf("]", matcherStart);
    const matcherBlock = source.slice(matcherStart, matcherEnd);
    const matches = matcherBlock.matchAll(/"(\/[^"]+)"/g);
    const inline = [...matches].map((m) => m[1]).sort();
    const expected = [...middlewareMatcher].sort();
    expect(inline).toEqual(expected);
  });

  it("middlewareMatcher entries all begin with a leading slash", () => {
    // Loud-fail if a future addition forgets the leading slash —
    // Next.js silently no-ops on malformed matcher entries, so a
    // missing slash would create an invisible coverage gap.
    for (const path of middlewareMatcher) {
      expect(path.startsWith("/"), `matcher path "${path}" must start with /`).toBe(true);
    }
  });
});

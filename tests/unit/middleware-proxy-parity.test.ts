import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";
import { middlewareMatcher } from "@/lib/portal/middleware-matcher";

/**
 * Plan 065 Phase 2: every authenticated path that Next.js proxies to
 * Go MUST be covered by the auth middleware so the lazy
 * `bridge.session` mint fires before the request reaches Go. If a
 * new GO_PROXY_ROUTES entry is added without a matching middleware
 * matcher entry, the cookie won't be set on requests to that path
 * and Go will fall back to the legacy JWE path forever — a silent
 * degradation that defeats the point of plan 065.
 *
 * This test reads `next.config.ts` as text (importing it would pull
 * in `NextConfig` types and the Turbopack runtime, which doesn't
 * load cleanly outside the Next.js build). Each `GO_PROXY_ROUTES`
 * entry is checked against `middlewareMatcher`.
 */
describe("middleware matcher is a strict superset of next.config.ts:GO_PROXY_ROUTES", () => {
  it("every GO_PROXY_ROUTES entry has a matching middlewareMatcher entry", () => {
    const source = readFileSync("next.config.ts", "utf8");
    const start = source.indexOf("const GO_PROXY_ROUTES = [");
    expect(start).toBeGreaterThanOrEqual(0);
    const end = source.indexOf("];", start);
    expect(end).toBeGreaterThan(start);
    const block = source.slice(start, end);

    const proxyRoutes = [...block.matchAll(/"(\/[^"]+)"/g)].map((m) => m[1]);
    expect(proxyRoutes.length).toBeGreaterThan(0);

    const matcherSet = new Set<string>(middlewareMatcher);

    const missing = proxyRoutes.filter((p) => !matcherSet.has(p));
    expect(
      missing,
      `GO_PROXY_ROUTES entries missing from middlewareMatcher: ${missing.join(", ")}.\n` +
        `Add them to BOTH src/middleware.ts AND src/lib/portal/middleware-matcher.ts; ` +
        `otherwise the bridge.session cookie won't be minted before the request reaches Go.`
    ).toEqual([]);
  });
});

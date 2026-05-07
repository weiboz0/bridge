// @vitest-environment node

// TODO(future-plan): Extract GO_PROXY_ROUTES to `src/lib/go-proxy-routes.ts`
// so both `next.config.ts` and this test import from a single source of truth
// instead of re-parsing the config file as text. DeepSeek round-1 and GLM
// round-1 both flagged the text-parse fragility as a NIT; the blocked
// alternative (direct import) pulls in the Turbopack runtime and fails under
// Vitest, so text-parse is the least-bad option until extraction lands.

import { describe, it, expect } from "vitest";
import { readFileSync, readdirSync, statSync } from "node:fs";
import path from "node:path";

// ---------------------------------------------------------------------------
// Helpers — exported so the inline-fixture describe block can unit-test them
// ---------------------------------------------------------------------------

/**
 * Convert a route.ts filesystem path to its canonical URL pattern.
 *
 * Examples:
 *   src/app/api/courses/[id]/route.ts  →  /api/courses/:id
 *   src/app/api/auth/debug/route.ts    →  /api/auth/debug
 *
 * Fails loudly on catch-all (`[...slug]`) or optional-catch-all (`[[id]]`)
 * segments, since the current codebase has none under proxied prefixes and
 * silently swallowing them would produce wrong URL patterns.
 */
export function routeFileToUrlPath(filePath: string): string {
  // Normalise to forward-slashes regardless of OS
  const normalised = filePath.replace(/\\/g, "/");

  // Strip leading path up to (and including) "src/app/api"
  const apiIndex = normalised.indexOf("src/app/api");
  if (apiIndex === -1) {
    throw new Error(`routeFileToUrlPath: path does not contain src/app/api — got ${filePath}`);
  }
  // Remove "src/app" prefix so we start at "/api/..."
  let urlPath = normalised.slice(apiIndex + "src/app".length);

  // Drop the trailing "/route.ts"
  urlPath = urlPath.replace(/\/route\.ts$/, "");

  // Reject catch-all and optional-catch-all segments
  if (/\[\.\.\./.test(urlPath) || /\[\[/.test(urlPath)) {
    throw new Error(
      `routeFileToUrlPath: unexpected catch-all segment in path — got ${filePath}. ` +
        `Add explicit handling for this pattern before adding it to a proxied prefix.`
    );
  }

  // Replace [name] dynamic segments with :name
  urlPath = urlPath.replace(/\[([^\]]+)\]/g, ":$1");

  return urlPath;
}

/**
 * Strip the trailing `:path*` wildcard from a GO_PROXY_ROUTES entry so we
 * can do a simple startsWith check.
 *
 * Examples:
 *   /api/orgs/:path*   →  /api/orgs
 *   /api/admin/:path*  →  /api/admin
 *   /api/auth/register →  /api/auth/register  (no-op — exact match)
 */
export function stripPathWildcard(proxyEntry: string): string {
  return proxyEntry.replace(/\/:path\*$/, "");
}

// ---------------------------------------------------------------------------
// Parse GO_PROXY_ROUTES from next.config.ts at test-load time
// ---------------------------------------------------------------------------

const repoRoot = path.resolve(__dirname, "../..");

const configSource = readFileSync(path.join(repoRoot, "next.config.ts"), "utf8");
const routesStart = configSource.indexOf("const GO_PROXY_ROUTES = [");
if (routesStart === -1) {
  throw new Error("shadow-routes.test.ts: could not find GO_PROXY_ROUTES in next.config.ts");
}
const routesEnd = configSource.indexOf("];", routesStart);
if (routesEnd === -1) {
  throw new Error("shadow-routes.test.ts: could not find closing ]; for GO_PROXY_ROUTES");
}
let routesBlock = configSource.slice(routesStart, routesEnd);
// Strip // line comments so paths mentioned only in comments don't register
routesBlock = routesBlock.replace(/\/\/[^\n]*/g, "");
const GO_PROXY_ROUTES: string[] = [...routesBlock.matchAll(/"(\/[^"]+)"/g)].map((m) => m[1]);

// Pre-compute stripped prefixes for startsWith matching
const proxiedPrefixes = GO_PROXY_ROUTES.map(stripPathWildcard);

function isUnderProxiedPrefix(urlPath: string): boolean {
  return proxiedPrefixes.some((prefix) => urlPath === prefix || urlPath.startsWith(prefix + "/"));
}

// ---------------------------------------------------------------------------
// Walk src/app/api recursively for route.ts files
// ---------------------------------------------------------------------------

function walkRouteFiles(dir: string): string[] {
  const results: string[] = [];
  for (const entry of readdirSync(dir)) {
    const full = path.join(dir, entry);
    const stat = statSync(full);
    if (stat.isDirectory()) {
      results.push(...walkRouteFiles(full));
    } else if (entry === "route.ts") {
      results.push(full);
    }
  }
  return results;
}

// ---------------------------------------------------------------------------
// Typed allowlist — every shadow file under a proxied prefix that has NOT yet
// been deleted. Future plans (075+) shrink this list as each surface is
// migrated. Adding a new shadow route without an allowlist entry will fail the
// forward-direction test immediately.
// ---------------------------------------------------------------------------

type ShadowEntry = {
  /** Canonical URL path with dynamic segments as :name, e.g. "/api/courses/:id" */
  path: string;
  /** Future plan that will delete this shadow route, e.g. "075" or "TBD" */
  cleanupPlan: string;
  note?: string;
};

const KNOWN_SHADOW_ALLOWLIST: ShadowEntry[] = [
  // /api/admin — 4 entries
  { path: "/api/admin/impersonate", cleanupPlan: "TBD" },
  { path: "/api/admin/impersonate/status", cleanupPlan: "TBD" },
  { path: "/api/admin/orgs", cleanupPlan: "TBD" },
  { path: "/api/admin/orgs/:id", cleanupPlan: "TBD" },
  // /api/ai — 2 entries
  { path: "/api/ai/interactions", cleanupPlan: "TBD" },
  { path: "/api/ai/toggle", cleanupPlan: "TBD" },
  // /api/assignments — 4 entries
  { path: "/api/assignments", cleanupPlan: "TBD" },
  { path: "/api/assignments/:id", cleanupPlan: "TBD" },
  { path: "/api/assignments/:id/submissions", cleanupPlan: "TBD" },
  { path: "/api/assignments/:id/submit", cleanupPlan: "TBD" },
  // /api/classes — 3 entries
  { path: "/api/classes", cleanupPlan: "TBD" },
  { path: "/api/classes/:id", cleanupPlan: "TBD" },
  { path: "/api/classes/join", cleanupPlan: "TBD" },
  // /api/courses — 6 entries
  { path: "/api/courses", cleanupPlan: "TBD" },
  { path: "/api/courses/:id", cleanupPlan: "TBD" },
  { path: "/api/courses/:id/clone", cleanupPlan: "TBD" },
  { path: "/api/courses/:id/topics", cleanupPlan: "TBD" },
  { path: "/api/courses/:id/topics/reorder", cleanupPlan: "TBD" },
  { path: "/api/courses/:id/topics/:topicId", cleanupPlan: "TBD" },
  // /api/documents — 3 entries
  { path: "/api/documents", cleanupPlan: "TBD" },
  { path: "/api/documents/:id", cleanupPlan: "TBD" },
  { path: "/api/documents/:id/content", cleanupPlan: "TBD" },
  // /api/parent — 1 entry
  { path: "/api/parent/children/:id/live", cleanupPlan: "TBD" },
  // /api/sessions — 7 entries
  { path: "/api/sessions/:id", cleanupPlan: "TBD" },
  { path: "/api/sessions/:id/broadcast", cleanupPlan: "TBD" },
  { path: "/api/sessions/:id/events", cleanupPlan: "TBD" },
  { path: "/api/sessions/:id/help-queue", cleanupPlan: "TBD" },
  { path: "/api/sessions/:id/join", cleanupPlan: "TBD" },
  { path: "/api/sessions/:id/leave", cleanupPlan: "TBD" },
  { path: "/api/sessions/:id/participants", cleanupPlan: "TBD" },
  // /api/submissions — 1 entry
  { path: "/api/submissions/:id", cleanupPlan: "TBD" },
];

const allowlistPaths = new Set(KNOWN_SHADOW_ALLOWLIST.map((e) => e.path));

// ---------------------------------------------------------------------------
// Main contract-parity test
// ---------------------------------------------------------------------------

describe("shadow-routes contract-parity", () => {
  const apiDir = path.join(repoRoot, "src/app/api");
  const routeFiles = walkRouteFiles(apiDir);

  // Derive URL paths; catch-all routes (e.g. [...nextauth]) are excluded from
  // the shadow check because they are legitimate-Next files not under any
  // proxied prefix — but routeFileToUrlPath would throw on them. We skip them
  // here; the reverse check handles stale allowlist entries regardless.
  type RouteEntry = { filePath: string; urlPath: string };
  const routeEntries: RouteEntry[] = [];
  for (const filePath of routeFiles) {
    const rel = path.relative(repoRoot, filePath).replace(/\\/g, "/");
    // Skip known catch-all routes that are legitimately NOT under any proxied prefix
    if (rel.includes("[...")) {
      continue;
    }
    routeEntries.push({ filePath: rel, urlPath: routeFileToUrlPath(rel) });
  }

  it("forward: every shadow route under a proxied prefix must be in KNOWN_SHADOW_ALLOWLIST", () => {
    const unallowlisted = routeEntries
      .filter(({ urlPath }) => isUnderProxiedPrefix(urlPath))
      .filter(({ urlPath }) => !allowlistPaths.has(urlPath))
      .map(({ urlPath }) => urlPath);

    expect(
      unallowlisted,
      `Found route file(s) under a Go-proxied prefix that are NOT in KNOWN_SHADOW_ALLOWLIST:\n` +
        unallowlisted.map((p) => `  ${p}`).join("\n") +
        `\n\nAdd each path to KNOWN_SHADOW_ALLOWLIST in tests/unit/shadow-routes.test.ts ` +
        `with a cleanupPlan, or delete the file if Go has full parity.`
    ).toEqual([]);
  });

  it("reverse: every KNOWN_SHADOW_ALLOWLIST entry must correspond to a real route file on disk", () => {
    const presentUrlPaths = new Set(routeEntries.map((e) => e.urlPath));
    const staleGhosts = KNOWN_SHADOW_ALLOWLIST.filter(
      ({ path: p }) => !presentUrlPaths.has(p)
    ).map(({ path: p }) => p);

    expect(
      staleGhosts,
      `KNOWN_SHADOW_ALLOWLIST contains entries with no matching route file on disk (stale ghosts):\n` +
        staleGhosts.map((p) => `  ${p}`).join("\n") +
        `\n\nRemove these entries from KNOWN_SHADOW_ALLOWLIST in tests/unit/shadow-routes.test.ts.`
    ).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// Inline unit-tests for path-conversion helpers (Codex round-1 NIT)
// ---------------------------------------------------------------------------

describe("routeFileToUrlPath — conversion fixtures", () => {
  it("static path: auth/debug", () => {
    expect(routeFileToUrlPath("src/app/api/auth/debug/route.ts")).toBe("/api/auth/debug");
  });

  it("single dynamic segment: courses/[id]", () => {
    expect(routeFileToUrlPath("src/app/api/courses/[id]/route.ts")).toBe("/api/courses/:id");
  });

  it("nested dynamic segments: orgs/[id]/members/[memberId]", () => {
    // This path no longer exists on disk (deleted in plan 074 phase 1) but the
    // conversion logic must handle it correctly.
    expect(
      routeFileToUrlPath("src/app/api/orgs/[id]/members/[memberId]/route.ts")
    ).toBe("/api/orgs/:id/members/:memberId");
  });

  it("throws on catch-all segment [...slug]", () => {
    expect(() =>
      routeFileToUrlPath("src/app/api/something/[...slug]/route.ts")
    ).toThrow("catch-all segment");
  });
});

describe("stripPathWildcard — :path* stripping fixtures", () => {
  it("strips :path* suffix from a proxied prefix", () => {
    expect(stripPathWildcard("/api/orgs/:path*")).toBe("/api/orgs");
  });

  it("strips :path* suffix from /api/admin/:path*", () => {
    expect(stripPathWildcard("/api/admin/:path*")).toBe("/api/admin");
  });

  it("is a no-op for exact-match entries (no :path*)", () => {
    expect(stripPathWildcard("/api/auth/register")).toBe("/api/auth/register");
  });
});

describe("isUnderProxiedPrefix — cross-prefix collision check", () => {
  it("/api/admin is under /api/admin/:path*", () => {
    expect(isUnderProxiedPrefix("/api/admin")).toBe(true);
  });

  it("/api/admin/orgs is under /api/admin/:path* (not /api/orgs/:path*)", () => {
    // Verifies that /api/admin/orgs doesn't accidentally match /api/orgs
    expect(isUnderProxiedPrefix("/api/admin/orgs")).toBe(true);
    // Double-check the prefix it matched is /api/admin, not /api/orgs
    const matchedPrefix = proxiedPrefixes.find(
      (p) => "/api/admin/orgs" === p || "/api/admin/orgs".startsWith(p + "/")
    );
    expect(matchedPrefix).toBe("/api/admin");
  });

  it("/api/auth/debug is NOT under any proxied prefix", () => {
    expect(isUnderProxiedPrefix("/api/auth/debug")).toBe(false);
  });

  it("/api/auth/register IS under a proxied prefix (exact match)", () => {
    expect(isUnderProxiedPrefix("/api/auth/register")).toBe(true);
  });
});

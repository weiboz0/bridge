import type { NextConfig } from "next";

// Plan 074 deleted the /api/orgs shadow Next handlers (security gap: PATCH/DELETE
// without the self-action guard the Go side gained in plan 069 phase 4). 31 shadow
// route files remain under proxied prefixes — see `tests/unit/shadow-routes.test.ts`
// `KNOWN_SHADOW_ALLOWLIST` for the canonical worklist. Each entry needs a
// follow-up plan that verifies Go parity before deletion. Adding a NEW shadow
// route without an allowlist entry fails CI immediately (forward check); deleting
// a shadow file without removing its allowlist entry also fails (reverse check).
const GO_API_URL = process.env.GO_API_URL || "http://localhost:8002";

// Routes that have been migrated to Go and should be proxied.
// Add routes here as they are migrated and contract-tested.
const GO_PROXY_ROUTES = [
  // Migrated routes — proxied to Go backend
  "/api/orgs/:path*",
  "/api/admin/:path*",
  "/api/auth/register",
  "/api/courses/:path*",
  "/api/classes/:path*",
  "/api/sessions/:path*",
  "/api/documents/:path*",
  "/api/assignments/:path*",
  "/api/submissions/:path*",
  "/api/annotations/:path*",
  "/api/ai/:path*",
  "/api/parent/:path*",
  "/api/me/:path*",
  "/api/teacher/:path*",
  "/api/org/:path*",
  "/api/schedule/:path*",
  "/api/topics/:path*",
  "/api/problems/:path*",
  "/api/test-cases/:path*",
  "/api/attempts/:path*",
  "/api/s/:path*",
  "/api/units/:path*",
  "/api/collections/:path*",
  "/api/uploads/:path*",
  // Plan 053 phase 1: client-side mint of Hocuspocus connection
  // tokens. The Go API owns the signing key
  // (HOCUSPOCUS_TOKEN_SECRET) and gates per-document access. The
  // sibling /api/internal/realtime/* path is internal-only (called
  // by the Hocuspocus Node process with the shared secret) and is
  // NOT proxied to the browser — it runs server-side only.
  "/api/realtime/:path*",
];

// Plan 065 — /api/internal/sessions is the server-to-server bridge.session
// mint endpoint, called by Next.js's Edge middleware via the
// BRIDGE_INTERNAL_SECRET bearer. It must NOT appear in GO_PROXY_ROUTES:
// that would expose the bearer-protected mint surface to browser traffic
// (the bearer would still gate access, but pushing this onto the public
// proxy is unnecessary attack surface). The mint helper at
// `src/lib/bridge-session-mint.ts` (added in Phase 2) calls Go directly
// via GO_API_URL on the server side, never through the rewrite list.

// SharedArrayBuffer (used by the Pyodide stdin protocol) requires the page
// be cross-origin isolated. Scope these headers to the Problem editor routes
// only — other routes (sign-in popups, embedded assets) keep their current
// less-strict policy.
const COOP_COEP_HEADERS = [
  { key: "Cross-Origin-Opener-Policy", value: "same-origin" },
  { key: "Cross-Origin-Embedder-Policy", value: "require-corp" },
];

const nextConfig: NextConfig = {
  turbopack: {},
  async headers() {
    return [
      {
        source: "/student/classes/:classId/problems/:rest*",
        headers: COOP_COEP_HEADERS,
      },
      {
        source: "/teacher/classes/:classId/problems/:rest*",
        headers: COOP_COEP_HEADERS,
      },
    ];
  },
  async rewrites() {
    const rules = GO_PROXY_ROUTES.map((source) => ({
      source,
      destination: `${GO_API_URL}${source}`,
    }));
    // Apply rewrites at both phases as belt-and-suspenders: `beforeFiles` covers
    // routes that collide with an existing Next.js API file (so the proxy wins),
    // and `fallback` covers routes with no Next.js file at all. Observed on Next 16
    // + turbopack, routes like /api/schedule/* only proxied reliably with `fallback`.
    return {
      beforeFiles: rules,
      fallback: rules,
    };
  },
};

export default nextConfig;

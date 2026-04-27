import type { NextConfig } from "next";

// TODO(plan-038): ~42 Next.js API route files in src/app/api/ overlap with
// GO_PROXY_ROUTES below. Each needs contract-parity verification before
// deletion. This cleanup needs its own migration plan — see docs/plans/038.
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
];

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

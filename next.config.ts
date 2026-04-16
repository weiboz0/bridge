import type { NextConfig } from "next";

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
  "/api/classrooms/:path*",
  "/api/ai/:path*",
  "/api/parent/:path*",
  "/api/me/:path*",
  "/api/teacher/:path*",
  "/api/org/:path*",
];

const nextConfig: NextConfig = {
  turbopack: {},
  async rewrites() {
    // Use beforeFiles to override Next.js API routes with Go proxy
    return {
      beforeFiles: GO_PROXY_ROUTES.map((source) => ({
        source,
        destination: `${GO_API_URL}${source}`,
      })),
    };
  },
};

export default nextConfig;

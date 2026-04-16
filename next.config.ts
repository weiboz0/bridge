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
  "/api/schedule/:path*",
];

const nextConfig: NextConfig = {
  turbopack: {},
  async rewrites() {
    const rules = GO_PROXY_ROUTES.map((source) => ({
      source,
      destination: `${GO_API_URL}${source}`,
    }));
    return {
      // beforeFiles: intercept routes that have existing Next.js API files
      beforeFiles: rules,
      // fallback: catch routes that have NO Next.js API files (e.g., /api/schedule/)
      fallback: rules,
    };
  },
};

export default nextConfig;

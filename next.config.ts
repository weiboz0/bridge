import type { NextConfig } from "next";

const GO_API_URL = process.env.GO_API_URL || "http://localhost:8002";

// Routes that have been migrated to Go and should be proxied.
// Add routes here as they are migrated and contract-tested.
const GO_PROXY_ROUTES = [
  // Phase 1: Orgs, Admin, Auth register
  // Uncomment each line after its contract test passes:
  // "/api/orgs/:path*",
  // "/api/admin/:path*",
  // "/api/auth/register",
];

const nextConfig: NextConfig = {
  turbopack: {},
  async rewrites() {
    return GO_PROXY_ROUTES.map((source) => ({
      source,
      destination: `${GO_API_URL}${source}`,
    }));
  },
};

export default nextConfig;

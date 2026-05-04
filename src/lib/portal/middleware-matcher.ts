/**
 * Single source of truth for which paths the Next.js auth middleware runs on.
 *
 * Lives in its own module so a unit test can assert the contract without
 * pulling in Auth.js (which imports `next/server` at module load and fails
 * outside a Next.js runtime).
 *
 * Two distinct concerns are matched:
 *   - Portal trees (`/teacher/*`, `/student/*`, `/parent/*`, `/org/*`,
 *     `/admin/*`): plan 040 phase 5 deep-link preservation. The
 *     `authorized` callback redirects unauthenticated users to
 *     `/login?callbackUrl=<original>`.
 *   - All authenticated API paths that proxy to Go (`GO_PROXY_ROUTES`
 *     in `next.config.ts`): plan 065 phase 2 added these so the lazy
 *     `bridge.session` mint fires before the request reaches Go. The
 *     parity test in `tests/unit/middleware-proxy-parity.test.ts`
 *     enforces this list is a strict superset of `GO_PROXY_ROUTES`.
 */
export const middlewareMatcher = [
  // Portal trees
  "/teacher/:path*",
  "/student/:path*",
  "/parent/:path*",
  "/org/:path*",
  "/admin/:path*",
  // API paths — mirrors next.config.ts:GO_PROXY_ROUTES
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
  "/api/realtime/:path*",
] as const;

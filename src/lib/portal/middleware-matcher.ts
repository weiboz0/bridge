/**
 * Single source of truth for which paths the Next.js auth middleware runs on.
 *
 * Lives in its own module so a unit test can assert the contract without
 * pulling in Auth.js (which imports `next/server` at module load and fails
 * outside a Next.js runtime).
 *
 * Two distinct concerns are matched:
 *   - `/api/orgs/*`, `/api/admin/*`: legacy API auth-guard (predates plan
 *     040). Returning false from the `authorized` callback emits 401.
 *   - portal trees (`/teacher/*`, `/student/*`, `/parent/*`, `/org/*`,
 *     `/admin/*`): plan 040 phase 5 deep-link preservation. The
 *     `authorized` callback redirects unauthenticated users to
 *     `/login?callbackUrl=<original>`.
 */
export const middlewareMatcher = [
  "/api/orgs/:path*",
  "/api/admin/:path*",
  "/teacher/:path*",
  "/student/:path*",
  "/parent/:path*",
  "/org/:path*",
  "/admin/:path*",
] as const;

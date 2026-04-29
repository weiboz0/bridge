// Auth.js's `auth` export both acts as middleware and consults the
// `authorized` callback in `src/lib/auth.ts`. The callback handles two
// distinct concerns based on the request path:
//
//   * `/api/orgs/*` and `/api/admin/*`: require auth, return 401 if not
//     (preserves the contract this file enforced before plan 040).
//   * Portal trees (`/teacher/*`, `/student/*`, `/parent/*`, `/org/*`,
//     `/admin/*`): require auth, redirect to `/login?callbackUrl=...`
//     when missing so deep links survive a sign-out → sign-in round-trip
//     (review-002 P2 #7 fix).
//
// Plan 047 phase 1: the matcher MUST be a literal in this file. Next.js
// 16 / Turbopack statically analyzes the `config` export and rejects
// imported references — pre-047 the import broke /login compilation
// in dev. The canonical matcher list is duplicated in
// `src/lib/portal/middleware-matcher.ts` (kept for unit tests of the
// `authorized` callback, which can't depend on this file because it
// pulls in Auth.js); a parity test in `tests/unit/middleware-matcher.test.ts`
// guards against drift.
export { auth as middleware } from "@/lib/auth";

export const config = {
  matcher: [
    "/api/orgs/:path*",
    "/api/admin/:path*",
    "/teacher/:path*",
    "/student/:path*",
    "/parent/:path*",
    "/org/:path*",
    "/admin/:path*",
  ],
};

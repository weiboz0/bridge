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
// The matcher is exported from a separate module so unit tests can
// assert the contract without pulling in Auth.js.
export { auth as middleware } from "@/lib/auth";

import { middlewareMatcher } from "@/lib/portal/middleware-matcher";

export const config = {
  matcher: middlewareMatcher,
};

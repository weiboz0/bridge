// Auth.js's `auth` export both acts as middleware and consults the
// `authorized` callback in `src/lib/auth.ts`. The callback handles two
// distinct concerns based on the request path:
//
//   * `/api/orgs/*`, `/api/admin/*`, and other authenticated proxy paths:
//     pass through to the route handler (which enforces auth itself);
//     the matcher list below decides which paths run middleware at all.
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
//
// Plan 065 Phase 2: the middleware is wrapped to lazy-mint a
// `bridge.session` cookie after Auth.js completes a sign-in. Once the
// `BRIDGE_SESSION_AUTH=1` flag flips on Go side (Phase 3), Go middleware
// verifies that cookie instead of decrypting the Auth.js JWE. The mint
// fires on every authenticated middleware invocation but only calls
// Go's mint endpoint when the existing cookie is missing or close to
// expiry — at most once per ~6 days for steady-state users.
import { NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import {
  bridgeSessionExpiringSoon,
  mintBridgeSession,
} from "@/lib/bridge-session-mint";

const BRIDGE_SESSION_COOKIE = "bridge.session";

export default auth(async (req) => {
  // The `authorized` callback in `src/lib/auth.ts` may have already
  // returned a Response (a redirect to /login for unauthenticated
  // portal hits, etc.). Auth.js wraps this Response back into a
  // NextResponse and ships it. Our handler runs only when authorized
  // returned `true` — i.e., the request is allowed to continue.
  const response = NextResponse.next();

  // No session = nothing to mint. Also clear any stale bridge.session
  // cookie that may be lingering from a previous logged-in tab.
  if (!req.auth?.user?.email) {
    if (req.cookies.get(BRIDGE_SESSION_COOKIE)) {
      response.cookies.delete(BRIDGE_SESSION_COOKIE);
    }
    return response;
  }

  const existing = req.cookies.get(BRIDGE_SESSION_COOKIE)?.value;
  if (!bridgeSessionExpiringSoon(existing)) {
    return response;
  }

  // Cookie missing or within 24h of expiry — mint a fresh one. Fails
  // closed: if Go is unreachable or returns an error, leave the
  // existing cookie alone and let Go fall back to JWE for this
  // request. The next middleware invocation will retry.
  const minted = await mintBridgeSession({
    email: req.auth.user.email,
    name: req.auth.user.name ?? "Unknown",
  });
  if (minted) {
    response.cookies.set(BRIDGE_SESSION_COOKIE, minted.token, {
      httpOnly: true,
      sameSite: "lax",
      path: "/",
      secure: process.env.APP_ENV === "production",
      expires: minted.expiresAt,
    });
  }
  return response;
});

export const config = {
  matcher: [
    // Portal trees (auth-redirect paths)
    "/teacher/:path*",
    "/student/:path*",
    "/parent/:path*",
    "/org/:path*",
    "/admin/:path*",
    // Authenticated API paths — strict superset of next.config.ts:GO_PROXY_ROUTES
    // so the lazy mint runs before the request reaches Go. Plan 065 Phase 2
    // adds a parity test (`tests/unit/middleware-proxy-parity.test.ts`) that
    // fails if a future GO_PROXY_ROUTES addition isn't mirrored here.
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
  ],
};

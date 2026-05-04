import { NextResponse } from "next/server";
import { AUTH_SESSION_COOKIE_NAMES } from "@/lib/auth-cookie";

// Plan 065 phase 2 — clear bridge.session alongside the Auth.js
// cookies. Without this, the cookie persists across sign-out for
// its full 7-day TTL, which becomes a real security issue once
// Phase 3 makes Go trust this cookie. The wrapper in
// `src/middleware.ts` also clears bridge.session when it sees a
// null session, but the middleware doesn't run on `/api/auth/*`
// routes (signout flows through them) — so this explicit cleanup
// is the load-bearing path.
const BRIDGE_SESSION_COOKIE = "bridge.session";

const ALL_SIGNOUT_COOKIES = [
  ...AUTH_SESSION_COOKIE_NAMES,
  BRIDGE_SESSION_COOKIE,
] as const;

/**
 * Explicitly expires every Auth.js session cookie variant the browser may
 * still hold AND the Bridge session cookie (plan 065). Auth.js client
 * `signOut()` only clears the cookie name it currently uses — when the
 * scheme changed (HTTP <-> HTTPS) or a prior deployment used different
 * attributes, the unmatched variant survives and re-injects stale
 * identity on the next sign-in.
 *
 * This route emits a `Set-Cookie` for each known cookie name with the
 * exact attributes used by the cookie's owner (Path=/, HttpOnly,
 * SameSite=Lax, Secure on the secure-prefix variant), so the browser
 * deletes them.
 *
 * Call this BEFORE `signOut()` so we have a fresh response to attach
 * cookies to; signOut then handles the Auth.js client state.
 */
export async function POST() {
  const res = NextResponse.json({ cleared: ALL_SIGNOUT_COOKIES });

  for (const name of AUTH_SESSION_COOKIE_NAMES) {
    res.cookies.set({
      name,
      value: "",
      path: "/",
      httpOnly: true,
      sameSite: "lax",
      secure: name.startsWith("__Secure-"),
      maxAge: 0,
    });
  }

  // bridge.session uses the SAME attributes as the Auth.js insecure
  // variant: Path=/, HttpOnly, SameSite=Lax, Secure-in-prod. Match
  // them exactly so the browser deletes the cookie.
  res.cookies.set({
    name: BRIDGE_SESSION_COOKIE,
    value: "",
    path: "/",
    httpOnly: true,
    sameSite: "lax",
    secure: process.env.APP_ENV === "production",
    maxAge: 0,
  });

  return res;
}

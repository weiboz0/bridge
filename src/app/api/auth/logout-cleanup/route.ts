import { NextResponse } from "next/server";
import { AUTH_SESSION_COOKIE_NAMES } from "@/lib/auth-cookie";

/**
 * Explicitly expires every Auth.js session cookie variant the browser may
 * still hold. Auth.js client `signOut()` only clears the cookie name it
 * currently uses — when the scheme changed (HTTP <-> HTTPS) or a prior
 * deployment used different attributes, the unmatched variant survives
 * and re-injects stale identity on the next sign-in.
 *
 * This route emits a `Set-Cookie` for each known cookie name with the
 * exact attributes Auth.js v5 uses (Path=/, HttpOnly, SameSite=Lax,
 * Secure on the secure-prefix variant), so the browser deletes them.
 *
 * Call this BEFORE `signOut()` so we have a fresh response to attach
 * cookies to; signOut then handles the Auth.js client state.
 */
export async function POST() {
  const res = NextResponse.json({ cleared: AUTH_SESSION_COOKIE_NAMES });

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

  return res;
}

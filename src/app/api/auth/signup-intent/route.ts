import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";

const SCHEMA = z.object({
  role: z.enum(["teacher", "student"]),
  inviteCode: z.string().min(1).max(64).optional(),
});

const COOKIE_NAME = "bridge-signup-intent";
const MAX_AGE_SECONDS = 5 * 60;

/**
 * Stores signup intent for the OAuth round-trip.
 *
 * Plan 043 phase 5 (Codex correction #4): Google OAuth registration was
 * losing the role + invite the user picked on the register page because
 * Auth.js's signIn callback runs server-side without access to the
 * register page's form state. Solution: register page POSTs to this
 * route before redirecting to Google, the cookie travels with the
 * OAuth round-trip, and the signIn callback in src/lib/auth.ts reads
 * + clears it when creating the OAuth user.
 *
 * Short Max-Age (5 min) keeps stale state from polluting the next
 * signup. HttpOnly so client JS can't read it. SameSite=Lax so the
 * cookie survives the Google redirect-back to the same site.
 */
export async function POST(request: NextRequest) {
  const body = await request.json().catch(() => null);
  const parsed = SCHEMA.safeParse(body);
  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid intent", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const res = NextResponse.json({ ok: true });
  res.cookies.set({
    name: COOKIE_NAME,
    value: JSON.stringify(parsed.data),
    httpOnly: true,
    sameSite: "lax",
    path: "/",
    secure: process.env.NODE_ENV === "production",
    maxAge: MAX_AGE_SECONDS,
  });
  return res;
}

export const SIGNUP_INTENT_COOKIE = COOKIE_NAME;

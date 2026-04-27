import { NextResponse } from "next/server";
import { cookies, headers } from "next/headers";
import { auth } from "@/lib/auth";
import { api, ApiError } from "@/lib/api-client";
import {
  getSessionCookieName,
  getOtherSessionCookieName,
  AUTH_SESSION_COOKIE_NAMES,
} from "@/lib/auth-cookie";

/**
 * Dev-only auth identity diagnostic.
 *
 * Reports what each layer sees:
 *  - nextAuthUserId: the user Auth.js resolved on the Next side
 *  - goClaimsUserId: the user the Go backend resolved from the same request
 *  - cookieNamesPresent / cookieNameUsed / staleVariantPresent
 *  - configured Auth.js scheme + xForwardedProto for layer comparison
 *  - match: true iff both sides agree (or both null)
 *
 * Returns 404 in production builds — this is a debugging aid, not an API.
 */
export async function GET() {
  if (process.env.NODE_ENV === "production") {
    return new NextResponse("Not found", { status: 404 });
  }

  const cookieStore = await cookies();
  const headerStore = await headers();
  const session = await auth();

  const canonicalName = getSessionCookieName();
  const otherName = getOtherSessionCookieName();
  const namesPresent = AUTH_SESSION_COOKIE_NAMES.filter(
    (n) => cookieStore.get(n)?.value
  );
  const cookieNameUsed = cookieStore.get(canonicalName)?.value ? canonicalName : null;
  const staleVariantPresent =
    cookieNameUsed === null && cookieStore.get(otherName)?.value ? otherName : null;

  let goClaimsUserId: string | null = null;
  let goError: string | null = null;
  try {
    const identity = await api<{ userId: string; impersonatedBy?: string }>(
      "/api/me/identity"
    );
    goClaimsUserId = identity.userId;
  } catch (err) {
    if (err instanceof ApiError) {
      goError = `${err.status}: ${err.message}`;
    } else {
      goError = err instanceof Error ? err.message : String(err);
    }
  }

  const nextAuthUserId = session?.user?.id ?? null;
  const match =
    goClaimsUserId !== null && nextAuthUserId !== null
      ? goClaimsUserId === nextAuthUserId
      : goClaimsUserId === nextAuthUserId; // both null also counts as match

  return NextResponse.json({
    nextAuthUserId,
    goClaimsUserId,
    goError,
    cookieNamesPresent: namesPresent,
    cookieNameUsed,
    staleVariantPresent,
    canonicalCookieName: canonicalName,
    xForwardedProto: headerStore.get("x-forwarded-proto"),
    authjsConfigUrl: process.env.NEXTAUTH_URL || process.env.AUTH_URL || "",
    match,
  });
}

import { NextResponse } from "next/server";
import { getIdentity } from "@/lib/identity";

// Plan 065 phase 4 — gate on identity.impersonatedBy. Codex pass-1
// flagged the original re-cookie-validation shape: under active
// impersonation, /api/me/identity returns the TARGET'S identity
// (with `impersonatedBy` set to the admin's user id), so the old
// `originalUserId === identity.userId` check would always fail.
// Reading `identity.impersonatedBy` directly is the right primitive
// because Go's middleware is already the single authority for
// whether impersonation is active and authorized — duplicating
// that logic here was the bug.
export async function GET() {
  const identity = await getIdentity();
  if (!identity?.userId) {
    return NextResponse.json({ impersonating: null });
  }
  if (!identity.impersonatedBy) {
    return NextResponse.json({ impersonating: null });
  }
  return NextResponse.json({
    impersonating: {
      targetUserId: identity.userId,
      targetName: identity.name,
      targetEmail: identity.email,
    },
  });
}

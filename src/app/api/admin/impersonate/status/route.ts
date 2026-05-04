import { NextResponse } from "next/server";
import { cookies } from "next/headers";
import { getIdentity } from "@/lib/identity";

export async function GET() {
  const identity = await getIdentity();
  if (!identity?.userId) {
    return NextResponse.json({ impersonating: null });
  }

  const cookieStore = await cookies();
  const impersonateCookie = cookieStore.get("bridge-impersonate");

  // Plan 065 phase 4 — admin status from /api/me/identity (live DB).
  if (!impersonateCookie?.value || !identity.isPlatformAdmin) {
    return NextResponse.json({ impersonating: null });
  }

  try {
    const data = JSON.parse(impersonateCookie.value);
    if (data.originalUserId !== identity.userId) {
      return NextResponse.json({ impersonating: null });
    }
    return NextResponse.json({
      impersonating: {
        targetUserId: data.targetUserId,
        targetName: data.targetName,
        targetEmail: data.targetEmail,
      },
    });
  } catch {
    return NextResponse.json({ impersonating: null });
  }
}

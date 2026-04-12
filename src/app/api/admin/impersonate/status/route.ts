import { NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { cookies } from "next/headers";

export async function GET() {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ impersonating: null });
  }

  const cookieStore = await cookies();
  const impersonateCookie = cookieStore.get("bridge-impersonate");

  if (!impersonateCookie?.value || !session.user.isPlatformAdmin) {
    return NextResponse.json({ impersonating: null });
  }

  try {
    const data = JSON.parse(impersonateCookie.value);
    if (data.originalUserId !== session.user.id) {
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

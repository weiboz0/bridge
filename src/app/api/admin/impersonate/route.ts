import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { users } from "@/lib/db/schema";
import { eq } from "drizzle-orm";
import { cookies } from "next/headers";

const impersonateSchema = z.object({
  userId: z.string().uuid(),
});

export async function POST(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id || !session.user.isPlatformAdmin) {
    return NextResponse.json({ error: "Platform admin required" }, { status: 403 });
  }

  const body = await request.json();
  const parsed = impersonateSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json({ error: "Invalid input" }, { status: 400 });
  }

  const [targetUser] = await db
    .select()
    .from(users)
    .where(eq(users.id, parsed.data.userId));

  if (!targetUser) {
    return NextResponse.json({ error: "User not found" }, { status: 404 });
  }

  // Store impersonation in a cookie
  const cookieStore = await cookies();
  cookieStore.set("bridge-impersonate", JSON.stringify({
    originalUserId: session.user.id,
    targetUserId: targetUser.id,
    targetName: targetUser.name,
    targetEmail: targetUser.email,
  }), {
    httpOnly: true,
    sameSite: "lax",
    path: "/",
    maxAge: 60 * 60, // 1 hour
  });

  return NextResponse.json({
    impersonating: {
      id: targetUser.id,
      name: targetUser.name,
      email: targetUser.email,
    },
  });
}

export async function DELETE() {
  const cookieStore = await cookies();
  cookieStore.delete("bridge-impersonate");
  return NextResponse.json({ stopped: true });
}

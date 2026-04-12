import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getLinkedChildren } from "@/lib/parent-links";
import { getActiveSessionForStudent } from "@/lib/attendance";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id: childId } = await params;

  // Verify parent-child link
  const children = await getLinkedChildren(db, session.user.id);
  if (!children.some((c) => c.userId === childId)) {
    return NextResponse.json({ error: "Not linked to this child" }, { status: 403 });
  }

  const activeSession = await getActiveSessionForStudent(db, childId);
  return NextResponse.json(activeSession);
}

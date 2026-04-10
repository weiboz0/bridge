import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getActiveSession } from "@/lib/sessions";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const activeSession = await getActiveSession(db, id);

  if (!activeSession) {
    return NextResponse.json(null);
  }

  return NextResponse.json(activeSession);
}

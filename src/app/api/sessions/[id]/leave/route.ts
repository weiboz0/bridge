import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { leaveSession } from "@/lib/sessions";
import { sessionEventBus } from "@/lib/sse";

export async function POST(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const participant = await leaveSession(db, id, session.user.id);

  if (!participant) {
    return NextResponse.json({ error: "Not a participant" }, { status: 404 });
  }

  sessionEventBus.emit(id, "student_left", {
    studentId: session.user.id,
    name: session.user.name,
  });

  return NextResponse.json(participant);
}

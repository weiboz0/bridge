import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSession, joinSession } from "@/lib/sessions";
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
  const liveSession = await getSession(db, id);

  if (!liveSession) {
    return NextResponse.json({ error: "Session not found" }, { status: 404 });
  }

  if (liveSession.status !== "active") {
    return NextResponse.json({ error: "Session has ended" }, { status: 400 });
  }

  const participant = await joinSession(db, id, session.user.id);

  sessionEventBus.emit(id, "student_joined", {
    studentId: session.user.id,
    name: session.user.name,
  });

  return NextResponse.json(participant || { sessionId: id, studentId: session.user.id });
}

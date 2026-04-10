import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSession, endSession } from "@/lib/sessions";
import { sessionEventBus } from "@/lib/sse";

export async function GET(
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
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  return NextResponse.json(liveSession);
}

export async function PATCH(
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
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  if (liveSession.teacherId !== session.user.id) {
    return NextResponse.json({ error: "Only the teacher can end a session" }, { status: 403 });
  }

  const ended = await endSession(db, id);
  sessionEventBus.emit(id, "session_ended", { sessionId: id });

  return NextResponse.json(ended);
}

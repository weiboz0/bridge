import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSessionParticipants, updateParticipantStatus } from "@/lib/sessions";
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
  const participants = await getSessionParticipants(db, id);
  const helpQueue = participants.filter((p) => p.helpRequestedAt);

  return NextResponse.json(helpQueue);
}

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const body = await request.json();
  const { raised } = body;

  const newStatus = raised ? "needs_help" : "active";
  const participant = await updateParticipantStatus(db, id, session.user.id, newStatus);

  if (!participant) {
    return NextResponse.json({ error: "Not a participant" }, { status: 404 });
  }

  sessionEventBus.emit(id, raised ? "hand_raised" : "hand_lowered", {
    studentId: session.user.id,
    name: session.user.name,
  });

  return NextResponse.json(participant);
}

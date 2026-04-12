import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSession } from "@/lib/sessions";
import { sessionEventBus } from "@/lib/sse";

const broadcastSchema = z.object({
  active: z.boolean(),
});

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;

  // Verify session exists and user is the teacher
  const liveSession = await getSession(db, id);
  if (!liveSession) {
    return NextResponse.json({ error: "Session not found" }, { status: 404 });
  }
  if (liveSession.teacherId !== session.user.id && !session.user.isPlatformAdmin) {
    return NextResponse.json({ error: "Only the teacher can broadcast" }, { status: 403 });
  }

  const body = await request.json();
  const parsed = broadcastSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json({ error: "Invalid input" }, { status: 400 });
  }

  const event = parsed.data.active ? "broadcast_started" : "broadcast_ended";
  sessionEventBus.emit(id, event, { teacherId: session.user.id });

  return NextResponse.json({ active: parsed.data.active });
}

import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { db } from "@/lib/db";
import { getIdentity } from "@/lib/identity";
import { getSession } from "@/lib/sessions";
import { sessionEventBus } from "@/lib/sse";

const broadcastSchema = z.object({
  active: z.boolean(),
});

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const identity = await getIdentity();
  if (!identity?.userId) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;

  // Verify session exists and user is the teacher
  const liveSession = await getSession(db, id);
  if (!liveSession) {
    return NextResponse.json({ error: "Session not found" }, { status: 404 });
  }
  // Plan 065 phase 4 — admin status from /api/me/identity (live DB).
  if (liveSession.teacherId !== identity.userId && !identity.isPlatformAdmin) {
    return NextResponse.json({ error: "Only the teacher can broadcast" }, { status: 403 });
  }

  const body = await request.json();
  const parsed = broadcastSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json({ error: "Invalid input" }, { status: 400 });
  }

  const event = parsed.data.active ? "broadcast_started" : "broadcast_ended";
  sessionEventBus.emit(id, event, { teacherId: identity.userId });

  return NextResponse.json({ active: parsed.data.active });
}

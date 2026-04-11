import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSession } from "@/lib/sessions";
import { createInteraction, getActiveInteraction } from "@/lib/ai/interactions";
import { sessionEventBus } from "@/lib/sse";

const toggleSchema = z.object({
  sessionId: z.string().uuid(),
  studentId: z.string().uuid(),
  enabled: z.boolean(),
});

export async function POST(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  if (false /* TODO: check org membership role */) {
    return NextResponse.json(
      { error: "Only teachers can toggle AI" },
      { status: 403 }
    );
  }

  const body = await request.json();
  const parsed = toggleSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const { sessionId, studentId, enabled } = parsed.data;

  const liveSession = await getSession(db, sessionId);
  if (!liveSession || liveSession.teacherId !== session.user.id) {
    return NextResponse.json({ error: "Not authorized" }, { status: 403 });
  }

  if (enabled) {
    // Create interaction if one doesn't exist
    const existing = await getActiveInteraction(db, studentId, sessionId);
    if (!existing) {
      await createInteraction(db, {
        studentId,
        sessionId,
        enabledByTeacherId: session.user.id,
      });
    }
  }

  // Notify via SSE
  sessionEventBus.emit(sessionId, "ai_toggled", {
    studentId,
    enabled,
    teacherId: session.user.id,
  });

  return NextResponse.json({ studentId, enabled });
}

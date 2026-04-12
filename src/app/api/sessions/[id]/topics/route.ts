import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSession } from "@/lib/sessions";
import { linkSessionTopic, unlinkSessionTopic, getSessionTopics } from "@/lib/session-topics";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const topics = await getSessionTopics(db, id);
  return NextResponse.json(topics);
}

const linkSchema = z.object({ topicId: z.string().uuid() });

async function verifyTeacher(sessionId: string, userId: string, isPlatformAdmin: boolean) {
  const liveSession = await getSession(db, sessionId);
  if (!liveSession) return null;
  if (liveSession.teacherId !== userId && !isPlatformAdmin) return null;
  return liveSession;
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

  const liveSession = await verifyTeacher(id, session.user.id, session.user.isPlatformAdmin);
  if (!liveSession) {
    return NextResponse.json({ error: "Only the session teacher can manage topics" }, { status: 403 });
  }

  const body = await request.json();
  const parsed = linkSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json({ error: "Invalid input" }, { status: 400 });
  }

  const link = await linkSessionTopic(db, id, parsed.data.topicId);
  return NextResponse.json(link, { status: 201 });
}

export async function DELETE(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;

  const liveSession = await verifyTeacher(id, session.user.id, session.user.isPlatformAdmin);
  if (!liveSession) {
    return NextResponse.json({ error: "Only the session teacher can manage topics" }, { status: 403 });
  }

  const body = await request.json();
  const parsed = linkSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json({ error: "Invalid input" }, { status: 400 });
  }

  await unlinkSessionTopic(db, id, parsed.data.topicId);
  return NextResponse.json({ success: true });
}

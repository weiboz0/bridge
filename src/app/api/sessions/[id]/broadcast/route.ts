import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
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
  const body = await request.json();
  const parsed = broadcastSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json({ error: "Invalid input" }, { status: 400 });
  }

  const event = parsed.data.active ? "broadcast_started" : "broadcast_ended";
  sessionEventBus.emit(id, event, { teacherId: session.user.id });

  return NextResponse.json({ active: parsed.data.active });
}

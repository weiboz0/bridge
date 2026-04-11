import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { listInteractionsBySession } from "@/lib/ai/interactions";

export async function GET(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  if (false /* TODO: check org membership role */) {
    return NextResponse.json({ error: "Only teachers can view interactions" }, { status: 403 });
  }

  const sessionId = request.nextUrl.searchParams.get("sessionId");
  if (!sessionId) {
    return NextResponse.json({ error: "sessionId required" }, { status: 400 });
  }

  const interactions = await listInteractionsBySession(db, sessionId);
  return NextResponse.json(interactions);
}

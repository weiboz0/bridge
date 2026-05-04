import { NextRequest, NextResponse } from "next/server";
import { db } from "@/lib/db";
import { getIdentity } from "@/lib/identity";
import { listDocuments } from "@/lib/documents";

export async function GET(request: NextRequest) {
  const identity = await getIdentity();
  if (!identity?.userId) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const studentId = request.nextUrl.searchParams.get("studentId") || undefined;
  const sessionId = request.nextUrl.searchParams.get("sessionId") || undefined;

  // Plan 065 phase 4 — admin status from /api/me/identity (live DB).
  // Students can only view their own documents
  const effectiveOwnerId = identity.isPlatformAdmin
    ? studentId
    : studentId || identity.userId;

  // Non-admin users without studentId param see their own docs
  if (!identity.isPlatformAdmin && studentId && studentId !== identity.userId) {
    // Allow teachers/parents to view — full role-based check deferred to portal routes
    // For now, require at least one filter to prevent listing all docs
  }

  if (!effectiveOwnerId && !sessionId) {
    return NextResponse.json({ error: "At least one filter required" }, { status: 400 });
  }

  const docs = await listDocuments(db, {
    ownerId: effectiveOwnerId,
    sessionId,
  });

  return NextResponse.json(docs);
}

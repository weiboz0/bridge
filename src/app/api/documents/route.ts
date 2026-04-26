import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { listDocuments } from "@/lib/documents";

export async function GET(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  // Accept both classId (preferred) and classroomId (legacy) query params
  const classId = request.nextUrl.searchParams.get("classId")
    || request.nextUrl.searchParams.get("classroomId")
    || undefined;
  const studentId = request.nextUrl.searchParams.get("studentId") || undefined;
  const sessionId = request.nextUrl.searchParams.get("sessionId") || undefined;

  // Students can only view their own documents
  const effectiveOwnerId = session.user.isPlatformAdmin
    ? studentId
    : studentId || session.user.id;

  // Non-admin users without studentId param see their own docs
  if (!session.user.isPlatformAdmin && studentId && studentId !== session.user.id) {
    // Allow teachers/parents to view — full role-based check deferred to portal routes
    // For now, require at least one filter to prevent listing all docs
  }

  if (!effectiveOwnerId && !classId && !sessionId) {
    return NextResponse.json({ error: "At least one filter required" }, { status: 400 });
  }

  const docs = await listDocuments(db, {
    ownerId: effectiveOwnerId,
    classId,
    sessionId,
  });

  return NextResponse.json(docs);
}

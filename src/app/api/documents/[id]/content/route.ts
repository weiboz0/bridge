import { NextRequest, NextResponse } from "next/server";
import { db } from "@/lib/db";
import { getIdentity } from "@/lib/identity";
import { getDocument } from "@/lib/documents";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const identity = await getIdentity();
  if (!identity?.userId) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const doc = await getDocument(db, id);

  if (!doc) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  // Plan 065 phase 4 — admin status from /api/me/identity (live DB).
  // Only owner or platform admin can view content
  // Teacher/parent access will be refined with class membership checks in portal routes
  if (doc.ownerId !== identity.userId && !identity.isPlatformAdmin) {
    return NextResponse.json({ error: "Access denied" }, { status: 403 });
  }

  // Return plain text only (for parent viewing, search, etc.)
  return NextResponse.json({
    id: doc.id,
    ownerId: doc.ownerId,
    language: doc.language,
    plainText: doc.plainText,
    updatedAt: doc.updatedAt,
  });
}

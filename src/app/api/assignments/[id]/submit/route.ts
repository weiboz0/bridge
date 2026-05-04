import { NextRequest, NextResponse } from "next/server";
import { db } from "@/lib/db";
import { getIdentity } from "@/lib/identity";
import { getAssignment } from "@/lib/assignments";
import { createSubmission } from "@/lib/submissions";
import { listClassMembers } from "@/lib/class-memberships";

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const identity = await getIdentity();
  if (!identity?.userId) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const assignment = await getAssignment(db, id);
  if (!assignment) {
    return NextResponse.json({ error: "Assignment not found" }, { status: 404 });
  }

  // Verify user is a student in the class
  const members = await listClassMembers(db, assignment.classId);
  const isMember = members.some((m) => m.userId === identity.userId);
  // Plan 065 phase 4 — admin status from /api/me/identity (live DB).
  if (!isMember && !identity.isPlatformAdmin) {
    return NextResponse.json({ error: "Not a member of this class" }, { status: 403 });
  }

  const body = await request.json().catch(() => ({}));
  const documentId = body.documentId || null;

  const submission = await createSubmission(db, {
    assignmentId: id,
    studentId: identity.userId,
    documentId,
  });

  if (!submission) {
    return NextResponse.json({ error: "Already submitted" }, { status: 409 });
  }

  return NextResponse.json(submission, { status: 201 });
}

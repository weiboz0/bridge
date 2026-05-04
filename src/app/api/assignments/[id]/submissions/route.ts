import { NextRequest, NextResponse } from "next/server";
import { db } from "@/lib/db";
import { getIdentity } from "@/lib/identity";
import { getAssignment } from "@/lib/assignments";
import { listSubmissionsByAssignment } from "@/lib/submissions";
import { listClassMembers } from "@/lib/class-memberships";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const identity = await getIdentity();
  if (!identity?.userId) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const assignment = await getAssignment(db, id);
  if (!assignment) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  // Only instructors can view all submissions
  const members = await listClassMembers(db, assignment.classId);
  const isInstructor = members.some(
    (m) => m.userId === identity.userId && (m.role === "instructor" || m.role === "ta")
  );
  // Plan 065 phase 4 — admin status from /api/me/identity (live DB).
  if (!isInstructor && !identity.isPlatformAdmin) {
    return NextResponse.json({ error: "Only instructors can view submissions" }, { status: 403 });
  }

  const list = await listSubmissionsByAssignment(db, id);
  return NextResponse.json(list);
}

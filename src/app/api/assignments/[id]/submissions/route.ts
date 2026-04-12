import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getAssignment } from "@/lib/assignments";
import { listSubmissionsByAssignment } from "@/lib/submissions";
import { listClassMembers } from "@/lib/class-memberships";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
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
    (m) => m.userId === session.user.id && (m.role === "instructor" || m.role === "ta")
  );
  if (!isInstructor && !session.user.isPlatformAdmin) {
    return NextResponse.json({ error: "Only instructors can view submissions" }, { status: 403 });
  }

  const list = await listSubmissionsByAssignment(db, id);
  return NextResponse.json(list);
}

import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSubmission, gradeSubmission } from "@/lib/submissions";
import { getAssignment } from "@/lib/assignments";
import { listClassMembers } from "@/lib/class-memberships";

const gradeSchema = z.object({
  grade: z.number().min(0).max(100),
  feedback: z.string().max(5000).optional(),
});

export async function PATCH(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const submission = await getSubmission(db, id);
  if (!submission) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  // Verify user is instructor in the assignment's class
  const assignment = await getAssignment(db, submission.assignmentId);
  if (!assignment) {
    return NextResponse.json({ error: "Assignment not found" }, { status: 404 });
  }

  const members = await listClassMembers(db, assignment.classId);
  const isInstructor = members.some(
    (m) => m.userId === session.user.id && (m.role === "instructor" || m.role === "ta")
  );
  if (!isInstructor && !session.user.isPlatformAdmin) {
    return NextResponse.json({ error: "Only instructors can grade" }, { status: 403 });
  }

  const body = await request.json();
  const parsed = gradeSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const graded = await gradeSubmission(db, id, parsed.data.grade, parsed.data.feedback);
  return NextResponse.json(graded);
}

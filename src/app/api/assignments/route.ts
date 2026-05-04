import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { getIdentity } from "@/lib/identity";
import { db } from "@/lib/db";
import { createAssignment, listAssignmentsByClass } from "@/lib/assignments";
import { listClassMembers } from "@/lib/class-memberships";

const createSchema = z.object({
  classId: z.string().uuid(),
  topicId: z.string().uuid().optional(),
  title: z.string().min(1).max(255),
  description: z.string().max(5000).optional(),
  starterCode: z.string().optional(),
  dueDate: z.string().datetime().optional(),
  rubric: z.record(z.string(), z.unknown()).optional(),
});

export async function POST(request: NextRequest) {
  const identity = await getIdentity();
  if (!identity?.userId) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const body = await request.json();
  const parsed = createSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  // Verify user is instructor/TA in this class
  const members = await listClassMembers(db, parsed.data.classId);
  const isInstructor = members.some(
    (m) => m.userId === identity.userId && (m.role === "instructor" || m.role === "ta")
  );
  // Plan 065 phase 4 — admin status from /api/me/identity (live DB).
  if (!isInstructor && !identity.isPlatformAdmin) {
    return NextResponse.json({ error: "Only instructors can create assignments" }, { status: 403 });
  }

  const assignment = await createAssignment(db, {
    ...parsed.data,
    dueDate: parsed.data.dueDate ? new Date(parsed.data.dueDate) : undefined,
  });

  return NextResponse.json(assignment, { status: 201 });
}

export async function GET(request: NextRequest) {
  const identity = await getIdentity();
  if (!identity?.userId) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const classId = request.nextUrl.searchParams.get("classId");
  if (!classId) {
    return NextResponse.json({ error: "classId required" }, { status: 400 });
  }

  // Verify user is a member of this class
  const members = await listClassMembers(db, classId);
  const isMember = members.some((m) => m.userId === identity.userId);
  if (!isMember && !identity.isPlatformAdmin) {
    return NextResponse.json({ error: "Not a member of this class" }, { status: 403 });
  }

  const list = await listAssignmentsByClass(db, classId);
  return NextResponse.json(list);
}

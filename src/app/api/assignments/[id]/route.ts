import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { db } from "@/lib/db";
import { getIdentity } from "@/lib/identity";
import { getAssignment, updateAssignment, deleteAssignment } from "@/lib/assignments";
import { listClassMembers } from "@/lib/class-memberships";

const updateSchema = z.object({
  title: z.string().min(1).max(255).optional(),
  description: z.string().max(5000).optional(),
  starterCode: z.string().optional(),
  dueDate: z.string().datetime().optional(),
  rubric: z.record(z.string(), z.unknown()).optional(),
});

async function verifyInstructor(classId: string, userId: string, isPlatformAdmin: boolean) {
  if (isPlatformAdmin) return true;
  const members = await listClassMembers(db, classId);
  return members.some(
    (m) => m.userId === userId && (m.role === "instructor" || m.role === "ta")
  );
}

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
  return NextResponse.json(assignment);
}

export async function PATCH(
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
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  // Plan 065 phase 4 — admin status from /api/me/identity (live DB).
  if (!await verifyInstructor(assignment.classId, identity.userId, identity.isPlatformAdmin)) {
    return NextResponse.json({ error: "Only instructors can update assignments" }, { status: 403 });
  }

  const body = await request.json();
  const parsed = updateSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const updated = await updateAssignment(db, id, {
    ...parsed.data,
    dueDate: parsed.data.dueDate ? new Date(parsed.data.dueDate) : undefined,
  });
  return NextResponse.json(updated);
}

export async function DELETE(
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

  if (!await verifyInstructor(assignment.classId, identity.userId, identity.isPlatformAdmin)) {
    return NextResponse.json({ error: "Only instructors can delete assignments" }, { status: 403 });
  }

  const deleted = await deleteAssignment(db, id);
  return NextResponse.json(deleted);
}

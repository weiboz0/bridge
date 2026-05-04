import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { db } from "@/lib/db";
import { getIdentity } from "@/lib/identity";
import { getCourse, updateCourse, deleteCourse } from "@/lib/courses";

const updateSchema = z.object({
  title: z.string().min(1).max(255).optional(),
  description: z.string().max(5000).optional(),
  gradeLevel: z.enum(["K-5", "6-8", "9-12"]).optional(),
  language: z.enum(["python", "javascript", "blockly"]).optional(),
  isPublished: z.boolean().optional(),
});

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const identity = await getIdentity();
  if (!identity?.userId) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const course = await getCourse(db, id);

  if (!course) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  return NextResponse.json(course);
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
  const course = await getCourse(db, id);

  if (!course) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  // Plan 065 phase 4 — admin status from /api/me/identity (live DB).
  if (course.createdBy !== identity.userId && !identity.isPlatformAdmin) {
    return NextResponse.json({ error: "Only the course creator can update" }, { status: 403 });
  }

  const body = await request.json();
  const parsed = updateSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const updated = await updateCourse(db, id, parsed.data);
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
  const course = await getCourse(db, id);

  if (!course) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  if (course.createdBy !== identity.userId && !identity.isPlatformAdmin) {
    return NextResponse.json({ error: "Only the course creator can delete" }, { status: 403 });
  }

  const deleted = await deleteCourse(db, id);
  return NextResponse.json(deleted);
}

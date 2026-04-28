import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getCourse } from "@/lib/courses";
import { getTopic, updateTopic, deleteTopic } from "@/lib/topics";

// Plan 044 phase 3: lessonContent and starterCode are no longer accepted
// on topic update. Strict mode rejects requests that still include the
// deprecated fields. Use POST /api/courses/{cid}/topics/{tid}/link-unit
// to attach a teaching_unit instead.
const updateSchema = z.object({
  title: z.string().min(1).max(255).optional(),
  description: z.string().max(5000).optional(),
}).strict();

async function verifyCourseOwnership(courseId: string, userId: string, isPlatformAdmin: boolean) {
  const course = await getCourse(db, courseId);
  if (!course) return false;
  return course.createdBy === userId || isPlatformAdmin;
}

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string; topicId: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id: courseId, topicId } = await params;

  if (!await verifyCourseOwnership(courseId, session.user.id, session.user.isPlatformAdmin)) {
    return NextResponse.json({ error: "Access denied" }, { status: 403 });
  }

  const topic = await getTopic(db, topicId);
  if (!topic) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  return NextResponse.json(topic);
}

export async function PATCH(
  request: NextRequest,
  { params }: { params: Promise<{ id: string; topicId: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id: courseId, topicId } = await params;

  if (!await verifyCourseOwnership(courseId, session.user.id, session.user.isPlatformAdmin)) {
    return NextResponse.json({ error: "Access denied" }, { status: 403 });
  }

  const body = await request.json();
  const parsed = updateSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const updated = await updateTopic(db, topicId, parsed.data);
  if (!updated) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  return NextResponse.json(updated);
}

export async function DELETE(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string; topicId: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id: courseId, topicId } = await params;

  if (!await verifyCourseOwnership(courseId, session.user.id, session.user.isPlatformAdmin)) {
    return NextResponse.json({ error: "Access denied" }, { status: 403 });
  }

  const deleted = await deleteTopic(db, topicId);
  if (!deleted) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  return NextResponse.json(deleted);
}

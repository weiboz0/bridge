import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { createSession } from "@/lib/sessions";
import { getClassroom } from "@/lib/classrooms";

const createSchema = z.object({
  classroomId: z.string().uuid(),
  settings: z.record(z.unknown()).optional(),
});

export async function POST(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  if (session.user.role !== "teacher" && session.user.role !== "admin") {
    return NextResponse.json(
      { error: "Only teachers can start sessions" },
      { status: 403 }
    );
  }

  const body = await request.json();
  const parsed = createSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const classroom = await getClassroom(db, parsed.data.classroomId);
  if (!classroom) {
    return NextResponse.json({ error: "Classroom not found" }, { status: 404 });
  }

  if (classroom.teacherId !== session.user.id) {
    return NextResponse.json(
      { error: "Only the classroom teacher can start sessions" },
      { status: 403 }
    );
  }

  const liveSession = await createSession(db, {
    classroomId: parsed.data.classroomId,
    teacherId: session.user.id,
    settings: parsed.data.settings,
  });

  return NextResponse.json(liveSession, { status: 201 });
}

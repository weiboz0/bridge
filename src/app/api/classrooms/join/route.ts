import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getClassroomByJoinCode, joinClassroom } from "@/lib/classrooms";

const joinSchema = z.object({
  joinCode: z.string().length(8),
});

export async function POST(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const body = await request.json();
  const parsed = joinSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid join code" },
      { status: 400 }
    );
  }

  const classroom = await getClassroomByJoinCode(db, parsed.data.joinCode);
  if (!classroom) {
    return NextResponse.json(
      { error: "Classroom not found" },
      { status: 404 }
    );
  }

  await joinClassroom(db, classroom.id, session.user.id);

  return NextResponse.json(classroom);
}

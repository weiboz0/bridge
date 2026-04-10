import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getClassroom, getClassroomMembers } from "@/lib/classrooms";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const classroom = await getClassroom(db, id);

  if (!classroom) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  const members = await getClassroomMembers(db, id);
  return NextResponse.json(members);
}

import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { createTopic, listTopicsByCourse } from "@/lib/topics";

// Plan 044 phase 3: lessonContent and starterCode are no longer accepted
// on topic create. Teaching material lives in the linked teaching_unit
// (1:1 via teaching_units.topic_id). Use POST /api/courses/{cid}/topics/
// {tid}/link-unit to attach material to a topic. Strict mode rejects
// any request that still includes the deprecated fields.
const createSchema = z.object({
  title: z.string().min(1).max(255),
  description: z.string().max(2000).optional(),
}).strict();

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id: courseId } = await params;
  const body = await request.json();
  const parsed = createSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const topic = await createTopic(db, { courseId, ...parsed.data });
  return NextResponse.json(topic, { status: 201 });
}

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id: courseId } = await params;
  const topicList = await listTopicsByCourse(db, courseId);
  return NextResponse.json(topicList);
}

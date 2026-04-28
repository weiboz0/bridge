import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { createTopic, listTopicsByCourse } from "@/lib/topics";

// Strict mode: unknown fields are rejected with 400. Teaching material
// is attached via POST /api/courses/{cid}/topics/{tid}/link-unit, not
// through the topic create body.
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

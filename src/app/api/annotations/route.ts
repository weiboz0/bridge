import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { createAnnotation, listAnnotations } from "@/lib/annotations";

const createSchema = z.object({
  documentId: z.string().min(1),
  lineStart: z.string().min(1),
  lineEnd: z.string().min(1),
  content: z.string().min(1).max(2000),
});

export async function POST(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
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

  const annotation = await createAnnotation(db, {
    ...parsed.data,
    authorId: session.user.id,
    authorType: "teacher", /* TODO: determine from org/class membership */
  });

  return NextResponse.json(annotation, { status: 201 });
}

export async function GET(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const documentId = request.nextUrl.searchParams.get("documentId");
  if (!documentId) {
    return NextResponse.json({ error: "documentId required" }, { status: 400 });
  }

  const annotations = await listAnnotations(db, documentId);
  return NextResponse.json(annotations);
}

import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { users } from "@/lib/db/schema";
import { eq } from "drizzle-orm";
import { getLinkedChildren } from "@/lib/parent-links";
import { listReports, generateReport } from "@/lib/parent-reports";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id: childId } = await params;

  const children = await getLinkedChildren(db, session.user.id);
  if (!children.some((c) => c.userId === childId)) {
    return NextResponse.json({ error: "Not linked to this child" }, { status: 403 });
  }

  const reports = await listReports(db, childId);
  return NextResponse.json(reports);
}

export async function POST(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id: childId } = await params;

  const children = await getLinkedChildren(db, session.user.id);
  if (!children.some((c) => c.userId === childId)) {
    return NextResponse.json({ error: "Not linked to this child" }, { status: 403 });
  }

  // Get child's name
  const [child] = await db.select().from(users).where(eq(users.id, childId));
  if (!child) {
    return NextResponse.json({ error: "Child not found" }, { status: 404 });
  }

  // Generate report for last 7 days
  const now = new Date();
  const weekAgo = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000);

  try {
    const report = await generateReport(db, {
      studentId: childId,
      studentName: child.name,
      generatedBy: session.user.id,
      periodStart: weekAgo,
      periodEnd: now,
    });

    return NextResponse.json(report, { status: 201 });
  } catch (err: any) {
    return NextResponse.json(
      { error: "Failed to generate report", details: err.message },
      { status: 500 }
    );
  }
}

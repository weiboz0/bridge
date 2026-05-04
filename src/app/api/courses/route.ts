import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { db } from "@/lib/db";
import { getIdentity } from "@/lib/identity";
import { createCourse, listCoursesByOrg } from "@/lib/courses";
import { getUserRoleInOrg } from "@/lib/org-memberships";

const createSchema = z.object({
  orgId: z.string().uuid(),
  title: z.string().min(1).max(255),
  description: z.string().max(5000).optional(),
  gradeLevel: z.enum(["K-5", "6-8", "9-12"]),
  language: z.enum(["python", "javascript", "blockly"]).optional(),
});

export async function POST(request: NextRequest) {
  const id = await getIdentity();
  if (!id?.userId) {
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

  // Verify user is teacher or org_admin in the org
  const roles = await getUserRoleInOrg(db, parsed.data.orgId, id.userId);
  const canCreate = roles.some((r) => r.role === "teacher" || r.role === "org_admin");
  // Plan 065 phase 4 — admin status is the live DB value via /api/me/identity.
  if (!canCreate && !id.isPlatformAdmin) {
    return NextResponse.json({ error: "Only teachers can create courses" }, { status: 403 });
  }

  const course = await createCourse(db, {
    ...parsed.data,
    createdBy: id.userId,
  });

  return NextResponse.json(course, { status: 201 });
}

export async function GET(request: NextRequest) {
  const id = await getIdentity();
  if (!id?.userId) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const orgId = request.nextUrl.searchParams.get("orgId");
  if (!orgId) {
    return NextResponse.json({ error: "orgId required" }, { status: 400 });
  }

  // Verify user is a member of the org
  const roles = await getUserRoleInOrg(db, orgId, id.userId);
  if (roles.length === 0 && !id.isPlatformAdmin) {
    return NextResponse.json({ error: "Not a member of this org" }, { status: 403 });
  }

  const courses = await listCoursesByOrg(db, orgId);
  return NextResponse.json(courses);
}

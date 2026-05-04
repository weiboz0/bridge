import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { db } from "@/lib/db";
import { getIdentity } from "@/lib/identity";
import { createClass, listClassesByOrg } from "@/lib/classes";
import { getUserRoleInOrg } from "@/lib/org-memberships";

const createSchema = z.object({
  courseId: z.string().uuid(),
  orgId: z.string().uuid(),
  title: z.string().min(1).max(255),
  term: z.string().max(100).optional(),
});

export async function POST(request: NextRequest) {
  const identity = await getIdentity();
  if (!identity?.userId) {
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
  const roles = await getUserRoleInOrg(db, parsed.data.orgId, identity.userId);
  const canCreate = roles.some((r) => r.role === "teacher" || r.role === "org_admin");
  // Plan 065 phase 4 — admin status from /api/me/identity (live DB).
  if (!canCreate && !identity.isPlatformAdmin) {
    return NextResponse.json({ error: "Only teachers can create classes" }, { status: 403 });
  }

  const cls = await createClass(db, {
    ...parsed.data,
    createdBy: identity.userId,
  });

  return NextResponse.json(cls, { status: 201 });
}

export async function GET(request: NextRequest) {
  const identity = await getIdentity();
  if (!identity?.userId) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const orgId = request.nextUrl.searchParams.get("orgId");
  if (!orgId) {
    return NextResponse.json({ error: "orgId required" }, { status: 400 });
  }

  const roles = await getUserRoleInOrg(db, orgId, identity.userId);
  if (roles.length === 0 && !identity.isPlatformAdmin) {
    return NextResponse.json({ error: "Not a member of this org" }, { status: 403 });
  }

  const classList = await listClassesByOrg(db, orgId);
  return NextResponse.json(classList);
}

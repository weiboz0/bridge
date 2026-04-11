import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { createClass, listClassesByOrg } from "@/lib/classes";
import { getUserRoleInOrg } from "@/lib/org-memberships";

const createSchema = z.object({
  courseId: z.string().uuid(),
  orgId: z.string().uuid(),
  title: z.string().min(1).max(255),
  term: z.string().max(100).optional(),
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

  // Verify user is teacher or org_admin in the org
  const roles = await getUserRoleInOrg(db, parsed.data.orgId, session.user.id);
  const canCreate = roles.some((r) => r.role === "teacher" || r.role === "org_admin");
  if (!canCreate && !session.user.isPlatformAdmin) {
    return NextResponse.json({ error: "Only teachers can create classes" }, { status: 403 });
  }

  const cls = await createClass(db, {
    ...parsed.data,
    createdBy: session.user.id,
  });

  return NextResponse.json(cls, { status: 201 });
}

export async function GET(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const orgId = request.nextUrl.searchParams.get("orgId");
  if (!orgId) {
    return NextResponse.json({ error: "orgId required" }, { status: 400 });
  }

  const roles = await getUserRoleInOrg(db, orgId, session.user.id);
  if (roles.length === 0 && !session.user.isPlatformAdmin) {
    return NextResponse.json({ error: "Not a member of this org" }, { status: 403 });
  }

  const classList = await listClassesByOrg(db, orgId);
  return NextResponse.json(classList);
}

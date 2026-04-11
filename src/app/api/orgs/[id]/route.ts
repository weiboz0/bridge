import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { organizations } from "@/lib/db/schema";
import { eq } from "drizzle-orm";
import { getOrganization } from "@/lib/organizations";
import { getUserRoleInOrg } from "@/lib/org-memberships";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const org = await getOrganization(db, id);

  if (!org) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  // Check user has membership in this org (or is platform admin)
  if (!session.user.isPlatformAdmin) {
    const roles = await getUserRoleInOrg(db, id, session.user.id);
    if (roles.length === 0) {
      return NextResponse.json({ error: "Not a member" }, { status: 403 });
    }
  }

  return NextResponse.json(org);
}

const updateOrgSchema = z.object({
  name: z.string().min(1).max(255).optional(),
  contactEmail: z.string().email().optional(),
  contactName: z.string().min(1).max(255).optional(),
  domain: z.string().max(255).optional(),
});

export async function PATCH(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;

  // Only org_admin or platform admin can update
  if (!session.user.isPlatformAdmin) {
    const roles = await getUserRoleInOrg(db, id, session.user.id);
    const isOrgAdmin = roles.some((r) => r.role === "org_admin");
    if (!isOrgAdmin) {
      return NextResponse.json({ error: "Only org admins can update" }, { status: 403 });
    }
  }

  const body = await request.json();
  const parsed = updateOrgSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const [updated] = await db
    .update(organizations)
    .set({ ...parsed.data, updatedAt: new Date() })
    .where(eq(organizations.id, id))
    .returning();

  if (!updated) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  return NextResponse.json(updated);
}

import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { users } from "@/lib/db/schema";
import { eq } from "drizzle-orm";
import { addOrgMember, listOrgMembers, getUserRoleInOrg } from "@/lib/org-memberships";

const addMemberSchema = z.object({
  email: z.string().email(),
  role: z.enum(["org_admin", "teacher", "student", "parent"]),
});

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id: orgId } = await params;

  // Check caller is org_admin
  const callerRoles = await getUserRoleInOrg(db, orgId, session.user.id);
  const isOrgAdmin = callerRoles.some((r) => r.role === "org_admin");
  if (!isOrgAdmin && !session.user.isPlatformAdmin) {
    return NextResponse.json({ error: "Only org admins can add members" }, { status: 403 });
  }

  const body = await request.json();
  const parsed = addMemberSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  // Find user by email
  const [user] = await db
    .select()
    .from(users)
    .where(eq(users.email, parsed.data.email));

  if (!user) {
    return NextResponse.json({ error: "User not found" }, { status: 404 });
  }

  const membership = await addOrgMember(db, {
    orgId,
    userId: user.id,
    role: parsed.data.role,
    status: "active",
    invitedBy: session.user.id,
  });

  return NextResponse.json(membership, { status: 201 });
}

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id: orgId } = await params;

  // Check caller is member of org
  const callerRoles = await getUserRoleInOrg(db, orgId, session.user.id);
  if (callerRoles.length === 0 && !session.user.isPlatformAdmin) {
    return NextResponse.json({ error: "Not a member" }, { status: 403 });
  }

  const members = await listOrgMembers(db, orgId);
  return NextResponse.json(members);
}

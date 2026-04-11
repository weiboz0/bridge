import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getUserRoleInOrg, getOrgMembership, updateMemberStatus, removeOrgMember } from "@/lib/org-memberships";

export async function PATCH(
  request: NextRequest,
  { params }: { params: Promise<{ id: string; memberId: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id: orgId, memberId } = await params;

  const callerRoles = await getUserRoleInOrg(db, orgId, session.user.id);
  const isOrgAdmin = callerRoles.some((r) => r.role === "org_admin");
  if (!isOrgAdmin && !session.user.isPlatformAdmin) {
    return NextResponse.json({ error: "Only org admins can update members" }, { status: 403 });
  }

  // Verify membership belongs to this org
  const membership = await getOrgMembership(db, memberId);
  if (!membership || membership.orgId !== orgId) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  const body = await request.json();
  const { status } = body;

  if (!["pending", "active", "suspended"].includes(status)) {
    return NextResponse.json({ error: "Invalid status" }, { status: 400 });
  }

  const updated = await updateMemberStatus(db, memberId, status);
  return NextResponse.json(updated);
}

export async function DELETE(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string; memberId: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id: orgId, memberId } = await params;

  const callerRoles = await getUserRoleInOrg(db, orgId, session.user.id);
  const isOrgAdmin = callerRoles.some((r) => r.role === "org_admin");
  if (!isOrgAdmin && !session.user.isPlatformAdmin) {
    return NextResponse.json({ error: "Only org admins can remove members" }, { status: 403 });
  }

  // Verify membership belongs to this org
  const membership = await getOrgMembership(db, memberId);
  if (!membership || membership.orgId !== orgId) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  const removed = await removeOrgMember(db, memberId);
  if (!removed) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  return NextResponse.json(removed);
}

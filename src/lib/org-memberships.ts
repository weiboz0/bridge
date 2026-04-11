import { eq, and } from "drizzle-orm";
import { orgMemberships, users } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

interface AddMemberInput {
  orgId: string;
  userId: string;
  role: "org_admin" | "teacher" | "student" | "parent";
  status?: "pending" | "active" | "suspended";
  invitedBy?: string;
}

export async function addOrgMember(db: Database, input: AddMemberInput) {
  const [membership] = await db
    .insert(orgMemberships)
    .values({
      ...input,
      status: input.status || "active",
    })
    .onConflictDoNothing()
    .returning();
  return membership;
}

export async function listOrgMembers(db: Database, orgId: string) {
  return db
    .select({
      id: orgMemberships.id,
      orgId: orgMemberships.orgId,
      userId: orgMemberships.userId,
      role: orgMemberships.role,
      status: orgMemberships.status,
      createdAt: orgMemberships.createdAt,
      name: users.name,
      email: users.email,
    })
    .from(orgMemberships)
    .innerJoin(users, eq(orgMemberships.userId, users.id))
    .where(eq(orgMemberships.orgId, orgId));
}

export async function getUserMemberships(db: Database, userId: string) {
  const { organizations } = await import("@/lib/db/schema");
  return db
    .select({
      id: orgMemberships.id,
      orgId: orgMemberships.orgId,
      userId: orgMemberships.userId,
      role: orgMemberships.role,
      status: orgMemberships.status,
      createdAt: orgMemberships.createdAt,
      orgName: organizations.name,
      orgSlug: organizations.slug,
      orgStatus: organizations.status,
    })
    .from(orgMemberships)
    .innerJoin(organizations, eq(orgMemberships.orgId, organizations.id))
    .where(eq(orgMemberships.userId, userId));
}

export async function getOrgMembership(db: Database, membershipId: string) {
  const [membership] = await db
    .select()
    .from(orgMemberships)
    .where(eq(orgMemberships.id, membershipId));
  return membership || null;
}

export async function getUserRoleInOrg(
  db: Database,
  orgId: string,
  userId: string
) {
  const memberships = await db
    .select()
    .from(orgMemberships)
    .where(
      and(
        eq(orgMemberships.orgId, orgId),
        eq(orgMemberships.userId, userId),
        eq(orgMemberships.status, "active")
      )
    );
  return memberships;
}

export async function updateMemberStatus(
  db: Database,
  membershipId: string,
  status: "pending" | "active" | "suspended"
) {
  const [updated] = await db
    .update(orgMemberships)
    .set({ status })
    .where(eq(orgMemberships.id, membershipId))
    .returning();
  return updated || null;
}

export async function updateMemberRole(
  db: Database,
  membershipId: string,
  role: "org_admin" | "teacher" | "student" | "parent"
) {
  const [updated] = await db
    .update(orgMemberships)
    .set({ role })
    .where(eq(orgMemberships.id, membershipId))
    .returning();
  return updated || null;
}

export async function removeOrgMember(db: Database, membershipId: string) {
  const [deleted] = await db
    .delete(orgMemberships)
    .where(eq(orgMemberships.id, membershipId))
    .returning();
  return deleted || null;
}

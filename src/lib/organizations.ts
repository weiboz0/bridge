import { eq } from "drizzle-orm";
import { organizations } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

interface CreateOrgInput {
  name: string;
  slug: string;
  type: "school" | "tutoring_center" | "bootcamp" | "other";
  contactEmail: string;
  contactName: string;
  domain?: string;
}

export async function createOrganization(db: Database, input: CreateOrgInput) {
  const [org] = await db
    .insert(organizations)
    .values(input)
    .returning();
  return org;
}

export async function getOrganization(db: Database, orgId: string) {
  const [org] = await db
    .select()
    .from(organizations)
    .where(eq(organizations.id, orgId));
  return org || null;
}

export async function getOrganizationBySlug(db: Database, slug: string) {
  const [org] = await db
    .select()
    .from(organizations)
    .where(eq(organizations.slug, slug));
  return org || null;
}

export async function listOrganizations(db: Database, status?: string) {
  if (status) {
    return db
      .select()
      .from(organizations)
      .where(eq(organizations.status, status as "pending" | "active" | "suspended"));
  }
  return db.select().from(organizations);
}

export async function updateOrgStatus(
  db: Database,
  orgId: string,
  status: "pending" | "active" | "suspended",
) {
  // Check existing org type — only set verifiedAt for schools
  const [existing] = await db
    .select()
    .from(organizations)
    .where(eq(organizations.id, orgId));

  if (!existing) return null;

  const updates: Record<string, unknown> = { status, updatedAt: new Date() };
  if (status === "active" && existing.type === "school" && !existing.verifiedAt) {
    updates.verifiedAt = new Date();
  }

  const [org] = await db
    .update(organizations)
    .set(updates)
    .where(eq(organizations.id, orgId))
    .returning();
  return org || null;
}

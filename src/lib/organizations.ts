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

export async function countOrganizations(db: Database, status?: string) {
  const { sql } = await import("drizzle-orm");
  if (status) {
    const result = await db
      .select({ count: sql<number>`count(*)` })
      .from(organizations)
      .where(eq(organizations.status, status as "pending" | "active" | "suspended"));
    return Number(result[0].count);
  }
  const result = await db
    .select({ count: sql<number>`count(*)` })
    .from(organizations);
  return Number(result[0].count);
}

export async function updateOrganization(
  db: Database,
  orgId: string,
  updates: Partial<Pick<typeof organizations.$inferInsert, "name" | "contactEmail" | "contactName" | "domain">>
) {
  const [org] = await db
    .update(organizations)
    .set({ ...updates, updatedAt: new Date() })
    .where(eq(organizations.id, orgId))
    .returning();
  return org || null;
}

export async function updateOrgStatus(
  db: Database,
  orgId: string,
  status: "pending" | "active" | "suspended",
) {
  // Plan 060 — flips status only. The auto-stamp of `verifiedAt = now()`
  // on first school activation was removed in parity with the Go-side
  // change in `platform/internal/store/orgs.go`: it conflated "admin
  // clicked Active" with "admin verified the school's signup
  // paperwork." A real verification flow gets its own helper when one
  // is built.
  const [org] = await db
    .update(organizations)
    .set({ status, updatedAt: new Date() })
    .where(eq(organizations.id, orgId))
    .returning();
  return org || null;
}

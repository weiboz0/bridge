import { eq, and, inArray } from "drizzle-orm";
import { classMemberships, parentLinks, users } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

/**
 * Plan 064 — get the children linked to a parent via the
 * `parent_links` table. Returns only ACTIVE links (revoked rows are
 * audit-only).
 *
 * Replaces the pre-064 implicit model that derived children from
 * `class_memberships role="parent"` — that leaked: every parent in
 * a class saw ALL students in that class. The new model is a
 * direct 1:N (parent, child) relationship per row.
 */
export async function getLinkedChildren(db: Database, parentUserId: string) {
  // 1. Active links → child user IDs.
  const links = await db
    .select({ childUserId: parentLinks.childUserId })
    .from(parentLinks)
    .where(
      and(
        eq(parentLinks.parentUserId, parentUserId),
        eq(parentLinks.status, "active"),
      ),
    );
  if (links.length === 0) return [];
  const childIds = Array.from(new Set(links.map((l) => l.childUserId)));

  // 2. Hydrate user rows.
  const userRows = await db
    .select({ userId: users.id, name: users.name, email: users.email })
    .from(users)
    .where(inArray(users.id, childIds));
  const userById = new Map(userRows.map((u) => [u.userId, u] as const));

  // 3. Count classes per child for the dashboard summary line.
  const classRows = await db
    .select({ userId: classMemberships.userId, classId: classMemberships.classId })
    .from(classMemberships)
    .where(
      and(
        inArray(classMemberships.userId, childIds),
        eq(classMemberships.role, "student"),
      ),
    );
  const classCountByUser = new Map<string, number>();
  for (const row of classRows) {
    classCountByUser.set(row.userId, (classCountByUser.get(row.userId) ?? 0) + 1);
  }

  // 4. Project — return rows in the same shape callers depended on,
  // ordered by name for deterministic dashboard layout.
  return childIds
    .map((id) => {
      const u = userById.get(id);
      if (!u) return null;
      return {
        userId: u.userId,
        name: u.name,
        email: u.email,
        classCount: classCountByUser.get(u.userId) ?? 0,
      };
    })
    .filter((r): r is { userId: string; name: string; email: string; classCount: number } => r !== null)
    .sort((a, b) => a.name.localeCompare(b.name));
}

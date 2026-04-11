import { eq, and } from "drizzle-orm";
import { classMemberships, classes, users } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

/**
 * Get children linked to a parent.
 * A parent is linked to children via class memberships where the parent
 * has role="parent" in the same class as the student.
 */
export async function getLinkedChildren(db: Database, parentUserId: string) {
  // Find classes where this user has parent role
  const parentMemberships = await db
    .select({ classId: classMemberships.classId })
    .from(classMemberships)
    .where(
      and(
        eq(classMemberships.userId, parentUserId),
        eq(classMemberships.role, "parent")
      )
    );

  if (parentMemberships.length === 0) return [];

  // Find students in those classes
  const children: { userId: string; name: string; email: string; classCount: number }[] = [];
  const seen = new Set<string>();

  for (const pm of parentMemberships) {
    const students = await db
      .select({
        userId: classMemberships.userId,
        name: users.name,
        email: users.email,
      })
      .from(classMemberships)
      .innerJoin(users, eq(classMemberships.userId, users.id))
      .where(
        and(
          eq(classMemberships.classId, pm.classId),
          eq(classMemberships.role, "student")
        )
      );

    for (const s of students) {
      if (!seen.has(s.userId)) {
        seen.add(s.userId);
        // Count classes for this student
        const studentClasses = await db
          .select({ classId: classMemberships.classId })
          .from(classMemberships)
          .where(
            and(
              eq(classMemberships.userId, s.userId),
              eq(classMemberships.role, "student")
            )
          );
        children.push({ ...s, classCount: studentClasses.length });
      }
    }
  }

  return children;
}

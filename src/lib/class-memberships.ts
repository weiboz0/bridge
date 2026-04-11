import { eq, and } from "drizzle-orm";
import { classMemberships, classes, users } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

interface AddClassMemberInput {
  classId: string;
  userId: string;
  role?: "instructor" | "ta" | "student" | "observer" | "guest" | "parent";
}

export async function addClassMember(db: Database, input: AddClassMemberInput) {
  const [member] = await db
    .insert(classMemberships)
    .values({
      classId: input.classId,
      userId: input.userId,
      role: input.role || "student",
    })
    .onConflictDoNothing()
    .returning();
  return member || null;
}

export async function listClassMembers(db: Database, classId: string) {
  return db
    .select({
      id: classMemberships.id,
      classId: classMemberships.classId,
      userId: classMemberships.userId,
      role: classMemberships.role,
      joinedAt: classMemberships.joinedAt,
      name: users.name,
      email: users.email,
    })
    .from(classMemberships)
    .innerJoin(users, eq(classMemberships.userId, users.id))
    .where(eq(classMemberships.classId, classId));
}

export async function getClassMembership(db: Database, membershipId: string) {
  const [member] = await db
    .select()
    .from(classMemberships)
    .where(eq(classMemberships.id, membershipId));
  return member || null;
}

export async function updateClassMemberRole(
  db: Database,
  membershipId: string,
  role: "instructor" | "ta" | "student" | "observer" | "guest" | "parent"
) {
  const [updated] = await db
    .update(classMemberships)
    .set({ role })
    .where(eq(classMemberships.id, membershipId))
    .returning();
  return updated || null;
}

export async function removeClassMember(db: Database, membershipId: string) {
  const [removed] = await db
    .delete(classMemberships)
    .where(eq(classMemberships.id, membershipId))
    .returning();
  return removed || null;
}

export async function joinClassByCode(
  db: Database,
  joinCode: string,
  userId: string
) {
  const [cls] = await db
    .select()
    .from(classes)
    .where(eq(classes.joinCode, joinCode));

  if (!cls) return null;
  if (cls.status !== "active") return null;

  const member = await addClassMember(db, {
    classId: cls.id,
    userId,
    role: "student",
  });

  return { class: cls, membership: member };
}

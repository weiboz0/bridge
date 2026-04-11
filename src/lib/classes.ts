import { eq } from "drizzle-orm";
import { classes, classMemberships, newClassrooms, courses, users } from "@/lib/db/schema";
import { generateJoinCode } from "@/lib/utils";
import type { Database } from "@/lib/db";

interface CreateClassInput {
  courseId: string;
  orgId: string;
  title: string;
  term?: string;
  createdBy: string; // teacher who creates the class — becomes instructor
}

export async function createClass(db: Database, input: CreateClassInput) {
  const { createdBy, ...classData } = input;

  // Create the class
  const [cls] = await db
    .insert(classes)
    .values({
      ...classData,
      joinCode: generateJoinCode(),
    })
    .returning();

  // Auto-create classroom (1:1) — set editorMode from course language
  const [course] = await db
    .select({ language: courses.language })
    .from(courses)
    .where(eq(courses.id, input.courseId));

  await db.insert(newClassrooms).values({
    classId: cls.id,
    editorMode: course?.language || "python",
  });

  // Add creator as instructor
  await db.insert(classMemberships).values({
    classId: cls.id,
    userId: createdBy,
    role: "instructor",
  });

  return cls;
}

export async function getClass(db: Database, classId: string) {
  const [cls] = await db
    .select()
    .from(classes)
    .where(eq(classes.id, classId));
  return cls || null;
}

export async function listClassesByOrg(db: Database, orgId: string, includeArchived = false) {
  if (includeArchived) {
    return db.select().from(classes).where(eq(classes.orgId, orgId));
  }
  const { and } = await import("drizzle-orm");
  return db
    .select()
    .from(classes)
    .where(and(eq(classes.orgId, orgId), eq(classes.status, "active")));
}

export async function listClassesByCourse(db: Database, courseId: string) {
  return db
    .select()
    .from(classes)
    .where(eq(classes.courseId, courseId));
}

export async function listClassesByUser(db: Database, userId: string) {
  const memberships = await db
    .select({ classId: classMemberships.classId, role: classMemberships.role })
    .from(classMemberships)
    .where(eq(classMemberships.userId, userId));

  if (memberships.length === 0) return [];

  const { or } = await import("drizzle-orm");
  const classList = await db
    .select()
    .from(classes)
    .where(or(...memberships.map((m) => eq(classes.id, m.classId))));

  return classList.map((cls) => ({
    ...cls,
    memberRole: memberships.find((m) => m.classId === cls.id)?.role || "student",
  }));
}

export async function getClassByJoinCode(db: Database, joinCode: string) {
  const [cls] = await db
    .select()
    .from(classes)
    .where(eq(classes.joinCode, joinCode));
  return cls || null;
}

export async function archiveClass(db: Database, classId: string) {
  const [cls] = await db
    .update(classes)
    .set({ status: "archived", updatedAt: new Date() })
    .where(eq(classes.id, classId))
    .returning();
  return cls || null;
}

export async function getClassroom(db: Database, classId: string) {
  const [classroom] = await db
    .select()
    .from(newClassrooms)
    .where(eq(newClassrooms.classId, classId));
  return classroom || null;
}

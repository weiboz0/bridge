import { eq, or } from "drizzle-orm";
import { classrooms, classroomMembers, users } from "@/lib/db/schema";
import { generateJoinCode } from "@/lib/utils";
import type { Database } from "@/lib/db";

interface CreateClassroomInput {
  teacherId: string;
  name: string;
  description?: string;
  gradeLevel: "K-5" | "6-8" | "9-12";
  editorMode: "blockly" | "python" | "javascript";
  schoolId?: string;
}

export async function createClassroom(db: Database, input: CreateClassroomInput) {
  const [classroom] = await db
    .insert(classrooms)
    .values({
      ...input,
      joinCode: generateJoinCode(),
    })
    .returning();
  return classroom;
}

export async function listClassrooms(db: Database, userId: string) {
  const teacherClassrooms = await db
    .select()
    .from(classrooms)
    .where(eq(classrooms.teacherId, userId));

  const memberClassroomIds = await db
    .select({ classroomId: classroomMembers.classroomId })
    .from(classroomMembers)
    .where(eq(classroomMembers.userId, userId));

  if (memberClassroomIds.length === 0) return teacherClassrooms;

  const memberClassrooms = await db
    .select()
    .from(classrooms)
    .where(
      or(
        ...memberClassroomIds.map((m) => eq(classrooms.id, m.classroomId))
      )
    );

  const seen = new Set(teacherClassrooms.map((c) => c.id));
  const result = [...teacherClassrooms];
  for (const c of memberClassrooms) {
    if (!seen.has(c.id)) {
      result.push(c);
      seen.add(c.id);
    }
  }
  return result;
}

export async function getClassroom(db: Database, classroomId: string) {
  const [classroom] = await db
    .select()
    .from(classrooms)
    .where(eq(classrooms.id, classroomId));
  return classroom || null;
}

export async function getClassroomByJoinCode(db: Database, joinCode: string) {
  const [classroom] = await db
    .select()
    .from(classrooms)
    .where(eq(classrooms.joinCode, joinCode));
  return classroom || null;
}

export async function joinClassroom(
  db: Database,
  classroomId: string,
  userId: string
) {
  const [member] = await db
    .insert(classroomMembers)
    .values({ classroomId, userId })
    .onConflictDoNothing()
    .returning();
  return member;
}

export async function getClassroomMembers(db: Database, classroomId: string) {
  return db
    .select({
      userId: classroomMembers.userId,
      joinedAt: classroomMembers.joinedAt,
      name: users.name,
      email: users.email,
      role: users.role,
    })
    .from(classroomMembers)
    .innerJoin(users, eq(classroomMembers.userId, users.id))
    .where(eq(classroomMembers.classroomId, classroomId));
}

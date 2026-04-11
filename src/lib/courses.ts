import { eq } from "drizzle-orm";
import { courses, topics } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

interface CreateCourseInput {
  orgId: string;
  createdBy: string;
  title: string;
  description?: string;
  gradeLevel: "K-5" | "6-8" | "9-12";
  language?: "python" | "javascript" | "blockly";
}

export async function createCourse(db: Database, input: CreateCourseInput) {
  const [course] = await db
    .insert(courses)
    .values(input)
    .returning();
  return course;
}

export async function getCourse(db: Database, courseId: string) {
  const [course] = await db
    .select()
    .from(courses)
    .where(eq(courses.id, courseId));
  return course || null;
}

export async function listCoursesByOrg(db: Database, orgId: string) {
  return db
    .select()
    .from(courses)
    .where(eq(courses.orgId, orgId));
}

export async function listCoursesByCreator(db: Database, createdBy: string) {
  return db
    .select()
    .from(courses)
    .where(eq(courses.createdBy, createdBy));
}

export async function updateCourse(
  db: Database,
  courseId: string,
  updates: Partial<Pick<typeof courses.$inferInsert, "title" | "description" | "gradeLevel" | "language" | "isPublished">>
) {
  const [course] = await db
    .update(courses)
    .set({ ...updates, updatedAt: new Date() })
    .where(eq(courses.id, courseId))
    .returning();
  return course || null;
}

export async function deleteCourse(db: Database, courseId: string) {
  const [deleted] = await db
    .delete(courses)
    .where(eq(courses.id, courseId))
    .returning();
  return deleted || null;
}

export async function cloneCourse(db: Database, courseId: string, newCreatedBy: string) {
  const original = await getCourse(db, courseId);
  if (!original) return null;

  const [cloned] = await db
    .insert(courses)
    .values({
      orgId: original.orgId,
      createdBy: newCreatedBy,
      title: `${original.title} (Copy)`,
      description: original.description,
      gradeLevel: original.gradeLevel,
      language: original.language,
      isPublished: false,
    })
    .returning();

  // Clone topics
  const originalTopics = await db
    .select()
    .from(topics)
    .where(eq(topics.courseId, courseId));

  for (const topic of originalTopics) {
    await db.insert(topics).values({
      courseId: cloned.id,
      title: topic.title,
      description: topic.description,
      sortOrder: topic.sortOrder,
      lessonContent: topic.lessonContent,
      starterCode: topic.starterCode,
    });
  }

  return cloned;
}

import { drizzle } from "drizzle-orm/postgres-js";
import postgres from "postgres";
import * as schema from "@/lib/db/schema";
import { nanoid } from "nanoid";

const testClient = postgres(
  process.env.DATABASE_URL || "postgresql://work@127.0.0.1:5432/bridge_test"
);
export const testDb = drizzle(testClient, { schema });

export async function cleanupDatabase() {
  await testDb.delete(schema.classroomMembers);
  await testDb.delete(schema.classrooms);
  await testDb.delete(schema.authProviders);
  await testDb.delete(schema.users);
  await testDb.delete(schema.schools);
}

export async function createTestSchool(overrides: Partial<typeof schema.schools.$inferInsert> = {}) {
  const [school] = await testDb
    .insert(schema.schools)
    .values({
      name: "Test School",
      ...overrides,
    })
    .returning();
  return school;
}

export async function createTestUser(
  overrides: Partial<typeof schema.users.$inferInsert> = {}
) {
  const [user] = await testDb
    .insert(schema.users)
    .values({
      name: "Test User",
      email: `test-${nanoid(6)}@example.com`,
      role: "teacher",
      ...overrides,
    })
    .returning();
  return user;
}

export async function createTestClassroom(
  teacherId: string,
  overrides: Partial<typeof schema.classrooms.$inferInsert> = {}
) {
  const [classroom] = await testDb
    .insert(schema.classrooms)
    .values({
      teacherId,
      name: "Test Classroom",
      gradeLevel: "6-8",
      editorMode: "python",
      joinCode: nanoid(8),
      ...overrides,
    })
    .returning();
  return classroom;
}

export async function closeTestDb() {
  await testClient.end();
}

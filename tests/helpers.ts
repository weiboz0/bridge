import { drizzle } from "drizzle-orm/postgres-js";
import postgres from "postgres";
import * as schema from "@/lib/db/schema";
import { nanoid } from "nanoid";

const testClient = postgres(
  process.env.DATABASE_URL || "postgresql://work@127.0.0.1:5432/bridge_test"
);
export const testDb = drizzle(testClient, { schema });

export async function cleanupDatabase() {
  await testDb.delete(schema.codeAnnotations);
  await testDb.delete(schema.aiInteractions);
  await testDb.delete(schema.sessionParticipants);
  await testDb.delete(schema.liveSessions);
  await testDb.delete(schema.classroomMembers);
  await testDb.delete(schema.classrooms);
  await testDb.delete(schema.orgMemberships);
  await testDb.delete(schema.authProviders);
  await testDb.delete(schema.users);
  await testDb.delete(schema.organizations);
}

export async function createTestOrg(
  overrides: Partial<typeof schema.organizations.$inferInsert> = {}
) {
  const [org] = await testDb
    .insert(schema.organizations)
    .values({
      name: "Test Org",
      slug: `test-org-${nanoid(6)}`,
      type: "school",
      status: "active",
      contactEmail: "admin@test.edu",
      contactName: "Admin",
      ...overrides,
    })
    .returning();
  return org;
}

export async function createTestOrgMembership(
  orgId: string,
  userId: string,
  overrides: Partial<typeof schema.orgMemberships.$inferInsert> = {}
) {
  const [membership] = await testDb
    .insert(schema.orgMemberships)
    .values({
      orgId,
      userId,
      role: "teacher",
      status: "active",
      ...overrides,
    })
    .returning();
  return membership;
}

export async function createTestUser(
  overrides: Partial<typeof schema.users.$inferInsert> = {}
) {
  const [user] = await testDb
    .insert(schema.users)
    .values({
      name: "Test User",
      email: `test-${nanoid(6)}@example.com`,
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

export async function createTestSession(
  classroomId: string,
  teacherId: string,
  overrides: Partial<typeof schema.liveSessions.$inferInsert> = {}
) {
  const [session] = await testDb
    .insert(schema.liveSessions)
    .values({
      classroomId,
      teacherId,
      ...overrides,
    })
    .returning();
  return session;
}

export async function closeTestDb() {
  await testClient.end();
}

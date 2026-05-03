import { drizzle } from "drizzle-orm/postgres-js";
import { sql } from "drizzle-orm";
import postgres from "postgres";
import * as schema from "@/lib/db/schema";
import { nanoid } from "nanoid";

const testClient = postgres(
  process.env.DATABASE_URL || "postgresql://work@127.0.0.1:5432/bridge_test"
);
export const testDb = drizzle(testClient, { schema });

export async function cleanupDatabase() {
  await testDb.delete(schema.parentReports);
  await testDb.delete(schema.parentLinks);
  await testDb.delete(schema.submissions);
  await testDb.delete(schema.assignments);
  await testDb.delete(schema.documents);
  await testDb.delete(schema.sessionTopics);
  await testDb.delete(schema.codeAnnotations);
  await testDb.delete(schema.aiInteractions);
  await testDb.delete(schema.sessionParticipants);
  await testDb.delete(schema.sessions);
  await testDb.delete(schema.classSettings);
  await testDb.delete(schema.classMemberships);
  await testDb.delete(schema.classes);
  // Plan 044: clean teaching_units before topics so the topic_id FK
  // (ON DELETE SET NULL) doesn't leave orphan unit rows.
  await testDb.delete(schema.unitDocuments);
  // Plan 049: problem-bank tables hold FKs into users (created_by /
  // attached_by). Wipe them before the user delete or the FK cascade
  // is too narrow to reach. test_cases lives outside the Drizzle
  // schema (drizzle/0008_problems.sql), so use raw SQL.
  await testDb.execute(sql`DELETE FROM test_cases`);
  await testDb.delete(schema.topicProblems);
  await testDb.delete(schema.problemSolutions);
  await testDb.delete(schema.teachingUnits);
  await testDb.delete(schema.problems);
  await testDb.delete(schema.topics);
  await testDb.delete(schema.courses);
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

export async function createTestCourse(
  orgId: string,
  createdBy: string,
  overrides: Partial<typeof schema.courses.$inferInsert> = {}
) {
  const [course] = await testDb
    .insert(schema.courses)
    .values({
      orgId,
      createdBy,
      title: "Test Course",
      gradeLevel: "6-8",
      ...overrides,
    })
    .returning();
  return course;
}

export async function createTestTopic(
  courseId: string,
  overrides: Partial<typeof schema.topics.$inferInsert> = {}
) {
  const [topic] = await testDb
    .insert(schema.topics)
    .values({
      courseId,
      title: "Test Topic",
      sortOrder: 0,
      ...overrides,
    })
    .returning();
  return topic;
}

// Plan 044 phase 1: minimal teaching_unit for tests of the topic↔unit
// link. Defaults to org scope, status=draft, materialType=notes.
export async function createTestTeachingUnit(
  scopeId: string,
  createdBy: string,
  overrides: Partial<typeof schema.teachingUnits.$inferInsert> = {}
) {
  const [unit] = await testDb
    .insert(schema.teachingUnits)
    .values({
      scope: "org",
      scopeId,
      title: "Test Unit",
      summary: "",
      createdBy,
      ...overrides,
    })
    .returning();
  return unit;
}

export async function createTestClass(
  courseId: string,
  orgId: string,
  overrides: Partial<typeof schema.classes.$inferInsert> = {}
) {
  const [cls] = await testDb
    .insert(schema.classes)
    .values({
      courseId,
      orgId,
      title: "Test Class",
      joinCode: nanoid(8),
      ...overrides,
    })
    .returning();
  return cls;
}

export async function createTestAssignment(
  classId: string,
  overrides: Partial<typeof schema.assignments.$inferInsert> = {}
) {
  const [assignment] = await testDb
    .insert(schema.assignments)
    .values({
      classId,
      title: "Test Assignment",
      ...overrides,
    })
    .returning();
  return assignment;
}

export async function createTestSubmission(
  assignmentId: string,
  studentId: string,
  overrides: Partial<typeof schema.submissions.$inferInsert> = {}
) {
  const [submission] = await testDb
    .insert(schema.submissions)
    .values({
      assignmentId,
      studentId,
      ...overrides,
    })
    .returning();
  return submission;
}

export async function closeTestDb() {
  await testClient.end();
}

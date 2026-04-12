import { eq, and } from "drizzle-orm";
import { submissions, users } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

interface CreateSubmissionInput {
  assignmentId: string;
  studentId: string;
  documentId?: string;
}

export async function createSubmission(db: Database, input: CreateSubmissionInput) {
  const [submission] = await db
    .insert(submissions)
    .values(input)
    .onConflictDoNothing()
    .returning();
  return submission || null;
}

export async function getSubmission(db: Database, submissionId: string) {
  const [submission] = await db
    .select()
    .from(submissions)
    .where(eq(submissions.id, submissionId));
  return submission || null;
}

export async function listSubmissionsByAssignment(db: Database, assignmentId: string) {
  return db
    .select({
      id: submissions.id,
      assignmentId: submissions.assignmentId,
      studentId: submissions.studentId,
      documentId: submissions.documentId,
      grade: submissions.grade,
      feedback: submissions.feedback,
      submittedAt: submissions.submittedAt,
      studentName: users.name,
      studentEmail: users.email,
    })
    .from(submissions)
    .innerJoin(users, eq(submissions.studentId, users.id))
    .where(eq(submissions.assignmentId, assignmentId));
}

export async function listSubmissionsByStudent(db: Database, studentId: string) {
  return db
    .select()
    .from(submissions)
    .where(eq(submissions.studentId, studentId));
}

export async function getSubmissionByAssignmentAndStudent(
  db: Database,
  assignmentId: string,
  studentId: string
) {
  const [submission] = await db
    .select()
    .from(submissions)
    .where(
      and(
        eq(submissions.assignmentId, assignmentId),
        eq(submissions.studentId, studentId)
      )
    );
  return submission || null;
}

export async function gradeSubmission(
  db: Database,
  submissionId: string,
  grade: number,
  feedback?: string
) {
  const updates: Record<string, unknown> = { grade };
  if (feedback !== undefined) {
    updates.feedback = feedback;
  }
  const [updated] = await db
    .update(submissions)
    .set(updates)
    .where(eq(submissions.id, submissionId))
    .returning();
  return updated || null;
}

import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, createTestOrg, createTestCourse, createTestAssignment, createTestSubmission } from "../helpers";
import { createSubmission, getSubmission, listSubmissionsByAssignment, listSubmissionsByStudent, getSubmissionByAssignmentAndStudent, gradeSubmission } from "@/lib/submissions";
import { createClass } from "@/lib/classes";

describe("submission operations", () => {
  let assignment: Awaited<ReturnType<typeof createTestAssignment>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;

  beforeEach(async () => {
    const org = await createTestOrg();
    const teacher = await createTestUser({ email: "teacher@test.edu" });
    student = await createTestUser({ email: "student@test.edu" });
    const course = await createTestCourse(org.id, teacher.id);
    const cls = await createClass(testDb, { courseId: course.id, orgId: org.id, title: "Test", createdBy: teacher.id });
    assignment = await createTestAssignment(cls.id);
  });

  it("creates a submission", async () => {
    const sub = await createSubmission(testDb, {
      assignmentId: assignment.id,
      studentId: student.id,
    });
    expect(sub).not.toBeNull();
    expect(sub!.assignmentId).toBe(assignment.id);
  });

  it("prevents duplicate submission", async () => {
    await createSubmission(testDb, { assignmentId: assignment.id, studentId: student.id });
    const dup = await createSubmission(testDb, { assignmentId: assignment.id, studentId: student.id });
    expect(dup).toBeNull();
  });

  it("gets submission by ID", async () => {
    const sub = await createTestSubmission(assignment.id, student.id);
    const found = await getSubmission(testDb, sub.id);
    expect(found).not.toBeNull();
  });

  it("lists submissions by assignment with student info", async () => {
    await createTestSubmission(assignment.id, student.id);
    const list = await listSubmissionsByAssignment(testDb, assignment.id);
    expect(list).toHaveLength(1);
    expect(list[0].studentName).toBe(student.name);
  });

  it("lists submissions by student", async () => {
    await createTestSubmission(assignment.id, student.id);
    const list = await listSubmissionsByStudent(testDb, student.id);
    expect(list).toHaveLength(1);
  });

  it("finds submission by assignment + student", async () => {
    await createTestSubmission(assignment.id, student.id);
    const found = await getSubmissionByAssignmentAndStudent(testDb, assignment.id, student.id);
    expect(found).not.toBeNull();
  });

  it("returns null when no submission exists", async () => {
    const found = await getSubmissionByAssignmentAndStudent(testDb, assignment.id, student.id);
    expect(found).toBeNull();
  });

  it("grades a submission", async () => {
    const sub = await createTestSubmission(assignment.id, student.id);
    const graded = await gradeSubmission(testDb, sub.id, 95, "Great work!");
    expect(graded!.grade).toBe(95);
    expect(graded!.feedback).toBe("Great work!");
  });

  it("re-grades a submission", async () => {
    const sub = await createTestSubmission(assignment.id, student.id);
    await gradeSubmission(testDb, sub.id, 80, "Good");
    const regraded = await gradeSubmission(testDb, sub.id, 90, "Better");
    expect(regraded!.grade).toBe(90);
    expect(regraded!.feedback).toBe("Better");
  });
});

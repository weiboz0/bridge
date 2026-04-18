import { notFound } from "next/navigation";
import { api, ApiError } from "@/lib/api-client";
import { TeacherWatchShell } from "@/components/problem/teacher-watch-shell";
import type {
  Problem,
  TestCase,
  Attempt,
} from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";

interface StudentAttemptsResponse {
  student: { id: string; name: string; email: string };
  attempts: Attempt[];
}

export default async function TeacherWatchStudentPage({
  params,
}: {
  params: Promise<{ id: string; problemId: string; studentId: string }>;
}) {
  const { id: classId, problemId, studentId } = await params;

  try {
    const [problem, testCases, payload] = await Promise.all([
      api<Problem>(`/api/problems/${problemId}`),
      api<TestCase[]>(`/api/problems/${problemId}/test-cases`),
      api<StudentAttemptsResponse>(
        `/api/teacher/problems/${problemId}/students/${studentId}/attempts`
      ),
    ]);

    return (
      <TeacherWatchShell
        classId={classId}
        problem={problem}
        testCases={testCases}
        student={payload.student}
        attempts={payload.attempts}
        initialAttemptId={payload.attempts[0]?.id ?? null}
      />
    );
  } catch (e) {
    if (e instanceof ApiError && (e.status === 404 || e.status === 403)) {
      notFound();
    }
    throw e;
  }
}

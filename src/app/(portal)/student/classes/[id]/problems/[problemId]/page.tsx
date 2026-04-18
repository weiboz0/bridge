import { notFound } from "next/navigation";
import { api, ApiError } from "@/lib/api-client";
import { ProblemShell } from "@/components/problem/problem-shell";

interface Problem {
  id: string;
  topicId: string;
  title: string;
  description: string;
  starterCode: string | null;
  language: string;
  order: number;
  createdBy: string;
}

interface TestCase {
  id: string;
  problemId: string;
  ownerId: string | null;
  name: string;
  stdin: string;
  expectedStdout: string | null;
  isExample: boolean;
  order: number;
}

interface Attempt {
  id: string;
  problemId: string;
  userId: string;
  title: string;
  language: string;
  plainText: string;
  createdAt: string;
  updatedAt: string;
}

export default async function StudentProblemPage({
  params,
}: {
  params: Promise<{ id: string; problemId: string }>;
}) {
  const { id: classId, problemId } = await params;

  try {
    const [problem, testCases, attempts] = await Promise.all([
      api<Problem>(`/api/problems/${problemId}`),
      api<TestCase[]>(`/api/problems/${problemId}/test-cases`),
      api<Attempt[]>(`/api/problems/${problemId}/attempts`),
    ]);

    return (
      <ProblemShell
        classId={classId}
        problem={problem}
        testCases={testCases}
        attempts={attempts}
        initialAttemptId={attempts[0]?.id ?? null}
      />
    );
  } catch (e) {
    if (e instanceof ApiError && (e.status === 404 || e.status === 403)) {
      notFound();
    }
    throw e;
  }
}

export type { Problem, TestCase, Attempt };

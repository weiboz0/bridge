import { notFound } from "next/navigation";
import { db } from "@/lib/db";
import { getClassSettings } from "@/lib/classes";
import { api, ApiError } from "@/lib/api-client";
import { ProblemShell } from "@/components/problem/problem-shell";
import type {
  Problem,
  TestCase,
  Attempt,
} from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";

export default async function StudentProblemAttemptPage({
  params,
}: {
  params: Promise<{ id: string; problemId: string; attemptId: string }>;
}) {
  const { id: classId, problemId, attemptId } = await params;

  try {
    const [problem, testCases, attempts, attempt, classSettings] = await Promise.all([
      api<Problem>(`/api/problems/${problemId}`),
      api<TestCase[]>(`/api/problems/${problemId}/test-cases`),
      api<Attempt[]>(`/api/problems/${problemId}/attempts`),
      api<Attempt>(`/api/attempts/${attemptId}`),
      getClassSettings(db, classId),
    ]);

    // The URL's attempt must belong to this problem. Anything else is a
    // malformed link — 404 rather than silently redirecting.
    if (attempt.problemId !== problemId) {
      notFound();
    }

    // Derive language from class settings (plan 028: problems no longer carry
    // a top-level language field). Fall back to the attempt's own language if
    // class settings are missing.
    const language = (classSettings?.editorMode ??
      attempt.language ??
      "python") as "python" | "javascript" | "blockly";

    // Starter code is a JSONB object keyed by language.
    const starterCode = problem.starterCode?.[language] ?? "";

    // Ensure the attempt is in the attempts list (it always should be, since
    // `/api/problems/{id}/attempts` returns the caller's attempts for this
    // problem). Harmless belt-and-suspenders.
    const attemptsWithActive = attempts.find((a) => a.id === attempt.id)
      ? attempts
      : [attempt, ...attempts];

    return (
      <ProblemShell
        classId={classId}
        problem={problem}
        testCases={testCases}
        attempts={attemptsWithActive}
        initialAttemptId={attempt.id}
        language={language}
        starterCode={starterCode}
      />
    );
  } catch (e) {
    // Handler returns 404 for cross-user attempts (Plan 024 Owner-only
    // guard), so we land here too.
    if (e instanceof ApiError && (e.status === 404 || e.status === 403)) {
      notFound();
    }
    throw e;
  }
}

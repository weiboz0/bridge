import { notFound } from "next/navigation";
import { db } from "@/lib/db";
import { getClassSettings } from "@/lib/classes";
import { api, ApiError } from "@/lib/api-client";
import { ProblemShell } from "@/components/problem/problem-shell";

interface Problem {
  id: string;
  title: string;
  description: string;
  /** JSONB object keyed by language: { python: "...", javascript: "..." } */
  starterCode: Record<string, string> | null;
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
    const [problem, testCases, initialAttempts, classSettings] = await Promise.all([
      api<Problem>(`/api/problems/${problemId}`),
      api<TestCase[]>(`/api/problems/${problemId}/test-cases`),
      api<Attempt[]>(`/api/problems/${problemId}/attempts`),
      getClassSettings(db, classId),
    ]);

    // Derive language from class settings (not from problem — problems no
    // longer carry a top-level language field since plan 028).
    const language = (classSettings?.editorMode ?? "python") as
      | "python"
      | "javascript"
      | "blockly";

    // Starter code is now a JSONB object keyed by language.
    const starter = problem.starterCode?.[language] ?? "";

    // Yjs persistence requires an attempt id at room name. If the student has
    // never opened this problem we eagerly create one seeded with starter_code
    // so the editor connects to a real room. Cost: at most one stray empty
    // attempt per (user, problem) the first time they visit; if they never
    // edit, the next visit reuses the same one (no further strays).
    let attempts = initialAttempts;
    if (attempts.length === 0) {
      const seed = await api<Attempt>(`/api/problems/${problemId}/attempts`, {
        method: "POST",
        body: { plainText: starter, language },
      });
      attempts = [seed];
    }

    return (
      <ProblemShell
        classId={classId}
        problem={problem}
        testCases={testCases}
        attempts={attempts}
        initialAttemptId={attempts[0]!.id}
        language={language}
        starterCode={starter}
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

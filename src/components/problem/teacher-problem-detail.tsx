import Link from "next/link";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ProblemActions } from "./problem-actions";
import { TestCasesCard } from "./test-cases-card";

// Plan 066 phase 2 — read-only detail page for a single problem in
// the teacher's bank. Server component (fetches via api()), passes
// the resulting Problem + TestCase[] to this presentation component.

export interface ProblemDetailData {
  id: string;
  scope: "platform" | "org" | "personal";
  scopeId: string | null;
  title: string;
  slug: string | null;
  description: string;
  starterCode: Record<string, string>;
  difficulty: string;
  gradeLevel: string | null;
  tags: string[];
  status: "draft" | "published" | "archived";
  forkedFrom: string | null;
  timeLimitMs: number | null;
  memoryLimitMb: number | null;
  createdAt: string;
  updatedAt: string;
}

export interface TestCaseData {
  id: string;
  problemId: string;
  ownerId: string | null;
  name: string;
  stdin: string;
  expectedStdout: string | null;
  isExample: boolean;
  order: number;
}

const SCOPE_LABELS: Record<string, string> = {
  platform: "Platform",
  org: "Org",
  personal: "Personal",
};

const STATUS_LABELS: Record<string, string> = {
  draft: "Draft",
  published: "Published",
  archived: "Archived",
};

function difficultyClasses(d: string): string {
  if (d === "easy") return "bg-emerald-50 text-emerald-700 border-emerald-200";
  if (d === "medium") return "bg-amber-50 text-amber-700 border-amber-200";
  if (d === "hard") return "bg-rose-50 text-rose-700 border-rose-200";
  return "bg-zinc-50 text-zinc-700 border-zinc-200";
}

function statusClasses(s: string): string {
  if (s === "published") return "bg-blue-50 text-blue-700 border-blue-200";
  if (s === "archived") return "bg-zinc-50 text-zinc-500 border-zinc-200";
  return "bg-amber-50 text-amber-700 border-amber-200"; // draft
}

interface Props {
  problem: ProblemDetailData;
  testCases: TestCaseData[];
  // True when the current viewer is the problem's author / has
  // edit privileges. Drives whether canonical hidden cases render
  // in full or only as a count.
  canAuthor: boolean;
}

export function TeacherProblemDetail({ problem, testCases, canAuthor }: Props) {
  // Filter to canonical (problem-owned) cases — the authored content
  // shown on this page. User-private cases live in the student "My
  // cases" surface and are intentionally hidden here.
  const canonicalCases = testCases.filter((c) => c.ownerId === null);

  const starterLanguages = Object.keys(problem.starterCode || {});

  return (
    <div className="p-6 max-w-5xl space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-2">
          <Link
            href="/teacher/problems"
            className="text-xs text-muted-foreground hover:text-foreground"
          >
            ← Back to Problem Bank
          </Link>
          <h1 className="text-2xl font-bold">{problem.title}</h1>
          <div className="flex flex-wrap items-center gap-2 text-xs">
            {problem.slug && (
              <span className="text-muted-foreground font-mono">{problem.slug}</span>
            )}
            <Badge className={difficultyClasses(problem.difficulty)}>
              {problem.difficulty}
            </Badge>
            <Badge className={statusClasses(problem.status)}>
              {STATUS_LABELS[problem.status] ?? problem.status}
            </Badge>
            <span className="text-muted-foreground">
              {SCOPE_LABELS[problem.scope] ?? problem.scope}
            </span>
            {problem.gradeLevel && (
              <span className="text-muted-foreground">{problem.gradeLevel}</span>
            )}
            {problem.tags.length > 0 && (
              <span className="text-muted-foreground">
                · {problem.tags.join(", ")}
              </span>
            )}
          </div>
        </div>
        <ProblemActions problem={problem} canAuthor={canAuthor} />
      </div>

      {/* Description */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
            Description
          </CardTitle>
        </CardHeader>
        <CardContent>
          {problem.description ? (
            <div className="prose prose-sm max-w-none">
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{problem.description}</ReactMarkdown>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground italic">
              No description yet.
            </p>
          )}
        </CardContent>
      </Card>

      {/* Starter code */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
            Starter Code
          </CardTitle>
        </CardHeader>
        <CardContent>
          {starterLanguages.length === 0 ? (
            <p className="text-sm text-muted-foreground italic">
              No starter code provided.
            </p>
          ) : (
            <div className="space-y-3">
              {starterLanguages.map((lang) => (
                <div key={lang} className="overflow-hidden rounded-lg border border-zinc-200">
                  <div className="bg-zinc-50/70 border-b border-zinc-200 px-3 py-1 font-mono text-[11px] uppercase tracking-wider text-zinc-600">
                    {lang}
                  </div>
                  <pre className="overflow-x-auto p-3 font-mono text-xs leading-relaxed">
                    {problem.starterCode[lang]}
                  </pre>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Test cases */}
      <TestCasesCard
        problemId={problem.id}
        canonicalCases={canonicalCases}
        canAuthor={canAuthor}
      />

      {/* Metadata footer */}
      <div className="text-xs text-muted-foreground">
        Created {new Date(problem.createdAt).toLocaleString()} · Last updated{" "}
        {new Date(problem.updatedAt).toLocaleString()}
        {problem.forkedFrom && (
          <>
            {" "}
            · Forked from{" "}
            <Link
              href={`/teacher/problems/${problem.forkedFrom}`}
              className="underline hover:text-foreground"
            >
              {problem.forkedFrom}
            </Link>
          </>
        )}
      </div>
    </div>
  );
}


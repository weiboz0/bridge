"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { TestCaseEditor } from "./test-case-editor";
import type { TestCaseData } from "./teacher-problem-detail";

// Plan 066 phase 4 — wraps the test-cases card on the teacher detail
// page with an "Edit cases" toggle. Only canAuthor viewers see the
// toggle; everyone else gets the existing read-only list (with
// hidden cases collapsed to a count for non-authors).

interface Props {
  problemId: string;
  /** Canonical (problem-owned) cases only. Non-canonical/private cases
   *  belong to the student "My cases" surface; they're filtered out
   *  upstream in TeacherProblemDetail. */
  canonicalCases: TestCaseData[];
  canAuthor: boolean;
}

export function TestCasesCard({ problemId, canonicalCases, canAuthor }: Props) {
  const router = useRouter();
  const [editing, setEditing] = useState(false);

  const exampleCases = canonicalCases.filter((c) => c.isExample);
  const hiddenCases = canonicalCases.filter((c) => !c.isExample);

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
        <CardTitle className="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
          Test Cases ({canonicalCases.length})
        </CardTitle>
        {canAuthor && !editing && (
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setEditing(true)}
          >
            Edit cases
          </Button>
        )}
      </CardHeader>
      <CardContent>
        {editing ? (
          <TestCaseEditor
            problemId={problemId}
            initial={canonicalCases}
            onCancel={() => setEditing(false)}
            onSaved={() => {
              setEditing(false);
              // Per Risks §1: refetch the case list rather than
              // trusting local state. router.refresh() re-runs the
              // server component, which re-fetches from Go.
              router.refresh();
            }}
          />
        ) : canonicalCases.length === 0 ? (
          <p className="text-sm text-muted-foreground italic">
            {canAuthor
              ? 'No test cases yet. Click "Edit cases" to add some.'
              : "No test cases yet."}
          </p>
        ) : (
          <div className="space-y-3">
            {exampleCases.map((c, i) => (
              <ReadOnlyTestCaseRow key={c.id} testCase={c} index={i} />
            ))}
            {hiddenCases.length > 0 && (
              <div className="pt-2 border-t border-zinc-200">
                <p className="text-xs font-mono uppercase tracking-wider text-zinc-500 mb-2">
                  Hidden cases ({hiddenCases.length})
                </p>
                {canAuthor ? (
                  <div className="space-y-3">
                    {hiddenCases.map((c, i) => (
                      <ReadOnlyTestCaseRow
                        key={c.id}
                        testCase={c}
                        index={i}
                        hidden
                      />
                    ))}
                  </div>
                ) : (
                  <p className="text-xs text-muted-foreground">
                    {hiddenCases.length} hidden case
                    {hiddenCases.length === 1 ? "" : "s"} run during Test
                    execution but aren&apos;t visible to non-authors.
                  </p>
                )}
              </div>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function ReadOnlyTestCaseRow({
  testCase,
  index,
  hidden,
}: {
  testCase: TestCaseData;
  index: number;
  hidden?: boolean;
}) {
  const label = testCase.name || `${hidden ? "Hidden" : "Example"} ${index + 1}`;
  return (
    <div className="overflow-hidden rounded-lg border border-zinc-200">
      <div className="flex items-center justify-between border-b border-zinc-200 bg-zinc-50/70 px-2.5 py-1">
        <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-600">
          {label}
        </span>
        <Badge
          className={
            hidden
              ? "bg-zinc-100 text-zinc-700 border-zinc-200"
              : "bg-emerald-50 text-emerald-700 border-emerald-200"
          }
        >
          {hidden ? "Hidden" : "Example"}
        </Badge>
      </div>
      <div className="grid grid-cols-2 divide-x divide-zinc-200">
        <div className="p-2.5">
          <p className="mb-1 font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">
            Input
          </p>
          <pre className="whitespace-pre-wrap font-mono text-[12px] leading-[1.55] text-zinc-800">
            {testCase.stdin || <span className="text-zinc-400">(empty)</span>}
          </pre>
        </div>
        <div className="p-2.5">
          <p className="mb-1 font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">
            Expected Output
          </p>
          <pre className="whitespace-pre-wrap font-mono text-[12px] leading-[1.55] text-zinc-800">
            {testCase.expectedStdout ?? (
              <span className="text-zinc-400">(any)</span>
            )}
          </pre>
        </div>
      </div>
    </div>
  );
}

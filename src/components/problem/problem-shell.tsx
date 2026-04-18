"use client";

import { useMemo, useState } from "react";
import type { Problem, TestCase, Attempt } from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";
import { SectionLabel, Tag } from "@/components/design/primitives";
import { ProblemDescription } from "@/components/problem/problem-description";

interface Props {
  classId: string;
  problem: Problem;
  testCases: TestCase[];
  attempts: Attempt[];
  initialAttemptId: string | null;
}

export function ProblemShell({ problem, testCases, attempts, initialAttemptId }: Props) {
  // Active attempt state. `null` when the student has never touched the problem
  // yet — the editor shows starter_code and Task 3's autosave hook will create
  // an Attempt on the first keystroke.
  const [activeAttemptId, setActiveAttemptId] = useState<string | null>(initialAttemptId);

  const activeAttempt = useMemo(
    () => attempts.find((a) => a.id === activeAttemptId) ?? null,
    [attempts, activeAttemptId]
  );

  return (
    <div className="flex h-[calc(100vh-var(--portal-header-height,56px))] overflow-hidden">
      {/* LEFT — problem description + test cases (Task 2 + Task 7) */}
      <aside className="flex w-[32%] min-w-[360px] flex-col border-r border-zinc-200 bg-white">
        <SectionLabel action={<Tag tone="zinc">Problem</Tag>}>Problem</SectionLabel>
        <div className="flex-1 overflow-auto p-5">
          <ProblemDescription problem={problem} testCases={testCases} />
        </div>
      </aside>

      {/* CENTER — attempt header + editor (Tasks 4, 5) */}
      <section className="flex min-w-0 flex-1 flex-col bg-white">
        <div className="flex h-11 items-center gap-3 border-b border-zinc-200 px-4">
          <span className="text-sm font-medium tracking-tight">
            {activeAttempt ? activeAttempt.title : "Start coding to create an attempt"}
          </span>
          <span className="ml-auto font-mono text-[11px] text-zinc-400">
            {attempts.length} attempt{attempts.length === 1 ? "" : "s"}
          </span>
        </div>
        <div className="flex-1 overflow-auto p-6 font-mono text-[13px] text-zinc-400">
          {/* Editor placeholder — replaced in Task 4 */}
          <pre className="whitespace-pre-wrap">{activeAttempt?.plainText ?? problem.starterCode ?? ""}</pre>
        </div>
      </section>

      {/* RIGHT — inputs + terminal (Tasks 7, 8) */}
      <aside className="flex w-[28%] min-w-[320px] flex-col border-l border-zinc-200 bg-white">
        <SectionLabel>Inputs</SectionLabel>
        <div className="p-4 text-sm text-zinc-500">Inputs picker coming in Task 7.</div>
        <SectionLabel>Terminal</SectionLabel>
        <div className="flex-1 p-4 text-sm text-zinc-500">Run output coming in Task 8.</div>
      </aside>
    </div>
  );
}

"use client";

import { useMemo } from "react";
import type { Problem, TestCase, Attempt } from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";
import { SectionLabel, Tag } from "@/components/design/primitives";
import { ProblemDescription } from "@/components/problem/problem-description";
import { CodeEditor } from "@/components/editor/code-editor";
import { useAutosaveAttempt } from "@/lib/problem/use-autosave-attempt";

interface Props {
  classId: string;
  problem: Problem;
  testCases: TestCase[];
  attempts: Attempt[];
  /** Attempt to load on first render; null = start from starter_code. */
  initialAttemptId: string | null;
}

export function ProblemShell({ problem, testCases, attempts, initialAttemptId }: Props) {
  const initialAttempt = useMemo(
    () => attempts.find((a) => a.id === initialAttemptId) ?? null,
    [attempts, initialAttemptId]
  );

  const { code, setCode, attempt, saveState, lastSavedAt } = useAutosaveAttempt({
    problemId: problem.id,
    initialAttempt,
    starterCode: problem.starterCode ?? "",
    language: problem.language,
  });

  // Editor remounts when the attempt ID changes so Monaco picks up the new
  // content (its `defaultValue` prop is uncontrolled — only set on mount).
  const editorKey = attempt?.id ?? "starter";

  return (
    <div className="flex h-[calc(100vh-var(--portal-header-height,56px))] overflow-hidden">
      {/* LEFT — problem description + test cases */}
      <aside className="flex w-[32%] min-w-[360px] flex-col border-r border-zinc-200 bg-white">
        <SectionLabel action={<Tag tone="zinc">Problem</Tag>}>Problem</SectionLabel>
        <div className="flex-1 overflow-auto p-5">
          <ProblemDescription problem={problem} testCases={testCases} />
        </div>
      </aside>

      {/* CENTER — attempt header + editor */}
      <section className="flex min-w-0 flex-1 flex-col bg-white">
        <div className="flex h-11 items-center gap-3 border-b border-zinc-200 px-4">
          <span className="text-sm font-medium tracking-tight">
            {attempt ? attempt.title : "Untitled (unsaved)"}
          </span>
          <span className="font-mono text-[11px] text-zinc-400">
            {attempts.length} attempt{attempts.length === 1 ? "" : "s"}
          </span>
          <span className="ml-auto font-mono text-[11px] uppercase tracking-[0.18em] text-zinc-400">
            <SaveIndicator state={saveState} lastSavedAt={lastSavedAt} />
          </span>
        </div>
        <div className="min-h-0 flex-1 p-3">
          <CodeEditor
            key={editorKey}
            initialCode={code}
            onChange={setCode}
            language={problem.language}
          />
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

function SaveIndicator({
  state,
  lastSavedAt,
}: {
  state: "idle" | "pending" | "saving" | "error";
  lastSavedAt: Date | null;
}) {
  if (state === "error") return <span className="text-rose-700 normal-case tracking-normal">save failed · retrying on next edit</span>;
  if (state === "saving") return <>saving…</>;
  if (state === "pending") return <>unsaved</>;
  if (!lastSavedAt) return <>not yet saved</>;
  const secs = Math.max(0, Math.floor((Date.now() - lastSavedAt.getTime()) / 1000));
  if (secs < 5) return <>saved · just now</>;
  if (secs < 60) return <>saved · {secs}s ago</>;
  return <>saved</>;
}

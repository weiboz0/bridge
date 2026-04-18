"use client";

import { useEffect, useMemo, useState } from "react";
import type { Problem, TestCase, Attempt } from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";
import { SectionLabel, Tag } from "@/components/design/primitives";
import { ProblemDescription } from "@/components/problem/problem-description";
import { CodeEditor } from "@/components/editor/code-editor";
import { AttemptHeader } from "@/components/problem/attempt-header";
import { useAutosaveAttempt } from "@/lib/problem/use-autosave-attempt";

interface Props {
  classId: string;
  problem: Problem;
  testCases: TestCase[];
  attempts: Attempt[];
  /** Attempt to load on first render; null = start from starter_code. */
  initialAttemptId: string | null;
}

export function ProblemShell({ problem, testCases, attempts: initialAttempts, initialAttemptId }: Props) {
  // The attempts list grows as the user creates new ones. Order by updated_at
  // DESC (server already sorted the initial page load).
  const [attempts, setAttempts] = useState<Attempt[]>(initialAttempts);

  const initialAttempt = useMemo(
    () => attempts.find((a) => a.id === initialAttemptId) ?? null,
    // only on first render — switching is handled via useAutosaveAttempt's setAttempt
    // eslint-disable-next-line react-hooks/exhaustive-deps
    []
  );

  const { code, setCode, attempt, setAttempt, saveState, lastSavedAt, flush } = useAutosaveAttempt({
    problemId: problem.id,
    initialAttempt,
    starterCode: problem.starterCode ?? "",
    language: problem.language,
  });

  // Keep the attempts list in sync with the active attempt's metadata
  // (title changes, updated_at bumps from autosave).
  useEffect(() => {
    if (!attempt) return;
    setAttempts((prev) => {
      const exists = prev.find((a) => a.id === attempt.id);
      if (!exists) return [attempt, ...prev];
      const merged = prev.map((a) => (a.id === attempt.id ? attempt : a));
      merged.sort((a, b) => b.updatedAt.localeCompare(a.updatedAt));
      return merged;
    });
  }, [attempt]);

  function handleNewAttempt(created: Attempt) {
    setAttempts((prev) => [created, ...prev]);
    setAttempt(created);
  }

  function handleRenamed(updated: Attempt) {
    setAttempts((prev) => prev.map((a) => (a.id === updated.id ? updated : a)));
    if (attempt?.id === updated.id) setAttempt(updated);
  }

  // Editor remounts when the attempt ID changes so Monaco picks up new content.
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
        <AttemptHeader
          problemId={problem.id}
          attempts={attempts}
          activeAttempt={attempt}
          totalCount={attempts.length}
          onSwitch={setAttempt}
          onCreated={handleNewAttempt}
          onRenamed={handleRenamed}
          currentCode={code}
          flushPending={flush}
          language={problem.language}
          saveIndicator={<SaveIndicator state={saveState} lastSavedAt={lastSavedAt} />}
        />
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
  const baseCls = "font-mono text-[11px] uppercase tracking-[0.18em] text-zinc-400";
  if (state === "error")
    return (
      <span className="font-mono text-[11px] text-rose-700">save failed</span>
    );
  if (state === "saving") return <span className={baseCls}>saving…</span>;
  if (state === "pending") return <span className={baseCls}>unsaved</span>;
  if (!lastSavedAt) return <span className={baseCls}>not yet saved</span>;
  const secs = Math.max(0, Math.floor((Date.now() - lastSavedAt.getTime()) / 1000));
  if (secs < 5) return <span className={baseCls}>saved · just now</span>;
  if (secs < 60) return <span className={baseCls}>saved · {secs}s ago</span>;
  return <span className={baseCls}>saved</span>;
}

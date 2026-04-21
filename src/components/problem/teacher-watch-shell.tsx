"use client";

import { useEffect, useMemo, useState } from "react";
import { useSession } from "next-auth/react";
import type {
  Problem,
  TestCase,
  Attempt,
} from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";
import { SectionLabel, StatusDot, Tag } from "@/components/design/primitives";
import { ProblemDescription } from "@/components/problem/problem-description";
import { AttemptCardsRow } from "@/components/problem/attempt-cards-row";
import { CodeEditor } from "@/components/editor/code-editor";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import {
  TestResultsCard,
  type TestRunSummary,
} from "@/components/problem/test-results-card";

interface Props {
  classId: string;
  problem: Problem;
  testCases: TestCase[];
  student: { id: string; name: string; email: string };
  attempts: Attempt[];
  initialAttemptId: string | null;
  /** Editor language derived from class settings (plan 028: problems no
   *  longer carry a top-level language field). */
  language: string;
}

export function TeacherWatchShell({
  problem,
  testCases,
  student,
  attempts,
  initialAttemptId,
  language,
}: Props) {
  const { data: session } = useSession();
  const teacherId = session?.user?.id ?? "";

  const [activeAttemptId, setActiveAttemptId] = useState<string | null>(initialAttemptId);

  const activeAttempt = useMemo(
    () => attempts.find((a) => a.id === activeAttemptId) ?? null,
    [attempts, activeAttemptId]
  );

  // Connect read-only to the active attempt's Yjs room. Hocuspocus enforces
  // read-only on the teacher side via teacherCanViewAttempt; CodeEditor
  // additionally renders Monaco with readOnly=true.
  const { yText, provider, connected } = useYjsProvider({
    documentName: teacherId && activeAttemptId ? `attempt:${activeAttemptId}` : "noop",
    token: teacherId ? `${teacherId}:teacher` : "",
  });

  // Editor remounts on attempt switch.
  const editorKey = activeAttemptId ?? "none";

  // Awareness subscription — student broadcasts lastRun + lastTestResult
  // into local awareness state. We mirror them here. Updates re-render via
  // setSnapshot.
  const [snapshot, setSnapshot] = useState<{
    lastRun?: { stdout: string; stderr: string; completedAt: string };
    lastTestResult?: TestRunSummary;
  }>({});

  useEffect(() => {
    if (!provider) return;
    const aware = provider.awareness;
    if (!aware) return;
    const sync = () => {
      // Find the student's awareness state (anyone with lastRun or lastTestResult).
      const states = Array.from(aware.getStates().values()) as Array<{
        lastRun?: { stdout: string; stderr: string; completedAt: string };
        lastTestResult?: TestRunSummary;
      }>;
      const next: typeof snapshot = {};
      for (const s of states) {
        if (s.lastRun) next.lastRun = s.lastRun;
        if (s.lastTestResult) next.lastTestResult = s.lastTestResult;
      }
      setSnapshot(next);
    };
    aware.on("update", sync);
    sync();
    return () => {
      aware.off("update", sync);
    };
  }, [provider]);

  // Build example labels for the snapshot card.
  const exampleLabels = useMemo<Record<string, string>>(() => {
    const out: Record<string, string> = {};
    let n = 0;
    for (const c of testCases) {
      if (c.ownerId === null && c.isExample) {
        n += 1;
        out[c.id] = c.name || `Example ${n}`;
      }
    }
    return out;
  }, [testCases]);

  return (
    <div className="flex h-[calc(100vh-var(--portal-header-height,56px))] overflow-hidden">
      {/* LEFT — brief problem + watching card */}
      <aside className="flex w-[26%] min-w-[300px] flex-col border-r border-zinc-200 bg-white">
        <SectionLabel action={<Tag tone="zinc">Problem</Tag>}>Problem</SectionLabel>
        <div className="flex-1 overflow-auto">
          <div className="p-5">
            <ProblemDescription problem={problem} testCases={testCases} language={language} />
          </div>
          <div className="mx-5 mb-5 rounded-lg border border-zinc-200 bg-zinc-50/60 p-3">
            <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-500">
              Watching
            </p>
            <div className="mt-1.5 flex items-center gap-2">
              <span className="inline-flex size-7 items-center justify-center rounded-full bg-zinc-900 font-mono text-[11px] font-medium text-white">
                {initials(student.name)}
              </span>
              <div className="min-w-0">
                <p className="truncate text-[14px] font-medium tracking-tight">{student.name}</p>
                <p className="truncate font-mono text-[11px] text-zinc-500">{student.email}</p>
              </div>
              <span
                className={
                  "ml-auto inline-flex items-center gap-1 rounded-md border px-1.5 py-[2px] font-mono text-[10px] uppercase tracking-[0.14em] " +
                  (connected
                    ? "border-emerald-300/70 bg-emerald-50 text-emerald-800"
                    : "border-zinc-300 bg-zinc-100 text-zinc-500")
                }
              >
                <StatusDot tone={connected ? "pass" : "idle"} />
                {connected ? "live" : "offline"}
              </span>
            </div>
          </div>
        </div>
      </aside>

      {/* CENTER — attempts strip + read-only editor */}
      <section className="flex min-w-0 flex-1 flex-col bg-white">
        <div className="border-b border-zinc-200">
          <div className="flex h-9 items-center justify-between px-4">
            <span className="font-mono text-[10px] uppercase tracking-[0.22em] text-zinc-500">
              Attempts · {attempts.length}
            </span>
            <span className="font-mono text-[10px] uppercase tracking-[0.16em] text-zinc-400">
              sorted by recent
            </span>
          </div>
          <AttemptCardsRow
            attempts={attempts}
            activeId={activeAttemptId}
            onSelect={(a) => setActiveAttemptId(a.id)}
          />
        </div>

        {activeAttempt ? (
          <>
            <div className="flex h-10 items-center gap-2 border-b border-zinc-200 px-4">
              <StatusDot tone={connected ? "pass" : "idle"} />
              <span className="text-[13px] font-medium tracking-tight">
                {activeAttempt.title}
              </span>
              <span className="font-mono text-[11px] text-zinc-500">
                updated {relTime(activeAttempt.updatedAt)}
              </span>
              <span className="ml-auto inline-flex items-center gap-2 font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-500">
                <span className="inline-flex size-1.5 rounded-full bg-zinc-400" />
                read-only
              </span>
            </div>
            <div className="min-h-0 flex-1 p-3">
              <CodeEditor
                key={editorKey}
                initialCode={activeAttempt.plainText}
                language={language}
                yText={yText}
                provider={provider}
                readOnly
              />
            </div>
          </>
        ) : (
          <div className="flex flex-1 items-center justify-center text-sm text-zinc-500">
            Select an attempt above to view.
          </div>
        )}
      </section>

      {/* RIGHT — student-broadcast snapshot */}
      <aside className="flex w-[26%] min-w-[280px] flex-col border-l border-zinc-200 bg-white">
        <SectionLabel>Activity</SectionLabel>
        <div className="flex-1 overflow-auto p-3 space-y-3">
          {snapshot.lastRun ? (
            <div className="rounded-lg border border-zinc-200 bg-zinc-50/50">
              <div className="flex h-7 items-center justify-between border-b border-zinc-200 px-2.5">
                <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-500">
                  Last run · {relTime(snapshot.lastRun.completedAt)}
                </span>
              </div>
              <pre className="px-3 py-2 font-mono text-[12px] leading-[1.55] text-zinc-800 whitespace-pre-wrap">
                {snapshot.lastRun.stdout || <span className="text-zinc-400">(no stdout)</span>}
              </pre>
              {snapshot.lastRun.stderr && (
                <pre className="border-t border-zinc-200 px-3 py-2 font-mono text-[12px] leading-[1.55] text-rose-700 whitespace-pre-wrap">
                  {snapshot.lastRun.stderr}
                </pre>
              )}
            </div>
          ) : (
            <p className="text-[12px] text-zinc-500">No runs broadcast yet.</p>
          )}

          {snapshot.lastTestResult && (
            <TestResultsCard
              attemptId={activeAttemptId ?? ""}
              exampleLabels={exampleLabels}
              result={snapshot.lastTestResult}
              canDiff={false}
            />
          )}
        </div>
      </aside>
    </div>
  );
}

function initials(name: string): string {
  const parts = name.trim().split(/\s+/);
  if (parts.length === 0 || !parts[0]) return "?";
  if (parts.length === 1) return parts[0]!.slice(0, 2).toUpperCase();
  return (parts[0]![0]! + parts[parts.length - 1]![0]!).toUpperCase();
}

function relTime(iso: string): string {
  const s = Math.max(0, Math.floor((Date.now() - new Date(iso).getTime()) / 1000));
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  return `${h}h ago`;
}

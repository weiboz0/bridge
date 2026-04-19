"use client";

import { useEffect, useMemo, useState } from "react";
import { useSession } from "next-auth/react";
import type {
  Problem,
  TestCase,
  Attempt,
} from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";
import { SectionLabel, Tag } from "@/components/design/primitives";
import { ProblemDescription } from "@/components/problem/problem-description";
import { TestCasesPanel } from "@/components/problem/test-cases-panel";
import {
  InputsPanel,
  selectionStdin,
  type InputSelection,
} from "@/components/problem/inputs-panel";
import { CodeEditor } from "@/components/editor/code-editor";
import { OutputPanel } from "@/components/editor/output-panel";
import { AttemptHeader } from "@/components/problem/attempt-header";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { usePyodide } from "@/lib/pyodide/use-pyodide";
import { Button } from "@/components/ui/button";
import { TestResultsCard, type TestRunSummary } from "@/components/problem/test-results-card";

interface Props {
  classId: string;
  problem: Problem;
  testCases: TestCase[];
  attempts: Attempt[];
  /** Attempt to load on first render. Server eagerly created one if needed,
   *  so this is always non-null in the student route. */
  initialAttemptId: string;
}

export function ProblemShell({
  problem,
  testCases: initialTestCases,
  attempts: initialAttempts,
  initialAttemptId,
}: Props) {
  const { data: session } = useSession();
  const userId = session?.user?.id ?? "";

  const [attempts, setAttempts] = useState<Attempt[]>(initialAttempts);
  const [testCases, setTestCases] = useState<TestCase[]>(initialTestCases);
  const [activeAttemptId, setActiveAttemptId] = useState<string>(initialAttemptId);

  const activeAttempt = useMemo(
    () => attempts.find((a) => a.id === activeAttemptId) ?? null,
    [attempts, activeAttemptId]
  );

  // Yjs binding for the active attempt. Doc-name change forces reconnect on
  // attempt switch.
  const { yText, provider, connected } = useYjsProvider({
    documentName: userId ? `attempt:${activeAttemptId}` : "noop",
    token: userId ? `${userId}:user` : "",
  });

  // Default input selection — first example case if any, else Custom.
  const defaultSelection = useMemo<InputSelection>(() => {
    const firstExample = initialTestCases.find((c) => c.ownerId === null && c.isExample);
    return firstExample ? { kind: "case", caseId: firstExample.id } : { kind: "custom" };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  const [selection, setSelection] = useState<InputSelection>(defaultSelection);
  const [customStdin, setCustomStdin] = useState("");

  const pyodide = usePyodide();

  // Test run state — null until the first Test, then the latest summary.
  const [testResult, setTestResult] = useState<TestRunSummary | null>(null);
  const [testing, setTesting] = useState(false);
  const [testError, setTestError] = useState<string | null>(null);

  // Map example caseId -> "Example 1", "Example 2", … for the results card.
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

  async function handleTest() {
    setTestError(null);
    setTesting(true);
    try {
      const res = await fetch(`/api/attempts/${activeAttemptId}/test`, { method: "POST" });
      if (!res.ok) {
        setTestError(`Failed (${res.status})`);
        return;
      }
      setTestResult(await res.json());
    } catch (e) {
      setTestError(String(e));
    } finally {
      setTesting(false);
    }
  }

  function handleRun() {
    if (problem.language !== "python") return;
    // Y.Text holds the live editor contents; toString() always reflects the
    // most recent state, even mid-typing.
    const code = yText?.toString() ?? activeAttempt?.plainText ?? "";
    const stdin = selectionStdin(selection, testCases, customStdin);
    pyodide.runCode(code, { stdin });
  }

  function handleNewAttempt(created: Attempt) {
    setAttempts((prev) => [created, ...prev]);
    setActiveAttemptId(created.id);
  }
  function handleRenamed(updated: Attempt) {
    setAttempts((prev) => prev.map((a) => (a.id === updated.id ? updated : a)));
  }

  // Mirror metadata (title) into the attempts list when the active attempt
  // changes (e.g. after rename PATCH returns).
  useEffect(() => {
    if (!activeAttempt) return;
    setAttempts((prev) => {
      const merged = prev.map((a) => (a.id === activeAttempt.id ? activeAttempt : a));
      return merged;
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeAttempt?.title]);

  // Editor remounts on attempt switch (Yjs doc swap) so y-monaco rebinds.
  const editorKey = activeAttemptId;

  // Snapshot of editor code for the New attempt button — read live from Y.Text.
  const liveCode = () => yText?.toString() ?? activeAttempt?.plainText ?? "";

  return (
    <div className="flex h-[calc(100vh-var(--portal-header-height,56px))] overflow-hidden">
      {/* LEFT */}
      <aside className="flex w-[32%] min-w-[360px] flex-col border-r border-zinc-200 bg-white">
        <SectionLabel action={<Tag tone="zinc">Problem</Tag>}>Problem</SectionLabel>
        <div className="flex-1 overflow-auto">
          <div className="p-5">
            <ProblemDescription problem={problem} testCases={testCases} />
          </div>
          <TestCasesPanel
            problemId={problem.id}
            testCases={testCases}
            onCasesChange={setTestCases}
          />
        </div>
      </aside>

      {/* CENTER */}
      <section className="flex min-w-0 flex-1 flex-col bg-white">
        <AttemptHeader
          problemId={problem.id}
          attempts={attempts}
          activeAttempt={activeAttempt}
          totalCount={attempts.length}
          onSwitch={(a) => setActiveAttemptId(a.id)}
          onCreated={handleNewAttempt}
          onRenamed={handleRenamed}
          currentCode={liveCode()}
          flushPending={async () => {
            // Hocuspocus debounces internally; nothing to flush at the React
            // layer. Keep the prop for API stability with the autosave-era
            // signature.
          }}
          language={problem.language}
          saveIndicator={<SyncIndicator connected={connected} />}
          runButton={
            <Button
              size="sm"
              className="bg-amber-600 text-white hover:bg-amber-700"
              disabled={!pyodide.ready || pyodide.running || problem.language !== "python"}
              onClick={handleRun}
              title={
                problem.language !== "python"
                  ? "Run in the browser is Python-only for v1"
                  : !pyodide.ready
                    ? "Python runtime loading…"
                    : undefined
              }
            >
              <svg viewBox="0 0 10 10" className="size-2.5">
                <path fill="currentColor" d="M2 1l6 4-6 4z" />
              </svg>
              {pyodide.running ? "Running…" : "Run"}
            </Button>
          }
          testButton={
            <Button
              variant="outline"
              size="sm"
              disabled={testing}
              onClick={handleTest}
              title="Run all canonical test cases on the server"
            >
              {testing ? "Testing…" : "Test"}
            </Button>
          }
        />
        <div className="min-h-0 flex-1 p-3">
          <CodeEditor
            key={editorKey}
            initialCode={activeAttempt?.plainText ?? problem.starterCode ?? ""}
            language={problem.language}
            yText={yText}
            provider={provider}
          />
        </div>
      </section>

      {/* RIGHT */}
      <aside className="flex w-[28%] min-w-[320px] flex-col border-l border-zinc-200 bg-white">
        <SectionLabel>Inputs</SectionLabel>
        <InputsPanel
          testCases={testCases}
          selection={selection}
          customStdin={customStdin}
          onSelectionChange={setSelection}
          onCustomStdinChange={setCustomStdin}
        />

        {(testResult || testError) && (
          <div className="border-t border-zinc-200/80 px-3 py-2">
            {testError && (
              <p className="mb-1 text-xs text-rose-700">{testError}</p>
            )}
            {testResult && (
              <TestResultsCard
                attemptId={activeAttemptId}
                exampleLabels={exampleLabels}
                result={testResult}
                onClose={() => setTestResult(null)}
              />
            )}
          </div>
        )}

        <SectionLabel
          action={
            pyodide.output.length > 0 ? (
              <button
                onClick={pyodide.clearOutput}
                className="rounded px-1 font-mono text-[10px] uppercase tracking-[0.16em] text-zinc-400 hover:text-zinc-800"
              >
                clear
              </button>
            ) : null
          }
        >
          Terminal
        </SectionLabel>
        <div className="min-h-0 flex-1 p-3">
          <OutputPanel output={pyodide.output} running={pyodide.running} />
        </div>
      </aside>
    </div>
  );
}

function SyncIndicator({ connected }: { connected: boolean }) {
  if (connected) {
    return (
      <span className="inline-flex items-center gap-1.5 font-mono text-[11px] uppercase tracking-[0.18em] text-emerald-700">
        <span className="size-1.5 rounded-full bg-emerald-500" />
        synced
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1.5 font-mono text-[11px] uppercase tracking-[0.18em] text-zinc-400">
      <span className="size-1.5 rounded-full bg-zinc-300 animate-pulse" />
      connecting
    </span>
  );
}

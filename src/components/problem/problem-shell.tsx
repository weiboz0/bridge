"use client";

import { useEffect, useMemo, useRef, useState } from "react";
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
import { useRealtimeToken } from "@/lib/realtime/use-realtime-token";
import { usePyodide } from "@/lib/pyodide/use-pyodide";
import { Button } from "@/components/ui/button";
import { TestResultsCard, type TestRunSummary } from "@/components/problem/test-results-card";
import { ResponsiveTabs, type NarrowTabId } from "@/components/problem/responsive-tabs";

interface Props {
  classId: string;
  problem: Problem;
  testCases: TestCase[];
  attempts: Attempt[];
  /** Attempt to load on first render. Server eagerly created one if needed,
   *  so this is always non-null in the student route. */
  initialAttemptId: string;
  /** Editor language derived from class settings (plan 028: problems no
   *  longer carry a top-level language field). */
  language: string;
  /** Resolved starter code for this language (problem.starterCode[language] ?? ""). */
  starterCode: string;
}

export function ProblemShell({
  problem,
  testCases: initialTestCases,
  attempts: initialAttempts,
  initialAttemptId,
  language,
  starterCode,
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
  const documentName = userId ? `attempt:${activeAttemptId}` : "noop";
  const realtimeToken = useRealtimeToken(documentName);
  const { yText, provider, connected } = useYjsProvider({
    documentName,
    token: realtimeToken,
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

  // After each Run completes, snapshot the output into Hocuspocus awareness so
  // the teacher's watch page can render a "Last run · Ns ago" card. Capped to
  // keep the awareness payload small.
  const prevRunningRef = useRef(false);
  useEffect(() => {
    const wasRunning = prevRunningRef.current;
    prevRunningRef.current = pyodide.running;
    if (!wasRunning || pyodide.running) return;
    const stdout = pyodide.output
      .filter((l) => l.type === "stdout")
      .map((l) => l.text)
      .join("")
      .slice(0, 8 * 1024);
    const stderr = pyodide.output
      .filter((l) => l.type === "stderr")
      .map((l) => l.text)
      .join("")
      .slice(0, 8 * 1024);
    provider?.awareness?.setLocalStateField("lastRun", {
      stdout,
      stderr,
      completedAt: new Date().toISOString(),
    });
  }, [pyodide.running, pyodide.output, provider]);

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
      const summary = (await res.json()) as TestRunSummary;
      setTestResult(summary);
      // Broadcast to teacher watchers via Hocuspocus awareness.
      provider?.awareness?.setLocalStateField("lastTestResult", summary);
    } catch (e) {
      setTestError(String(e));
    } finally {
      setTesting(false);
    }
  }

  function handleRun() {
    if (language !== "python") return;
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

  // Plan 042 narrow-viewport tab state. Default to "code" — user lands
  // on the editor, the load-bearing pane.
  const [narrowTab, setNarrowTab] = useState<NarrowTabId>("code");

  // Plan 042 + 043 phase 4: below the @3xl/shell container breakpoint
  // (768px container width), only one of the three panes is visible at
  // a time, switched via a tab bar. Above @3xl/shell, all three render
  // side-by-side as before. The state is read only by className flags;
  // `@3xl/shell:flex` overrides `hidden` at wide widths so this state
  // stays dormant when there's enough room while every pane stays
  // mounted (preserving Yjs/Monaco state across narrow tab switches).
  //
  // `flex-1` on the active narrow pane is load-bearing: without it, the
  // pane content-sizes inside the column-flex outer container and Monaco
  // (or the I/O panel) collapses to its content height. At wide widths,
  // `@3xl/shell:flex-initial` resets to the existing wide-screen sizing.
  //
  // Plan 043 phase 4 (Codex review #5): switched from viewport `lg:` to
  // `@container/shell` queries so the breakpoint reacts to the actual
  // pane width, not the window width. With the portal sidebar consuming
  // ~224px on desktop, a 1024px viewport leaves ~800px of content area
  // — which is `@3xl` (768px) territory, not `@5xl` (1024px). The wide
  // layout's min-w floors sum to 680px, leaving ~88px for editor at the
  // floor — tight but real.
  const paneClass = (id: NarrowTabId) =>
    narrowTab === id ? "flex flex-1 @3xl/shell:flex-initial" : "hidden @3xl/shell:flex";

  return (
    <div className="@container/shell flex h-[calc(100vh-var(--portal-header-height,56px))] flex-col overflow-hidden @3xl/shell:flex-row">
      <ResponsiveTabs active={narrowTab} onChange={setNarrowTab} />

      {/* LEFT */}
      <aside
        role="tabpanel"
        id="problem-pane-problem"
        aria-labelledby="problem-tab-problem"
        className={
          paneClass("problem") +
          " w-full @3xl/shell:w-[32%] @3xl/shell:min-w-[360px] flex-col border-r border-zinc-200 bg-white"
        }
      >
        <SectionLabel action={<Tag tone="zinc">Problem</Tag>}>Problem</SectionLabel>
        <div className="flex-1 overflow-auto">
          <div className="p-5">
            <ProblemDescription problem={problem} testCases={testCases} language={language} />
          </div>
          <TestCasesPanel
            problemId={problem.id}
            testCases={testCases}
            onCasesChange={setTestCases}
          />
        </div>
      </aside>

      {/* CENTER */}
      <section
        role="tabpanel"
        id="problem-pane-code"
        aria-labelledby="problem-tab-code"
        className={paneClass("code") + " min-w-0 flex-1 flex-col bg-white"}
      >
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
          language={language}
          saveIndicator={<SyncIndicator connected={connected} />}
          runButton={
            <Button
              size="sm"
              className="bg-amber-600 text-white hover:bg-amber-700"
              disabled={!pyodide.ready || pyodide.running || language !== "python"}
              onClick={handleRun}
              title={
                language !== "python"
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
            initialCode={activeAttempt?.plainText ?? starterCode}
            language={language}
            yText={yText}
            provider={provider}
          />
        </div>
      </section>

      {/* RIGHT */}
      <aside
        role="tabpanel"
        id="problem-pane-io"
        aria-labelledby="problem-tab-io"
        className={
          paneClass("io") +
          " w-full @3xl/shell:w-[28%] @3xl/shell:min-w-[320px] flex-col border-l border-zinc-200 bg-white"
        }
      >
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
          <OutputPanel
            output={pyodide.output}
            running={pyodide.running}
            awaitingInput={pyodide.awaitingInput}
            onStdin={pyodide.provideStdin}
          />
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

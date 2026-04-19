"use client";

import { useState } from "react";

export interface TestCaseResult {
  caseId: string;
  isExample: boolean;
  status: "pass" | "fail" | "timeout" | "skipped";
  durationMs: number;
  reason?: string;
}

export interface TestRunSummary {
  ranAt: string;
  summary: {
    passed: number;
    failed: number;
    skipped: number;
    total: number;
  };
  cases: TestCaseResult[];
}

interface Props {
  attemptId: string;
  /** Maps caseId -> human label (e.g. "Example 1"). Hidden cases get a counter. */
  exampleLabels: Record<string, string>;
  result: TestRunSummary;
  onClose?: () => void;
  /** Whether the viewer can fetch the diff endpoint. Owner-only — set to false
   *  for the teacher snapshot to hide the show-diff button. Defaults to true. */
  canDiff?: boolean;
}

/**
 * Compact pass/fail card per Test run. Hidden-case failures show only
 * `Hidden case N · wrong output`; example-case failures get an "expand"
 * affordance that fetches the diff endpoint and renders side-by-side.
 */
export function TestResultsCard({ attemptId, exampleLabels, result, onClose, canDiff = true }: Props) {
  // Number the hidden cases for display (their UUIDs aren't useful here).
  let hiddenN = 0;
  const labels = result.cases.map((c) => {
    if (c.isExample) return exampleLabels[c.caseId] ?? "Example";
    hiddenN += 1;
    return `Hidden #${hiddenN}`;
  });

  return (
    <div className="rounded-lg border border-zinc-200 bg-white">
      <div className="flex h-8 items-center justify-between border-b border-zinc-200 px-3">
        <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-500">
          Test · {fmtRelative(result.ranAt)}
        </span>
        <span className="inline-flex items-center gap-2 font-mono text-[10px]">
          <span className={
            result.summary.failed === 0
              ? "text-emerald-700"
              : "text-rose-700"
          }>
            {result.summary.passed}/{result.summary.total}
          </span>
          {onClose && (
            <button
              onClick={onClose}
              className="text-zinc-400 hover:text-zinc-700"
              aria-label="Close"
            >
              ×
            </button>
          )}
        </span>
      </div>
      <div className="space-y-px px-3 py-2 font-mono text-[12px]">
        {result.cases.map((c, i) => (
          <CaseRow
            key={c.caseId}
            attemptId={attemptId}
            testCase={c}
            label={labels[i] ?? "Case"}
            canDiff={canDiff}
          />
        ))}
      </div>
    </div>
  );
}

function CaseRow({
  attemptId,
  testCase,
  label,
  canDiff,
}: {
  attemptId: string;
  testCase: TestCaseResult;
  label: string;
  canDiff: boolean;
}) {
  const [expanded, setExpanded] = useState(false);
  const canExpand = canDiff && testCase.status === "fail" && testCase.isExample;
  const statusColor =
    testCase.status === "pass"
      ? "text-emerald-700"
      : testCase.status === "timeout"
        ? "text-amber-700"
        : testCase.status === "fail"
          ? "text-rose-700"
          : "text-zinc-500";
  const statusLabel =
    testCase.status === "pass"
      ? `pass · ${testCase.durationMs}ms`
      : testCase.status === "timeout"
        ? "timeout"
        : testCase.status === "fail"
          ? testCase.reason || "fail"
          : "skipped";
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between">
        <span className="text-zinc-700">{label}</span>
        <span className="flex items-center gap-2">
          {canExpand && (
            <button
              onClick={() => setExpanded((v) => !v)}
              className="text-zinc-400 hover:text-zinc-900"
            >
              {expanded ? "hide diff" : "show diff"}
            </button>
          )}
          <span className={statusColor}>{statusLabel}</span>
        </span>
      </div>
      {expanded && (
        <DiffPanel attemptId={attemptId} caseId={testCase.caseId} />
      )}
    </div>
  );
}

function DiffPanel({ attemptId, caseId }: { attemptId: string; caseId: string }) {
  const [data, setData] = useState<{ actualStdout: string; expectedStdout: string } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  if (loading && !data && !error) {
    void fetch(`/api/attempts/${attemptId}/test/${caseId}/diff`, { method: "POST" })
      .then(async (res) => {
        if (!res.ok) {
          setError(`Failed (${res.status})`);
          return;
        }
        setData(await res.json());
      })
      .catch((e) => setError(String(e)))
      .finally(() => setLoading(false));
  }

  if (loading) return <p className="px-2 py-1 text-zinc-500">Loading diff…</p>;
  if (error) return <p className="px-2 py-1 text-rose-700">{error}</p>;
  if (!data) return null;

  return (
    <div className="grid grid-cols-2 divide-x divide-zinc-200 rounded border border-zinc-200 bg-zinc-50/50">
      <div className="p-2">
        <p className="mb-1 font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">Got</p>
        <pre className="whitespace-pre-wrap text-zinc-800">{data.actualStdout || "(empty)"}</pre>
      </div>
      <div className="p-2">
        <p className="mb-1 font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">Expected</p>
        <pre className="whitespace-pre-wrap text-zinc-800">{data.expectedStdout || "(empty)"}</pre>
      </div>
    </div>
  );
}

function fmtRelative(iso: string): string {
  const s = Math.max(0, Math.floor((Date.now() - new Date(iso).getTime()) / 1000));
  if (s < 5) return "just now";
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  return `${h}h ago`;
}

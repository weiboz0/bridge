"use client";

import { useState } from "react";
import type { TestCase } from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";
import { Tag } from "@/components/design/primitives";
import { MyTestCaseEditor } from "@/components/problem/my-test-case-editor";

interface Props {
  problemId: string;
  testCases: TestCase[];
  onCasesChange: (next: TestCase[]) => void;
}

export function TestCasesPanel({ problemId, testCases, onCasesChange }: Props) {
  const privateCases = testCases.filter((c) => c.ownerId !== null);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);

  function handleCreated(tc: TestCase) {
    onCasesChange([...testCases, tc]);
    setCreating(false);
  }
  function handleUpdated(tc: TestCase) {
    onCasesChange(testCases.map((c) => (c.id === tc.id ? tc : c)));
    setEditingId(null);
  }
  async function handleDelete(id: string) {
    const res = await fetch(`/api/test-cases/${id}`, { method: "DELETE" });
    if (!res.ok) {
      console.error("Failed to delete test case", res.status);
      return;
    }
    onCasesChange(testCases.filter((c) => c.id !== id));
  }

  return (
    <details className="border-t border-zinc-200/80 open:bg-zinc-50/30" open>
      <summary className="flex h-9 cursor-pointer list-none items-center justify-between px-5 font-mono text-[10px] uppercase tracking-[0.22em] text-zinc-500 hover:text-zinc-800">
        <span>My Test Cases · {privateCases.length}</span>
        <button
          type="button"
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            setCreating(true);
          }}
          className="font-sans text-[11px] normal-case tracking-normal text-amber-700/80 hover:text-amber-800"
        >
          + Add case
        </button>
      </summary>

      <div className="space-y-2 px-5 pb-5 pt-1">
        {privateCases.length === 0 && !creating && (
          <p className="text-[12px] text-zinc-500">
            Add your own input cases to debug against.
          </p>
        )}

        {privateCases.map((c) =>
          editingId === c.id ? (
            <MyTestCaseEditor
              key={c.id}
              problemId={problemId}
              initial={c}
              onSaved={handleUpdated}
              onCancel={() => setEditingId(null)}
            />
          ) : (
            <CaseRow
              key={c.id}
              testCase={c}
              onEdit={() => setEditingId(c.id)}
              onDelete={() => void handleDelete(c.id)}
            />
          )
        )}

        {creating && (
          <MyTestCaseEditor
            problemId={problemId}
            onSaved={handleCreated}
            onCancel={() => setCreating(false)}
          />
        )}
      </div>
    </details>
  );
}

function CaseRow({
  testCase,
  onEdit,
  onDelete,
}: {
  testCase: TestCase;
  onEdit: () => void;
  onDelete: () => void;
}) {
  return (
    <div className="overflow-hidden rounded-lg border border-amber-300/60">
      <div className="flex items-center justify-between border-b border-amber-300/60 bg-amber-50/70 px-2.5 py-1">
        <span className="text-[12px] font-medium text-amber-900">
          {testCase.name || "Untitled"}
        </span>
        <div className="flex items-center gap-1">
          <Tag tone="amber">Private</Tag>
          <button
            onClick={onEdit}
            className="rounded px-1.5 py-px font-mono text-[10px] uppercase tracking-[0.14em] text-zinc-500 hover:text-zinc-900"
          >
            edit
          </button>
          <button
            onClick={onDelete}
            className="rounded px-1.5 py-px font-mono text-[10px] uppercase tracking-[0.14em] text-rose-700 hover:text-rose-900"
          >
            delete
          </button>
        </div>
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
            Expected
          </p>
          <pre className="whitespace-pre-wrap font-mono text-[12px] leading-[1.55] text-zinc-800">
            {testCase.expectedStdout ?? <span className="text-zinc-400">(no check)</span>}
          </pre>
        </div>
      </div>
    </div>
  );
}

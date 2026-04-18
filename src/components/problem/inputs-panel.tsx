"use client";

import type { TestCase } from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";

export type InputSelection =
  | { kind: "case"; caseId: string }
  | { kind: "custom" };

interface Props {
  testCases: TestCase[];
  selection: InputSelection;
  customStdin: string;
  onSelectionChange: (s: InputSelection) => void;
  onCustomStdinChange: (s: string) => void;
}

export function InputsPanel({
  testCases,
  selection,
  customStdin,
  onSelectionChange,
  onCustomStdinChange,
}: Props) {
  const examples = testCases.filter((c) => c.ownerId === null && c.isExample);
  const privateCases = testCases.filter((c) => c.ownerId !== null);

  const activeCase =
    selection.kind === "case"
      ? testCases.find((c) => c.id === selection.caseId)
      : undefined;

  return (
    <div className="flex flex-col">
      <div className="px-4 pb-3 pt-3">
        <p className="mb-2 font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">
          Pick a case
        </p>
        <div className="-mx-0.5 flex flex-wrap gap-1.5">
          {examples.map((c, i) => (
            <Chip
              key={c.id}
              selected={selection.kind === "case" && selection.caseId === c.id}
              onClick={() => onSelectionChange({ kind: "case", caseId: c.id })}
            >
              {c.name || `Example ${i + 1}`}
            </Chip>
          ))}
          {privateCases.map((c) => (
            <Chip
              key={c.id}
              tone="amber"
              selected={selection.kind === "case" && selection.caseId === c.id}
              onClick={() => onSelectionChange({ kind: "case", caseId: c.id })}
            >
              Mine · {c.name || "Untitled"}
            </Chip>
          ))}
          <Chip
            tone="ghost"
            selected={selection.kind === "custom"}
            onClick={() => onSelectionChange({ kind: "custom" })}
          >
            Custom…
          </Chip>
        </div>

        {selection.kind === "case" && activeCase && (
          <div className="mt-3 rounded-lg border border-zinc-200 bg-zinc-50/60">
            <div className="flex h-7 items-center justify-between border-b border-zinc-200 px-2.5">
              <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-500">
                {activeCase.name || "Example"}
              </span>
              <span className="font-mono text-[10px] text-zinc-400">
                {activeCase.stdin.split("\n").length} line
                {activeCase.stdin.split("\n").length === 1 ? "" : "s"}
              </span>
            </div>
            <pre className="px-3 py-2 font-mono text-[12.5px] leading-[1.55] text-zinc-800 whitespace-pre-wrap">
              {activeCase.stdin || <span className="text-zinc-400">(empty)</span>}
            </pre>
          </div>
        )}

        {selection.kind === "custom" && (
          <div className="mt-3">
            <textarea
              value={customStdin}
              onChange={(e) => onCustomStdinChange(e.target.value)}
              rows={5}
              placeholder="Type stdin here, one line per input()"
              className="w-full rounded-md border border-zinc-200 bg-zinc-50/60 px-3 py-2 font-mono text-[12.5px]"
            />
          </div>
        )}
      </div>
    </div>
  );
}

/** Returns the stdin string implied by the current selection. */
export function selectionStdin(selection: InputSelection, testCases: TestCase[], customStdin: string): string {
  if (selection.kind === "custom") return customStdin;
  const tc = testCases.find((c) => c.id === selection.caseId);
  return tc?.stdin ?? "";
}

function Chip({
  children,
  selected,
  tone,
  onClick,
}: {
  children: React.ReactNode;
  selected?: boolean;
  tone?: "amber" | "ghost";
  onClick: () => void;
}) {
  if (selected) {
    return (
      <button
        type="button"
        onClick={onClick}
        className="inline-flex items-center rounded-md border border-zinc-900 bg-zinc-900 px-2 py-[3px] text-[11.5px] font-medium text-white"
      >
        {children}
      </button>
    );
  }
  if (tone === "amber") {
    return (
      <button
        type="button"
        onClick={onClick}
        className="inline-flex items-center rounded-md border border-amber-300/60 bg-amber-50 px-2 py-[3px] text-[11.5px] text-amber-800 hover:border-amber-400"
      >
        {children}
      </button>
    );
  }
  if (tone === "ghost") {
    return (
      <button
        type="button"
        onClick={onClick}
        className="inline-flex items-center rounded-md border border-dashed border-zinc-300 bg-white px-2 py-[3px] text-[11.5px] text-zinc-500 hover:border-zinc-400 hover:text-zinc-800"
      >
        {children}
      </button>
    );
  }
  return (
    <button
      type="button"
      onClick={onClick}
      className="inline-flex items-center rounded-md border border-zinc-200 bg-white px-2 py-[3px] text-[11.5px] text-zinc-700 hover:border-zinc-300"
    >
      {children}
    </button>
  );
}

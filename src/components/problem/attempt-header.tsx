"use client";

import { useState } from "react";
import type { Attempt } from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AttemptSwitcher } from "@/components/problem/attempt-switcher";
import { StatusDot } from "@/components/design/primitives";

interface Props {
  problemId: string;
  attempts: Attempt[];
  activeAttempt: Attempt | null;
  totalCount: number;
  onSwitch: (a: Attempt) => void;
  onCreated: (a: Attempt) => void;
  onRenamed: (a: Attempt) => void;
  /** Current code in the editor — used to seed a New attempt. */
  currentCode: string;
  /** Called before New to flush any pending save. */
  flushPending: () => Promise<void>;
  language: string;
  saveIndicator: React.ReactNode;
  runButton: React.ReactNode;
  testButton: React.ReactNode;
}

export function AttemptHeader({
  problemId,
  attempts,
  activeAttempt,
  totalCount,
  onSwitch,
  onCreated,
  onRenamed,
  currentCode,
  flushPending,
  language,
  saveIndicator,
  runButton,
  testButton,
}: Props) {
  const [creating, setCreating] = useState(false);
  const [renaming, setRenaming] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleNewAttempt() {
    setError(null);
    setCreating(true);
    try {
      await flushPending(); // avoid PATCH landing on the about-to-be-replaced attempt
      const res = await fetch(`/api/problems/${problemId}/attempts`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ plainText: currentCode, language }),
      });
      if (!res.ok) {
        setError(`Failed (${res.status})`);
        return;
      }
      const created = (await res.json()) as Attempt;
      onCreated(created);
    } finally {
      setCreating(false);
    }
  }

  return (
    <div className="flex h-11 items-center gap-2 border-b border-zinc-200 px-4">
      <StatusDot tone="idle" />

      {activeAttempt ? (
        renaming ? (
          <RenameField
            attempt={activeAttempt}
            onDone={(updated) => {
              setRenaming(false);
              if (updated) onRenamed(updated);
            }}
          />
        ) : (
          <button
            type="button"
            onClick={() => setRenaming(true)}
            className="text-left text-[13px] font-medium tracking-tight hover:text-amber-700"
            title="Rename"
          >
            {activeAttempt.title}
          </button>
        )
      ) : (
        <span className="text-[13px] font-medium tracking-tight text-zinc-500">Untitled (unsaved)</span>
      )}

      <span className="font-mono text-[11px] text-zinc-400">
        {totalCount} attempt{totalCount === 1 ? "" : "s"}
      </span>

      <AttemptSwitcher attempts={attempts} activeId={activeAttempt?.id ?? null} onSwitch={onSwitch} />

      <div className="ml-auto flex items-center gap-2">
        {saveIndicator}
        <Button variant="outline" size="sm" onClick={handleNewAttempt} disabled={creating}>
          <span className="font-mono text-[11px]">+</span>
          {creating ? "…" : "New attempt"}
        </Button>
        {testButton}
        {runButton}
      </div>

      {error && <span className="ml-2 text-xs text-rose-700">{error}</span>}
    </div>
  );
}

function RenameField({
  attempt,
  onDone,
}: {
  attempt: Attempt;
  onDone: (updated: Attempt | null) => void;
}) {
  const [value, setValue] = useState(attempt.title);
  const [submitting, setSubmitting] = useState(false);

  async function commit() {
    const next = value.trim();
    if (!next || next === attempt.title) {
      onDone(null);
      return;
    }
    setSubmitting(true);
    try {
      const res = await fetch(`/api/attempts/${attempt.id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ title: next }),
      });
      if (!res.ok) {
        onDone(null);
        return;
      }
      const updated = (await res.json()) as Attempt;
      onDone(updated);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Input
      autoFocus
      value={value}
      disabled={submitting}
      onChange={(e) => setValue(e.target.value)}
      onBlur={() => void commit()}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          void commit();
        } else if (e.key === "Escape") {
          onDone(null);
        }
      }}
      className="h-7 w-40 px-2 text-[13px]"
    />
  );
}

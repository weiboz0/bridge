"use client";

import { useState } from "react";
import type { TestCase } from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

interface Props {
  problemId: string;
  /** Populate the form from an existing case (edit mode). Omit for create. */
  initial?: TestCase;
  onSaved: (tc: TestCase) => void;
  onCancel: () => void;
}

/**
 * Inline form for creating or editing a student's private test case.
 * Canonical cases are authored elsewhere — this form only sends
 * `isCanonical: false`, which the server enforces by setting owner_id
 * to the caller's user id.
 */
export function MyTestCaseEditor({ problemId, initial, onSaved, onCancel }: Props) {
  const [name, setName] = useState(initial?.name ?? "");
  const [stdin, setStdin] = useState(initial?.stdin ?? "");
  const [expected, setExpected] = useState(initial?.expectedStdout ?? "");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      let res: Response;
      if (initial) {
        res = await fetch(`/api/test-cases/${initial.id}`, {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            name,
            stdin,
            expectedStdout: expected, // "" clears — matches store UpdateTestCaseInput semantics
          }),
        });
      } else {
        res = await fetch(`/api/problems/${problemId}/test-cases`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            name,
            stdin,
            expectedStdout: expected ? expected : null,
            isExample: false,
            isCanonical: false,
          }),
        });
      }
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        setError(body?.error || `Failed (${res.status})`);
        return;
      }
      const saved = (await res.json()) as TestCase;
      onSaved(saved);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="rounded-lg border border-amber-300/60 bg-amber-50/40 p-3 space-y-2">
      <div>
        <Label className="text-[10px] font-mono uppercase tracking-[0.18em] text-zinc-500">
          Name
        </Label>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="e.g., Negatives"
          className="h-7 text-[12.5px]"
        />
      </div>
      <div className="grid grid-cols-2 gap-2">
        <div>
          <Label className="text-[10px] font-mono uppercase tracking-[0.18em] text-zinc-500">
            stdin
          </Label>
          <textarea
            value={stdin}
            onChange={(e) => setStdin(e.target.value)}
            rows={3}
            className="w-full rounded-md border border-zinc-200 bg-white px-2 py-1 font-mono text-[12.5px]"
            placeholder="e.g., -1 -2"
          />
        </div>
        <div>
          <Label className="text-[10px] font-mono uppercase tracking-[0.18em] text-zinc-500">
            expected (optional)
          </Label>
          <textarea
            value={expected}
            onChange={(e) => setExpected(e.target.value)}
            rows={3}
            className="w-full rounded-md border border-zinc-200 bg-white px-2 py-1 font-mono text-[12.5px]"
            placeholder="Leave empty for no check"
          />
        </div>
      </div>
      <div className="flex items-center gap-2">
        <Button type="submit" size="sm" disabled={submitting}>
          {submitting ? "Saving…" : initial ? "Save" : "Add"}
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={onCancel} disabled={submitting}>
          Cancel
        </Button>
        {error && <span className="text-[11px] text-rose-700">{error}</span>}
      </div>
    </form>
  );
}

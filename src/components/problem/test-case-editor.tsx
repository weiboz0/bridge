"use client";

import { useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { TestCaseData } from "./teacher-problem-detail";

// Plan 066 phase 4 — inline canonical-test-case editor for the
// teacher detail page. Decisions §10 — every case sent here is
// `isCanonical: true` (problem-owned). Non-canonical cases are the
// student "My cases" surface, edited elsewhere.
//
// The editor diffs a local row list against the original server
// snapshot at save time and dispatches per-row mutations in
// parallel:
//   - server-side row marked deleted     → DELETE /api/test-cases/{id}
//   - new row (id=null, not deleted)     → POST /api/problems/{pid}/test-cases
//   - server-side row with edits         → PATCH /api/test-cases/{id}
// After all settle (success OR partial failure), the card calls
// router.refresh() and the page re-fetches the canonical case list.
// Local-state-only mutation is intentionally avoided (Risks §1):
// partial-failure recovery is much simpler when the source of truth
// remains the server.

type RowFields = {
  name: string;
  stdin: string;
  // The store treats null as "no expected output" and an empty string
  // as "clear to NULL" on PATCH. Local state keeps these unified as
  // strings; we serialize back to null on POST/PATCH when the value
  // is empty AND the original was also null/empty.
  expectedStdout: string;
  isExample: boolean;
  order: number;
};

type EditorRow = RowFields & {
  /** Server id; null for rows added in this editing session. */
  id: string | null;
  /** Snapshot of server values for diffing; null for new rows. */
  original: RowFields | null;
  /** Marked for deletion on save. Only meaningful when id != null. */
  deleted: boolean;
  /** Stable key for React (uuid for new rows; server id otherwise). */
  key: string;
};

type RowResult =
  | { kind: "delete-ok"; key: string }
  | { kind: "delete-fail"; key: string; msg: string }
  | { kind: "create-ok"; key: string; serverId: string }
  | { kind: "create-fail"; key: string; msg: string }
  | { kind: "update-ok"; key: string }
  | { kind: "update-fail"; key: string; msg: string };

interface Props {
  problemId: string;
  initial: TestCaseData[];
  onCancel: () => void;
  /** Called after the save batch settles successfully. The card uses
   *  this to flip back to read mode and trigger router.refresh(). */
  onSaved: () => void;
}

export function TestCaseEditor({ problemId, initial, onCancel, onSaved }: Props) {
  const [rows, setRows] = useState<EditorRow[]>(() =>
    initial.map((c) => seedRow(c)),
  );
  const [submitting, setSubmitting] = useState(false);
  const [errors, setErrors] = useState<string[]>([]);

  const visibleRows = useMemo(() => rows.filter((r) => !r.deleted), [rows]);

  function updateRow(key: string, patch: Partial<RowFields>) {
    setRows((prev) =>
      prev.map((r) => (r.key === key ? { ...r, ...patch } : r)),
    );
  }

  function markDeleted(key: string) {
    setRows((prev) =>
      prev
        .map((r) => {
          if (r.key !== key) return r;
          // New rows (id=null) just disappear; server rows stay
          // marked-deleted so the diff knows to issue DELETE.
          if (r.id === null) return null;
          return { ...r, deleted: true };
        })
        .filter((r): r is EditorRow => r !== null),
    );
  }

  function addRow(isExample: boolean) {
    const nextOrder = rows.length === 0
      ? 0
      : Math.max(...rows.map((r) => r.order)) + 1;
    setRows((prev) => [
      ...prev,
      {
        id: null,
        original: null,
        key: `new-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
        name: "",
        stdin: "",
        expectedStdout: "",
        isExample,
        order: nextOrder,
        deleted: false,
      },
    ]);
  }

  async function handleSave() {
    setErrors([]);

    const errs: string[] = [];
    for (const r of visibleRows) {
      if (r.stdin === "") {
        errs.push(`Row "${r.name || "(unnamed)"}": stdin is required`);
      }
    }
    if (errs.length > 0) {
      setErrors(errs);
      return;
    }

    const toDelete = rows.filter((r) => r.deleted && r.id !== null);
    const toCreate = rows.filter((r) => r.id === null && !r.deleted);
    const toUpdate = rows.filter(
      (r) => r.id !== null && !r.deleted && hasDiff(r),
    );

    if (toDelete.length === 0 && toCreate.length === 0 && toUpdate.length === 0) {
      // Nothing changed — exit edit mode without a network round-trip.
      onSaved();
      return;
    }

    setSubmitting(true);

    const tasks: Promise<RowResult>[] = [];

    for (const r of toDelete) {
      tasks.push(
        fetchTC("DELETE", `/api/test-cases/${r.id}`)
          .then(() => ({ kind: "delete-ok" as const, key: r.key }))
          .catch((e: Error) => ({
            kind: "delete-fail" as const,
            key: r.key,
            msg: `Delete failed (${rowLabel(r)}): ${e.message}`,
          })),
      );
    }

    for (const r of toCreate) {
      tasks.push(
        fetchTC<{ id: string }>("POST", `/api/problems/${problemId}/test-cases`, {
          name: r.name,
          stdin: r.stdin,
          expectedStdout: r.expectedStdout === "" ? null : r.expectedStdout,
          isExample: r.isExample,
          order: r.order,
          isCanonical: true,
        })
          .then((created) => ({
            kind: "create-ok" as const,
            key: r.key,
            serverId: created.id,
          }))
          .catch((e: Error) => ({
            kind: "create-fail" as const,
            key: r.key,
            msg: `Create failed (${rowLabel(r)}): ${e.message}`,
          })),
      );
    }

    for (const r of toUpdate) {
      tasks.push(
        fetchTC("PATCH", `/api/test-cases/${r.id}`, buildPatchBody(r))
          .then(() => ({ kind: "update-ok" as const, key: r.key }))
          .catch((e: Error) => ({
            kind: "update-fail" as const,
            key: r.key,
            msg: `Update failed (${rowLabel(r)}): ${e.message}`,
          })),
      );
    }

    const results = await Promise.all(tasks);
    setSubmitting(false);

    // Hydrate local state from results — Codex Q5: without this, a
    // partial-failure retry would re-POST already-created rows
    // (creating duplicates) and re-PATCH already-saved updates (a
    // wasted round-trip but not a duplication risk). Successful
    // creates get their server id; successful updates re-seed
    // `original` so the diff shrinks to zero on the next pass.
    setRows((prev) => applyResults(prev, results));

    const failed = results.filter(
      (r): r is Extract<RowResult, { msg: string }> =>
        r.kind === "delete-fail" ||
        r.kind === "create-fail" ||
        r.kind === "update-fail",
    );
    if (failed.length > 0) {
      setErrors(failed.map((f) => f.msg));
      return;
    }

    onSaved();
  }

  return (
    <div className="space-y-3">
      {visibleRows.length === 0 ? (
        <p className="text-sm text-muted-foreground italic">
          No cases yet. Add an example or hidden case below.
        </p>
      ) : (
        visibleRows.map((row) => (
          <EditorRowView
            key={row.key}
            row={row}
            onChange={(patch) => updateRow(row.key, patch)}
            onDelete={() => markDeleted(row.key)}
          />
        ))
      )}

      <div className="flex flex-wrap items-center gap-2 pt-1">
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => addRow(true)}
        >
          + Add example case
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => addRow(false)}
        >
          + Add hidden case
        </Button>
      </div>

      {errors.length > 0 && (
        <div
          role="alert"
          className="rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-800 space-y-1"
        >
          {errors.map((e, i) => (
            <div key={i}>{e}</div>
          ))}
        </div>
      )}

      <div className="flex items-center gap-2 pt-2 border-t border-zinc-200">
        <Button type="button" size="sm" onClick={handleSave} disabled={submitting}>
          {submitting ? "Saving…" : "Save changes"}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={onCancel}
          disabled={submitting}
        >
          Cancel
        </Button>
      </div>
    </div>
  );
}

function EditorRowView({
  row,
  onChange,
  onDelete,
}: {
  row: EditorRow;
  onChange: (patch: Partial<RowFields>) => void;
  onDelete: () => void;
}) {
  const tag = row.isExample ? "Example" : "Hidden";
  const tagClass = row.isExample
    ? "bg-emerald-50 text-emerald-700 border-emerald-200"
    : "bg-zinc-100 text-zinc-700 border-zinc-200";
  return (
    <div className="overflow-hidden rounded-lg border border-zinc-200">
      <div className="flex items-center justify-between gap-2 border-b border-zinc-200 bg-zinc-50/70 px-2.5 py-1">
        <div className="flex items-center gap-2">
          <select
            value={row.isExample ? "example" : "hidden"}
            onChange={(e) =>
              onChange({ isExample: e.target.value === "example" })
            }
            className={`rounded-md border px-2 py-0.5 text-[10px] uppercase tracking-[0.18em] outline-none ${tagClass}`}
            aria-label={`Visibility for ${row.name || "test case"}`}
          >
            <option value="example">Example</option>
            <option value="hidden">Hidden</option>
          </select>
          {row.id === null && (
            <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">
              new
            </span>
          )}
          {row.id !== null && hasDiff(row) && (
            <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-amber-600">
              edited
            </span>
          )}
        </div>
        <button
          type="button"
          onClick={onDelete}
          className="text-xs text-zinc-500 hover:text-rose-700"
          aria-label={`Remove ${tag.toLowerCase()} case`}
        >
          Remove
        </button>
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-2 divide-y sm:divide-y-0 sm:divide-x divide-zinc-200">
        <div className="p-2.5 space-y-1">
          <label className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">
            Name (optional)
          </label>
          <Input
            value={row.name}
            onChange={(e) => onChange({ name: e.target.value })}
            placeholder="e.g., Empty list"
            className="h-7 text-xs"
          />
        </div>
        <div className="p-2.5 space-y-1">
          <label className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">
            Order
          </label>
          <Input
            type="number"
            value={row.order}
            onChange={(e) =>
              onChange({ order: parseInt(e.target.value, 10) || 0 })
            }
            className="h-7 text-xs"
          />
        </div>
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-2 divide-y sm:divide-y-0 sm:divide-x divide-zinc-200 border-t border-zinc-200">
        <div className="p-2.5 space-y-1">
          <label className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">
            Input (stdin)
          </label>
          <textarea
            value={row.stdin}
            onChange={(e) => onChange({ stdin: e.target.value })}
            rows={3}
            className="block w-full rounded-md border border-input bg-transparent px-2 py-1 font-mono text-[12px] leading-relaxed outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 resize-y"
            placeholder="3&#10;1 2 3"
          />
        </div>
        <div className="p-2.5 space-y-1">
          <label className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">
            Expected output (blank = any)
          </label>
          <textarea
            value={row.expectedStdout}
            onChange={(e) => onChange({ expectedStdout: e.target.value })}
            rows={3}
            className="block w-full rounded-md border border-input bg-transparent px-2 py-1 font-mono text-[12px] leading-relaxed outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 resize-y"
            placeholder="6"
          />
        </div>
      </div>
    </div>
  );
}

// ---- helpers ----

function seedRow(c: TestCaseData): EditorRow {
  const fields: RowFields = {
    name: c.name,
    stdin: c.stdin,
    expectedStdout: c.expectedStdout ?? "",
    isExample: c.isExample,
    order: c.order,
  };
  return {
    id: c.id,
    original: fields,
    key: c.id,
    deleted: false,
    ...fields,
  };
}

function hasDiff(row: EditorRow): boolean {
  if (!row.original) return true;
  return (
    row.name !== row.original.name ||
    row.stdin !== row.original.stdin ||
    row.expectedStdout !== row.original.expectedStdout ||
    row.isExample !== row.original.isExample ||
    row.order !== row.original.order
  );
}

function buildPatchBody(row: EditorRow): Record<string, unknown> {
  // Only send changed fields. The backend's UpdateTestCaseInput uses
  // pointer fields where unset = unchanged. ExpectedStdout has a
  // special semantic: empty string clears to NULL — see the store
  // comment at platform/internal/store/test_cases.go:36-41.
  if (!row.original) return {};
  const body: Record<string, unknown> = {};
  if (row.name !== row.original.name) body.name = row.name;
  if (row.stdin !== row.original.stdin) body.stdin = row.stdin;
  if (row.expectedStdout !== row.original.expectedStdout) {
    body.expectedStdout = row.expectedStdout;
  }
  if (row.isExample !== row.original.isExample) body.isExample = row.isExample;
  if (row.order !== row.original.order) body.order = row.order;
  return body;
}

function rowLabel(row: EditorRow): string {
  if (row.name) return row.name;
  return row.isExample ? "example case" : "hidden case";
}

async function fetchTC<T = unknown>(
  method: string,
  url: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(url, {
    method,
    credentials: "include",
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as
      | { error?: string }
      | null;
    throw new Error(errBody?.error ?? `HTTP ${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json().catch(() => undefined)) as T;
}

function applyResults(rows: EditorRow[], results: RowResult[]): EditorRow[] {
  // Index successes by row key. Failures don't mutate state — the row
  // stays as-is so a retry will re-issue the same mutation.
  const byKey = new Map<string, RowResult>();
  for (const r of results) {
    if (r.kind === "delete-ok" || r.kind === "create-ok" || r.kind === "update-ok") {
      byKey.set(r.key, r);
    }
  }
  const out: EditorRow[] = [];
  for (const row of rows) {
    const result = byKey.get(row.key);
    if (!result) {
      out.push(row);
      continue;
    }
    if (result.kind === "delete-ok") {
      // Drop deleted rows from local state. A retry without this
      // would re-issue the DELETE and 404.
      continue;
    }
    if (result.kind === "create-ok") {
      // Promote synthetic-key row to server-keyed row. Re-seed
      // `original` so the next diff pass skips this row in toUpdate.
      const fields = extractFields(row);
      out.push({
        ...row,
        id: result.serverId,
        key: result.serverId,
        original: fields,
      });
      continue;
    }
    if (result.kind === "update-ok") {
      // Re-seed original so subsequent saves only PATCH new edits.
      out.push({ ...row, original: extractFields(row) });
      continue;
    }
  }
  return out;
}

function extractFields(row: EditorRow): RowFields {
  return {
    name: row.name,
    stdin: row.stdin,
    expectedStdout: row.expectedStdout,
    isExample: row.isExample,
    order: row.order,
  };
}

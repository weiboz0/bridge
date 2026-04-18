"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { Attempt } from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";

export type SaveState = "idle" | "pending" | "saving" | "error";

interface UseAutosaveAttemptOptions {
  problemId: string;
  initialAttempt: Attempt | null;
  /** Fallback code shown when no attempt exists yet. */
  starterCode: string;
  /** Language to use when creating a fresh attempt. */
  language: string;
  debounceMs?: number;
}

interface UseAutosaveAttemptReturn {
  /** Source of truth for the editor's contents. */
  code: string;
  /** Editor onChange binding. */
  setCode: (next: string) => void;
  /** Latest attempt row (null until the first keystroke creates one). */
  attempt: Attempt | null;
  /** Replace the active attempt (e.g. when the user picks a different one from the switcher). */
  setAttempt: (a: Attempt) => void;
  /** Save lifecycle state. `pending` after a keystroke before the debounce fires. */
  saveState: SaveState;
  /** Timestamp of the most recent successful save. */
  lastSavedAt: Date | null;
  /** Imperative flush — used by "New attempt" etc. to avoid races. */
  flush: () => Promise<void>;
}

/**
 * Debounced autosave for a Problem attempt.
 *
 * - If no attempt exists yet, the first keystroke POSTs a new row. Subsequent
 *   keystrokes PATCH it.
 * - A pending save is always flushed before the next operation (e.g. calling
 *   `setAttempt` or `flush`) so edits don't race.
 */
export function useAutosaveAttempt({
  problemId,
  initialAttempt,
  starterCode,
  language,
  debounceMs = 500,
}: UseAutosaveAttemptOptions): UseAutosaveAttemptReturn {
  const [attempt, setAttemptState] = useState<Attempt | null>(initialAttempt);
  const [code, setCodeState] = useState<string>(initialAttempt?.plainText ?? starterCode);
  const [saveState, setSaveState] = useState<SaveState>("idle");
  const [lastSavedAt, setLastSavedAt] = useState<Date | null>(
    initialAttempt ? new Date(initialAttempt.updatedAt) : null
  );

  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pendingRef = useRef<{ code: string } | null>(null);
  // Kept in refs so the save function sees the latest values without being
  // re-created on every render.
  const attemptRef = useRef(attempt);
  const problemIdRef = useRef(problemId);
  const languageRef = useRef(language);
  useEffect(() => {
    attemptRef.current = attempt;
  }, [attempt]);
  useEffect(() => {
    problemIdRef.current = problemId;
  }, [problemId]);
  useEffect(() => {
    languageRef.current = language;
  }, [language]);

  const performSave = useCallback(async () => {
    if (!pendingRef.current) return;
    const toSave = pendingRef.current;
    pendingRef.current = null;

    setSaveState("saving");
    try {
      const current = attemptRef.current;
      if (!current) {
        const res = await fetch(`/api/problems/${problemIdRef.current}/attempts`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ plainText: toSave.code, language: languageRef.current }),
        });
        if (!res.ok) throw new Error(`create failed: ${res.status}`);
        const created = (await res.json()) as Attempt;
        attemptRef.current = created;
        setAttemptState(created);
        setLastSavedAt(new Date(created.updatedAt));
      } else {
        const res = await fetch(`/api/attempts/${current.id}`, {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ plainText: toSave.code }),
        });
        if (!res.ok) throw new Error(`update failed: ${res.status}`);
        const updated = (await res.json()) as Attempt;
        attemptRef.current = updated;
        setAttemptState(updated);
        setLastSavedAt(new Date(updated.updatedAt));
      }
      setSaveState("idle");
    } catch (err) {
      console.error("[autosave] failed", err);
      setSaveState("error");
    }
  }, []);

  const setCode = useCallback(
    (next: string) => {
      setCodeState(next);

      // If the content matches the last saved version, don't enqueue a save.
      const baseline = attemptRef.current?.plainText ?? starterCode;
      if (next === baseline) {
        if (timerRef.current) {
          clearTimeout(timerRef.current);
          timerRef.current = null;
        }
        pendingRef.current = null;
        setSaveState("idle");
        return;
      }

      pendingRef.current = { code: next };
      setSaveState("pending");
      if (timerRef.current) clearTimeout(timerRef.current);
      timerRef.current = setTimeout(() => {
        timerRef.current = null;
        void performSave();
      }, debounceMs);
    },
    [debounceMs, performSave, starterCode]
  );

  const flush = useCallback(async () => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    await performSave();
  }, [performSave]);

  const setAttempt = useCallback(
    (a: Attempt) => {
      // Flush any in-flight edit before switching so it doesn't land on the
      // new attempt. Intentionally fire-and-forget — the switcher updates the
      // UI optimistically and the flushed save writes to the prior attempt ID.
      void flush();
      attemptRef.current = a;
      setAttemptState(a);
      setCodeState(a.plainText);
      setLastSavedAt(new Date(a.updatedAt));
      setSaveState("idle");
      pendingRef.current = null;
    },
    [flush]
  );

  // Clear the timer on unmount.
  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, []);

  return { code, setCode, attempt, setAttempt, saveState, lastSavedAt, flush };
}

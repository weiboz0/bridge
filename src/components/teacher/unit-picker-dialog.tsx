"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { searchUnits, type SearchResultItem, type SearchError } from "@/lib/unit-search";

/**
 * Plan 045 — UnitPickerDialog
 *
 * A search-and-pick modal for attaching a teaching_unit to a topic.
 * Hand-rolled (no shadcn Dialog dependency) to match the project's
 * existing modal pattern (see editor/tiptap/help-overlay.tsx).
 *
 * The dialog calls /api/units/search?linkableForCourse=<courseId>
 * which returns Units the picker's caller can actually attach,
 * decorated with linkedTopicId / linkedTopicTitle / canLink. Already-
 * linked rows render disabled with an "Already linked" badge.
 */

interface UnitPickerDialogProps {
  open: boolean;
  onClose: () => void;
  courseId: string;
  /**
   * Called when the user picks a Unit. The dialog itself does NOT
   * call the link-unit endpoint — the parent does, so it can refresh
   * its own state and handle errors in its own context.
   */
  onPicked: (unitId: string) => Promise<void> | void;
  /**
   * The topic this picker is attaching to. Used to suppress the
   * "Already linked" disabled state when the row is the topic's own
   * currently-linked Unit (you'd just be re-picking what's there —
   * the parent can decide to no-op or show a "Currently linked here"
   * badge).
   */
  currentTopicId: string;
}

const GRADE_LEVELS = ["", "K-5", "6-8", "9-12"] as const;
const MATERIAL_TYPES = ["", "notes", "slides", "worksheet", "reference"] as const;
const SEARCH_DEBOUNCE_MS = 250;
const PAGE_SIZE = 20;

export function UnitPickerDialog({
  open,
  onClose,
  courseId,
  onPicked,
  currentTopicId,
}: UnitPickerDialogProps) {
  const [query, setQuery] = useState("");
  const [grade, setGrade] = useState<string>("");
  const [materialType, setMaterialType] = useState<string>("");
  const [items, setItems] = useState<SearchResultItem[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<SearchError | null>(null);
  const [picking, setPicking] = useState<string | null>(null);
  const debounceRef = useRef<NodeJS.Timeout | null>(null);

  const runSearch = useCallback(
    async (cursor: string | null) => {
      const isLoadMore = cursor != null;
      if (isLoadMore) setLoadingMore(true);
      else setLoading(true);
      const result = await searchUnits({
        q: query || undefined,
        gradeLevel: grade || undefined,
        materialType: materialType || undefined,
        linkableForCourse: courseId,
        limit: PAGE_SIZE,
        cursor: cursor ?? undefined,
      });
      if (isLoadMore) {
        setItems((prev) => [...prev, ...result.items]);
        setLoadingMore(false);
      } else {
        setItems(result.items);
        setLoading(false);
      }
      setNextCursor(result.nextCursor);
      setError(result.error);
    },
    [query, grade, materialType, courseId]
  );

  // Re-run search when the dialog opens or filters change. Debounced.
  useEffect(() => {
    if (!open) return;
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      runSearch(null);
    }, SEARCH_DEBOUNCE_MS);
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [open, query, grade, materialType, runSearch]);

  // Reset state when the dialog closes so re-opening starts clean.
  useEffect(() => {
    if (!open) {
      setQuery("");
      setGrade("");
      setMaterialType("");
      setItems([]);
      setNextCursor(null);
      setError(null);
      setPicking(null);
    }
  }, [open]);

  // Escape closes.
  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  async function handlePick(unitId: string) {
    setPicking(unitId);
    try {
      await onPicked(unitId);
      onClose();
    } finally {
      setPicking(null);
    }
  }

  if (!open) return null;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="unit-picker-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
      onClick={(e) => {
        // Close on backdrop click only — never on inner-card click.
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="w-full max-w-2xl max-h-[80vh] flex flex-col rounded-lg border bg-background shadow-xl">
        <div className="flex items-center justify-between border-b px-4 py-3">
          <h2 id="unit-picker-title" className="text-lg font-semibold">
            Pick a Teaching Unit
          </h2>
          <Button variant="ghost" size="sm" onClick={onClose} aria-label="Close picker">
            ×
          </Button>
        </div>

        <div className="border-b px-4 py-3 space-y-2">
          <div>
            <Label htmlFor="picker-query" className="text-xs">
              Search
            </Label>
            <Input
              id="picker-query"
              autoFocus
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Title or summary…"
            />
          </div>
          <div className="flex gap-2">
            <div className="flex-1">
              <Label htmlFor="picker-grade" className="text-xs">
                Grade
              </Label>
              <select
                id="picker-grade"
                value={grade}
                onChange={(e) => setGrade(e.target.value)}
                className="w-full rounded border px-2 py-1 text-sm bg-background"
              >
                {GRADE_LEVELS.map((g) => (
                  <option key={g} value={g}>
                    {g === "" ? "Any grade" : g}
                  </option>
                ))}
              </select>
            </div>
            <div className="flex-1">
              <Label htmlFor="picker-material" className="text-xs">
                Material type
              </Label>
              <select
                id="picker-material"
                value={materialType}
                onChange={(e) => setMaterialType(e.target.value)}
                className="w-full rounded border px-2 py-1 text-sm bg-background"
              >
                {MATERIAL_TYPES.map((m) => (
                  <option key={m} value={m}>
                    {m === "" ? "Any type" : m}
                  </option>
                ))}
              </select>
            </div>
          </div>
        </div>

        <div className="flex-1 overflow-auto px-4 py-3">
          {loading && (
            <div className="space-y-2" data-testid="picker-loading">
              {[0, 1, 2].map((i) => (
                <div
                  key={i}
                  className="h-16 animate-pulse rounded border bg-muted/30"
                />
              ))}
            </div>
          )}
          {!loading && error && (
            <div
              role="alert"
              className="rounded border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive"
            >
              Couldn&apos;t load units (
              {error === "network" ? "network error" : "server error"}). Try again.
            </div>
          )}
          {!loading && !error && items.length === 0 && (
            <p className="py-6 text-center text-sm text-muted-foreground">
              No matching units. Try a broader search or create one in the{" "}
              <a href="/teacher/units" className="underline">
                Units library
              </a>
              .
            </p>
          )}
          {!loading && !error && items.length > 0 && (
            <ul className="space-y-2">
              {items.map((item) => {
                const linkedElsewhere =
                  item.linkedTopicId != null && item.linkedTopicId !== currentTopicId;
                const linkedHere =
                  item.linkedTopicId != null && item.linkedTopicId === currentTopicId;
                const disabled =
                  linkedElsewhere || item.canLink === false || picking === item.id;
                return (
                  <li
                    key={item.id}
                    className="flex items-start justify-between gap-3 rounded border bg-background p-3"
                  >
                    <div className="min-w-0 flex-1">
                      <p className="font-medium truncate">{item.title}</p>
                      <div className="mt-1 flex flex-wrap items-center gap-1.5 text-[10px] uppercase tracking-wider text-muted-foreground">
                        <span className="font-mono">{item.materialType}</span>
                        <span>·</span>
                        <span className="font-mono">{item.status}</span>
                        {item.gradeLevel && (
                          <>
                            <span>·</span>
                            <span className="font-mono">{item.gradeLevel}</span>
                          </>
                        )}
                        <span>·</span>
                        <span className="font-mono">{item.scope}</span>
                      </div>
                      {item.summary && (
                        <p className="mt-1 text-xs text-muted-foreground line-clamp-2">
                          {item.summary}
                        </p>
                      )}
                    </div>
                    <div className="shrink-0">
                      {linkedHere ? (
                        <span className="inline-flex items-center rounded-md border border-amber-300 bg-amber-50 px-2 py-1 text-xs text-amber-800">
                          Linked here
                        </span>
                      ) : linkedElsewhere ? (
                        <span
                          className="inline-flex items-center rounded-md border bg-muted px-2 py-1 text-xs text-muted-foreground"
                          title={
                            item.linkedTopicTitle
                              ? `Linked to: ${item.linkedTopicTitle}`
                              : "Linked to another focus area"
                          }
                        >
                          Already linked
                          {item.linkedTopicTitle && (
                            <span className="ml-1 font-medium">
                              · {item.linkedTopicTitle}
                            </span>
                          )}
                        </span>
                      ) : item.canLink === false ? (
                        <span
                          className="inline-flex items-center rounded-md border bg-muted px-2 py-1 text-xs text-muted-foreground"
                          title="You don't have permission to link this unit"
                        >
                          Cannot link
                        </span>
                      ) : (
                        <Button
                          size="sm"
                          disabled={disabled}
                          onClick={() => handlePick(item.id)}
                        >
                          {picking === item.id ? "Linking…" : "Pick"}
                        </Button>
                      )}
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
          {nextCursor && !loading && !error && (
            <div className="mt-3 flex justify-center">
              <Button
                variant="outline"
                size="sm"
                disabled={loadingMore}
                onClick={() => runSearch(nextCursor)}
              >
                {loadingMore ? "Loading…" : "Load more"}
              </Button>
            </div>
          )}
        </div>

        <div className="flex items-center justify-end border-t px-4 py-3">
          <Button variant="ghost" size="sm" onClick={onClose}>
            Cancel
          </Button>
        </div>
      </div>
    </div>
  );
}

"use client";

import { useEffect, useMemo, useRef, useState } from "react";

// Plan 084 — open the suggestion list on focus for small orgs (browser
// review 011 §P2 #7). AUTO_OPEN_THRESHOLD doubles as the display cap;
// the threshold and the cap are deliberately the same so we never show
// more than this many options at once. Larger orgs keep the existing
// "type to search" behavior to avoid blasting 50+ names on focus.
const AUTO_OPEN_THRESHOLD = 8;

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { OrgStudentRow } from "@/app/(portal)/org/parent-links/page";

// Plan 070 phase 2 — create-parent-link modal.
//
// Controlled-input form with:
//   - parent email (free text — backend resolves to user_id)
//   - child autocomplete from the org's eligible students
// On submit, POST /api/orgs/{orgId}/parent-links with
// `{ parentEmail, childUserId }`. 404/403/409 surface inline.

interface Props {
  orgId: string;
  students: OrgStudentRow[];
  studentsError: { status: number | null; message: string } | null;
  onClose: () => void;
  onCreated: () => void;
}

export function CreateParentLinkModal({
  orgId,
  students,
  studentsError,
  onClose,
  onCreated,
}: Props) {
  const [parentEmail, setParentEmail] = useState("");
  const [childQuery, setChildQuery] = useState("");
  const [childUserId, setChildUserId] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [highlightedIndex, setHighlightedIndex] = useState<number>(-1);
  const [isInputFocused, setIsInputFocused] = useState(false);
  const backdropRef = useRef<HTMLDivElement | null>(null);
  // Plan 084 + Kimi K2.6 code-review NIT: hold the blur timer so we can
  // clear it on unmount (and on re-focus before the timer fires) to
  // avoid the post-unmount state-update no-op. Functional behavior is
  // unchanged either way — React 19 swallows post-unmount setters — but
  // explicit cleanup is the textbook pattern.
  const blurTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    return () => {
      if (blurTimerRef.current) clearTimeout(blurTimerRef.current);
    };
  }, []);

  // Autocomplete: case-insensitive match on name OR email. Plan 084 — for
  // small orgs (<= AUTO_OPEN_THRESHOLD students), return the full list on
  // empty query so the picker isn't a hidden-affordance on focus.
  const suggestions = useMemo(() => {
    const q = childQuery.trim().toLowerCase();
    const cap = AUTO_OPEN_THRESHOLD;
    if (!q) {
      const total = students?.length ?? 0;
      if (total > 0 && total <= cap) return students.slice(0, cap);
      return [];
    }
    return students
      .filter(
        (s) =>
          s.name.toLowerCase().includes(q) ||
          s.email.toLowerCase().includes(q),
      )
      .slice(0, cap);
  }, [childQuery, students]);

  // Plan 084 — single source of truth for the listbox visibility. The
  // `<ul>` render gate AND both `aria-expanded` and `aria-controls` MUST
  // derive from this expression; if they drift, AT users see ARIA
  // claiming an expanded listbox that isn't in the DOM (WAI-ARIA
  // combobox spec violation — Codex + DeepSeek + GLM converged).
  const listboxVisible =
    (isInputFocused || childQuery.length > 0) &&
    suggestions.length > 0 &&
    !childUserId;

  // Close modal on Escape (the input's onKeyDown also handles Escape for
  // clearing the highlight, but window-level listener closes the dialog).
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  function selectChild(s: OrgStudentRow) {
    setChildUserId(s.userId);
    setChildQuery(`${s.name} <${s.email}>`);
  }

  function clearChild() {
    setChildUserId(null);
    setChildQuery("");
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    if (!parentEmail.trim()) {
      setError("Parent email is required");
      return;
    }
    if (!childUserId) {
      setError("Pick a child from the autocomplete suggestions");
      return;
    }

    setSubmitting(true);
    try {
      const res = await fetch(`/api/orgs/${orgId}/parent-links`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          parentEmail: parentEmail.trim(),
          childUserId,
        }),
      });
      if (!res.ok) {
        const body = (await res.json().catch(() => null)) as
          | { error?: string }
          | null;
        const msg = body?.error ?? `Request failed: ${res.status}`;
        if (res.status === 404) {
          setError(
            `${msg} — ask the parent to register at /register first, then try again.`,
          );
        } else {
          setError(msg);
        }
        return;
      }
      onCreated();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div
      ref={backdropRef}
      role="dialog"
      aria-modal="true"
      aria-label="Add parent link"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={(e) => {
        // Click on the backdrop itself (not bubbled from the inner
        // dialog) closes the modal. Codex post-impl phase 2 NIT-2:
        // the previous compare-against-inner-ref check inverted the
        // hit test, so backdrop clicks did nothing.
        if (e.target === backdropRef.current) onClose();
      }}
    >
      <div
        className="w-full max-w-md rounded-lg bg-background p-6 shadow-xl"
      >
        <h2 className="text-lg font-semibold mb-1">Add parent link</h2>
        <p className="text-sm text-muted-foreground mb-4">
          Wire a parent&apos;s account to a student. The parent will see the
          student&apos;s sessions and progress on next sign-in.
        </p>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="parentEmail">Parent email</Label>
            <Input
              id="parentEmail"
              type="email"
              value={parentEmail}
              onChange={(e) => setParentEmail(e.target.value)}
              placeholder="parent@example.com"
              required
              autoFocus
            />
            <p className="text-xs text-muted-foreground">
              The parent must have a registered account first.
            </p>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="childQuery">Child</Label>
            <div className="relative">
              <Input
                id="childQuery"
                role="combobox"
                // Plan 084 — `aria-expanded` and `aria-controls` MUST
                // derive from the same `listboxVisible` boolean as the
                // <ul> render below. Earlier code used `suggestions.length
                // > 0 && !childUserId` which fell out of sync the moment
                // we added open-on-focus (the listbox would close on blur
                // but ARIA would still claim expanded). Three external
                // reviewers (Codex, DeepSeek, GLM) converged on this drift
                // as a WAI-ARIA combobox spec violation.
                aria-expanded={listboxVisible}
                aria-controls={
                  listboxVisible ? "child-autocomplete-listbox" : undefined
                }
                aria-autocomplete="list"
                aria-activedescendant={
                  highlightedIndex >= 0 && suggestions[highlightedIndex]
                    ? `option-${suggestions[highlightedIndex].userId}`
                    : undefined
                }
                value={childQuery}
                onFocus={() => {
                  // Cancel any pending blur-close so a re-focus while the
                  // listbox is still visible doesn't subsequently collapse it.
                  if (blurTimerRef.current) {
                    clearTimeout(blurTimerRef.current);
                    blurTimerRef.current = null;
                  }
                  setIsInputFocused(true);
                }}
                // Plan 084 — 150ms blur delay so the listbox stays open
                // long enough for screen-reader virtual cursors to dispatch
                // their `click` event (those don't emit a preceding
                // `mousedown`, so the `<li onMouseDown={preventDefault}>`
                // path doesn't cover them). Mouse users are already
                // handled by the mousedown preventDefault. DO NOT optimize
                // this timeout away — it's the AT path.
                //
                // Also reset highlightedIndex on the delayed close (Codex
                // code-review BLOCKER): otherwise the derived
                // aria-activedescendant below would reference an option
                // that's no longer rendered, which is a WAI-ARIA error AT
                // users hear as "focused item that doesn't exist".
                onBlur={() => {
                  blurTimerRef.current = setTimeout(() => {
                    setIsInputFocused(false);
                    setHighlightedIndex(-1);
                    blurTimerRef.current = null;
                  }, 150);
                }}
                onChange={(e) => {
                  setChildQuery(e.target.value);
                  // GLM 5.1 post-impl NIT-2: reset the highlight inline
                  // here rather than via a useEffect on `childQuery`. The
                  // effect approach caused two renders per keystroke
                  // (state set → render → effect → state set → render);
                  // batching the two setters into one onChange is one
                  // render and avoids the intermediate stale-index frame.
                  setHighlightedIndex(-1);
                  if (childUserId) setChildUserId(null);
                }}
                onKeyDown={(e) => {
                  const count = suggestions.length;
                  if (!childUserId && count > 0) {
                    if (e.key === "ArrowDown") {
                      e.preventDefault();
                      setHighlightedIndex((i) => (i + 1) % count);
                      return;
                    }
                    if (e.key === "ArrowUp") {
                      e.preventDefault();
                      setHighlightedIndex((i) => (i <= 0 ? count - 1 : i - 1));
                      return;
                    }
                    if (e.key === "Enter" && highlightedIndex >= 0) {
                      // Codex post-impl NIT-Q5: a fast-type race can fire
                      // Enter before the highlightedIndex-reset useEffect
                      // commits, leaving the index stale relative to the
                      // refreshed suggestions list. Guard the lookup so we
                      // never pass `undefined` to selectChild.
                      const target = suggestions[highlightedIndex];
                      if (!target) return;
                      e.preventDefault();
                      selectChild(target);
                      return;
                    }
                  }
                  if (e.key === "Escape") {
                    // Clear the highlight so the list visually closes; the
                    // window-level Escape handler will dismiss the whole dialog.
                    setHighlightedIndex(-1);
                  }
                }}
                placeholder="Search by name or email…"
                autoComplete="off"
              />
              {childUserId && (
                <button
                  type="button"
                  onClick={clearChild}
                  className="absolute right-2 top-1/2 -translate-y-1/2 text-xs text-muted-foreground hover:text-foreground"
                  aria-label="Clear child selection"
                >
                  ×
                </button>
              )}
              {listboxVisible && (
                <ul
                  id="child-autocomplete-listbox"
                  role="listbox"
                  aria-label="Child suggestions"
                  className="absolute z-10 mt-1 w-full max-h-48 overflow-auto rounded-md border border-input bg-background shadow-lg"
                >
                  {suggestions.map((s, idx) => (
                    <li
                      key={s.userId}
                      id={`option-${s.userId}`}
                      role="option"
                      aria-selected={highlightedIndex === idx}
                      onMouseDown={(e) => {
                        // Prevent the input from losing focus before the click
                        // registers so that selectChild fires correctly. This
                        // covers the mouse path (and most touch).
                        e.preventDefault();
                        selectChild(s);
                      }}
                      onClick={() => {
                        // Codex post-impl NIT-Q2: screen-reader virtual
                        // cursors typically dispatch `click` directly without
                        // a preceding `mousedown`. Without this fallback,
                        // AT-driven activation would silently no-op. The
                        // mousedown path already calls selectChild, but
                        // selectChild is idempotent so a follow-up click
                        // does no harm.
                        selectChild(s);
                      }}
                      className={`cursor-pointer px-3 py-2 text-sm ${
                        highlightedIndex === idx ? "bg-muted" : "hover:bg-muted"
                      }`}
                    >
                      <div className="font-medium">{s.name}</div>
                      <div className="text-xs text-muted-foreground font-mono">
                        {s.email}
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </div>
            {studentsError ? (
              <p className="text-xs text-rose-700">
                Couldn&apos;t load the student roster ({studentsError.message}).
                Refresh to retry.
              </p>
            ) : students.length === 0 ? (
              <p className="text-xs text-rose-700">
                No students enrolled in any active class yet — add students to
                a class first.
              </p>
            ) : null}
          </div>

          {error && (
            <div
              role="alert"
              className="rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-800"
            >
              {error}
            </div>
          )}

          <div className="flex justify-end gap-2 pt-2">
            <Button
              type="button"
              variant="outline"
              onClick={onClose}
              disabled={submitting}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              // Plan 084 — visible "form-not-ready" signal. The runtime
              // check in handleSubmit stays as defense-in-depth (covers
              // paste-and-submit / programmatic edge cases). This change
              // shifts the UX from "click → error toast" to "can't click
              // yet → fix the fields → can click" — standard form pattern.
              disabled={submitting || !parentEmail.trim() || !childUserId}
            >
              {submitting ? "Linking…" : "Create link"}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}

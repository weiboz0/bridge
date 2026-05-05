"use client";

import { useEffect, useRef, useState } from "react";
import type { TeacherParentLinkRow } from "@/app/(portal)/teacher/classes/[id]/page";

// Plan 070 phase 3 — class-detail student list with a "P" badge per
// student that opens a parent-links popover. Read-only — teachers
// see who is linked but write actions live on the org-admin page
// (`/org/parent-links`).

export interface StudentRow {
  id: string; // class_membership row id
  userId: string;
  name: string;
  email: string;
  parents: TeacherParentLinkRow[];
}

interface Props {
  students: StudentRow[];
}

export function ClassStudentsList({ students }: Props) {
  const [openId, setOpenId] = useState<string | null>(null);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const popoverRef = useRef<HTMLDivElement | null>(null);
  const closeButtonRef = useRef<HTMLButtonElement | null>(null);

  // Track the previously-open userId so focus can be restored on close.
  const prevOpenIdRef = useRef<string | null>(null);
  useEffect(() => {
    if (openId !== null) {
      prevOpenIdRef.current = openId;
    }
  }, [openId]);

  // On open: move focus to the close button (deferred one tick so the popover
  // has rendered).
  // On close: restore focus to the badge button that triggered the open
  // (identified via aria-controls="parents-popover-{userId}").
  useEffect(() => {
    if (openId) {
      const timerId = setTimeout(() => {
        closeButtonRef.current?.focus();
      }, 0);
      return () => clearTimeout(timerId);
    } else if (prevOpenIdRef.current !== null) {
      const opener = document.querySelector<HTMLElement>(
        `[aria-controls="parents-popover-${prevOpenIdRef.current}"]`,
      );
      opener?.focus();
    }
  }, [openId]);

  // Close the popover on Escape or when clicking outside the row group.
  // Also handle Tab / Shift-Tab focus-trap inside the popover.
  // v1: the popover has only one focusable element (the close button), so
  // the trap is a no-op in practice — Tab stays on the close button.
  // The structure is in place for richer popovers in future iterations.
  useEffect(() => {
    if (!openId) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        setOpenId(null);
        return;
      }
      // Focus trap: cycle focusable elements within the popover.
      if (e.key === "Tab" && popoverRef.current) {
        const focusable = Array.from(
          popoverRef.current.querySelectorAll<HTMLElement>(
            'a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])',
          ),
        );
        if (focusable.length === 0) return;
        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        if (e.shiftKey) {
          if (document.activeElement === first) {
            e.preventDefault();
            last.focus();
          }
        } else {
          if (document.activeElement === last) {
            e.preventDefault();
            first.focus();
          }
        }
      }
    }
    function onClick(e: MouseEvent) {
      if (
        containerRef.current &&
        !containerRef.current.contains(e.target as Node)
      ) {
        setOpenId(null);
      }
    }
    window.addEventListener("keydown", onKey);
    window.addEventListener("mousedown", onClick);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("mousedown", onClick);
    };
  }, [openId]);

  return (
    <div ref={containerRef} className="space-y-2">
      {students.map((s) => {
        const parentCount = s.parents.length;
        const isOpen = openId === s.userId;
        return (
          <div
            key={s.id}
            className="relative flex items-center justify-between gap-3 py-2 border-b last:border-0"
          >
            <span className="text-sm font-medium">{s.name}</span>
            <div className="flex items-center gap-3">
              <span className="text-xs text-muted-foreground">{s.email}</span>
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  setOpenId(isOpen ? null : s.userId);
                }}
                title={
                  parentCount === 0
                    ? "No parents linked yet"
                    : `${parentCount} parent${parentCount === 1 ? "" : "s"} linked`
                }
                aria-label={
                  parentCount === 0
                    ? `${s.name} has no linked parents — click to open popover`
                    : `${s.name} has ${parentCount} linked parent${parentCount === 1 ? "" : "s"} — click to view`
                }
                aria-expanded={isOpen}
                aria-controls={`parents-popover-${s.userId}`}
                className={`inline-flex h-6 min-w-6 items-center justify-center rounded-full border px-1.5 text-[10px] font-mono uppercase tracking-wider transition-colors ${
                  parentCount > 0
                    ? "border-emerald-300 bg-emerald-50 text-emerald-800 hover:border-emerald-400"
                    : "border-zinc-200 bg-zinc-50 text-zinc-500 hover:border-zinc-300"
                }`}
              >
                P{parentCount > 0 ? `·${parentCount}` : ""}
              </button>
            </div>

            {isOpen && (
              <div
                ref={popoverRef}
                id={`parents-popover-${s.userId}`}
                role="dialog"
                aria-label={`Parents linked to ${s.name}`}
                className="absolute right-0 top-full z-20 mt-1 w-72 rounded-md border border-zinc-200 bg-background p-3 shadow-lg"
                onMouseDown={(e) => e.stopPropagation()}
              >
                <div className="mb-2 flex items-center justify-between">
                  <p className="text-xs font-mono uppercase tracking-wider text-zinc-500">
                    Parents · {s.name}
                  </p>
                  <button
                    ref={closeButtonRef}
                    type="button"
                    onClick={() => setOpenId(null)}
                    className="text-xs text-zinc-500 hover:text-zinc-800"
                    aria-label="Close"
                  >
                    ×
                  </button>
                </div>
                {s.parents.length === 0 ? (
                  <p className="text-xs text-muted-foreground italic">
                    No parents linked yet. An org admin can add one at
                    <code className="mx-1">/org/parent-links</code>.
                  </p>
                ) : (
                  <ul className="space-y-2">
                    {s.parents.map((p) => (
                      <li key={p.linkId} className="text-sm">
                        <div className="font-medium">{p.parentName}</div>
                        <div className="text-xs text-muted-foreground font-mono">
                          {p.parentEmail}
                        </div>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

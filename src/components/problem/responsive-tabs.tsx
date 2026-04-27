"use client";

import { useRef } from "react";

export type NarrowTabId = "problem" | "code" | "io";

const TABS: ReadonlyArray<{ id: NarrowTabId; label: string }> = [
  { id: "problem", label: "Problem" },
  { id: "code", label: "Code" },
  { id: "io", label: "I/O" },
];

export const NARROW_TAB_IDS: ReadonlyArray<NarrowTabId> = TABS.map((t) => t.id);

interface ResponsiveTabsProps {
  active: NarrowTabId;
  onChange: (id: NarrowTabId) => void;
}

/**
 * Tab bar for the narrow-viewport problem-editor layout (plan 042).
 * Implements the standard ARIA tabs pattern:
 *   - role="tablist" with aria-label
 *   - role="tab" with aria-selected, aria-controls, and roving tabIndex
 *     so the whole bar is one Tab stop and ArrowLeft/ArrowRight move
 *     within it (per WAI-ARIA Authoring Practices).
 *
 * Visible only at narrow widths via the parent's `lg:hidden`. Pane
 * visibility itself is owned by the parent (Tailwind `lg:flex` rules).
 *
 * Extracted as its own component so the Vitest contract test and the
 * real shell exercise the exact same markup — Codex post-impl review
 * #7 flagged that an in-test reimplementation would let the real
 * component drift silently.
 */
export function ResponsiveTabs({ active, onChange }: ResponsiveTabsProps) {
  const buttonRefs = useRef<(HTMLButtonElement | null)[]>([]);

  const focusByIndex = (i: number) => {
    const next = (i + TABS.length) % TABS.length;
    buttonRefs.current[next]?.focus();
    onChange(TABS[next].id);
  };

  const handleKeyDown = (e: React.KeyboardEvent, currentIndex: number) => {
    switch (e.key) {
      case "ArrowRight":
      case "Right":
        e.preventDefault();
        focusByIndex(currentIndex + 1);
        break;
      case "ArrowLeft":
      case "Left":
        e.preventDefault();
        focusByIndex(currentIndex - 1);
        break;
      case "Home":
        e.preventDefault();
        focusByIndex(0);
        break;
      case "End":
        e.preventDefault();
        focusByIndex(TABS.length - 1);
        break;
    }
  };

  return (
    <div
      role="tablist"
      aria-label="Problem editor sections"
      className="flex border-b border-zinc-200 bg-white lg:hidden"
    >
      {TABS.map((tab, i) => {
        const isActive = active === tab.id;
        return (
          <button
            key={tab.id}
            ref={(el) => {
              buttonRefs.current[i] = el;
            }}
            role="tab"
            id={`problem-tab-${tab.id}`}
            aria-selected={isActive}
            aria-controls={`problem-pane-${tab.id}`}
            tabIndex={isActive ? 0 : -1}
            onClick={() => onChange(tab.id)}
            onKeyDown={(e) => handleKeyDown(e, i)}
            className={
              "flex-1 px-4 py-3 text-sm font-medium transition-colors " +
              (isActive
                ? "border-b-2 border-amber-600 text-zinc-900"
                : "text-zinc-500 hover:text-zinc-800")
            }
          >
            {tab.label}
          </button>
        );
      })}
    </div>
  );
}

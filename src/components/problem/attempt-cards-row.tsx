"use client";

import type { Attempt } from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";
import { StatusDot, Tag } from "@/components/design/primitives";

interface Props {
  attempts: Attempt[];
  activeId: string | null;
  onSelect: (a: Attempt) => void;
}

/**
 * Top strip on the teacher watch page — top-3 most recent attempts as cards.
 * The active attempt is the one the teacher is currently viewing in the
 * editor below; an "Active" tag marks it.
 */
export function AttemptCardsRow({ attempts, activeId, onSelect }: Props) {
  const top = attempts.slice(0, 3);
  if (top.length === 0) {
    return (
      <div className="border-t border-zinc-200 bg-zinc-50/60 px-4 py-3 text-[13px] text-zinc-500">
        Not started yet — no attempts.
      </div>
    );
  }
  return (
    <div className="grid grid-cols-3 gap-0 border-t border-zinc-200 bg-zinc-50/40">
      {top.map((a, i) => (
        <button
          key={a.id}
          onClick={() => onSelect(a)}
          className={
            "group relative flex flex-col items-stretch gap-1.5 border-r border-zinc-200 px-3 py-2 text-left transition-colors " +
            (a.id === activeId
              ? "bg-white shadow-[inset_0_2px_0_rgba(217,119,6,0.9)]"
              : "hover:bg-white")
          }
        >
          <div className="flex items-center gap-2">
            <span className="font-mono text-[10px] tabular-nums text-zinc-400">
              #{attempts.length - i}
            </span>
            <span className="min-w-0 truncate text-[13px] font-medium tracking-tight text-zinc-900">
              {a.title}
            </span>
            {a.id === activeId ? (
              <Tag tone="amber" className="ml-auto">Active</Tag>
            ) : (
              <span className="ml-auto inline-flex items-center gap-1 font-mono text-[10px] uppercase tracking-[0.14em] text-zinc-400">
                <StatusDot tone="idle" />
              </span>
            )}
          </div>
          <pre className="line-clamp-3 whitespace-pre-wrap font-mono text-[11px] leading-[1.5] text-zinc-500 group-hover:text-zinc-700">
            {a.plainText.split("\n").slice(0, 3).join("\n") || "(empty)"}
          </pre>
          <div className="flex items-center justify-between font-mono text-[10px] uppercase tracking-[0.16em] text-zinc-400">
            <span>{relativeTime(a.updatedAt)}</span>
            {a.id !== activeId ? (
              <span className="opacity-0 group-hover:opacity-100 normal-case tracking-normal text-amber-700/80">
                view →
              </span>
            ) : null}
          </div>
        </button>
      ))}
    </div>
  );
}

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const s = Math.max(0, Math.floor(diff / 1000));
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
}

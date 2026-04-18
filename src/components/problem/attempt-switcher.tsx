"use client";

import type { Attempt } from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

interface Props {
  attempts: Attempt[];
  activeId: string | null;
  onSwitch: (a: Attempt) => void;
}

export function AttemptSwitcher({ attempts, activeId, onSwitch }: Props) {
  if (attempts.length === 0) return null;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="inline-flex h-7 items-center gap-1 rounded-md border border-zinc-200 bg-white px-2 text-[11px] text-zinc-600 hover:border-zinc-300 hover:text-zinc-900">
        <span className="font-mono tracking-wide">switch</span>
        <svg viewBox="0 0 8 8" className="size-2 text-zinc-500">
          <path fill="currentColor" d="M1 3l3 3 3-3z" />
        </svg>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="min-w-[220px]">
        {attempts.map((a, i) => (
          <DropdownMenuItem
            key={a.id}
            onClick={() => onSwitch(a)}
            className={
              a.id === activeId
                ? "bg-amber-50 text-amber-900 focus:bg-amber-100"
                : ""
            }
          >
            <div className="flex w-full items-center gap-2">
              <span className="font-mono text-[10px] tabular-nums text-zinc-400">
                #{attempts.length - i}
              </span>
              <span className="truncate">{a.title}</span>
              <span className="ml-auto font-mono text-[10px] uppercase tracking-[0.16em] text-zinc-400">
                {relativeTime(a.updatedAt)}
              </span>
            </div>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const s = Math.max(0, Math.floor(diff / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h`;
  const d = Math.floor(h / 24);
  return `${d}d`;
}

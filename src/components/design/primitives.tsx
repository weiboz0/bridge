import { cn } from "@/lib/utils";

export function Tag({
  tone = "neutral",
  children,
  className,
}: {
  tone?: "neutral" | "amber" | "emerald" | "rose" | "zinc";
  children: React.ReactNode;
  className?: string;
}) {
  const tones = {
    neutral: "border-zinc-300 bg-white text-zinc-600",
    amber: "border-amber-300/70 bg-amber-50 text-amber-800",
    emerald: "border-emerald-300/70 bg-emerald-50 text-emerald-800",
    rose: "border-rose-300/70 bg-rose-50 text-rose-800",
    zinc: "border-zinc-200 bg-zinc-100 text-zinc-600",
  } as const;
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-md border px-1.5 py-[1px] font-mono text-[10px] uppercase tracking-[0.14em]",
        tones[tone],
        className
      )}
    >
      {children}
    </span>
  );
}

export function StatusDot({ tone }: { tone: "idle" | "pass" | "fail" | "running" }) {
  const tones = {
    idle: "bg-zinc-300",
    pass: "bg-emerald-500",
    fail: "bg-rose-500",
    running: "bg-amber-500 animate-pulse",
  } as const;
  return <span className={cn("inline-block size-1.5 rounded-full", tones[tone])} />;
}

export function Kbd({ children }: { children: React.ReactNode }) {
  return (
    <kbd className="rounded border border-zinc-200 bg-zinc-50 px-1 py-px font-mono text-[10px] text-zinc-600">
      {children}
    </kbd>
  );
}

// Hand-rolled syntax highlight for the Monaco placeholder. Only good enough
// to set the visual tone; real editor uses the real Monaco instance.
export function PyCode({ lines }: { lines: Array<Array<[string, string]>> }) {
  const tones: Record<string, string> = {
    kw: "text-fuchsia-600",
    def: "text-amber-700",
    fn: "text-blue-700",
    str: "text-emerald-700",
    num: "text-violet-600",
    op: "text-zinc-500",
    cm: "text-zinc-400 italic",
    id: "text-zinc-800",
    pl: "text-zinc-800",
  };
  return (
    <pre className="font-mono text-[13px] leading-[1.55]">
      {lines.map((line, i) => (
        <div key={i} className="group flex">
          <span className="w-10 shrink-0 select-none pr-3 text-right tabular-nums text-zinc-300">
            {i + 1}
          </span>
          <span className="flex-1 whitespace-pre">
            {line.map(([tone, text], j) => (
              <span key={j} className={tones[tone] ?? tones.pl}>
                {text}
              </span>
            ))}
          </span>
        </div>
      ))}
    </pre>
  );
}

export function SectionLabel({
  children,
  action,
}: {
  children: React.ReactNode;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex h-8 items-center justify-between border-b border-zinc-200/80 px-3">
      <span className="font-mono text-[10px] uppercase tracking-[0.22em] text-zinc-500">
        {children}
      </span>
      {action ? <div className="flex items-center gap-1.5">{action}</div> : null}
    </div>
  );
}

export function BlinkCursor() {
  return (
    <span
      aria-hidden
      className="ml-[1px] inline-block h-[13px] w-[7px] translate-y-[2px] bg-zinc-800"
      style={{ animation: "bridge-blink 1s steps(2, start) infinite" }}
    />
  );
}

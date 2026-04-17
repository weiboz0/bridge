import {
  PyCode,
  SectionLabel,
  StatusDot,
  Tag,
} from "@/components/design/primitives";

export default function ProblemTeacherDesign() {
  return (
    <div className="flex h-[calc(100vh-44px)] overflow-hidden">
      {/* LEFT — brief problem */}
      <aside className="flex w-[26%] min-w-[300px] flex-col border-r border-zinc-200 bg-white">
        <SectionLabel action={<Tag tone="zinc">Topic · Arrays</Tag>}>Problem</SectionLabel>
        <div className="flex-1 overflow-auto px-5 py-5">
          <p className="font-mono text-[11px] uppercase tracking-[0.22em] text-amber-700/80">
            Problem 03 · Easy
          </p>
          <h1 className="mt-2 text-[20px] font-semibold leading-tight tracking-tight">
            Two Sum
          </h1>
          <p className="mt-3 text-[13.5px] leading-[1.65] text-zinc-600">
            Given a list of integers and a target number, return the indices of the
            two numbers that add up to the target. Exactly one solution exists per
            input; elements are not reused.
          </p>
          <div className="mt-5 space-y-2 text-[12.5px]">
            <StatRow label="Example cases" value="2" />
            <StatRow label="Hidden cases" value="2" />
            <StatRow label="Language" value="Python" />
            <StatRow label="Class median" value="14m" muted="time-to-pass" />
          </div>
          <div className="mt-6 rounded-lg border border-zinc-200 bg-zinc-50/60 p-3">
            <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-500">
              Watching
            </p>
            <div className="mt-1.5 flex items-center gap-2">
              <span className="inline-flex size-7 items-center justify-center rounded-full bg-zinc-900 font-mono text-[11px] font-medium text-white">
                AD
              </span>
              <div className="min-w-0">
                <p className="truncate text-[14px] font-medium tracking-tight">
                  Ada Lovelace
                </p>
                <p className="truncate font-mono text-[11px] text-zinc-500">
                  alice@demo.edu
                </p>
              </div>
              <span className="ml-auto inline-flex items-center gap-1 rounded-md border border-emerald-300/70 bg-emerald-50 px-1.5 py-[2px] font-mono text-[10px] uppercase tracking-[0.14em] text-emerald-800">
                <StatusDot tone="pass" /> live
              </span>
            </div>
          </div>
        </div>
      </aside>

      {/* CENTER — attempt history + editor */}
      <section className="flex min-w-0 flex-1 flex-col bg-white">
        {/* Attempt list */}
        <div className="border-b border-zinc-200">
          <div className="flex h-9 items-center justify-between px-4">
            <span className="font-mono text-[10px] uppercase tracking-[0.22em] text-zinc-500">
              Attempts · 3
            </span>
            <span className="font-mono text-[10px] uppercase tracking-[0.16em] text-zinc-400">
              sorted by recent
            </span>
          </div>
          <div className="grid grid-cols-3 gap-0 border-t border-zinc-200 bg-zinc-50/40">
            <AttemptCard
              active
              n={3}
              title="My first try"
              time="edited 2s ago"
              status="running"
              preview={[
                "def solve(nums, target):",
                "    seen = {}",
                "    for i, n in enumerate(nums):",
              ]}
            />
            <AttemptCard
              n={2}
              title="Hashmap idea"
              time="14m ago"
              status="pass"
              preview={[
                "def solve(nums, target):",
                "    d = dict()",
                "    for i in range(len(nums)):",
              ]}
            />
            <AttemptCard
              n={1}
              title="O(n²) brute"
              time="1h ago"
              status="fail"
              preview={[
                "def solve(nums, target):",
                "    for i in range(len(nums)):",
                "        for j in range(i+1, len(nums)):",
              ]}
            />
          </div>
        </div>

        {/* Active attempt header */}
        <div className="flex h-10 items-center gap-2 border-b border-zinc-200 px-4">
          <StatusDot tone="running" />
          <span className="text-[13px] font-medium tracking-tight">
            Attempt #3 · My first try
          </span>
          <Tag tone="amber" className="ml-1">
            Active
          </Tag>
          <span className="ml-2 font-mono text-[11px] text-zinc-500">
            updated 2s ago
          </span>
          <span className="ml-auto inline-flex items-center gap-2 font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-500">
            <span className="inline-flex size-1.5 rounded-full bg-zinc-400" />
            read-only
          </span>
        </div>

        {/* Editor placeholder */}
        <div className="flex-1 overflow-auto bg-white py-3">
          <PyCode
            lines={[
              [["cm", "# Read input: first line is the array, second is the target"]],
              [["kw", "def "], ["def", "solve"], ["op", "("], ["pl", "nums"], ["op", ", "], ["pl", "target"], ["op", "):"]],
              [["op", "    "], ["pl", "seen"], ["op", " = "], ["op", "{}"]],
              [["op", "    "], ["kw", "for "], ["pl", "i"], ["op", ", "], ["pl", "n"], ["kw", " in "], ["fn", "enumerate"], ["op", "("], ["pl", "nums"], ["op", "):"]],
              [["op", "        "], ["pl", "need"], ["op", " = "], ["pl", "target"], ["op", " - "], ["pl", "n"]],
              [["op", "        "], ["kw", "if "], ["pl", "need"], ["kw", " in "], ["pl", "seen"], ["op", ":"]],
              [["op", "            "], ["kw", "return "], ["pl", "seen"], ["op", "["], ["pl", "need"], ["op", "], "], ["pl", "i"]],
              [["op", "        "], ["pl", "seen"], ["op", "["], ["pl", "n"], ["op", "] = "], ["pl", "i"]],
              [["pl", ""]],
              [["pl", "nums"], ["op", " = "], ["fn", "list"], ["op", "("], ["fn", "map"], ["op", "("], ["fn", "int"], ["op", ", "], ["fn", "input"], ["op", "()."], ["fn", "split"], ["op", "()))"]],
              [["pl", "target"], ["op", " = "], ["fn", "int"], ["op", "("], ["fn", "input"], ["op", "())"]],
              [["pl", "a"], ["op", ", "], ["pl", "b"], ["op", " = "], ["fn", "solve"], ["op", "("], ["pl", "nums"], ["op", ", "], ["pl", "target"], ["op", ")"]],
              [["fn", "print"], ["op", "("], ["pl", "a"], ["op", ", "], ["pl", "b"], ["op", ")"]],
            ]}
          />
        </div>
      </section>

      {/* RIGHT — compact terminal */}
      <aside className="flex w-[26%] min-w-[280px] flex-col border-l border-zinc-200 bg-white">
        <SectionLabel
          action={
            <span className="inline-flex items-center gap-1.5 font-mono text-[10px] uppercase tracking-[0.16em] text-zinc-500">
              <StatusDot tone="running" /> last run · 24s ago
            </span>
          }
        >
          Terminal
        </SectionLabel>
        <div className="flex-1 overflow-auto bg-zinc-50/50">
          <div className="px-4 py-3 font-mono text-[12.5px] leading-[1.65] text-zinc-900">
            <div className="text-zinc-400">
              <span className="text-emerald-700">$</span> python solution.py
            </div>
            <div className="mt-1 whitespace-pre-wrap">4 2 7 11 15</div>
            <div className="whitespace-pre-wrap">9</div>
            <div className="mt-1 whitespace-pre-wrap text-zinc-800">0 1</div>
            <div className="mt-2 text-emerald-700 font-medium">
              ✓ Example 1 passed
            </div>
            <div className="text-zinc-500">
              exited with code 0 · 18ms
            </div>
          </div>
          <div className="mx-3 my-2 rounded-lg border border-zinc-200 bg-white">
            <div className="flex h-7 items-center justify-between border-b border-zinc-200 px-2.5">
              <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-500">
                Test run · 14m ago
              </span>
              <span className="inline-flex items-center gap-1 font-mono text-[10px] text-emerald-700">
                2/2 examples
              </span>
            </div>
            <div className="space-y-0.5 px-3 py-2 font-mono text-[12px]">
              <div className="flex items-center justify-between">
                <span className="text-zinc-600">Example 1</span>
                <span className="text-emerald-700">pass · 12ms</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-zinc-600">Example 2</span>
                <span className="text-emerald-700">pass · 9ms</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-zinc-600">Hidden #1</span>
                <span className="text-emerald-700">pass · 11ms</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-zinc-600">Hidden #2</span>
                <span className="text-rose-700">fail · wrong output</span>
              </div>
            </div>
          </div>
        </div>
      </aside>
    </div>
  );
}

/* ---------- internal ---------- */

function StatRow({
  label,
  value,
  muted,
}: {
  label: string;
  value: string;
  muted?: string;
}) {
  return (
    <div className="flex items-baseline justify-between gap-2 border-b border-dashed border-zinc-200 pb-1.5">
      <span className="text-zinc-500">{label}</span>
      <span className="flex items-baseline gap-2">
        <span className="font-mono text-[12.5px] tabular-nums text-zinc-800">
          {value}
        </span>
        {muted ? (
          <span className="font-mono text-[10px] uppercase tracking-[0.14em] text-zinc-400">
            {muted}
          </span>
        ) : null}
      </span>
    </div>
  );
}

function AttemptCard({
  n,
  title,
  time,
  preview,
  active,
  status,
}: {
  n: number;
  title: string;
  time: string;
  preview: string[];
  active?: boolean;
  status: "pass" | "fail" | "running";
}) {
  const statusTag =
    status === "pass"
      ? <span className="inline-flex items-center gap-1 rounded-md border border-emerald-300/70 bg-emerald-50 px-1.5 py-[1px] font-mono text-[10px] uppercase tracking-[0.14em] text-emerald-800"><StatusDot tone="pass" /> pass</span>
      : status === "fail"
      ? <span className="inline-flex items-center gap-1 rounded-md border border-rose-300/70 bg-rose-50 px-1.5 py-[1px] font-mono text-[10px] uppercase tracking-[0.14em] text-rose-800"><StatusDot tone="fail" /> fail</span>
      : <span className="inline-flex items-center gap-1 rounded-md border border-amber-300/70 bg-amber-50 px-1.5 py-[1px] font-mono text-[10px] uppercase tracking-[0.14em] text-amber-800"><StatusDot tone="running" /> running</span>;

  return (
    <button
      className={
        "group relative flex flex-col items-stretch gap-1.5 px-3 py-2 text-left transition-colors " +
        (active
          ? "bg-white shadow-[inset_0_2px_0_rgba(217,119,6,0.9)] border-r border-zinc-200"
          : "hover:bg-white border-r border-zinc-200")
      }
    >
      <div className="flex items-center gap-2">
        <span className="font-mono text-[10px] tabular-nums text-zinc-400">#{n}</span>
        <span className="min-w-0 truncate text-[13px] font-medium tracking-tight text-zinc-900">
          {title}
        </span>
        {active ? <Tag tone="amber" className="ml-auto">Active</Tag> : <span className="ml-auto">{statusTag}</span>}
      </div>
      <pre className="line-clamp-3 whitespace-pre-wrap font-mono text-[11px] leading-[1.5] text-zinc-500 group-hover:text-zinc-700">
        {preview.join("\n")}
      </pre>
      <div className="flex items-center justify-between font-mono text-[10px] uppercase tracking-[0.16em] text-zinc-400">
        <span>{time}</span>
        {!active ? <span className="opacity-0 group-hover:opacity-100 text-amber-700/80 normal-case tracking-normal">view →</span> : null}
      </div>
    </button>
  );
}

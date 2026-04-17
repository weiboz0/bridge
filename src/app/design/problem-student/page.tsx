import { Button } from "@/components/ui/button";
import {
  BlinkCursor,
  Kbd,
  PyCode,
  SectionLabel,
  StatusDot,
  Tag,
} from "@/components/design/primitives";

export default function ProblemStudentDesign() {
  return (
    <>
      <style>{`@keyframes bridge-blink{to{visibility:hidden}}`}</style>
      <div className="flex h-[calc(100vh-44px)] overflow-hidden">
        {/* LEFT — problem */}
        <aside className="flex w-[32%] min-w-[360px] flex-col border-r border-zinc-200 bg-white">
          <SectionLabel action={<Tag tone="zinc">Topic · Arrays</Tag>}>
            Problem
          </SectionLabel>

          <div className="flex-1 overflow-auto">
            <div className="px-5 pb-4 pt-5">
              <p className="font-mono text-[11px] uppercase tracking-[0.22em] text-amber-700/80">
                Problem 03 · Easy
              </p>
              <h1 className="mt-2 text-[22px] font-semibold leading-tight tracking-tight">
                Two Sum
              </h1>
              <div className="mt-4 space-y-3 text-[14px] leading-[1.65] text-zinc-700">
                <p>
                  Given a list of integers <code className="rounded bg-zinc-100 px-1 py-[1px] font-mono text-[12.5px]">nums</code>{" "}
                  and a target number <code className="rounded bg-zinc-100 px-1 py-[1px] font-mono text-[12.5px]">target</code>,
                  return the indices of the two numbers that add up to the target.
                </p>
                <p>
                  You may assume that each input has exactly one solution, and you may
                  not use the same element twice. The answer can be returned in any
                  order.
                </p>
                <p className="text-[13px] text-zinc-500">
                  Constraints: 2 ≤ len(nums) ≤ 10⁴ · −10⁹ ≤ nums[i] ≤ 10⁹
                </p>
              </div>
            </div>

            {/* Examples */}
            <div className="px-5 pb-4">
              <p className="mb-2 font-mono text-[10px] uppercase tracking-[0.22em] text-zinc-500">
                Examples
              </p>
              <div className="space-y-2">
                <ExampleBlock
                  n={1}
                  stdin={`4 2 7 11 15\n9`}
                  stdout={`0 1`}
                  note="Because nums[0] + nums[1] == 9"
                />
                <ExampleBlock
                  n={2}
                  stdin={`3 3 2 4\n6`}
                  stdout={`0 1`}
                />
              </div>
              <p className="mt-3 flex items-center gap-2 text-[12px] text-zinc-500">
                <span className="inline-flex size-4 items-center justify-center rounded-full border border-zinc-300 font-mono text-[9px] text-zinc-500">
                  2
                </span>
                more hidden test cases run on <span className="font-medium text-zinc-700">Test</span>
              </p>
            </div>

            {/* My Test Cases */}
            <details className="border-t border-zinc-200/80 open:bg-zinc-50/40" open>
              <summary className="flex h-9 cursor-pointer list-none items-center justify-between px-5 font-mono text-[10px] uppercase tracking-[0.22em] text-zinc-500 hover:text-zinc-800">
                <span>My Test Cases · 1</span>
                <span className="font-sans text-[11px] normal-case tracking-normal text-amber-700/80 hover:text-amber-800">
                  + Add case
                </span>
              </summary>
              <div className="space-y-2 px-5 pb-5 pt-1">
                <MyCaseBlock
                  name="Negatives"
                  stdin={`-3 -1 -4 -2\n-5`}
                  expected={`0 3`}
                />
                <div className="grid grid-cols-2 gap-2 rounded-lg border border-dashed border-zinc-300 bg-white/50 p-3">
                  <div>
                    <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">
                      stdin
                    </p>
                    <div className="mt-1 h-16 rounded-md border border-zinc-200 bg-white" />
                  </div>
                  <div>
                    <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">
                      expected (optional)
                    </p>
                    <div className="mt-1 h-16 rounded-md border border-zinc-200 bg-white" />
                  </div>
                </div>
              </div>
            </details>
          </div>
        </aside>

        {/* CENTER — editor */}
        <section className="flex min-w-0 flex-1 flex-col bg-white">
          {/* Attempt bar */}
          <div className="flex h-11 items-center gap-3 border-b border-zinc-200 px-4">
            <div className="flex items-center gap-2 text-[13px]">
              <StatusDot tone="idle" />
              <span className="font-medium tracking-tight">My first try</span>
              <span className="text-zinc-400">·</span>
              <span className="font-mono text-[11px] text-zinc-500">attempt 3 of 3</span>
            </div>
            <button className="ml-2 inline-flex items-center gap-1 rounded-md border border-zinc-200 bg-white px-2 py-1 text-[11px] text-zinc-600 hover:border-zinc-300 hover:text-zinc-900">
              <span className="font-mono tracking-wide">switch</span>
              <svg viewBox="0 0 8 8" className="size-2 text-zinc-500"><path fill="currentColor" d="M1 3l3 3 3-3z" /></svg>
            </button>
            <div className="ml-auto flex items-center gap-2">
              <Button variant="outline" size="sm">
                <span className="font-mono text-[11px]">+</span>
                New attempt
              </Button>
              <Button variant="outline" size="sm" className="border-zinc-200">
                Test
                <Kbd>⌘⇧R</Kbd>
              </Button>
              <Button
                size="sm"
                className="bg-amber-600 text-white hover:bg-amber-700"
              >
                <svg viewBox="0 0 10 10" className="size-2.5"><path fill="currentColor" d="M2 1l6 4-6 4z" /></svg>
                Run
                <Kbd>⌘R</Kbd>
              </Button>
            </div>
          </div>

          {/* Editor placeholder */}
          <div className="flex-1 overflow-auto">
            <div className="px-0 py-3">
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
          </div>

          {/* Attempt tabs strip */}
          <div className="flex h-9 items-center gap-0 border-t border-zinc-200 bg-zinc-50/50 px-2 text-[12px]">
            <AttemptChip active title="My first try" time="now" n={3} />
            <AttemptChip title="Hashmap idea" time="14m ago" n={2} status="pass" />
            <AttemptChip title="O(n²) brute" time="1h ago" n={1} status="fail" />
            <span className="ml-auto px-2 font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">
              autosaved · 2s ago
            </span>
          </div>
        </section>

        {/* RIGHT — inputs + terminal */}
        <aside className="flex w-[28%] min-w-[320px] flex-col border-l border-zinc-200 bg-white">
          {/* Inputs */}
          <SectionLabel
            action={
              <span className="font-mono text-[10px] uppercase tracking-[0.16em] text-zinc-400">
                stdin
              </span>
            }
          >
            Inputs
          </SectionLabel>
          <div className="px-4 pb-3 pt-3">
            <p className="mb-2 font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">
              Pick a case
            </p>
            <div className="-mx-0.5 flex flex-wrap gap-1.5">
              <Chip selected>Example 1</Chip>
              <Chip>Example 2</Chip>
              <Chip tone="amber">Mine · Negatives</Chip>
              <Chip tone="ghost">Custom…</Chip>
            </div>

            <div className="mt-3 rounded-lg border border-zinc-200 bg-zinc-50/60">
              <div className="flex h-7 items-center justify-between border-b border-zinc-200 px-2.5">
                <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-500">
                  Example 1
                </span>
                <span className="font-mono text-[10px] text-zinc-400">2 lines</span>
              </div>
              <pre className="font-mono text-[12.5px] leading-[1.55] text-zinc-800 px-3 py-2">
                <span>4 2 7 11 15</span>
                {"\n"}
                <span>9</span>
              </pre>
            </div>
          </div>

          {/* Terminal */}
          <SectionLabel
            action={
              <>
                <StatusDot tone="running" />
                <span className="font-mono text-[10px] uppercase tracking-[0.16em] text-amber-700">
                  Running
                </span>
                <button className="ml-1 rounded px-1 font-mono text-[10px] text-zinc-400 hover:text-zinc-700">
                  clear
                </button>
              </>
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
              <div className="mt-1 text-zinc-500">Enter your name:</div>
              <div className="flex items-center">
                <span className="text-zinc-700">Ada</span>
                <BlinkCursor />
              </div>
              <div className="mt-2 text-rose-700 whitespace-pre-wrap">
                NameError: name &apos;targte&apos; is not defined
              </div>
              <div className="mt-1 text-zinc-500 italic">
                exited with code 1 · 24ms
              </div>
            </div>
          </div>
        </aside>
      </div>
    </>
  );
}

/* ---------- internal bits ---------- */

function ExampleBlock({
  n,
  stdin,
  stdout,
  note,
}: {
  n: number;
  stdin: string;
  stdout: string;
  note?: string;
}) {
  return (
    <div className="overflow-hidden rounded-lg border border-zinc-200">
      <div className="flex items-center justify-between border-b border-zinc-200 bg-zinc-50/70 px-2.5 py-1">
        <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-600">
          Example {n}
        </span>
        <Tag tone="neutral">Example</Tag>
      </div>
      <div className="grid grid-cols-2 divide-x divide-zinc-200">
        <div className="p-2.5">
          <p className="mb-1 font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">
            Input
          </p>
          <pre className="whitespace-pre-wrap font-mono text-[12px] leading-[1.55] text-zinc-800">
            {stdin}
          </pre>
        </div>
        <div className="p-2.5">
          <p className="mb-1 font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">
            Output
          </p>
          <pre className="whitespace-pre-wrap font-mono text-[12px] leading-[1.55] text-zinc-800">
            {stdout}
          </pre>
        </div>
      </div>
      {note ? (
        <p className="border-t border-zinc-200 bg-zinc-50/40 px-2.5 py-1.5 text-[12px] text-zinc-500">
          {note}
        </p>
      ) : null}
    </div>
  );
}

function MyCaseBlock({
  name,
  stdin,
  expected,
}: {
  name: string;
  stdin: string;
  expected: string;
}) {
  return (
    <div className="overflow-hidden rounded-lg border border-amber-300/60">
      <div className="flex items-center justify-between border-b border-amber-300/60 bg-amber-50/70 px-2.5 py-1">
        <span className="font-medium text-[12px] text-amber-900">{name}</span>
        <Tag tone="amber">Private</Tag>
      </div>
      <div className="grid grid-cols-2 divide-x divide-zinc-200">
        <div className="p-2.5">
          <p className="mb-1 font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">
            Input
          </p>
          <pre className="whitespace-pre-wrap font-mono text-[12px] leading-[1.55] text-zinc-800">
            {stdin}
          </pre>
        </div>
        <div className="p-2.5">
          <p className="mb-1 font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">
            Expected
          </p>
          <pre className="whitespace-pre-wrap font-mono text-[12px] leading-[1.55] text-zinc-800">
            {expected}
          </pre>
        </div>
      </div>
    </div>
  );
}

function Chip({
  children,
  selected,
  tone,
}: {
  children: React.ReactNode;
  selected?: boolean;
  tone?: "amber" | "ghost";
}) {
  if (selected) {
    return (
      <button className="inline-flex items-center rounded-md border border-zinc-900 bg-zinc-900 px-2 py-[3px] text-[11.5px] font-medium text-white">
        {children}
      </button>
    );
  }
  if (tone === "amber") {
    return (
      <button className="inline-flex items-center rounded-md border border-amber-300/60 bg-amber-50 px-2 py-[3px] text-[11.5px] text-amber-800 hover:border-amber-400">
        {children}
      </button>
    );
  }
  if (tone === "ghost") {
    return (
      <button className="inline-flex items-center rounded-md border border-dashed border-zinc-300 bg-white px-2 py-[3px] text-[11.5px] text-zinc-500 hover:border-zinc-400 hover:text-zinc-800">
        {children}
      </button>
    );
  }
  return (
    <button className="inline-flex items-center rounded-md border border-zinc-200 bg-white px-2 py-[3px] text-[11.5px] text-zinc-700 hover:border-zinc-300">
      {children}
    </button>
  );
}

function AttemptChip({
  title,
  time,
  n,
  active,
  status,
}: {
  title: string;
  time: string;
  n: number;
  active?: boolean;
  status?: "pass" | "fail";
}) {
  return (
    <button
      className={
        "group flex h-7 items-center gap-2 rounded-md px-2.5 transition-colors " +
        (active
          ? "bg-white shadow-[inset_0_0_0_1px_rgba(217,119,6,0.35)] text-zinc-900"
          : "text-zinc-600 hover:bg-white hover:text-zinc-900")
      }
    >
      <span className="font-mono text-[10px] tabular-nums text-zinc-400">
        #{n}
      </span>
      <span className="font-medium tracking-tight">{title}</span>
      <span className="font-mono text-[10px] uppercase tracking-[0.16em] text-zinc-400">
        {time}
      </span>
      {status === "pass" ? <StatusDot tone="pass" /> : null}
      {status === "fail" ? <StatusDot tone="fail" /> : null}
    </button>
  );
}

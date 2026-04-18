import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { Problem, TestCase } from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";
import { Tag } from "@/components/design/primitives";

interface Props {
  problem: Problem;
  testCases: TestCase[];
}

export function ProblemDescription({ problem, testCases }: Props) {
  // Canonical example cases — what students see inline with the problem.
  const examples = testCases.filter((c) => c.ownerId === null && c.isExample);

  // Hidden canonical cases — the handler redacts these from non-authors, so
  // this is 0 for students and N for authors. Either way we show a count,
  // never the contents.
  const hiddenCount = testCases.filter((c) => c.ownerId === null && !c.isExample).length;

  return (
    <div className="space-y-5">
      <div>
        <p className="font-mono text-[11px] uppercase tracking-[0.22em] text-amber-700/80">
          {problem.language}
        </p>
        <h1 className="mt-1 text-xl font-semibold leading-tight tracking-tight">
          {problem.title}
        </h1>
      </div>

      {problem.description && (
        <div className="prose prose-sm max-w-none">
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{problem.description}</ReactMarkdown>
        </div>
      )}

      {examples.length > 0 && (
        <div>
          <p className="mb-2 font-mono text-[10px] uppercase tracking-[0.22em] text-zinc-500">
            Examples
          </p>
          <div className="space-y-2">
            {examples.map((ex, i) => (
              <ExampleBlock key={ex.id} n={i + 1} stdin={ex.stdin} stdout={ex.expectedStdout ?? ""} name={ex.name} />
            ))}
          </div>
        </div>
      )}

      {hiddenCount > 0 && (
        <p className="flex items-center gap-2 text-[12px] text-zinc-500">
          <span className="inline-flex size-4 items-center justify-center rounded-full border border-zinc-300 font-mono text-[9px] text-zinc-500">
            {hiddenCount}
          </span>
          more hidden test case{hiddenCount === 1 ? "" : "s"} run on{" "}
          <span className="font-medium text-zinc-700">Test</span>
        </p>
      )}
    </div>
  );
}

function ExampleBlock({
  n,
  stdin,
  stdout,
  name,
}: {
  n: number;
  stdin: string;
  stdout: string;
  name: string;
}) {
  return (
    <div className="overflow-hidden rounded-lg border border-zinc-200">
      <div className="flex items-center justify-between border-b border-zinc-200 bg-zinc-50/70 px-2.5 py-1">
        <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-600">
          {name || `Example ${n}`}
        </span>
        <Tag tone="neutral">Example</Tag>
      </div>
      <div className="grid grid-cols-2 divide-x divide-zinc-200">
        <div className="p-2.5">
          <p className="mb-1 font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">
            Input
          </p>
          <pre className="whitespace-pre-wrap font-mono text-[12px] leading-[1.55] text-zinc-800">
            {stdin || <span className="text-zinc-400">(empty)</span>}
          </pre>
        </div>
        <div className="p-2.5">
          <p className="mb-1 font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-400">
            Output
          </p>
          <pre className="whitespace-pre-wrap font-mono text-[12px] leading-[1.55] text-zinc-800">
            {stdout || <span className="text-zinc-400">(any)</span>}
          </pre>
        </div>
      </div>
    </div>
  );
}

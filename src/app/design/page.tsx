import Link from "next/link";

export default function DesignIndex() {
  return (
    <main className="mx-auto max-w-3xl px-6 py-16">
      <p className="font-mono text-[11px] uppercase tracking-[0.22em] text-amber-700/80">
        Design Mocks
      </p>
      <h1 className="mt-2 text-3xl font-semibold tracking-tight">
        Problem-based editor workflow
      </h1>
      <p className="mt-3 max-w-xl text-[15px] leading-relaxed text-zinc-600">
        Two routes to review the layout, density, and visual language before wiring
        any of it up. Iterate on these pages until the direction feels right, then
        the implementation plan pulls shape from here.
      </p>
      <div className="mt-10 grid gap-3 sm:grid-cols-2">
        <Link
          href="/design/problem-student"
          className="group rounded-xl border border-zinc-200 bg-white p-5 transition-all hover:border-amber-600/60 hover:shadow-[0_2px_20px_-10px_rgba(217,119,6,0.4)]"
        >
          <p className="font-mono text-[11px] uppercase tracking-[0.22em] text-zinc-400 group-hover:text-amber-700/80">
            Student
          </p>
          <p className="mt-1.5 text-base font-medium tracking-tight">
            /student/classes/.../problems/...
          </p>
          <p className="mt-1 text-sm text-zinc-500">
            Three panes: problem + cases · editor · inputs + terminal.
          </p>
        </Link>
        <Link
          href="/design/problem-teacher"
          className="group rounded-xl border border-zinc-200 bg-white p-5 transition-all hover:border-amber-600/60 hover:shadow-[0_2px_20px_-10px_rgba(217,119,6,0.4)]"
        >
          <p className="font-mono text-[11px] uppercase tracking-[0.22em] text-zinc-400 group-hover:text-amber-700/80">
            Teacher
          </p>
          <p className="mt-1.5 text-base font-medium tracking-tight">
            /teacher/.../watch-student
          </p>
          <p className="mt-1 text-sm text-zinc-500">
            Briefer problem · attempt history · read-only editor · compact terminal.
          </p>
        </Link>
      </div>
    </main>
  );
}

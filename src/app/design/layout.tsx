import Link from "next/link";

export default function DesignLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen bg-[#fafaf9] text-zinc-900">
      <header className="sticky top-0 z-30 border-b border-zinc-200/80 bg-[#fafaf9]/90 backdrop-blur-md">
        <div className="flex h-11 items-center gap-6 px-5 text-[13px]">
          <Link href="/design" className="font-semibold tracking-tight text-zinc-900">
            Bridge · <span className="font-normal text-zinc-500">design review</span>
          </Link>
          <nav className="flex items-center gap-3 text-zinc-500">
            <Link href="/design/problem-student" className="hover:text-zinc-900 transition-colors">
              Student
            </Link>
            <span className="text-zinc-300">·</span>
            <Link href="/design/problem-teacher" className="hover:text-zinc-900 transition-colors">
              Teacher
            </Link>
          </nav>
          <span className="ml-auto font-mono text-[11px] uppercase tracking-[0.18em] text-zinc-400">
            Spec 007 · Problem Workflow
          </span>
        </div>
      </header>
      {children}
    </div>
  );
}

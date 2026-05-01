import Link from "next/link";
import { redirect } from "next/navigation";
import { api, ApiError } from "@/lib/api-client";
import { Card, CardContent } from "@/components/ui/card";

interface Problem {
  id: string;
  scope: string;
  scopeId: string | null;
  title: string;
  slug: string | null;
  difficulty: string;
  gradeLevel: string | null;
  tags: string[];
  status: string;
  createdAt: string;
}

interface ListResponse {
  items: Problem[];
  nextCursor: string | null;
}

const SCOPE_LABELS: Record<string, string> = {
  platform: "Platform",
  org: "Org",
  personal: "Personal",
};

const DIFFICULTY_LABELS: Record<string, string> = {
  easy: "Easy",
  medium: "Medium",
  hard: "Hard",
};

function badgeColor(difficulty: string): string {
  if (difficulty === "easy") return "bg-emerald-50 text-emerald-700 border-emerald-200";
  if (difficulty === "medium") return "bg-amber-50 text-amber-700 border-amber-200";
  if (difficulty === "hard") return "bg-rose-50 text-rose-700 border-rose-200";
  return "bg-zinc-50 text-zinc-700 border-zinc-200";
}

export default async function TeacherProblemsPage({
  searchParams,
}: {
  searchParams: Promise<{ scope?: string; q?: string }>;
}) {
  const sp = await searchParams;
  const params = new URLSearchParams();
  if (sp.scope) params.set("scope", sp.scope);
  if (sp.q) params.set("q", sp.q);
  params.set("limit", "100");
  const path = `/api/problems?${params.toString()}`;

  let data: ListResponse;
  try {
    data = await api<ListResponse>(path);
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) redirect("/login");
    throw e;
  }
  const items = data.items ?? [];

  const scope = sp.scope ?? "";
  const tabs = [
    { id: "", label: "All" },
    { id: "platform", label: "Platform" },
    { id: "org", label: "Org" },
    { id: "personal", label: "Personal" },
  ];

  return (
    <div className="p-6 space-y-6">
      <div>
        <h1 className="text-2xl font-bold">Problem Bank</h1>
        <p className="text-sm text-muted-foreground">
          Problems you can attach to topics. Platform-scope problems are part
          of the Bridge HQ library; org-scope live within your school; personal
          problems are your own.
        </p>
      </div>

      <div className="flex items-center gap-4 text-sm">
        <form className="flex-1 max-w-sm">
          {scope && <input type="hidden" name="scope" value={scope} />}
          <input
            name="q"
            defaultValue={sp.q ?? ""}
            placeholder="Search title or slug…"
            className="w-full rounded-md border border-zinc-200 px-3 py-1.5 text-sm bg-background"
          />
        </form>
        <div className="flex gap-1">
          {tabs.map((t) => {
            const href =
              t.id === ""
                ? sp.q
                  ? `/teacher/problems?q=${encodeURIComponent(sp.q)}`
                  : "/teacher/problems"
                : `/teacher/problems?scope=${t.id}${sp.q ? `&q=${encodeURIComponent(sp.q)}` : ""}`;
            const active = scope === t.id;
            return (
              <Link
                key={t.label}
                href={href}
                className={`rounded-md px-2.5 py-1 text-xs font-medium ${
                  active ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:bg-muted"
                }`}
              >
                {t.label}
              </Link>
            );
          })}
        </div>
      </div>

      {items.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-sm text-muted-foreground">
            No problems match. Try a broader filter, or import the Python 101
            curriculum from <code>content/python-101/</code>.
          </CardContent>
        </Card>
      ) : (
        <div className="border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-muted/50">
              <tr className="text-left">
                <th className="px-4 py-2 font-medium">Title</th>
                <th className="px-4 py-2 font-medium">Slug</th>
                <th className="px-4 py-2 font-medium">Scope</th>
                <th className="px-4 py-2 font-medium">Difficulty</th>
                <th className="px-4 py-2 font-medium">Grade</th>
                <th className="px-4 py-2 font-medium">Status</th>
                <th className="px-4 py-2 font-medium">Tags</th>
              </tr>
            </thead>
            <tbody>
              {items.map((p) => (
                <tr key={p.id} className="border-t">
                  <td className="px-4 py-2 font-medium">{p.title}</td>
                  <td className="px-4 py-2 text-xs text-muted-foreground font-mono">
                    {p.slug ?? "—"}
                  </td>
                  <td className="px-4 py-2 text-xs">{SCOPE_LABELS[p.scope] ?? p.scope}</td>
                  <td className="px-4 py-2">
                    <span
                      className={`inline-flex items-center rounded-md border px-1.5 py-0.5 text-[11px] font-medium ${badgeColor(p.difficulty)}`}
                    >
                      {DIFFICULTY_LABELS[p.difficulty] ?? p.difficulty}
                    </span>
                  </td>
                  <td className="px-4 py-2 text-xs text-muted-foreground">
                    {p.gradeLevel ?? "—"}
                  </td>
                  <td className="px-4 py-2 text-xs text-muted-foreground">{p.status}</td>
                  <td className="px-4 py-2 text-xs text-muted-foreground">
                    {p.tags.slice(0, 3).join(", ") || "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {data.nextCursor && (
        <p className="text-xs text-muted-foreground">
          More results available — refine your search to narrow down.
        </p>
      )}
    </div>
  );
}

import Link from "next/link";
import { redirect } from "next/navigation";
import { api, ApiError } from "@/lib/api-client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

interface Unit {
  id: string;
  scope: string;
  scopeId: string | null;
  title: string;
  slug: string | null;
  summary: string;
  gradeLevel: string | null;
  status: string;
  materialType: string;
  createdAt: string;
}

interface SearchResponse {
  items: Unit[];
  nextCursor: string | null;
}

const SCOPE_LABELS: Record<string, string> = {
  platform: "Platform",
  org: "Org",
  personal: "Personal",
};

const STATUS_LABELS: Record<string, string> = {
  draft: "Draft",
  reviewed: "Reviewed",
  classroom_ready: "Classroom Ready",
  coach_ready: "Coach Ready",
  archived: "Archived",
};

function statusBadge(status: string): string {
  if (status === "classroom_ready" || status === "coach_ready")
    return "bg-emerald-50 text-emerald-700 border-emerald-200";
  if (status === "reviewed") return "bg-sky-50 text-sky-700 border-sky-200";
  if (status === "archived") return "bg-zinc-50 text-zinc-500 border-zinc-200";
  return "bg-amber-50 text-amber-700 border-amber-200";
}

export default async function AdminUnitsPage({
  searchParams,
}: {
  searchParams: Promise<{ scope?: string; q?: string }>;
}) {
  const sp = await searchParams;
  const params = new URLSearchParams();
  if (sp.scope) params.set("scope", sp.scope);
  if (sp.q) params.set("q", sp.q);
  params.set("limit", "100");
  const path = `/api/units/search?${params.toString()}`;

  let data: SearchResponse;
  try {
    data = await api<SearchResponse>(path);
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) redirect("/login");
    if (e instanceof ApiError && e.status === 403) {
      return (
        <div className="p-6 max-w-2xl">
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Platform admin access required</CardTitle>
            </CardHeader>
            <CardContent className="text-sm text-muted-foreground">
              <p>This page is for platform admins. Return to <a href="/" className="underline text-primary">your dashboard</a>.</p>
            </CardContent>
          </Card>
        </div>
      );
    }
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
        <h1 className="text-2xl font-bold">Units</h1>
        <p className="text-sm text-muted-foreground">
          Every teaching unit on the platform. Platform-scope units are
          published library content; org-scope live within a school;
          personal units are individual teachers&apos; drafts.
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
                  ? `/admin/units?q=${encodeURIComponent(sp.q)}`
                  : "/admin/units"
                : `/admin/units?scope=${t.id}${sp.q ? `&q=${encodeURIComponent(sp.q)}` : ""}`;
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
            No units match. Try a broader filter.
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
                <th className="px-4 py-2 font-medium">Status</th>
                <th className="px-4 py-2 font-medium">Type</th>
                <th className="px-4 py-2 font-medium">Grade</th>
              </tr>
            </thead>
            <tbody>
              {items.map((u) => (
                <tr key={u.id} className="border-t hover:bg-muted/30">
                  <td className="px-4 py-2 font-medium">
                    <Link href={`/teacher/units/${u.id}/edit`} className="text-primary underline-offset-2 hover:underline">
                      {u.title}
                    </Link>
                  </td>
                  <td className="px-4 py-2 text-xs text-muted-foreground font-mono">
                    {u.slug ?? "—"}
                  </td>
                  <td className="px-4 py-2 text-xs">{SCOPE_LABELS[u.scope] ?? u.scope}</td>
                  <td className="px-4 py-2">
                    <span
                      className={`inline-flex items-center rounded-md border px-1.5 py-0.5 text-[11px] font-medium ${statusBadge(u.status)}`}
                    >
                      {STATUS_LABELS[u.status] ?? u.status}
                    </span>
                  </td>
                  <td className="px-4 py-2 text-xs text-muted-foreground">{u.materialType}</td>
                  <td className="px-4 py-2 text-xs text-muted-foreground">
                    {u.gradeLevel ?? "—"}
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

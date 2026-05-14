import Link from "next/link";
import { api } from "@/lib/api-client";
import { ApiError } from "@/lib/api-error";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

// Plan 079 — read-only platform-admin chapter detail. The list page at
// /admin/chapters links here; the previous link to /teacher/chapters/{id}/edit
// bounced platform admins without the teacher role back to /admin
// because <PortalShell portalRole="teacher"> redirects on missing role.
//
// Scope is read-only metadata. No content preview (Yjs body is encoded
// state, not human-readable). No edit/publish/archive in v1 — defer to a
// separate plan that handles realtime-collab semantics.

interface ChapterDetail {
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
  createdBy: string;
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

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export default async function AdminChapterDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  if (!UUID_RE.test(id)) {
    return (
      <NotFoundState id={id} reason="Invalid chapter id format" />
    );
  }

  let chapter: ChapterDetail;
  try {
    chapter = await api<ChapterDetail>(`/api/chapters/${id}`);
  } catch (e) {
    if (e instanceof ApiError) {
      // Per Kimi K2.6 plan-review BLOCKER 4(b): the Go handler returns
      // 404 for both missing-row AND unauthorized-row cases. Platform
      // admins bypass authorization regardless. So 403 is dead code in
      // practice. Two visible error states only: 404 and 5xx.
      if (e.status === 404) {
        return <NotFoundState id={id} />;
      }
      return <ErrorState status={e.status} message={e.message} />;
    }
    return (
      <ErrorState
        status={0}
        message={e instanceof Error ? e.message : String(e)}
      />
    );
  }

  return (
    <div className="p-6 space-y-6 max-w-3xl">
      <BackLink />
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h1 className="text-2xl font-bold">{chapter.title}</h1>
          {chapter.slug && (
            <p className="text-xs text-muted-foreground font-mono">{chapter.slug}</p>
          )}
        </div>
        <span
          className={`shrink-0 inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${statusBadge(chapter.status)}`}
        >
          {STATUS_LABELS[chapter.status] ?? chapter.status}
        </span>
      </div>

      {chapter.summary && (
        <p className="text-sm text-muted-foreground">{chapter.summary}</p>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Metadata</CardTitle>
        </CardHeader>
        <CardContent className="grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
          <Field label="Scope" value={SCOPE_LABELS[chapter.scope] ?? chapter.scope} />
          <Field label="Scope ID" value={chapter.scopeId ?? "—"} mono />
          <Field
            label="Material type"
            value={chapter.materialType || "—"}
          />
          <Field
            label="Grade level"
            value={chapter.gradeLevel ?? "—"}
          />
          <Field
            label="Created by"
            value={chapter.createdBy}
            mono
          />
          <Field
            label="Created at"
            value={new Date(chapter.createdAt).toLocaleString()}
          />
        </CardContent>
      </Card>
    </div>
  );
}

function BackLink() {
  return (
    <Link
      href="/admin/chapters"
      className="text-sm text-primary hover:underline inline-flex items-center gap-1"
    >
      ← Back to all chapters
    </Link>
  );
}

function Field({
  label,
  value,
  mono,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  // Plan 079 + Kimi K2.6 code-review: original used <dt>/<dd> without a
  // parent <dl> (invalid HTML5). The card grid isn't semantically a
  // definition list — these are arbitrary metadata pairs, not term/def.
  // Use plain <div>s; pair label and value visually only.
  return (
    <div className="space-y-0.5">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className={mono ? "font-mono text-xs break-all" : ""}>{value}</div>
    </div>
  );
}

function NotFoundState({ id, reason }: { id: string; reason?: string }) {
  return (
    <div className="p-6 max-w-2xl space-y-3">
      <BackLink />
      <h1 className="text-2xl font-bold">Chapter not found</h1>
      <p className="text-sm text-muted-foreground">
        No chapter with id <span className="font-mono text-xs">{id}</span>{" "}
        {reason ? `(${reason})` : "exists, or it has been deleted."}
      </p>
    </div>
  );
}

function ErrorState({
  status,
  message,
}: {
  status: number;
  message: string;
}) {
  return (
    <div className="p-6 max-w-2xl space-y-3">
      <BackLink />
      <h1 className="text-2xl font-bold">Couldn&apos;t load chapter</h1>
      <p className="text-sm text-muted-foreground">
        Something went wrong while loading this unit.
      </p>
      <pre className="text-xs bg-muted p-2 rounded">
        {status > 0 ? `HTTP ${status}: ` : ""}
        {message}
      </pre>
      <p className="text-xs text-muted-foreground">
        Try refreshing the page. If this keeps happening, check the API logs.
      </p>
    </div>
  );
}

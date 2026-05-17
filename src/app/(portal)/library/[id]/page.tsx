import Link from "next/link";
import { redirect } from "next/navigation";
import { api } from "@/lib/api-client";
import { ApiError } from "@/lib/api-error";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { LibraryBookEditTrigger } from "./library-book-edit-trigger";
import type { UserRole } from "@/lib/portal/types";

// ---------------------------------------------------------------------------
// Local types
// ---------------------------------------------------------------------------

interface BookDetail {
  id: string;
  title: string;
  description: string;
  scope: "platform" | "org";
  scopeId: string | null;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
}

interface Chapter {
  id: string;
  title: string;
  status: string;
  scope: string;
  bookId: string | null;
}

interface Org {
  id: string;
  name: string;
}

interface PortalAccessResponse {
  authorized: boolean;
  userName: string;
  roles: UserRole[];
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

const STATUS_LABELS: Record<string, string> = {
  draft: "Draft",
  reviewed: "Reviewed",
  classroom_ready: "Classroom Ready",
  coach_ready: "Coach Ready",
  archived: "Archived",
};

function scopeBadgeClass(scope: string) {
  if (scope === "platform") return "bg-sky-50 text-sky-700 border-sky-200";
  return "bg-violet-50 text-violet-700 border-violet-200";
}

// Resolve the chapter detail base path based on the user's highest-priority
// role. Admin → /admin/chapters; teacher/org_admin → /teacher/chapters;
// others → null (chapters rendered as plain text, not links).
function chapterBasePath(roles: UserRole[]): string | null {
  if (roles.some((r) => r.role === "admin")) return "/admin/chapters";
  if (roles.some((r) => r.role === "teacher" || r.role === "org_admin"))
    return "/teacher/chapters";
  return null;
}

// ---------------------------------------------------------------------------
// Page component
// ---------------------------------------------------------------------------

export default async function LibraryBookDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  // Invalid UUID → 404 immediately (no-leak semantics, Decision #6, plan 089).
  if (!UUID_RE.test(id)) {
    return (
      <div className="p-6 max-w-2xl space-y-3">
        <BackLink />
        <h1 className="text-2xl font-bold">Book not found</h1>
        <p className="text-sm text-muted-foreground">
          <span className="font-mono text-xs">{id}</span> is not a valid book ID.
        </p>
      </div>
    );
  }

  // Fetch portal-access for role detection (Decision #3, #11, plan 089).
  // Run in parallel with the book fetch; both are needed before rendering.
  let portalAccess: PortalAccessResponse;
  let book: BookDetail;

  try {
    [portalAccess, book] = await Promise.all([
      api<PortalAccessResponse>("/api/me/portal-access"),
      api<BookDetail>(`/api/books/${id}`),
    ]);
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.status === 401) redirect("/login");
      // 403 or 404: no-leak — both surface as "not found" (Decision #6).
      if (e.status === 403 || e.status === 404) {
        return (
          <div className="p-6 max-w-2xl space-y-3">
            <BackLink />
            <h1 className="text-2xl font-bold">Book not found</h1>
            <p className="text-sm text-muted-foreground">
              No book with id <span className="font-mono text-xs">{id}</span> exists, or
              you don&apos;t have access to it.
            </p>
          </div>
        );
      }
    }
    throw e;
  }

  if (!portalAccess.authorized) redirect("/login");

  const { roles } = portalAccess;
  const isPlatformAdmin = roles.some((r) => r.role === "admin");

  // Build org-name map from roles[] (Decision #4).
  const orgMap = new Map<string, string>();
  for (const r of roles) {
    if (r.orgId && r.orgName) orgMap.set(r.orgId, r.orgName);
  }

  // Platform admin: augment org-name map from /api/admin/orgs (Decision #4).
  if (isPlatformAdmin && book.scope === "org" && book.scopeId) {
    if (!orgMap.has(book.scopeId)) {
      try {
        const org = await api<Org>(`/api/admin/orgs/${book.scopeId}`);
        orgMap.set(org.id, org.name);
      } catch {
        // Non-fatal — falls back to UUID prefix.
      }
    }
  }

  // Resolve org display name.
  const orgLabel =
    book.scope === "platform"
      ? "Platform"
      : book.scopeId
      ? (orgMap.get(book.scopeId) ?? `Org · ${book.scopeId.slice(0, 8)}`)
      : "—";

  // Fetch chapters for this book (Decision #12, plan 089).
  let chapters: Chapter[] = [];
  const chaptersQs = new URLSearchParams({ scope: book.scope, bookId: id });
  if (book.scope === "org" && book.scopeId) chaptersQs.set("scopeId", book.scopeId);
  try {
    const resp = await api<{ items: Chapter[] }>(`/api/chapters?${chaptersQs.toString()}`);
    chapters = resp.items ?? [];
  } catch {
    // Empty list on failure — non-critical for the metadata page.
  }

  const chapterBase = chapterBasePath(roles);

  // Determine if user can edit this book: admin always can; org members can
  // edit books scoped to their org.
  const orgMemberships = roles.filter(
    (r) => (r.role === "teacher" || r.role === "org_admin") && r.orgId
  );
  const canEdit =
    isPlatformAdmin ||
    (book.scope === "org" &&
      book.scopeId !== null &&
      orgMemberships.some((r) => r.orgId === book.scopeId));

  return (
    <div className="p-6 space-y-6 max-w-3xl">
      <div className="flex items-center justify-between">
        <BackLink />
        {canEdit && (
          <LibraryBookEditTrigger
            book={{
              id: book.id,
              title: book.title,
              description: book.description,
              scope: book.scope,
              scopeId: book.scopeId,
            }}
          />
        )}
      </div>

      <div className="flex items-start justify-between gap-4">
        <h1 className="text-2xl font-bold">{book.title}</h1>
        <span
          className={`shrink-0 inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${scopeBadgeClass(book.scope)}`}
        >
          {book.scope === "platform" ? "Platform" : "Org"}
        </span>
      </div>

      {book.description && (
        <p className="text-sm text-muted-foreground">{book.description}</p>
      )}

      {/* Metadata card */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Metadata</CardTitle>
        </CardHeader>
        <CardContent className="grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
          <Field label="Scope" value={book.scope === "platform" ? "Platform" : "Org"} />
          <Field label="Owner" value={orgLabel} />
          <Field label="Created" value={new Date(book.createdAt).toLocaleString()} />
          <Field label="Last updated" value={new Date(book.updatedAt).toLocaleString()} />
        </CardContent>
      </Card>

      {/* Chapters in this book */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Chapters in this book</CardTitle>
        </CardHeader>
        <CardContent>
          {chapters.length === 0 ? (
            <p className="text-sm text-muted-foreground">No chapters assigned yet.</p>
          ) : (
            <div className="border rounded-lg overflow-hidden">
              <table className="w-full text-sm">
                <thead className="bg-muted/50">
                  <tr className="text-left">
                    <th className="px-4 py-2 font-medium">Title</th>
                    <th className="px-4 py-2 font-medium">Status</th>
                  </tr>
                </thead>
                <tbody>
                  {chapters.map((c) => (
                    <tr key={c.id} className="border-t hover:bg-muted/30">
                      <td className="px-4 py-2">
                        {chapterBase ? (
                          <Link
                            href={`${chapterBase}/${c.id}`}
                            className="text-primary underline-offset-2 hover:underline"
                          >
                            {c.title}
                          </Link>
                        ) : (
                          c.title
                        )}
                      </td>
                      <td className="px-4 py-2 text-xs text-muted-foreground">
                        {STATUS_LABELS[c.status] ?? c.status}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function BackLink() {
  return (
    <Link
      href="/library"
      className="text-sm text-primary hover:underline inline-flex items-center gap-1"
    >
      ← Back to library
    </Link>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="space-y-0.5">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div>{value}</div>
    </div>
  );
}

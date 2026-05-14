import Link from "next/link";
import { redirect } from "next/navigation";
import { api } from "@/lib/api-client";
import { ApiError } from "@/lib/api-error";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { BookEditTrigger } from "./book-edit-trigger";

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

interface AdminOrg {
  id: string;
  name: string;
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

const STATUS_LABELS: Record<string, string> = {
  draft: "Draft",
  reviewed: "Reviewed",
  classroom_ready: "Classroom Ready",
  coach_ready: "Coach Ready",
  archived: "Archived",
};

export default async function AdminBookDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

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

  let book: BookDetail;
  // chapters in this book; backend filter by bookId not yet supported —
  // TODO(plan 088 phase 3): backend doesn't filter chapters by bookId via
  // GET /api/chapters yet. Leaving chapter list empty for now.
  const chapters: Chapter[] = [];
  let orgName: string | null = null;

  try {
    book = await api<BookDetail>(`/api/books/${id}`);
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.status === 401) {
        redirect("/login");
      }
      if (e.status === 403) {
        return (
          <div className="p-6 max-w-2xl">
            <Card>
              <CardHeader>
                <CardTitle className="text-lg">Platform admin access required</CardTitle>
              </CardHeader>
              <CardContent className="space-y-2 text-sm text-muted-foreground">
                <p>
                  Your session doesn&apos;t have platform-admin privileges. If you were just
                  granted the role, sign out and back in to refresh your session.
                </p>
                <p>
                  If you don&apos;t expect to be a platform admin, return to{" "}
                  <Link href="/" className="underline text-primary">
                    your dashboard
                  </Link>
                  .
                </p>
              </CardContent>
            </Card>
          </div>
        );
      }
      if (e.status === 404) {
        return (
          <div className="p-6 max-w-2xl space-y-3">
            <BackLink />
            <h1 className="text-2xl font-bold">Book not found</h1>
            <p className="text-sm text-muted-foreground">
              No book with id <span className="font-mono text-xs">{id}</span> exists, or it
              has been deleted.
            </p>
          </div>
        );
      }
    }
    throw e;
  }

  // Resolve org name if this is an org-scoped book.
  if (book.scope === "org" && book.scopeId) {
    try {
      const org = await api<AdminOrg>(`/api/admin/orgs/${book.scopeId}`);
      orgName = org.name;
    } catch {
      // Org might be deleted or inaccessible; fall back to ID.
    }
  }

  function scopeBadgeClass(s: string) {
    if (s === "platform") return "bg-sky-50 text-sky-700 border-sky-200";
    return "bg-violet-50 text-violet-700 border-violet-200";
  }

  return (
    <div className="p-6 space-y-6 max-w-3xl">
      <div className="flex items-center justify-between">
        <BackLink />
        <BookEditTrigger
          book={{
            id: book.id,
            title: book.title,
            description: book.description,
            scope: book.scope,
            scopeId: book.scopeId,
          }}
        />
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
          <Field
            label="Owner"
            value={
              book.scope === "platform"
                ? "Platform"
                : orgName ?? (book.scopeId ?? "—")
            }
          />
          <Field
            label="Created"
            value={new Date(book.createdAt).toLocaleString()}
          />
          <Field
            label="Last updated"
            value={new Date(book.updatedAt).toLocaleString()}
          />
        </CardContent>
      </Card>

      {/* Chapters in this book */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Chapters in this book</CardTitle>
        </CardHeader>
        <CardContent>
          {chapters.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No chapters assigned yet.{" "}
              {/* TODO(plan 088 phase 3): once backend supports ?bookId= filter
                  on GET /api/chapters, fetch and display chapters here. */}
            </p>
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
                        <Link
                          href={`/admin/chapters/${c.id}`}
                          className="text-primary underline-offset-2 hover:underline"
                        >
                          {c.title}
                        </Link>
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
      href="/admin/books"
      className="text-sm text-primary hover:underline inline-flex items-center gap-1"
    >
      ← Back to books
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

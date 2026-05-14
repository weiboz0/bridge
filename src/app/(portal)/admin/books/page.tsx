import Link from "next/link";
import { redirect } from "next/navigation";
import { api } from "@/lib/api-client";
import { ApiError } from "@/lib/api-error";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { BookActions } from "@/components/books/book-actions";
import { BookCreateTrigger } from "./book-create-trigger";

interface Book {
  id: string;
  title: string;
  description: string;
  scope: "platform" | "org";
  scopeId: string | null;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
}

interface Org {
  id: string;
  name: string;
}

export default async function AdminBooksPage({
  searchParams,
}: {
  searchParams: Promise<{ scope?: string; scopeId?: string }>;
}) {
  const { scope, scopeId } = await searchParams;

  // Build the book-fetch URL with optional scope/scopeId filters.
  const params = new URLSearchParams();
  if (scope) params.set("scope", scope);
  if (scopeId) params.set("scopeId", scopeId);
  const bookPath = params.size > 0 ? `/api/books?${params.toString()}` : "/api/books";

  // Fetch books and orgs. Books call is the auth gate (401/403 bubble up).
  // Orgs are needed only for the filter dropdown; failures fall back to [].
  let books: Book[];
  let orgs: Org[];
  try {
    const booksResp = await api<{ items: Book[] }>(bookPath);
    books = booksResp.items ?? [];
    // Orgs fetch is best-effort; errors are swallowed.
    try {
      const orgsResp = await api<{ items: Org[] }>("/api/admin/orgs");
      orgs = orgsResp.items ?? [];
    } catch {
      orgs = [];
    }
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) {
      redirect("/login");
    }
    if (e instanceof ApiError && e.status === 403) {
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
    throw e;
  }

  const activeScope = scope ?? "";
  const tabs = [
    { id: "", label: "All" },
    { id: "platform", label: "Platform" },
    { id: "org", label: "Org" },
  ];

  function scopeBadgeClass(s: string) {
    if (s === "platform") return "bg-sky-50 text-sky-700 border-sky-200";
    if (s === "org") return "bg-violet-50 text-violet-700 border-violet-200";
    return "bg-zinc-50 text-zinc-500 border-zinc-200";
  }

  // Build a map of orgId → orgName for the Owner column.
  const orgMap = new Map<string, string>(orgs.map((o) => [o.id, o.name]));

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Books</h1>
        <BookCreateTrigger availableOrgs={orgs} />
      </div>

      {/* Scope filter chips */}
      <div className="flex items-center gap-4 text-sm flex-wrap">
        <div className="flex gap-1">
          {tabs.map((t) => {
            const href =
              t.id === ""
                ? "/admin/books"
                : `/admin/books?scope=${t.id}`;
            const active = activeScope === t.id;
            return (
              <Link
                key={t.id}
                href={href}
                className={`rounded-md px-2.5 py-1 text-xs font-medium ${
                  active
                    ? "bg-primary text-primary-foreground"
                    : "text-muted-foreground hover:bg-muted"
                }`}
              >
                {t.label}
              </Link>
            );
          })}
        </div>

        {/* Org dropdown (only shown when filtering by org scope) */}
        {activeScope === "org" && orgs.length > 0 && (
          <form method="get" action="/admin/books">
            <input type="hidden" name="scope" value="org" />
            <select
              name="scopeId"
              defaultValue={scopeId ?? ""}
              onChange={(e) => e.currentTarget.form?.submit()}
              className="border rounded px-2 py-1 text-sm bg-background text-foreground"
              aria-label="Filter by organization"
            >
              <option value="">All orgs</option>
              {orgs.map((o) => (
                <option key={o.id} value={o.id}>
                  {o.name}
                </option>
              ))}
            </select>
          </form>
        )}
      </div>

      {books.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-sm text-muted-foreground">
            No books found.
          </CardContent>
        </Card>
      ) : (
        <div className="border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-muted/50">
              <tr className="text-left">
                <th className="px-4 py-2 font-medium">Title</th>
                <th className="px-4 py-2 font-medium">Scope</th>
                <th className="px-4 py-2 font-medium">Owner</th>
                <th className="px-4 py-2 font-medium">Chapters</th>
                <th className="px-4 py-2 font-medium">Updated</th>
                <th className="px-4 py-2 font-medium sr-only">Actions</th>
              </tr>
            </thead>
            <tbody>
              {books.map((book) => (
                <tr key={book.id} className="border-t hover:bg-muted/30">
                  <td className="px-4 py-2 font-medium">
                    <Link
                      href={`/admin/books/${book.id}`}
                      className="text-primary underline-offset-2 hover:underline"
                    >
                      {book.title}
                    </Link>
                  </td>
                  <td className="px-4 py-2">
                    <span
                      className={`inline-flex items-center rounded-md border px-1.5 py-0.5 text-[11px] font-medium ${scopeBadgeClass(book.scope)}`}
                    >
                      {book.scope === "platform" ? "Platform" : "Org"}
                    </span>
                  </td>
                  <td className="px-4 py-2 text-xs text-muted-foreground">
                    {book.scope === "platform"
                      ? "Platform"
                      : book.scopeId
                      ? (orgMap.get(book.scopeId) ?? book.scopeId)
                      : "—"}
                  </td>
                  <td className="px-4 py-2 text-xs text-muted-foreground">—</td>
                  <td className="px-4 py-2 text-xs text-muted-foreground">
                    {new Date(book.updatedAt).toLocaleDateString()}
                  </td>
                  <td className="px-4 py-2">
                    <BookActions
                      bookId={book.id}
                      bookTitle={book.title}
                      bookScope={book.scope}
                      bookScopeId={book.scopeId}
                      bookDescription={book.description}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

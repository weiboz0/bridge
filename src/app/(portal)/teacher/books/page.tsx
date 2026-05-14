import Link from "next/link";
import { redirect } from "next/navigation";
import { api } from "@/lib/api-client";
import { ApiError } from "@/lib/api-error";
import { Card, CardContent } from "@/components/ui/card";
import { BookActions } from "@/components/books/book-actions";
import { TeacherBookCreateTrigger } from "./teacher-book-create-trigger";

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

interface OrgMembership {
  orgId: string;
  orgName: string;
  role: string;
  status: string;
  orgStatus: string;
}

export default async function TeacherBooksPage() {
  // Get the teacher's org memberships to know which org books to show.
  let orgs: OrgMembership[];
  try {
    const raw = await api<OrgMembership[]>("/api/orgs");
    orgs = raw.filter(
      (m) =>
        m.status === "active" &&
        m.orgStatus === "active" &&
        (m.role === "teacher" || m.role === "org_admin")
    );
    // Dedupe by orgId.
    const byOrg = new Map<string, OrgMembership>();
    for (const m of orgs) {
      if (!byOrg.has(m.orgId)) byOrg.set(m.orgId, m);
    }
    orgs = Array.from(byOrg.values());
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) redirect("/login");
    orgs = [];
  }

  // Fetch platform books + org books for each org the teacher belongs to.
  const allBooks: Book[] = [];
  try {
    const fetches: Promise<{ items: Book[] }>[] = [
      api<{ items: Book[] }>("/api/books?scope=platform"),
    ];
    for (const m of orgs) {
      fetches.push(
        api<{ items: Book[] }>(`/api/books?scope=org&scopeId=${m.orgId}`).catch(() => ({
          items: [],
        }))
      );
    }
    const results = await Promise.all(fetches);
    // Merge and deduplicate by book id.
    const seen = new Set<string>();
    for (const r of results) {
      for (const b of r.items ?? []) {
        if (!seen.has(b.id)) {
          seen.add(b.id);
          allBooks.push(b);
        }
      }
    }
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) redirect("/login");
    // Other errors: surface empty list — teacher portal doesn't need a hard error here.
  }

  // Build orgId → orgName map for the Owner column.
  const orgMap = new Map<string, string>(orgs.map((m) => [m.orgId, m.orgName]));

  function scopeBadgeClass(s: string) {
    if (s === "platform") return "bg-sky-50 text-sky-700 border-sky-200";
    return "bg-violet-50 text-violet-700 border-violet-200";
  }

  // The orgs list for the create dialog: the teacher can only create org-scope books.
  const availableOrgs = orgs.map((m) => ({ id: m.orgId, name: m.orgName }));

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Books</h1>
        <TeacherBookCreateTrigger availableOrgs={availableOrgs} />
      </div>

      {allBooks.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-sm text-muted-foreground">
            No books available. Platform or org admins can create books and assign chapters to them.
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
              {allBooks.map((book) => (
                <tr key={book.id} className="border-t hover:bg-muted/30">
                  <td className="px-4 py-2 font-medium">
                    <Link
                      href={`/teacher/books/${book.id}`}
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

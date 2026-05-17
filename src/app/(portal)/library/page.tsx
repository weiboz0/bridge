import Link from "next/link";
import { redirect } from "next/navigation";
import { api } from "@/lib/api-client";
import { ApiError } from "@/lib/api-error";
import { Card, CardContent } from "@/components/ui/card";
import { BookActions } from "@/components/books/book-actions";
import { LibraryBookCreateTrigger } from "./library-book-create-trigger";
import type { UserRole } from "@/lib/portal/types";

// ---------------------------------------------------------------------------
// Local type aliases — mirroring what the backend returns.
// ---------------------------------------------------------------------------

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

interface PortalAccessResponse {
  authorized: boolean;
  userName: string;
  roles: UserRole[];
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function scopeBadgeClass(scope: string) {
  if (scope === "platform") return "bg-sky-50 text-sky-700 border-sky-200";
  if (scope === "org") return "bg-violet-50 text-violet-700 border-violet-200";
  return "bg-zinc-50 text-zinc-500 border-zinc-200";
}

// ---------------------------------------------------------------------------
// Page component
// ---------------------------------------------------------------------------

export default async function LibraryPage() {
  // Fetch portal-access first — role detection is entirely from this response
  // (Decision #3, #11, plan 089). Do NOT call auth() or /api/orgs.
  let portalAccess: PortalAccessResponse;
  try {
    portalAccess = await api<PortalAccessResponse>("/api/me/portal-access");
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) redirect("/login");
    throw e;
  }

  if (!portalAccess.authorized) redirect("/login");

  const { roles } = portalAccess;

  // Platform admin: any role with role==="admin".
  const isPlatformAdmin = roles.some((r) => r.role === "admin");

  // Org members who can create books: teacher or org_admin with an orgId.
  const orgMemberships = roles.filter(
    (r) => (r.role === "teacher" || r.role === "org_admin") && r.orgId
  );

  // Build an org-name map from roles[] (Decision #4).
  const orgMap = new Map<string, string>();
  for (const r of roles) {
    if (r.orgId && r.orgName) orgMap.set(r.orgId, r.orgName);
  }

  // Fetch books. The backend's visibility filter is already correct for every
  // role — one call suffices (Decision #12, plan 089).
  let books: Book[] = [];
  try {
    const booksResp = await api<{ items: Book[] }>("/api/books");
    books = booksResp.items ?? [];
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) redirect("/login");
    // Other errors: surface empty list with no hard crash.
  }

  // Platform admin additionally fetches /api/admin/orgs to resolve org names
  // for cross-org books not in their own memberships (Decision #4).
  if (isPlatformAdmin) {
    try {
      const orgsResp = await api<{ items: Org[] }>("/api/admin/orgs");
      for (const o of orgsResp.items ?? []) {
        if (!orgMap.has(o.id)) orgMap.set(o.id, o.name);
      }
    } catch {
      // Non-fatal — org names that aren't in roles[] fall back to UUID prefix.
    }
  }

  // Compute create-button affordance (Decision #5).
  // Admin: scope picker covers platform + all orgs from /api/admin/orgs.
  // Org member with teacher/org_admin: org-only picker restricted to their orgs.
  // Other roles (student/parent): no create button.
  let createTriggerOrgs: Org[] | null = null;
  if (isPlatformAdmin) {
    // Use the merged orgMap so all orgs (including non-membership ones) appear.
    createTriggerOrgs = Array.from(orgMap.entries()).map(([id, name]) => ({ id, name }));
  } else if (orgMemberships.length > 0) {
    createTriggerOrgs = orgMemberships.map((r) => ({
      id: r.orgId!,
      name: r.orgName ?? r.orgId!,
    }));
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Library</h1>
        {createTriggerOrgs !== null && (
          <LibraryBookCreateTrigger availableOrgs={createTriggerOrgs} />
        )}
      </div>

      {books.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-sm text-muted-foreground">
            No books available.
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
              {books.map((book) => {
                const ownerLabel =
                  book.scope === "platform"
                    ? "Platform"
                    : book.scopeId
                    ? (orgMap.get(book.scopeId) ??
                      `Org · ${book.scopeId.slice(0, 8)}`)
                    : "—";

                return (
                  <tr key={book.id} className="border-t hover:bg-muted/30">
                    <td className="px-4 py-2 font-medium">
                      <Link
                        href={`/library/${book.id}`}
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
                      {ownerLabel}
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
                        detailBasePath="/library"
                      />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

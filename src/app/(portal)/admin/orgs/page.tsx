import { redirect } from "next/navigation";
import { api, ApiError } from "@/lib/api-client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { OrgActions } from "@/components/admin/org-actions";

interface Org {
  id: string;
  name: string;
  type: string;
  contactEmail: string;
  slug: string;
  status: string;
}

export default async function AdminOrgsPage({
  searchParams,
}: {
  searchParams: Promise<{ status?: string }>;
}) {
  const { status } = await searchParams;
  const path = status ? `/api/admin/orgs?status=${encodeURIComponent(status)}` : "/api/admin/orgs";

  // PortalShell already gates admin access via /api/me/portal-access,
  // but defend in depth: a session whose JWT doesn't carry
  // `isPlatformAdmin` (e.g., admin granted in the DB after the
  // session was issued) can pass the portal check via membership but
  // still fail RequireAdmin on Go's admin endpoints. Show a clean
  // card instead of throwing the raw ApiError.
  let orgs: Org[];
  try {
    orgs = await api<Org[]>(path);
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
                Your session doesn&apos;t have platform-admin privileges. If
                you were just granted the role, sign out and back in to
                refresh your session.
              </p>
              <p>
                If you don&apos;t expect to be a platform admin, return to{" "}
                <a href="/" className="underline text-primary">your dashboard</a>.
              </p>
            </CardContent>
          </Card>
        </div>
      );
    }
    throw e; // surface unexpected errors
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Organizations</h1>
        <div className="flex gap-2 text-sm">
          <a href="/admin/orgs" className={`px-2 py-1 rounded ${!status ? "bg-primary text-primary-foreground" : "text-muted-foreground"}`}>All</a>
          <a href="/admin/orgs?status=pending" className={`px-2 py-1 rounded ${status === "pending" ? "bg-primary text-primary-foreground" : "text-muted-foreground"}`}>Pending</a>
          <a href="/admin/orgs?status=active" className={`px-2 py-1 rounded ${status === "active" ? "bg-primary text-primary-foreground" : "text-muted-foreground"}`}>Active</a>
          <a href="/admin/orgs?status=suspended" className={`px-2 py-1 rounded ${status === "suspended" ? "bg-primary text-primary-foreground" : "text-muted-foreground"}`}>Suspended</a>
        </div>
      </div>

      {orgs.length === 0 ? (
        <p className="text-muted-foreground">No organizations found.</p>
      ) : (
        <div className="space-y-3">
          {orgs.map((org) => (
            <Card key={org.id}>
              <CardContent className="flex items-center justify-between py-4">
                <div>
                  <p className="font-medium">{org.name}</p>
                  <p className="text-sm text-muted-foreground">
                    {org.type} · {org.contactEmail} · {org.slug}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  <span className={`text-xs px-2 py-1 rounded ${
                    org.status === "active" ? "bg-green-100 text-green-700" :
                    org.status === "pending" ? "bg-yellow-100 text-yellow-700" :
                    "bg-red-100 text-red-700"
                  }`}>
                    {org.status}
                  </span>
                  <OrgActions orgId={org.id} status={org.status} />
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}

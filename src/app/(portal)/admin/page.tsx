import { api, ApiError } from "@/lib/api-client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import Link from "next/link";

interface AdminStatsResponse {
  pendingOrgs: number;
  activeOrgs: number;
  totalUsers: number;
}

export default async function AdminDashboard() {
  let stats: AdminStatsResponse | null = null;
  let errorStatus: number | null = null;
  let errorMessage: string | null = null;

  try {
    stats = await api<AdminStatsResponse>("/api/admin/stats");
  } catch (err) {
    if (err instanceof ApiError) {
      errorStatus = err.status;
      errorMessage = err.message;
    } else {
      errorMessage = err instanceof Error ? err.message : String(err);
    }
  }

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">Platform Admin</h1>
      {stats ? (
        <div className="grid gap-4 md:grid-cols-3">
          <Card>
            <CardHeader><CardTitle className="text-sm text-muted-foreground">Pending Organizations</CardTitle></CardHeader>
            <CardContent>
              <p className="text-3xl font-bold">{stats.pendingOrgs}</p>
              {stats.pendingOrgs > 0 && (
                <Link href="/admin/orgs?status=pending" className="text-sm text-primary mt-2 block">
                  Review pending →
                </Link>
              )}
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle className="text-sm text-muted-foreground">Active Organizations</CardTitle></CardHeader>
            <CardContent><p className="text-3xl font-bold">{stats.activeOrgs}</p></CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle className="text-sm text-muted-foreground">Total Users</CardTitle></CardHeader>
            <CardContent><p className="text-3xl font-bold">{stats.totalUsers}</p></CardContent>
          </Card>
        </div>
      ) : (
        <Card className="border-destructive/50">
          <CardHeader>
            <CardTitle className="text-destructive">
              Couldn&rsquo;t load platform stats{errorStatus ? ` (HTTP ${errorStatus})` : ""}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-3 text-sm">
            <p className="text-muted-foreground">{errorMessage ?? "Unknown error."}</p>
            {errorStatus === 403 && (
              <p>
                The Go API rejected this admin request. If you just signed in,
                check <code>/api/auth/debug</code> to confirm both layers
                resolved the same identity.
              </p>
            )}
            <div className="flex gap-3 pt-2">
              <Link href="/admin" className="text-primary underline">
                Retry
              </Link>
              <Link href="/admin/orgs" className="text-primary underline">
                Open Organizations
              </Link>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}

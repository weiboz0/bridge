import { api, ApiError } from "@/lib/api-client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import Link from "next/link";

interface AdminStatsResponse {
  pendingOrgs: number;
  activeOrgs: number;
  totalUsers: number;
}

interface RealtimeHealthResponse {
  status: "ok" | "degraded";
  goApi: { status: string };
  realtime: {
    tokenMinting: "ok" | "misconfigured";
    hocuspocus: "configured" | "blocked";
    hocuspocusTokenSecret: "set" | "missing";
  };
  bridgeSession: {
    authFlag: "on" | "off";
    secrets: "set" | "missing";
    internalBearer: "set" | "missing";
  };
}

export default async function AdminDashboard() {
  let stats: AdminStatsResponse | null = null;
  let errorStatus: number | null = null;
  let errorMessage: string | null = null;
  let realtimeHealth: RealtimeHealthResponse | null = null;
  let realtimeHealthError: string | null = null;

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

  try {
    realtimeHealth = await api<RealtimeHealthResponse>("/api/health/realtime");
  } catch (err) {
    realtimeHealthError = err instanceof Error ? err.message : String(err);
  }

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">Platform Admin</h1>
      <Card className={realtimeHealth?.status === "ok" ? "" : "border-amber-300"}>
        <CardHeader>
          <CardTitle className="text-sm text-muted-foreground">Realtime health</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          {realtimeHealth ? (
            <>
              <div className="flex flex-wrap gap-2">
                <span
                  className={`inline-flex rounded-md border px-2 py-0.5 text-xs font-medium ${
                    realtimeHealth.status === "ok"
                      ? "border-emerald-200 bg-emerald-50 text-emerald-700"
                      : "border-amber-200 bg-amber-50 text-amber-800"
                  }`}
                >
                  {realtimeHealth.status === "ok" ? "Ready" : "Needs configuration"}
                </span>
                <span className="inline-flex rounded-md border px-2 py-0.5 text-xs text-muted-foreground">
                  Go API {realtimeHealth.goApi.status}
                </span>
                <span className="inline-flex rounded-md border px-2 py-0.5 text-xs text-muted-foreground">
                  Bridge session auth {realtimeHealth.bridgeSession.authFlag}
                </span>
              </div>
              {realtimeHealth.status === "ok" ? (
                <p className="text-muted-foreground">
                  Realtime token minting and the Hocuspocus shared secret are configured.
                </p>
              ) : (
                <p className="text-amber-900">
                  Set <code>HOCUSPOCUS_TOKEN_SECRET</code> on both the Go API and
                  Hocuspocus processes, then reload this page.
                </p>
              )}
            </>
          ) : (
            <p className="text-amber-900">
              Couldn&rsquo;t check realtime health{realtimeHealthError ? `: ${realtimeHealthError}` : "."}
            </p>
          )}
        </CardContent>
      </Card>
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

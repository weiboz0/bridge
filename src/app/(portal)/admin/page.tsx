import { db } from "@/lib/db";
import { countOrganizations } from "@/lib/organizations";
import { countUsers } from "@/lib/users";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";

export default async function AdminDashboard() {
  const [pendingOrgs, activeOrgs, totalUsers] = await Promise.all([
    countOrganizations(db, "pending"),
    countOrganizations(db, "active"),
    countUsers(db),
  ]);

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">Platform Admin</h1>
      <div className="grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader><CardTitle className="text-sm text-muted-foreground">Pending Organizations</CardTitle></CardHeader>
          <CardContent>
            <p className="text-3xl font-bold">{pendingOrgs}</p>
            {pendingOrgs > 0 && (
              <Link href="/admin/orgs?status=pending" className="text-sm text-primary mt-2 block">
                Review pending →
              </Link>
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle className="text-sm text-muted-foreground">Active Organizations</CardTitle></CardHeader>
          <CardContent><p className="text-3xl font-bold">{activeOrgs}</p></CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle className="text-sm text-muted-foreground">Total Users</CardTitle></CardHeader>
          <CardContent><p className="text-3xl font-bold">{totalUsers}</p></CardContent>
        </Card>
      </div>
    </div>
  );
}

import { db } from "@/lib/db";
import { listOrganizations, updateOrgStatus } from "@/lib/organizations";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { revalidatePath } from "next/cache";

export default async function AdminOrgsPage({
  searchParams,
}: {
  searchParams: Promise<{ status?: string }>;
}) {
  const { status } = await searchParams;
  const orgs = await listOrganizations(db, status);

  async function approveOrg(formData: FormData) {
    "use server";
    const { auth: getAuth } = await import("@/lib/auth");
    const session = await getAuth();
    if (!session?.user?.isPlatformAdmin) return;
    const orgId = formData.get("orgId") as string;
    if (!orgId) return;
    await updateOrgStatus(db, orgId, "active");
    revalidatePath("/admin/orgs");
  }

  async function suspendOrg(formData: FormData) {
    "use server";
    const { auth: getAuth } = await import("@/lib/auth");
    const session = await getAuth();
    if (!session?.user?.isPlatformAdmin) return;
    const orgId = formData.get("orgId") as string;
    if (!orgId) return;
    await updateOrgStatus(db, orgId, "suspended");
    revalidatePath("/admin/orgs");
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
                  {org.status === "pending" && (
                    <form action={approveOrg}>
                      <input type="hidden" name="orgId" value={org.id} />
                      <Button size="sm" type="submit">Approve</Button>
                    </form>
                  )}
                  {org.status === "active" && (
                    <form action={suspendOrg}>
                      <input type="hidden" name="orgId" value={org.id} />
                      <Button size="sm" variant="destructive" type="submit">Suspend</Button>
                    </form>
                  )}
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}

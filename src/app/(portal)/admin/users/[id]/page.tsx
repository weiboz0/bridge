import Link from "next/link";
import { redirect } from "next/navigation";
import { api } from "@/lib/api-client";
import { ApiError } from "@/lib/api-error";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

// Plan 085 — read-only platform-admin user detail.
// The list page at /admin/users links here via the "View details" action.
// v1 scope: metadata card + Activity placeholder. No per-user actions here —
// Suspend/Reactivate/Toggle-admin live only on the list-page row dropdown.

interface AdminUser {
  id: string;
  name: string;
  email: string;
  avatarUrl: string | null;
  isPlatformAdmin: boolean;
  status: "active" | "suspended";
  orgRole: string | null;
  orgId: string | null;
  orgName: string | null;
  hasPassword: boolean;
  createdAt: string;
  updatedAt: string;
}

const ROLE_LABELS: Record<string, string> = {
  org_admin: "Org admin",
  teacher: "Teacher",
  student: "Student",
  parent: "Parent",
};

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export default async function AdminUserDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  if (!UUID_RE.test(id)) {
    return (
      <div className="p-6 max-w-2xl space-y-3">
        <BackLink />
        <h1 className="text-2xl font-bold">User not found</h1>
        <p className="text-sm text-muted-foreground">
          <span className="font-mono text-xs">{id}</span> is not a valid user ID.
        </p>
      </div>
    );
  }

  let user: AdminUser;
  try {
    user = await api<AdminUser>(`/api/admin/users/${id}`);
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
                  Your session doesn&apos;t have platform-admin privileges. If
                  you were just granted the role, sign out and back in to
                  refresh your session.
                </p>
                <p>
                  If you don&apos;t expect to be a platform admin, return to{" "}
                  <Link href="/" className="underline text-primary">your dashboard</Link>.
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
            <h1 className="text-2xl font-bold">User not found</h1>
            <p className="text-sm text-muted-foreground">
              No user with id <span className="font-mono text-xs">{id}</span> exists, or it has
              been deleted.
            </p>
          </div>
        );
      }
    }
    throw e;
  }

  const roleLabel = user.orgRole ? (ROLE_LABELS[user.orgRole] ?? user.orgRole) : "—";

  return (
    <div className="p-6 space-y-6 max-w-3xl">
      <div className="flex items-center justify-between">
        <BackLink />
      </div>

      <div className="flex items-start justify-between gap-4">
        <h1 className="text-2xl font-bold">{user.name}</h1>
        <span
          className={`shrink-0 inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${
            user.status === "suspended"
              ? "bg-red-50 text-red-700 border-red-200"
              : "bg-emerald-50 text-emerald-700 border-emerald-200"
          }`}
        >
          {user.status === "suspended" ? "Suspended" : "Active"}
        </span>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Metadata</CardTitle>
        </CardHeader>
        <CardContent className="grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
          <Field label="Email" value={user.email} />
          <Field label="Platform admin" value={user.isPlatformAdmin ? "Yes" : "No"} />
          <Field label="Org role" value={roleLabel} />
          <Field label="Org" value={user.orgName ?? "—"} />
          <Field label="Joined" value={new Date(user.createdAt).toLocaleString()} />
          <Field label="Last updated" value={new Date(user.updatedAt).toLocaleString()} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Activity</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            Session history, audit log, and per-user metrics will appear here.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}

function BackLink() {
  return (
    <Link
      href="/admin/users"
      className="text-sm text-primary hover:underline inline-flex items-center gap-1"
    >
      ← Back to users
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

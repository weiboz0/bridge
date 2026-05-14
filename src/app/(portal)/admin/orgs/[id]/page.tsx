import Link from "next/link";
import { redirect } from "next/navigation";
import { api } from "@/lib/api-client";
import { ApiError } from "@/lib/api-error";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { OrgEditTrigger } from "./org-edit-trigger";

// Plan 086 — org detail placeholder page.
// The list page at /admin/orgs links here via clickable org name.
// v1 scope: metadata card (with member counts) + Activity placeholder card.
// Edit button (name + contactName + contactEmail) via OrgEditTrigger client wrapper.

interface AdminOrg {
  id: string;
  name: string;
  slug: string;
  type: string;
  status: "pending" | "active" | "suspended";
  contactEmail: string;
  contactName: string;
  domain: string | null;
  settings: string;
  verifiedAt: string | null;
  createdAt: string;
  updatedAt: string;
  teacherCount: number;
  studentCount: number;
  parentCount: number;
  adminCount: number;
  totalActive: number;
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export default async function AdminOrgDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  if (!UUID_RE.test(id)) {
    return (
      <div className="p-6 max-w-2xl space-y-3">
        <BackLink />
        <h1 className="text-2xl font-bold">Organization not found</h1>
        <p className="text-sm text-muted-foreground">
          <span className="font-mono text-xs">{id}</span> is not a valid organization ID.
        </p>
      </div>
    );
  }

  let org: AdminOrg;
  try {
    org = await api<AdminOrg>(`/api/admin/orgs/${id}`);
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
            <h1 className="text-2xl font-bold">Organization not found</h1>
            <p className="text-sm text-muted-foreground">
              No organization with id <span className="font-mono text-xs">{id}</span> exists, or it
              has been deleted.
            </p>
          </div>
        );
      }
    }
    throw e;
  }

  const statusBadgeClass =
    org.status === "active"
      ? "bg-emerald-50 text-emerald-700 border-emerald-200"
      : org.status === "pending"
      ? "bg-yellow-50 text-yellow-700 border-yellow-200"
      : "bg-red-50 text-red-700 border-red-200";

  const statusLabel =
    org.status === "active" ? "Active" : org.status === "pending" ? "Pending" : "Suspended";

  const membersSummary =
    org.totalActive === 0
      ? "No active members yet"
      : `${org.teacherCount} teachers · ${org.studentCount} students · ${org.parentCount} parents · ${org.adminCount} org admins · ${org.totalActive} active total`;

  return (
    <div className="p-6 space-y-6 max-w-3xl">
      <div className="flex items-center justify-between">
        <BackLink />
        <OrgEditTrigger
          org={{
            id: org.id,
            name: org.name,
            contactName: org.contactName,
            contactEmail: org.contactEmail,
          }}
        />
      </div>

      <div className="flex items-start justify-between gap-4">
        <h1 className="text-2xl font-bold">{org.name}</h1>
        <span
          className={`shrink-0 inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${statusBadgeClass}`}
        >
          {statusLabel}
        </span>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Metadata</CardTitle>
        </CardHeader>
        <CardContent className="grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
          <Field label="Type" value={org.type} />
          <Field label="Slug" value={org.slug} />
          <Field label="Contact name" value={org.contactName || "—"} />
          <Field label="Contact email" value={org.contactEmail || "—"} />
          <Field label="Joined" value={new Date(org.createdAt).toLocaleString()} />
          <Field label="Last updated" value={new Date(org.updatedAt).toLocaleString()} />
          <div className="col-span-2 space-y-0.5">
            <div className="text-xs text-muted-foreground">Members</div>
            <div>{membersSummary}</div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Activity</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            Session volume, recent admin actions, and per-org metrics will appear here.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}

function BackLink() {
  return (
    <Link
      href="/admin/orgs"
      className="text-sm text-primary hover:underline inline-flex items-center gap-1"
    >
      ← Back to organizations
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

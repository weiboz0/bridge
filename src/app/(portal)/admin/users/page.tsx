import Link from "next/link";
import { redirect } from "next/navigation";
import { api } from "@/lib/api-client";
import { ApiError } from "@/lib/api-error";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { UserActions } from "@/components/admin/user-actions";
import { OrgFilterSelect } from "@/components/admin/org-filter-select";

interface UserItem {
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

interface AdminOrg {
  id: string;
  name: string;
}

interface IdentityResponse {
  userId: string;
  status?: string;
}

const ROLE_LABELS: Record<string, string> = {
  org_admin: "Org admin",
  teacher: "Teacher",
  student: "Student",
  parent: "Parent",
};

const ROLE_CHIPS = [
  { label: "All", value: undefined as string | undefined },
  { label: "Org admin", value: "org_admin" },
  { label: "Teacher", value: "teacher" },
  { label: "Student", value: "student" },
  { label: "Parent", value: "parent" },
  { label: "Platform admin", value: "platform_admin" },
  { label: "Unassigned", value: "unassigned" },
];

function FilterChip({
  current,
  value,
  orgId,
  children,
}: {
  current: string | undefined;
  value: string | undefined;
  orgId: string | undefined;
  children: React.ReactNode;
}) {
  const isActive = current === value;
  const params = new URLSearchParams();
  if (value) params.set("role", value);
  if (orgId) params.set("orgId", orgId);
  const href = `/admin/users${params.toString() ? `?${params.toString()}` : ""}`;

  return (
    <a
      href={href}
      className={`px-2 py-1 rounded text-sm ${
        isActive
          ? "bg-primary text-primary-foreground"
          : "text-muted-foreground hover:text-foreground"
      }`}
    >
      {children}
    </a>
  );
}

export default async function AdminUsersPage({
  searchParams,
}: {
  searchParams: Promise<{ role?: string; orgId?: string }>;
}) {
  const { role, orgId } = await searchParams;

  // Build the users API path with filters.
  const usersParams = new URLSearchParams();
  if (role) usersParams.set("role", role);
  if (orgId) usersParams.set("orgId", orgId);
  const usersPath = `/api/admin/users${usersParams.toString() ? `?${usersParams.toString()}` : ""}`;

  let identity: IdentityResponse;
  let userList: UserItem[];
  let orgs: AdminOrg[];

  try {
    [identity, userList, orgs] = await Promise.all([
      api<IdentityResponse>("/api/me/identity"),
      api<UserItem[]>(usersPath),
      api<AdminOrg[]>("/api/admin/orgs?status=active"),
    ]);
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
                <Link href="/" className="underline text-primary">your dashboard</Link>.
              </p>
            </CardContent>
          </Card>
        </div>
      );
    }
    throw e;
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Users ({userList.length})</h1>
      </div>

      {/* Filter row: role chips on the left, org select pinned right */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex gap-2 text-sm flex-wrap">
          {ROLE_CHIPS.map((chip) => (
            <FilterChip
              key={chip.value ?? "__all__"}
              current={role}
              value={chip.value}
              orgId={orgId}
            >
              {chip.label}
            </FilterChip>
          ))}
        </div>
        <OrgFilterSelect orgs={orgs} current={orgId ?? null} role={role ?? null} />
      </div>

      <div className="border rounded-lg overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-muted/50">
            <tr>
              <th className="text-left px-4 py-2">Name</th>
              <th className="text-left px-4 py-2">Email</th>
              <th className="text-left px-4 py-2">Role</th>
              <th className="text-left px-4 py-2">Org</th>
              <th className="text-left px-4 py-2">Admin</th>
              <th className="text-left px-4 py-2">Status</th>
              <th className="text-left px-4 py-2">Joined</th>
              <th className="text-right px-4 py-2">Actions</th>
            </tr>
          </thead>
          <tbody>
            {userList.map((user) => (
              <tr key={user.id} className="border-t">
                <td className="px-4 py-2">{user.name}</td>
                <td className="px-4 py-2 text-muted-foreground">{user.email}</td>
                <td className="px-4 py-2">
                  {user.orgRole ? (ROLE_LABELS[user.orgRole] ?? user.orgRole) : "—"}
                </td>
                <td className="px-4 py-2">{user.orgName ?? "—"}</td>
                <td className="px-4 py-2">{user.isPlatformAdmin ? "Yes" : ""}</td>
                <td className="px-4 py-2">
                  {user.status === "suspended" ? (
                    <span className="inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium bg-red-100 text-red-700">
                      Suspended
                    </span>
                  ) : null}
                </td>
                <td className="px-4 py-2 text-muted-foreground">
                  {new Date(user.createdAt).toLocaleDateString()}
                </td>
                <td className="px-4 py-2 text-right">
                  <UserActions
                    userId={user.id}
                    userName={user.name}
                    status={user.status}
                    isPlatformAdmin={user.isPlatformAdmin}
                    isSelf={user.id === identity.userId}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

"use client";

import { useRouter } from "next/navigation";
import type { UserRole, PortalRole } from "@/lib/portal/types";
import { getPortalPath } from "@/lib/portal/roles";

interface RoleSwitcherProps {
  roles: UserRole[];
  currentRole: string;
  collapsed: boolean;
}

const labels: Record<string, string> = {
  admin: "Admin",
  org_admin: "Org Admin",
  teacher: "Teacher",
  student: "Student",
  parent: "Parent",
};

// Plan 043 phase 3.4: a user with `org_admin` in two different orgs
// previously produced duplicate React keys here. Composite key on
// (role, orgId) makes each row unique.
function rowKey(r: UserRole): string {
  return `${r.role}:${r.orgId ?? "none"}`;
}

// Plan 043 phase 3.4: when switching to an org-scoped role, carry the
// orgId so the destination loads the correct org context immediately
// instead of falling back to the user's first org_admin membership.
function destinationFor(r: UserRole): string {
  const path = getPortalPath(r.role as PortalRole);
  if (!r.orgId) return path;
  const sep = path.includes("?") ? "&" : "?";
  return `${path}${sep}orgId=${encodeURIComponent(r.orgId)}`;
}

export function RoleSwitcher({ roles, currentRole, collapsed }: RoleSwitcherProps) {
  const router = useRouter();

  if (roles.length <= 1) return null;

  function switchRole(r: UserRole) {
    router.push(destinationFor(r));
  }

  if (collapsed) {
    return (
      <div className="px-2 py-1">
        {roles.map((r) => (
          <button
            key={rowKey(r)}
            onClick={() => switchRole(r)}
            className={`block w-full text-center text-xs py-1 rounded ${
              r.role === currentRole
                ? "bg-primary/10 text-primary"
                : "text-muted-foreground hover:text-foreground"
            }`}
            title={r.orgName ? `${labels[r.role]} · ${r.orgName}` : labels[r.role]}
          >
            {labels[r.role]?.[0] || "?"}
          </button>
        ))}
      </div>
    );
  }

  return (
    <div className="px-4 py-2 border-b border-border/50">
      <p className="text-xs text-muted-foreground mb-1">Switch role</p>
      <div className="flex flex-wrap gap-1">
        {roles.map((r) => (
          <button
            key={rowKey(r)}
            onClick={() => switchRole(r)}
            className={`text-xs px-2 py-1 rounded transition-colors ${
              r.role === currentRole
                ? "bg-primary text-primary-foreground"
                : "bg-muted text-muted-foreground hover:text-foreground"
            }`}
            title={r.orgName ? `${labels[r.role]} · ${r.orgName}` : labels[r.role]}
          >
            {labels[r.role]}
            {r.orgName && roles.filter((o) => o.role === r.role).length > 1 ? (
              <span className="ml-1 text-[10px] opacity-75">{r.orgName}</span>
            ) : null}
          </button>
        ))}
      </div>
    </div>
  );
}

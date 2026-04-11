"use client";

import { useRouter } from "next/navigation";
import type { UserRole } from "@/lib/portal/types";
import { getPortalPath } from "@/lib/portal/roles";

interface RoleSwitcherProps {
  roles: UserRole[];
  currentRole: string;
  collapsed: boolean;
}

export function RoleSwitcher({ roles, currentRole, collapsed }: RoleSwitcherProps) {
  const router = useRouter();

  if (roles.length <= 1) return null;

  const labels: Record<string, string> = {
    admin: "Admin",
    org_admin: "Org Admin",
    teacher: "Teacher",
    student: "Student",
    parent: "Parent",
  };

  function switchRole(role: string) {
    const path = getPortalPath(role as any);
    router.push(path);
  }

  if (collapsed) {
    return (
      <div className="px-2 py-1">
        {roles.map((r) => (
          <button
            key={r.role}
            onClick={() => switchRole(r.role)}
            className={`block w-full text-center text-xs py-1 rounded ${
              r.role === currentRole
                ? "bg-primary/10 text-primary"
                : "text-muted-foreground hover:text-foreground"
            }`}
            title={labels[r.role]}
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
            key={r.role}
            onClick={() => switchRole(r.role)}
            className={`text-xs px-2 py-1 rounded transition-colors ${
              r.role === currentRole
                ? "bg-primary text-primary-foreground"
                : "bg-muted text-muted-foreground hover:text-foreground"
            }`}
          >
            {labels[r.role]}
          </button>
        ))}
      </div>
    </div>
  );
}

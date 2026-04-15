import { api } from "@/lib/api-client";
import { getPortalConfig } from "@/lib/portal/nav-config";
import { Sidebar } from "./sidebar";
import { redirect } from "next/navigation";
import type { PortalRole } from "@/lib/portal/types";
import type { UserRole } from "@/lib/portal/types";

interface PortalAccessResponse {
  authorized: boolean;
  userName: string;
  roles: UserRole[];
  currentRole: { role: string; orgId?: string; orgName?: string } | null;
}

interface PortalShellProps {
  portalRole: PortalRole;
  children: React.ReactNode;
}

export async function PortalShell({ portalRole, children }: PortalShellProps) {
  let data: PortalAccessResponse;
  try {
    data = await api<PortalAccessResponse>("/api/me/portal-access");
  } catch {
    redirect("/login");
  }

  if (!data.authorized) {
    redirect("/login");
  }

  // Check if user has the required role for this portal
  const hasRole = data.roles.some((r) => r.role === portalRole);
  if (!hasRole) {
    redirect("/");
  }

  const config = getPortalConfig(portalRole);
  if (!config) {
    redirect("/");
  }

  return (
    <div className="flex min-h-screen">
      <Sidebar
        navItems={config.navItems}
        userName={data.userName || "User"}
        roles={data.roles}
        currentRole={portalRole}
      />
      <main className="flex-1 min-h-screen pb-16 md:pb-0">
        {children}
      </main>
    </div>
  );
}

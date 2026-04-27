import { api, ApiError } from "@/lib/api-client";
import { getPortalConfig } from "@/lib/portal/nav-config";
import { Sidebar } from "./sidebar";
import { redirect } from "next/navigation";
import { headers } from "next/headers";
import type { PortalRole, UserRole } from "@/lib/portal/types";

interface PortalAccessResponse {
  authorized: boolean;
  userName: string;
  roles: UserRole[];
}

interface PortalShellProps {
  portalRole: PortalRole;
  children: React.ReactNode;
}

export async function PortalShell({ portalRole, children }: PortalShellProps) {
  // Capture the current path so we can redirect back after login
  const hdrs = await headers();
  const currentPath = hdrs.get("x-invoke-path") || hdrs.get("x-url") || "";

  function loginRedirect(): never {
    const loginUrl = currentPath
      ? `/login?callbackUrl=${encodeURIComponent(currentPath)}`
      : "/login";
    redirect(loginUrl);
  }

  let data: PortalAccessResponse;
  try {
    data = await api<PortalAccessResponse>("/api/me/portal-access");
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) {
      loginRedirect();
    }
    throw e; // surface infrastructure errors
  }

  if (!data.authorized) {
    loginRedirect();
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

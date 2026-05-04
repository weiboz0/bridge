import { api, ApiError } from "@/lib/api-client";
import { getPortalConfig } from "@/lib/portal/nav-config";
import { Sidebar } from "./sidebar";
import { redirect } from "next/navigation";
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
  // Plan 040 phase 5: middleware (`src/middleware.ts` + the `authorized`
  // callback in `src/lib/auth.ts`) handles unauthenticated portal access
  // — redirects to /login?callbackUrl=<original> before this component
  // ever renders. The fallbacks below stay as defense-in-depth for the
  // unlikely case middleware didn't run; they redirect to /login with no
  // callbackUrl because by the time we're here the original URL is no
  // longer trivially recoverable from the server-component context.

  let data: PortalAccessResponse;
  try {
    data = await api<PortalAccessResponse>("/api/me/portal-access");
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) {
      redirect("/login");
    }
    throw e; // surface infrastructure errors
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

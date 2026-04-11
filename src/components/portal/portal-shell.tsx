import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getUserMemberships } from "@/lib/org-memberships";
import { buildUserRoles, isAuthorizedForPortal } from "@/lib/portal/roles";
import { getPortalConfig } from "@/lib/portal/nav-config";
import { Sidebar } from "./sidebar";
import { redirect } from "next/navigation";
import type { PortalRole } from "@/lib/portal/types";

interface PortalShellProps {
  portalRole: PortalRole;
  children: React.ReactNode;
}

export async function PortalShell({ portalRole, children }: PortalShellProps) {
  const session = await auth();

  if (!session?.user?.id) {
    redirect("/login");
  }

  const memberships = await getUserMemberships(db, session.user.id);
  const roles = buildUserRoles(session.user.isPlatformAdmin, memberships);

  if (!isAuthorizedForPortal(roles, portalRole)) {
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
        userName={session.user.name || "User"}
        roles={roles}
        currentRole={portalRole}
      />
      <main className="flex-1 min-h-screen pb-16 md:pb-0">
        {children}
      </main>
    </div>
  );
}

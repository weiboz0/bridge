"use client";

import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import { useSidebar } from "@/lib/hooks/use-sidebar";
import { useSidebarSections, sectionKey } from "@/lib/hooks/use-sidebar-sections";
import { SidebarHeader } from "./sidebar-header";
import { SidebarSection } from "./sidebar-section";
import { SidebarFooter } from "./sidebar-footer";
import { getIconChar } from "@/lib/portal/icons";
import { getPortalConfig } from "@/lib/portal/nav-config";
import { getPrimaryRole } from "@/lib/portal/roles";
import type { UserRole } from "@/lib/portal/types";

interface SidebarProps {
  userName: string;
  roles: UserRole[];
  // Kept for backward compatibility with `<PortalShell />`. Plan 067
  // doesn't actually use this for the sectioned-nav rendering — the
  // active section is computed from `usePathname()` + `useSearchParams()`
  // — but PortalShell still passes it for theming/breadcrumb hooks
  // outside this component.
  currentRole: string;
}

const ROLE_LABELS: Record<string, string> = {
  admin: "Platform Admin",
  org_admin: "Organization",
  teacher: "Teacher",
  student: "Student",
  parent: "Parent",
};

// Plan 067 — Sidebar renders one SidebarSection per UserRole.
// RoleSwitcher is gone; the user sees ALL their accessible sections
// at once, with the section that matches the current URL auto-expanded.
//
// Active-section detection (Decisions §1):
//   - For non-org-scoped roles (admin/teacher/student/parent): match
//     by basePath only (`pathname.startsWith(config.basePath)`).
//   - For org_admin: match by basePath === "/org" AND
//     role.orgId === searchParams.get("orgId"). Without orgId in the
//     URL, no org_admin section is auto-expanded — the user is
//     presumed to be on a non-org-specific page.

export function Sidebar({ userName, roles, currentRole: _currentRole }: SidebarProps) {
  const { collapsed, toggle } = useSidebar();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const urlOrgId = searchParams.get("orgId");

  const activeKey = computeActiveKey(pathname, urlOrgId, roles);
  const { isExpanded, toggle: toggleSection } = useSidebarSections(activeKey);

  // Build (role, config) tuples to render.
  const sections = roles
    .map((role) => ({ role, config: getPortalConfig(role.role) }))
    .filter(
      (s): s is { role: UserRole; config: NonNullable<ReturnType<typeof getPortalConfig>> } =>
        s.config !== null
    );

  const multiRole = sections.length > 1;

  // Mobile bottom-nav uses the primary role's nav items (Decisions §4)
  // — uniform across portals so the user sees the same 4-icon strip
  // regardless of which page they're on.
  const primary = getPrimaryRole(roles);
  const primaryConfig = primary ? getPortalConfig(primary.role) : null;
  const mobileItems = primaryConfig?.navItems.slice(0, 4) ?? [];

  return (
    <>
      {/* Desktop sidebar */}
      <aside
        className={`hidden md:flex flex-col h-screen fixed left-0 top-0 bg-background border-r border-border transition-all duration-200 z-40 ${
          collapsed ? "w-14" : "w-56"
        }`}
      >
        <SidebarHeader collapsed={collapsed} onToggle={toggle} />
        <div className="flex-1 overflow-auto">
          {sections.map(({ role, config }) => {
            const key = sectionKey(role.role, role.orgId);
            return (
              <SidebarSection
                key={key}
                role={role}
                label={ROLE_LABELS[role.role] ?? config.label}
                navItems={config.navItems}
                collapsed={collapsed}
                expanded={isExpanded(key)}
                multiRole={multiRole}
                onToggle={() => toggleSection(key)}
              />
            );
          })}
        </div>
        <SidebarFooter userName={userName} collapsed={collapsed} />
      </aside>

      {/* Mobile bottom nav — uses primary role's nav items uniformly.
          Active item highlighted via the same pathname-prefix match as
          the desktop sidebar so users can locate themselves at a
          glance on small screens. */}
      <nav className="md:hidden fixed bottom-0 left-0 right-0 bg-background border-t border-border z-40 flex justify-around py-2">
        {mobileItems.map((item) => {
          const itemPath = item.href.split("?")[0];
          const isActive =
            pathname === itemPath || pathname.startsWith(itemPath + "/");
          return (
            <Link
              key={item.href}
              href={item.href}
              aria-current={isActive ? "page" : undefined}
              className={`flex flex-col items-center text-xs transition-colors ${
                isActive
                  ? "text-primary font-medium"
                  : "text-muted-foreground hover:text-foreground"
              }`}
            >
              <span>{getIconChar(item.icon)}</span>
              <span>{item.label}</span>
            </Link>
          );
        })}
      </nav>

      {/* Spacer for main content */}
      <div className={`hidden md:block shrink-0 transition-all duration-200 ${collapsed ? "w-14" : "w-56"}`} />
    </>
  );
}

// computeActiveKey returns the sectionKey that matches the current URL,
// or null when no section matches (e.g., user is on an org_admin page
// without an orgId in the URL).
function computeActiveKey(pathname: string, urlOrgId: string | null, roles: UserRole[]): string | null {
  for (const role of roles) {
    const config = getPortalConfig(role.role);
    if (!config) continue;
    const onPortalPath = pathname === config.basePath || pathname.startsWith(config.basePath + "/");
    if (!onPortalPath) continue;

    if (role.role === "org_admin") {
      // Multi-org disambiguation. Plan 067 Decisions §1 says: with no
      // URL orgId, no org_admin section auto-expands. Practical
      // refinement: when the user has only ONE org_admin membership,
      // the disambiguation isn't needed and not auto-expanding feels
      // broken (the section just sits collapsed on the org's own
      // pages). For the multi-org case the strict rule still holds.
      if (urlOrgId && role.orgId === urlOrgId) {
        return sectionKey(role.role, role.orgId);
      }
      const orgAdminCount = roles.filter((r) => r.role === "org_admin").length;
      if (orgAdminCount === 1) {
        return sectionKey(role.role, role.orgId);
      }
    } else {
      return sectionKey(role.role, role.orgId);
    }
  }
  return null;
}

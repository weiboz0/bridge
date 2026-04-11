"use client";

import Link from "next/link";
import { useSidebar } from "@/lib/hooks/use-sidebar";
import { SidebarHeader } from "./sidebar-header";
import { SidebarNav } from "./sidebar-nav";
import { SidebarFooter } from "./sidebar-footer";
import { RoleSwitcher } from "./role-switcher";
import { getIconChar } from "@/lib/portal/icons";
import type { NavItem, UserRole } from "@/lib/portal/types";

interface SidebarProps {
  navItems: NavItem[];
  userName: string;
  roles: UserRole[];
  currentRole: string;
}

export function Sidebar({ navItems, userName, roles, currentRole }: SidebarProps) {
  const { collapsed, toggle } = useSidebar();

  return (
    <>
      {/* Desktop sidebar */}
      <aside
        className={`hidden md:flex flex-col h-screen fixed left-0 top-0 bg-background border-r border-border transition-all duration-200 z-40 ${
          collapsed ? "w-14" : "w-56"
        }`}
      >
        <SidebarHeader collapsed={collapsed} onToggle={toggle} />
        <RoleSwitcher roles={roles} currentRole={currentRole} collapsed={collapsed} />
        <SidebarNav items={navItems} collapsed={collapsed} />
        <SidebarFooter userName={userName} collapsed={collapsed} />
      </aside>

      {/* Mobile bottom nav */}
      <nav className="md:hidden fixed bottom-0 left-0 right-0 bg-background border-t border-border z-40 flex justify-around py-2">
        {navItems.slice(0, 4).map((item) => (
          <Link
            key={item.href}
            href={item.href}
            className="flex flex-col items-center text-xs text-muted-foreground"
          >
            <span>{getIconChar(item.icon)}</span>
            <span>{item.label}</span>
          </Link>
        ))}
      </nav>

      {/* Spacer for main content */}
      <div className={`hidden md:block shrink-0 transition-all duration-200 ${collapsed ? "w-14" : "w-56"}`} />
    </>
  );
}

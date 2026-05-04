"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { NavItem, UserRole } from "@/lib/portal/types";
import { getIconChar } from "@/lib/portal/icons";
import { findActiveIndex } from "@/lib/portal/active-match";

interface SidebarSectionProps {
  role: UserRole;
  label: string;
  navItems: NavItem[];
  collapsed: boolean; // sidebar (icon-only) collapsed
  expanded: boolean; // section expanded
  multiRole: boolean; // true when the user has more than one section to render
  onToggle: () => void;
}

// Plan 067 — one collapsible section per UserRole.
//
// When the user has only one role, the section header is hidden
// (renders just the nav items, preserving the pre-067 look for the
// common case). When the sidebar itself is icon-collapsed, the
// section renders as a vertical icon strip with hover tooltips —
// the role label moves into the tooltip.

export function SidebarSection({
  role,
  label,
  navItems,
  collapsed,
  expanded,
  multiRole,
  onToggle,
}: SidebarSectionProps) {
  const pathname = usePathname();

  // Plan 067 §"Org context preservation": for org-scoped roles, each
  // nav item's href carries the role's orgId so navigation lands in
  // the correct org context immediately. Non-org-scoped sections
  // pass items through unchanged.
  const itemsWithOrgContext = role.orgId
    ? navItems.map((item) => {
        const sep = item.href.includes("?") ? "&" : "?";
        return { ...item, href: `${item.href}${sep}orgId=${encodeURIComponent(role.orgId!)}` };
      })
    : navItems;

  // Single-role users get the pre-067 look: no header, items render
  // directly. Multi-role users get a collapsible group with the role
  // label as the header.
  if (!multiRole) {
    return <SectionItems items={itemsWithOrgContext} collapsed={collapsed} pathname={pathname} />;
  }

  // Build the header label. For org-scoped roles with multiple sections
  // for the same role at different orgs, append the org name to
  // disambiguate.
  const headerLabel = role.orgName ? `${label} · ${role.orgName}` : label;

  if (collapsed) {
    // Icon-only: render the items as a vertical strip with tooltips.
    // Skip the header — there's no room for chevron + label.
    return (
      <div className="border-b border-border/30 py-1" title={headerLabel}>
        <SectionItems items={itemsWithOrgContext} collapsed={true} pathname={pathname} />
      </div>
    );
  }

  return (
    <div className="border-b border-border/30">
      <button
        type="button"
        onClick={onToggle}
        className="w-full flex items-center justify-between px-4 py-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground hover:text-foreground transition-colors"
        aria-expanded={expanded}
      >
        <span className="truncate">{headerLabel}</span>
        <span className="text-[10px] opacity-60">{expanded ? "▾" : "▸"}</span>
      </button>
      {expanded && <SectionItems items={itemsWithOrgContext} collapsed={false} pathname={pathname} />}
    </div>
  );
}

interface SectionItemsProps {
  items: NavItem[];
  collapsed: boolean;
  pathname: string;
}

// Inline replacement for SidebarNav — same render shape, but as an
// internal helper so the section can wrap it without an extra div.
function SectionItems({ items, collapsed, pathname }: SectionItemsProps) {
  // Longest-match wins (Codex review of plan 067 phases 2+3): the
  // naive `startsWith(itemPath + "/")` check would highlight both
  // "Dashboard" (/teacher) and "Units" (/teacher/units) on
  // /teacher/units. Compute a single active index per render.
  const activeIndex = findActiveIndex(pathname, items);
  return (
    <nav className="py-1">
      {items.map((item, i) => {
        const isActive = i === activeIndex;
        return (
          <Link
            key={item.href}
            href={item.href}
            aria-current={isActive ? "page" : undefined}
            className={`flex items-center gap-3 px-4 py-2 text-sm transition-colors ${
              isActive
                ? "bg-primary/10 text-primary font-medium"
                : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
            }`}
            title={collapsed ? item.label : undefined}
          >
            <span className="w-5 h-5 shrink-0 text-center">{getIconChar(item.icon)}</span>
            {!collapsed && <span>{item.label}</span>}
          </Link>
        );
      })}
    </nav>
  );
}

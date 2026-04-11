# Portal Shell Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the portal layout shell — the sidebar navigation, theming infrastructure, route groups, role detection/routing, and role switcher that all portal pages sit inside. This plan does NOT include portal-specific pages (those come in plan 010).

**Architecture:** Each portal (admin, teacher, student, parent) gets a Next.js route group with its own `layout.tsx` that wraps a shared `PortalShell` component, passing role-specific nav config. The `PortalShell` renders a collapsible sidebar with branding, nav links, role switcher, theme toggle, and user profile. Theming uses CSS custom properties with a `dark` class on `<html>`, persisted in localStorage with an inline script to prevent FOUC. Role detection happens server-side: on login redirect, a middleware-like page reads the user's OrgMemberships and `isPlatformAdmin` flag to route them to the correct portal. Users with multiple roles see a dropdown to switch between portals.

**Tech Stack:** Next.js 16 App Router, React Server Components + Client Components, Tailwind CSS v4 with CSS variables, lucide-react icons, localStorage for preferences, Vitest + Testing Library

**Depends on:** Plan 006 (org-and-roles) — assumes `organizations`, `orgMemberships` tables and `getUserMemberships()` exist. Plan 007 (course-hierarchy) — assumes `classes`, `classMemberships` tables exist.

**Key constraints:**
- shadcn/ui uses `@base-ui/react` — NO `asChild` prop on Button, use `buttonVariants()` with `<Link>` or `<a>` instead
- Tailwind CSS v4 with `@tailwindcss/postcss` — `@custom-variant dark` already defined in globals.css
- Auth.js v5 session has `user.id`, `user.name`, `user.email`, `user.isPlatformAdmin`
- `fileParallelism: false` in Vitest — `.tsx` tests need `// @vitest-environment jsdom`
- Geist + Geist_Mono fonts already configured in root layout

---

## File Structure

```
src/
├── lib/
│   ├── portal/
│   │   ├── nav-config.ts              # Create: per-portal nav item definitions
│   │   ├── roles.ts                   # Create: role detection + primary portal logic
│   │   └── types.ts                   # Create: shared portal types (NavItem, PortalRole, etc.)
│   └── hooks/
│       ├── use-sidebar.ts             # Create: sidebar collapse state hook
│       └── use-theme.ts               # Create: theme toggle hook
├── components/
│   ├── portal/
│   │   ├── portal-shell.tsx           # Create: main shell layout (server wrapper)
│   │   ├── sidebar.tsx                # Create: sidebar client component
│   │   ├── sidebar-nav.tsx            # Create: nav link list
│   │   ├── sidebar-header.tsx         # Create: branding + collapse toggle
│   │   ├── sidebar-footer.tsx         # Create: user profile + theme toggle + sign out
│   │   ├── role-switcher.tsx          # Create: role switching dropdown
│   │   ├── theme-toggle.tsx           # Create: light/dark toggle button
│   │   └── mobile-overlay.tsx         # Create: mobile sidebar overlay
│   └── portal/
│       (above)
├── app/
│   ├── layout.tsx                     # Modify: add inline FOUC-prevention script
│   ├── page.tsx                       # Modify: redirect logic (roles → portal, no roles → onboarding)
│   ├── (portal)/
│   │   ├── admin/
│   │   │   └── layout.tsx             # Create: admin portal layout
│   │   ├── teacher/
│   │   │   └── layout.tsx             # Create: teacher portal layout
│   │   ├── student/
│   │   │   └── layout.tsx             # Create: student portal layout
│   │   └── parent/
│   │       └── layout.tsx             # Create: parent portal layout
│   └── onboarding/
│       └── page.tsx                   # Create: unaffiliated user onboarding
tests/
├── unit/
│   ├── portal-roles.test.ts           # Create: role detection logic tests
│   ├── nav-config.test.ts             # Create: nav config tests
│   ├── sidebar.test.tsx               # Create: sidebar component tests
│   ├── role-switcher.test.tsx         # Create: role switcher tests
│   └── theme-toggle.test.tsx          # Create: theme toggle tests
```

---

## Task 1: Portal Types and Nav Configuration

**Files:**
- Create: `src/lib/portal/types.ts`
- Create: `src/lib/portal/nav-config.ts`
- Create: `tests/unit/nav-config.test.ts`

Define the shared types and per-portal navigation configurations.

- [ ] **Step 1: Create portal types**

Create `src/lib/portal/types.ts`:

```typescript
import type { LucideIcon } from "lucide-react";

/**
 * The portal roles that determine which sidebar nav a user sees.
 * "admin" is the platform admin portal (isPlatformAdmin flag).
 * The rest correspond to OrgMembership roles.
 */
export type PortalRole = "admin" | "org_admin" | "teacher" | "student" | "parent";

/**
 * A single navigation item in the sidebar.
 */
export interface NavItem {
  /** Display label */
  label: string;
  /** Route path (relative to portal prefix, e.g., "/teacher/courses") */
  href: string;
  /** Lucide icon name (resolved at render time) */
  icon: string;
  /** Optional badge count (e.g., unread notifications) */
  badge?: number;
}

/**
 * Configuration for a portal — its nav items, display name, and URL prefix.
 */
export interface PortalConfig {
  /** Display name for the portal (e.g., "Teacher Portal") */
  label: string;
  /** URL prefix (e.g., "/teacher") */
  prefix: string;
  /** Navigation items shown in the sidebar */
  navItems: NavItem[];
}

/**
 * A role the current user holds, used for the role switcher.
 */
export interface UserRole {
  role: PortalRole;
  /** Display label (e.g., "Teacher at Lincoln High") */
  label: string;
  /** The URL prefix to navigate to when switching */
  href: string;
}
```

- [ ] **Step 2: Create nav configuration**

Create `src/lib/portal/nav-config.ts`:

```typescript
import type { PortalConfig, PortalRole } from "./types";

const adminConfig: PortalConfig = {
  label: "Platform Admin",
  prefix: "/admin",
  navItems: [
    { label: "Organizations", href: "/admin/organizations", icon: "Building2" },
    { label: "Users", href: "/admin/users", icon: "Users" },
    { label: "System Settings", href: "/admin/settings", icon: "Settings" },
  ],
};

const orgAdminConfig: PortalConfig = {
  label: "Org Admin",
  prefix: "/org",
  navItems: [
    { label: "Dashboard", href: "/org", icon: "LayoutDashboard" },
    { label: "Teachers", href: "/org/teachers", icon: "GraduationCap" },
    { label: "Students", href: "/org/students", icon: "Users" },
    { label: "Courses", href: "/org/courses", icon: "BookOpen" },
    { label: "Classes", href: "/org/classes", icon: "School" },
    { label: "Settings", href: "/org/settings", icon: "Settings" },
  ],
};

const teacherConfig: PortalConfig = {
  label: "Teacher",
  prefix: "/teacher",
  navItems: [
    { label: "Dashboard", href: "/teacher", icon: "LayoutDashboard" },
    { label: "My Courses", href: "/teacher/courses", icon: "BookOpen" },
    { label: "My Classes", href: "/teacher/classes", icon: "School" },
    { label: "Schedule", href: "/teacher/schedule", icon: "Calendar" },
    { label: "Reports", href: "/teacher/reports", icon: "BarChart3" },
  ],
};

const studentConfig: PortalConfig = {
  label: "Student",
  prefix: "/student",
  navItems: [
    { label: "Dashboard", href: "/student", icon: "LayoutDashboard" },
    { label: "My Classes", href: "/student/classes", icon: "School" },
    { label: "My Code", href: "/student/code", icon: "Code" },
    { label: "Help", href: "/student/help", icon: "HelpCircle" },
  ],
};

const parentConfig: PortalConfig = {
  label: "Parent",
  prefix: "/parent",
  navItems: [
    { label: "Dashboard", href: "/parent", icon: "LayoutDashboard" },
    { label: "My Children", href: "/parent/children", icon: "Baby" },
    { label: "Reports", href: "/parent/reports", icon: "BarChart3" },
  ],
};

const portalConfigs: Record<PortalRole, PortalConfig> = {
  admin: adminConfig,
  org_admin: orgAdminConfig,
  teacher: teacherConfig,
  student: studentConfig,
  parent: parentConfig,
};

/**
 * Get the portal configuration for a given role.
 */
export function getPortalConfig(role: PortalRole): PortalConfig {
  return portalConfigs[role];
}

/**
 * Get all portal configurations (for iteration/testing).
 */
export function getAllPortalConfigs(): Record<PortalRole, PortalConfig> {
  return portalConfigs;
}
```

- [ ] **Step 3: Write nav config tests**

Create `tests/unit/nav-config.test.ts`:

```typescript
import { describe, it, expect } from "vitest";
import { getPortalConfig, getAllPortalConfigs } from "@/lib/portal/nav-config";
import type { PortalRole } from "@/lib/portal/types";

describe("nav-config", () => {
  describe("getPortalConfig", () => {
    const roles: PortalRole[] = ["admin", "org_admin", "teacher", "student", "parent"];

    it.each(roles)("returns config for %s role", (role) => {
      const config = getPortalConfig(role);
      expect(config).toBeDefined();
      expect(config.label).toBeTruthy();
      expect(config.prefix).toMatch(/^\//);
      expect(config.navItems.length).toBeGreaterThan(0);
    });

    it("admin config has correct prefix and items", () => {
      const config = getPortalConfig("admin");
      expect(config.prefix).toBe("/admin");
      expect(config.navItems).toHaveLength(3);
      expect(config.navItems.map((i) => i.label)).toEqual([
        "Organizations",
        "Users",
        "System Settings",
      ]);
    });

    it("teacher config has correct prefix and items", () => {
      const config = getPortalConfig("teacher");
      expect(config.prefix).toBe("/teacher");
      expect(config.navItems).toHaveLength(5);
      expect(config.navItems[0].label).toBe("Dashboard");
    });

    it("student config has correct prefix and items", () => {
      const config = getPortalConfig("student");
      expect(config.prefix).toBe("/student");
      expect(config.navItems).toHaveLength(4);
    });

    it("parent config has correct prefix and items", () => {
      const config = getPortalConfig("parent");
      expect(config.prefix).toBe("/parent");
      expect(config.navItems).toHaveLength(3);
    });

    it("org_admin config has correct prefix and items", () => {
      const config = getPortalConfig("org_admin");
      expect(config.prefix).toBe("/org");
      expect(config.navItems).toHaveLength(6);
    });

    it("every nav item has a non-empty icon string", () => {
      const allConfigs = getAllPortalConfigs();
      for (const [, config] of Object.entries(allConfigs)) {
        for (const item of config.navItems) {
          expect(item.icon).toBeTruthy();
          expect(typeof item.icon).toBe("string");
        }
      }
    });

    it("every nav item href starts with its portal prefix", () => {
      const allConfigs = getAllPortalConfigs();
      for (const [, config] of Object.entries(allConfigs)) {
        for (const item of config.navItems) {
          expect(item.href).toMatch(new RegExp(`^${config.prefix}`));
        }
      }
    });
  });

  describe("getAllPortalConfigs", () => {
    it("returns configs for all 5 roles", () => {
      const configs = getAllPortalConfigs();
      expect(Object.keys(configs)).toHaveLength(5);
      expect(Object.keys(configs).sort()).toEqual([
        "admin",
        "org_admin",
        "parent",
        "student",
        "teacher",
      ]);
    });
  });
});
```

- [ ] **Step 4: Run tests, verify pass**

```bash
bun run test -- tests/unit/nav-config.test.ts
```

- [ ] **Step 5: Commit**

```
git add src/lib/portal/types.ts src/lib/portal/nav-config.ts tests/unit/nav-config.test.ts
git commit -m "Add portal types and per-role nav configuration"
```

---

## Task 2: Role Detection and Primary Portal Logic

**Files:**
- Create: `src/lib/portal/roles.ts`
- Create: `tests/unit/portal-roles.test.ts`

Build the logic that determines which portals a user can access and which one is their primary (default) portal.

- [ ] **Step 1: Create role detection module**

Create `src/lib/portal/roles.ts`:

```typescript
import type { PortalRole, UserRole } from "./types";
import { getPortalConfig } from "./nav-config";

/**
 * The shape of a membership record returned by getUserMemberships().
 * We only use the fields we need to avoid coupling to Drizzle types.
 */
interface MembershipRecord {
  role: "org_admin" | "teacher" | "student" | "parent";
  status: "pending" | "active" | "suspended";
  orgName: string;
  orgSlug: string;
  orgStatus: "pending" | "active" | "suspended";
}

/**
 * Priority order for portal roles. Lower index = higher priority.
 * Used to determine the primary (default) portal when a user has multiple roles.
 */
const ROLE_PRIORITY: PortalRole[] = [
  "admin",
  "org_admin",
  "teacher",
  "student",
  "parent",
];

/**
 * Given a user's platform admin flag and org memberships,
 * return the list of portal roles they can access (deduplicated, sorted by priority).
 */
export function detectUserRoles(
  isPlatformAdmin: boolean,
  memberships: MembershipRecord[]
): PortalRole[] {
  const roles = new Set<PortalRole>();

  if (isPlatformAdmin) {
    roles.add("admin");
  }

  for (const m of memberships) {
    // Only count active memberships in active orgs
    if (m.status === "active" && m.orgStatus === "active") {
      roles.add(m.role);
    }
  }

  // Sort by priority
  return ROLE_PRIORITY.filter((r) => roles.has(r));
}

/**
 * Get the primary portal role for a user — the highest-priority role they hold.
 * Returns null if the user has no roles (unaffiliated).
 */
export function getPrimaryRole(roles: PortalRole[]): PortalRole | null {
  if (roles.length === 0) return null;
  // roles are already sorted by priority from detectUserRoles
  return roles[0];
}

/**
 * Get the redirect path for a user's primary portal.
 * Returns "/onboarding" if the user has no roles.
 */
export function getPrimaryPortalPath(roles: PortalRole[]): string {
  const primary = getPrimaryRole(roles);
  if (!primary) return "/onboarding";
  return getPortalConfig(primary).prefix;
}

/**
 * Build the list of UserRole entries for the role switcher dropdown.
 * Each entry includes a display label and the portal URL.
 */
export function buildUserRoles(
  isPlatformAdmin: boolean,
  memberships: MembershipRecord[]
): UserRole[] {
  const result: UserRole[] = [];

  if (isPlatformAdmin) {
    result.push({
      role: "admin",
      label: "Platform Admin",
      href: "/admin",
    });
  }

  // Group memberships by role, collecting org names
  const roleOrgs = new Map<string, string[]>();
  for (const m of memberships) {
    if (m.status === "active" && m.orgStatus === "active") {
      const existing = roleOrgs.get(m.role) || [];
      existing.push(m.orgName);
      roleOrgs.set(m.role, existing);
    }
  }

  // Build user role entries in priority order
  const orgRolePriority: Array<"org_admin" | "teacher" | "student" | "parent"> = [
    "org_admin",
    "teacher",
    "student",
    "parent",
  ];

  for (const role of orgRolePriority) {
    const orgs = roleOrgs.get(role);
    if (orgs && orgs.length > 0) {
      const config = getPortalConfig(role);
      const orgLabel = orgs.length === 1 ? orgs[0] : `${orgs.length} orgs`;
      result.push({
        role,
        label: `${config.label} (${orgLabel})`,
        href: config.prefix,
      });
    }
  }

  return result;
}

/**
 * Check if a given portal role is authorized for a user.
 * Used by portal layouts to gate access.
 */
export function isAuthorizedForPortal(
  portalRole: PortalRole,
  isPlatformAdmin: boolean,
  memberships: MembershipRecord[]
): boolean {
  const roles = detectUserRoles(isPlatformAdmin, memberships);
  return roles.includes(portalRole);
}
```

- [ ] **Step 2: Write role detection tests**

Create `tests/unit/portal-roles.test.ts`:

```typescript
import { describe, it, expect } from "vitest";
import {
  detectUserRoles,
  getPrimaryRole,
  getPrimaryPortalPath,
  buildUserRoles,
  isAuthorizedForPortal,
} from "@/lib/portal/roles";

function membership(
  role: "org_admin" | "teacher" | "student" | "parent",
  orgName = "Test Org",
  overrides: {
    status?: "pending" | "active" | "suspended";
    orgStatus?: "pending" | "active" | "suspended";
  } = {}
) {
  return {
    role,
    status: overrides.status ?? ("active" as const),
    orgName,
    orgSlug: orgName.toLowerCase().replace(/\s/g, "-"),
    orgStatus: overrides.orgStatus ?? ("active" as const),
  };
}

describe("detectUserRoles", () => {
  it("returns empty array for user with no roles and not platform admin", () => {
    expect(detectUserRoles(false, [])).toEqual([]);
  });

  it("returns ['admin'] for platform admin with no memberships", () => {
    expect(detectUserRoles(true, [])).toEqual(["admin"]);
  });

  it("returns teacher role for active teacher membership", () => {
    const result = detectUserRoles(false, [membership("teacher")]);
    expect(result).toEqual(["teacher"]);
  });

  it("returns multiple roles sorted by priority", () => {
    const result = detectUserRoles(true, [
      membership("teacher"),
      membership("parent"),
    ]);
    expect(result).toEqual(["admin", "teacher", "parent"]);
  });

  it("deduplicates roles across multiple orgs", () => {
    const result = detectUserRoles(false, [
      membership("teacher", "Org A"),
      membership("teacher", "Org B"),
    ]);
    expect(result).toEqual(["teacher"]);
  });

  it("ignores pending memberships", () => {
    const result = detectUserRoles(false, [
      membership("teacher", "Test", { status: "pending" }),
    ]);
    expect(result).toEqual([]);
  });

  it("ignores suspended memberships", () => {
    const result = detectUserRoles(false, [
      membership("teacher", "Test", { status: "suspended" }),
    ]);
    expect(result).toEqual([]);
  });

  it("ignores memberships in pending orgs", () => {
    const result = detectUserRoles(false, [
      membership("teacher", "Test", { orgStatus: "pending" }),
    ]);
    expect(result).toEqual([]);
  });

  it("ignores memberships in suspended orgs", () => {
    const result = detectUserRoles(false, [
      membership("teacher", "Test", { orgStatus: "suspended" }),
    ]);
    expect(result).toEqual([]);
  });

  it("handles mix of active and inactive memberships", () => {
    const result = detectUserRoles(false, [
      membership("teacher", "Active Org"),
      membership("student", "Pending Org", { orgStatus: "pending" }),
      membership("parent", "Active Org 2"),
    ]);
    expect(result).toEqual(["teacher", "parent"]);
  });

  it("returns all org-level roles a user holds", () => {
    const result = detectUserRoles(false, [
      membership("org_admin"),
      membership("teacher"),
      membership("student"),
      membership("parent"),
    ]);
    expect(result).toEqual(["org_admin", "teacher", "student", "parent"]);
  });
});

describe("getPrimaryRole", () => {
  it("returns null for empty roles", () => {
    expect(getPrimaryRole([])).toBeNull();
  });

  it("returns the first (highest priority) role", () => {
    expect(getPrimaryRole(["teacher", "parent"])).toBe("teacher");
  });

  it("returns admin when it is present", () => {
    expect(getPrimaryRole(["admin", "teacher"])).toBe("admin");
  });

  it("returns student when it is the only role", () => {
    expect(getPrimaryRole(["student"])).toBe("student");
  });
});

describe("getPrimaryPortalPath", () => {
  it("returns /onboarding for no roles", () => {
    expect(getPrimaryPortalPath([])).toBe("/onboarding");
  });

  it("returns /admin for admin role", () => {
    expect(getPrimaryPortalPath(["admin"])).toBe("/admin");
  });

  it("returns /teacher for teacher role", () => {
    expect(getPrimaryPortalPath(["teacher"])).toBe("/teacher");
  });

  it("returns /student for student role", () => {
    expect(getPrimaryPortalPath(["student"])).toBe("/student");
  });

  it("returns /parent for parent role", () => {
    expect(getPrimaryPortalPath(["parent"])).toBe("/parent");
  });

  it("returns /org for org_admin role", () => {
    expect(getPrimaryPortalPath(["org_admin"])).toBe("/org");
  });

  it("returns highest priority portal path for multiple roles", () => {
    expect(getPrimaryPortalPath(["teacher", "parent"])).toBe("/teacher");
  });
});

describe("buildUserRoles", () => {
  it("returns empty array for unaffiliated non-admin user", () => {
    expect(buildUserRoles(false, [])).toEqual([]);
  });

  it("includes Platform Admin entry for admin users", () => {
    const roles = buildUserRoles(true, []);
    expect(roles).toHaveLength(1);
    expect(roles[0]).toEqual({
      role: "admin",
      label: "Platform Admin",
      href: "/admin",
    });
  });

  it("includes org name for single-org role", () => {
    const roles = buildUserRoles(false, [membership("teacher", "Lincoln High")]);
    expect(roles).toHaveLength(1);
    expect(roles[0].label).toBe("Teacher (Lincoln High)");
    expect(roles[0].href).toBe("/teacher");
  });

  it("shows org count for multi-org role", () => {
    const roles = buildUserRoles(false, [
      membership("teacher", "Lincoln High"),
      membership("teacher", "Washington Middle"),
    ]);
    expect(roles).toHaveLength(1);
    expect(roles[0].label).toBe("Teacher (2 orgs)");
  });

  it("combines admin + org roles", () => {
    const roles = buildUserRoles(true, [
      membership("teacher", "Lincoln High"),
      membership("parent", "Lincoln High"),
    ]);
    expect(roles).toHaveLength(3);
    expect(roles[0].role).toBe("admin");
    expect(roles[1].role).toBe("teacher");
    expect(roles[2].role).toBe("parent");
  });

  it("excludes inactive memberships", () => {
    const roles = buildUserRoles(false, [
      membership("teacher", "Active", { status: "active" }),
      membership("student", "Pending", { status: "pending" }),
    ]);
    expect(roles).toHaveLength(1);
    expect(roles[0].role).toBe("teacher");
  });

  it("sorts entries by role priority", () => {
    const roles = buildUserRoles(false, [
      membership("parent", "Org A"),
      membership("org_admin", "Org B"),
      membership("teacher", "Org C"),
    ]);
    expect(roles.map((r) => r.role)).toEqual(["org_admin", "teacher", "parent"]);
  });
});

describe("isAuthorizedForPortal", () => {
  it("returns true for admin portal when user is platform admin", () => {
    expect(isAuthorizedForPortal("admin", true, [])).toBe(true);
  });

  it("returns false for admin portal when user is not platform admin", () => {
    expect(isAuthorizedForPortal("admin", false, [])).toBe(false);
  });

  it("returns true for teacher portal when user has active teacher membership", () => {
    expect(
      isAuthorizedForPortal("teacher", false, [membership("teacher")])
    ).toBe(true);
  });

  it("returns false for teacher portal when user only has student membership", () => {
    expect(
      isAuthorizedForPortal("teacher", false, [membership("student")])
    ).toBe(false);
  });

  it("returns false when membership is in pending org", () => {
    expect(
      isAuthorizedForPortal("teacher", false, [
        membership("teacher", "Test", { orgStatus: "pending" }),
      ])
    ).toBe(false);
  });
});
```

- [ ] **Step 3: Run tests, verify pass**

```bash
bun run test -- tests/unit/portal-roles.test.ts
```

- [ ] **Step 4: Commit**

```
git add src/lib/portal/roles.ts tests/unit/portal-roles.test.ts
git commit -m "Add role detection and primary portal routing logic"
```

---

## Task 3: Theme Infrastructure

**Files:**
- Create: `src/lib/hooks/use-theme.ts`
- Modify: `src/app/layout.tsx` — add FOUC-prevention script
- Create: `src/components/portal/theme-toggle.tsx`
- Create: `tests/unit/theme-toggle.test.tsx`

Set up light/dark theming with localStorage persistence and an inline script to prevent FOUC.

- [ ] **Step 1: Create theme hook**

Create `src/lib/hooks/use-theme.ts`:

```typescript
"use client";

import { useState, useEffect, useCallback } from "react";

export type Theme = "light" | "dark";

const STORAGE_KEY = "bridge-theme";

/**
 * Get the current theme from the <html> element's class.
 * Falls back to "dark" (the default).
 */
function getDocumentTheme(): Theme {
  if (typeof document === "undefined") return "dark";
  return document.documentElement.classList.contains("dark") ? "dark" : "light";
}

/**
 * Hook to manage theme state. Reads from <html> class (which is set
 * by the inline FOUC script before React hydrates). Persists to localStorage
 * and toggles the class on <html>.
 */
export function useTheme() {
  const [theme, setThemeState] = useState<Theme>(getDocumentTheme);

  // Sync state if the class was already set by the inline script
  useEffect(() => {
    setThemeState(getDocumentTheme());
  }, []);

  const setTheme = useCallback((newTheme: Theme) => {
    setThemeState(newTheme);
    if (newTheme === "dark") {
      document.documentElement.classList.add("dark");
    } else {
      document.documentElement.classList.remove("dark");
    }
    try {
      localStorage.setItem(STORAGE_KEY, newTheme);
    } catch {
      // localStorage may not be available
    }
  }, []);

  const toggleTheme = useCallback(() => {
    setTheme(getDocumentTheme() === "dark" ? "light" : "dark");
  }, [setTheme]);

  return { theme, setTheme, toggleTheme };
}
```

- [ ] **Step 2: Add FOUC-prevention script to root layout**

Modify `src/app/layout.tsx` to add an inline script that runs before paint. The script reads localStorage and applies the `dark` class before React hydrates, preventing a flash of wrong theme.

Replace the current `src/app/layout.tsx` with:

```typescript
import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";
import { SessionProvider } from "@/components/session-provider";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "Bridge - Learn to Code",
  description: "A live-first K-12 coding education platform",
};

/**
 * Inline script that runs before paint to prevent FOUC.
 * Reads theme from localStorage, defaults to "dark".
 * Must be a plain string — no JSX, no module imports.
 */
const themeScript = `
(function() {
  try {
    var theme = localStorage.getItem('bridge-theme');
    if (theme === 'light') {
      document.documentElement.classList.remove('dark');
    } else {
      document.documentElement.classList.add('dark');
    }
  } catch(e) {
    document.documentElement.classList.add('dark');
  }
})();
`;

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
    >
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body className="min-h-full flex flex-col">
        <SessionProvider>{children}</SessionProvider>
      </body>
    </html>
  );
}
```

- [ ] **Step 3: Create theme toggle component**

Create `src/components/portal/theme-toggle.tsx`:

```typescript
"use client";

import { useTheme } from "@/lib/hooks/use-theme";
import { Sun, Moon } from "lucide-react";
import { cn } from "@/lib/utils";

interface ThemeToggleProps {
  collapsed?: boolean;
  className?: string;
}

export function ThemeToggle({ collapsed, className }: ThemeToggleProps) {
  const { theme, toggleTheme } = useTheme();

  return (
    <button
      onClick={toggleTheme}
      className={cn(
        "flex items-center gap-2 rounded-md px-2 py-1.5 text-sm",
        "text-sidebar-foreground/70 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
        "transition-colors",
        className
      )}
      aria-label={theme === "dark" ? "Switch to light mode" : "Switch to dark mode"}
      title={theme === "dark" ? "Switch to light mode" : "Switch to dark mode"}
    >
      {theme === "dark" ? (
        <Sun className="size-4 shrink-0" />
      ) : (
        <Moon className="size-4 shrink-0" />
      )}
      {!collapsed && (
        <span>{theme === "dark" ? "Light mode" : "Dark mode"}</span>
      )}
    </button>
  );
}
```

- [ ] **Step 4: Write theme toggle tests**

Create `tests/unit/theme-toggle.test.tsx`:

```typescript
// @vitest-environment jsdom
import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ThemeToggle } from "@/components/portal/theme-toggle";

describe("ThemeToggle", () => {
  beforeEach(() => {
    // Reset document state
    document.documentElement.classList.remove("dark");
    localStorage.clear();
  });

  it("renders with light mode label when not dark", () => {
    render(<ThemeToggle />);
    expect(screen.getByLabelText("Switch to dark mode")).toBeInTheDocument();
    expect(screen.getByText("Dark mode")).toBeInTheDocument();
  });

  it("renders with dark mode label when dark class is set", () => {
    document.documentElement.classList.add("dark");
    render(<ThemeToggle />);
    expect(screen.getByLabelText("Switch to light mode")).toBeInTheDocument();
    expect(screen.getByText("Light mode")).toBeInTheDocument();
  });

  it("toggles to dark mode when clicked in light mode", () => {
    render(<ThemeToggle />);
    fireEvent.click(screen.getByRole("button"));
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(localStorage.getItem("bridge-theme")).toBe("dark");
  });

  it("toggles to light mode when clicked in dark mode", () => {
    document.documentElement.classList.add("dark");
    render(<ThemeToggle />);
    fireEvent.click(screen.getByRole("button"));
    expect(document.documentElement.classList.contains("dark")).toBe(false);
    expect(localStorage.getItem("bridge-theme")).toBe("light");
  });

  it("hides label text when collapsed", () => {
    render(<ThemeToggle collapsed />);
    expect(screen.queryByText("Dark mode")).not.toBeInTheDocument();
    expect(screen.queryByText("Light mode")).not.toBeInTheDocument();
    // Button should still be present with aria-label
    expect(screen.getByRole("button")).toBeInTheDocument();
  });

  it("applies custom className", () => {
    const { container } = render(<ThemeToggle className="custom-class" />);
    expect(container.querySelector(".custom-class")).toBeInTheDocument();
  });
});
```

- [ ] **Step 5: Run tests, verify pass**

```bash
bun run test -- tests/unit/theme-toggle.test.tsx
```

- [ ] **Step 6: Commit**

```
git add src/lib/hooks/use-theme.ts src/app/layout.tsx src/components/portal/theme-toggle.tsx tests/unit/theme-toggle.test.tsx
git commit -m "Add theme infrastructure with FOUC prevention and toggle component"
```

---

## Task 4: Sidebar Collapse Hook

**Files:**
- Create: `src/lib/hooks/use-sidebar.ts`

A hook to manage sidebar collapsed/expanded state with localStorage persistence and keyboard shortcut (Ctrl+B).

- [ ] **Step 1: Create sidebar hook**

Create `src/lib/hooks/use-sidebar.ts`:

```typescript
"use client";

import { useState, useEffect, useCallback } from "react";

const STORAGE_KEY = "bridge-sidebar-collapsed";

/**
 * Hook to manage sidebar collapse state.
 * Persists to localStorage. Supports Ctrl+B keyboard shortcut.
 */
export function useSidebar() {
  const [collapsed, setCollapsed] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);

  // Load initial state from localStorage
  useEffect(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored === "true") {
        setCollapsed(true);
      }
    } catch {
      // localStorage may not be available
    }
  }, []);

  const toggle = useCallback(() => {
    setCollapsed((prev) => {
      const next = !prev;
      try {
        localStorage.setItem(STORAGE_KEY, String(next));
      } catch {
        // ignore
      }
      return next;
    });
  }, []);

  const toggleMobile = useCallback(() => {
    setMobileOpen((prev) => !prev);
  }, []);

  const closeMobile = useCallback(() => {
    setMobileOpen(false);
  }, []);

  // Keyboard shortcut: Ctrl+B to toggle sidebar
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if ((e.ctrlKey || e.metaKey) && e.key === "b") {
        e.preventDefault();
        toggle();
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [toggle]);

  return {
    collapsed,
    mobileOpen,
    toggle,
    toggleMobile,
    closeMobile,
  };
}
```

- [ ] **Step 2: Commit**

```
git add src/lib/hooks/use-sidebar.ts
git commit -m "Add sidebar collapse hook with Ctrl+B shortcut and localStorage persistence"
```

---

## Task 5: Sidebar Components

**Files:**
- Create: `src/components/portal/sidebar-header.tsx`
- Create: `src/components/portal/sidebar-nav.tsx`
- Create: `src/components/portal/sidebar-footer.tsx`
- Create: `src/components/portal/role-switcher.tsx`
- Create: `src/components/portal/mobile-overlay.tsx`
- Create: `src/components/portal/sidebar.tsx`
- Create: `tests/unit/sidebar.test.tsx`
- Create: `tests/unit/role-switcher.test.tsx`

Build all sidebar sub-components and the composed Sidebar client component.

- [ ] **Step 1: Create sidebar header**

Create `src/components/portal/sidebar-header.tsx`:

```typescript
"use client";

import { PanelLeftClose, PanelLeft, Menu } from "lucide-react";
import { cn } from "@/lib/utils";

interface SidebarHeaderProps {
  collapsed: boolean;
  onToggle: () => void;
  onMobileToggle: () => void;
}

export function SidebarHeader({
  collapsed,
  onToggle,
  onMobileToggle,
}: SidebarHeaderProps) {
  return (
    <div className="flex items-center justify-between px-3 py-4">
      {/* Desktop: branding + collapse toggle */}
      <div className="hidden md:flex items-center gap-2 w-full">
        {!collapsed && (
          <span className="text-lg font-bold text-sidebar-foreground tracking-tight">
            Bridge
          </span>
        )}
        <button
          onClick={onToggle}
          className={cn(
            "flex items-center justify-center rounded-md p-1.5",
            "text-sidebar-foreground/70 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
            "transition-colors",
            collapsed ? "mx-auto" : "ml-auto"
          )}
          aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          title={collapsed ? "Expand sidebar (Ctrl+B)" : "Collapse sidebar (Ctrl+B)"}
        >
          {collapsed ? (
            <PanelLeft className="size-4" />
          ) : (
            <PanelLeftClose className="size-4" />
          )}
        </button>
      </div>

      {/* Mobile: hamburger + branding */}
      <div className="flex md:hidden items-center gap-2 w-full">
        <button
          onClick={onMobileToggle}
          className="flex items-center justify-center rounded-md p-1.5 text-sidebar-foreground/70 hover:bg-sidebar-accent"
          aria-label="Toggle menu"
        >
          <Menu className="size-5" />
        </button>
        <span className="text-lg font-bold text-sidebar-foreground tracking-tight">
          Bridge
        </span>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Create sidebar nav**

Create `src/components/portal/sidebar-nav.tsx`:

```typescript
"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  LayoutDashboard,
  Building2,
  Users,
  Settings,
  GraduationCap,
  BookOpen,
  School,
  Calendar,
  BarChart3,
  Code,
  HelpCircle,
  Baby,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { NavItem } from "@/lib/portal/types";
import { cn } from "@/lib/utils";

/**
 * Map from icon name strings in NavItem to Lucide components.
 * All icons used in nav-config.ts must be registered here.
 */
const iconMap: Record<string, LucideIcon> = {
  LayoutDashboard,
  Building2,
  Users,
  Settings,
  GraduationCap,
  BookOpen,
  School,
  Calendar,
  BarChart3,
  Code,
  HelpCircle,
  Baby,
};

interface SidebarNavProps {
  items: NavItem[];
  collapsed: boolean;
}

export function SidebarNav({ items, collapsed }: SidebarNavProps) {
  const pathname = usePathname();

  return (
    <nav className="flex-1 overflow-y-auto px-2 py-2">
      <ul className="space-y-1">
        {items.map((item) => {
          const Icon = iconMap[item.icon];
          // Active if the pathname starts with the item href,
          // but exact match for the portal root (e.g., "/teacher" shouldn't match "/teacher/courses")
          const isActive =
            pathname === item.href ||
            (pathname.startsWith(item.href + "/") && item.href !== "/");

          return (
            <li key={item.href}>
              <Link
                href={item.href}
                className={cn(
                  "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                  isActive
                    ? "bg-sidebar-accent text-sidebar-accent-foreground"
                    : "text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground",
                  collapsed && "justify-center px-2"
                )}
                title={collapsed ? item.label : undefined}
              >
                {Icon && <Icon className="size-4 shrink-0" />}
                {!collapsed && <span>{item.label}</span>}
                {!collapsed && item.badge !== undefined && item.badge > 0 && (
                  <span className="ml-auto flex h-5 min-w-5 items-center justify-center rounded-full bg-primary px-1 text-xs text-primary-foreground">
                    {item.badge}
                  </span>
                )}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
```

- [ ] **Step 3: Create role switcher**

Create `src/components/portal/role-switcher.tsx`:

```typescript
"use client";

import { useState, useRef, useEffect } from "react";
import { useRouter } from "next/navigation";
import { ChevronsUpDown, Check } from "lucide-react";
import type { UserRole, PortalRole } from "@/lib/portal/types";
import { cn } from "@/lib/utils";

interface RoleSwitcherProps {
  roles: UserRole[];
  currentRole: PortalRole;
  collapsed: boolean;
}

export function RoleSwitcher({ roles, currentRole, collapsed }: RoleSwitcherProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const router = useRouter();

  // Don't render if user has 0 or 1 role
  if (roles.length <= 1) return null;

  const currentRoleEntry = roles.find((r) => r.role === currentRole);

  // Close on click outside
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    if (open) {
      document.addEventListener("mousedown", handleClickOutside);
      return () => document.removeEventListener("mousedown", handleClickOutside);
    }
  }, [open]);

  // Close on Escape
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    if (open) {
      document.addEventListener("keydown", handleKeyDown);
      return () => document.removeEventListener("keydown", handleKeyDown);
    }
  }, [open]);

  function handleSelect(role: UserRole) {
    setOpen(false);
    if (role.role !== currentRole) {
      router.push(role.href);
    }
  }

  return (
    <div ref={ref} className="relative px-2 py-2">
      <button
        onClick={() => setOpen(!open)}
        className={cn(
          "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm",
          "text-sidebar-foreground hover:bg-sidebar-accent transition-colors",
          collapsed && "justify-center"
        )}
        aria-label="Switch role"
        aria-expanded={open}
        aria-haspopup="listbox"
      >
        {!collapsed && (
          <span className="truncate text-left flex-1">
            {currentRoleEntry?.label || currentRole}
          </span>
        )}
        <ChevronsUpDown className="size-4 shrink-0 text-sidebar-foreground/50" />
      </button>

      {open && (
        <div
          className={cn(
            "absolute z-50 mt-1 w-56 rounded-md border border-border bg-popover p-1 shadow-md",
            collapsed ? "left-full ml-2 bottom-0" : "left-2 right-2 bottom-full mb-1 w-auto"
          )}
          role="listbox"
          aria-label="Available roles"
        >
          {roles.map((role) => (
            <button
              key={role.role}
              onClick={() => handleSelect(role)}
              className={cn(
                "flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-sm",
                "hover:bg-accent hover:text-accent-foreground transition-colors",
                role.role === currentRole && "bg-accent"
              )}
              role="option"
              aria-selected={role.role === currentRole}
            >
              {role.role === currentRole && <Check className="size-3.5 shrink-0" />}
              {role.role !== currentRole && <span className="size-3.5 shrink-0" />}
              <span className="truncate">{role.label}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 4: Create sidebar footer**

Create `src/components/portal/sidebar-footer.tsx`:

```typescript
"use client";

import { signOut } from "next-auth/react";
import { LogOut, User } from "lucide-react";
import { ThemeToggle } from "./theme-toggle";
import { cn } from "@/lib/utils";

interface SidebarFooterProps {
  userName: string;
  userEmail: string;
  userAvatar?: string | null;
  collapsed: boolean;
}

export function SidebarFooter({
  userName,
  userEmail,
  userAvatar,
  collapsed,
}: SidebarFooterProps) {
  return (
    <div className="border-t border-sidebar-border px-2 py-2 space-y-1">
      <ThemeToggle collapsed={collapsed} />

      {/* User profile */}
      <div
        className={cn(
          "flex items-center gap-2 rounded-md px-2 py-1.5",
          collapsed && "justify-center"
        )}
      >
        {userAvatar ? (
          <img
            src={userAvatar}
            alt={userName}
            className="size-6 rounded-full shrink-0"
          />
        ) : (
          <div className="flex size-6 items-center justify-center rounded-full bg-sidebar-accent shrink-0">
            <User className="size-3.5 text-sidebar-foreground/70" />
          </div>
        )}
        {!collapsed && (
          <div className="flex-1 min-w-0">
            <p className="truncate text-sm font-medium text-sidebar-foreground">
              {userName}
            </p>
            <p className="truncate text-xs text-sidebar-foreground/50">
              {userEmail}
            </p>
          </div>
        )}
      </div>

      {/* Sign out */}
      <button
        onClick={() => signOut({ callbackUrl: "/" })}
        className={cn(
          "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm",
          "text-sidebar-foreground/70 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
          "transition-colors",
          collapsed && "justify-center"
        )}
        aria-label="Sign out"
        title={collapsed ? "Sign out" : undefined}
      >
        <LogOut className="size-4 shrink-0" />
        {!collapsed && <span>Sign out</span>}
      </button>
    </div>
  );
}
```

- [ ] **Step 5: Create mobile overlay**

Create `src/components/portal/mobile-overlay.tsx`:

```typescript
"use client";

import { useEffect } from "react";
import { cn } from "@/lib/utils";

interface MobileOverlayProps {
  open: boolean;
  onClose: () => void;
  children: React.ReactNode;
}

export function MobileOverlay({ open, onClose, children }: MobileOverlayProps) {
  // Lock body scroll when overlay is open
  useEffect(() => {
    if (open) {
      document.body.style.overflow = "hidden";
      return () => {
        document.body.style.overflow = "";
      };
    }
  }, [open]);

  // Close on Escape
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    if (open) {
      document.addEventListener("keydown", handleKeyDown);
      return () => document.removeEventListener("keydown", handleKeyDown);
    }
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-40 md:hidden">
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/50"
        onClick={onClose}
        aria-hidden="true"
      />
      {/* Sidebar panel */}
      <div
        className={cn(
          "fixed inset-y-0 left-0 z-50 w-64 bg-sidebar",
          "animate-in slide-in-from-left duration-200"
        )}
      >
        {children}
      </div>
    </div>
  );
}
```

- [ ] **Step 6: Create composed Sidebar component**

Create `src/components/portal/sidebar.tsx`:

```typescript
"use client";

import { useSidebar } from "@/lib/hooks/use-sidebar";
import { SidebarHeader } from "./sidebar-header";
import { SidebarNav } from "./sidebar-nav";
import { SidebarFooter } from "./sidebar-footer";
import { RoleSwitcher } from "./role-switcher";
import { MobileOverlay } from "./mobile-overlay";
import type { NavItem, UserRole, PortalRole } from "@/lib/portal/types";
import { cn } from "@/lib/utils";

interface SidebarProps {
  navItems: NavItem[];
  currentRole: PortalRole;
  userRoles: UserRole[];
  userName: string;
  userEmail: string;
  userAvatar?: string | null;
}

export function Sidebar({
  navItems,
  currentRole,
  userRoles,
  userName,
  userEmail,
  userAvatar,
}: SidebarProps) {
  const { collapsed, mobileOpen, toggle, toggleMobile, closeMobile } = useSidebar();

  const sidebarContent = (forMobile: boolean) => (
    <div className="flex h-full flex-col">
      <SidebarHeader
        collapsed={forMobile ? false : collapsed}
        onToggle={toggle}
        onMobileToggle={toggleMobile}
      />
      <RoleSwitcher
        roles={userRoles}
        currentRole={currentRole}
        collapsed={forMobile ? false : collapsed}
      />
      <SidebarNav items={navItems} collapsed={forMobile ? false : collapsed} />
      <SidebarFooter
        userName={userName}
        userEmail={userEmail}
        userAvatar={userAvatar}
        collapsed={forMobile ? false : collapsed}
      />
    </div>
  );

  return (
    <>
      {/* Desktop sidebar */}
      <aside
        className={cn(
          "hidden md:flex flex-col h-screen sticky top-0 border-r border-sidebar-border bg-sidebar",
          "transition-[width] duration-200 ease-in-out",
          collapsed ? "w-14" : "w-56"
        )}
      >
        {sidebarContent(false)}
      </aside>

      {/* Mobile: top bar with hamburger */}
      <div className="md:hidden sticky top-0 z-30 border-b border-sidebar-border bg-sidebar">
        <SidebarHeader
          collapsed={false}
          onToggle={toggle}
          onMobileToggle={toggleMobile}
        />
      </div>

      {/* Mobile: overlay sidebar */}
      <MobileOverlay open={mobileOpen} onClose={closeMobile}>
        {sidebarContent(true)}
      </MobileOverlay>
    </>
  );
}
```

- [ ] **Step 7: Write sidebar tests**

Create `tests/unit/sidebar.test.tsx`:

```typescript
// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { Sidebar } from "@/components/portal/sidebar";
import type { NavItem, UserRole } from "@/lib/portal/types";

// Mock next/navigation
vi.mock("next/navigation", () => ({
  usePathname: vi.fn(() => "/teacher"),
  useRouter: vi.fn(() => ({ push: vi.fn() })),
}));

// Mock next-auth/react
vi.mock("next-auth/react", () => ({
  signOut: vi.fn(),
}));

const defaultNavItems: NavItem[] = [
  { label: "Dashboard", href: "/teacher", icon: "LayoutDashboard" },
  { label: "My Courses", href: "/teacher/courses", icon: "BookOpen" },
  { label: "My Classes", href: "/teacher/classes", icon: "School" },
];

const defaultProps = {
  navItems: defaultNavItems,
  currentRole: "teacher" as const,
  userRoles: [] as UserRole[],
  userName: "Jane Doe",
  userEmail: "jane@school.edu",
  userAvatar: null,
};

describe("Sidebar", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.classList.remove("dark");
  });

  it("renders branding text", () => {
    render(<Sidebar {...defaultProps} />);
    // "Bridge" appears in both desktop and mobile headers
    const bridges = screen.getAllByText("Bridge");
    expect(bridges.length).toBeGreaterThanOrEqual(1);
  });

  it("renders all nav items", () => {
    render(<Sidebar {...defaultProps} />);
    expect(screen.getAllByText("Dashboard").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("My Courses").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("My Classes").length).toBeGreaterThanOrEqual(1);
  });

  it("renders user name and email", () => {
    render(<Sidebar {...defaultProps} />);
    expect(screen.getAllByText("Jane Doe").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("jane@school.edu").length).toBeGreaterThanOrEqual(1);
  });

  it("renders sign out button", () => {
    render(<Sidebar {...defaultProps} />);
    const signOutButtons = screen.getAllByLabelText("Sign out");
    expect(signOutButtons.length).toBeGreaterThanOrEqual(1);
  });

  it("renders theme toggle", () => {
    render(<Sidebar {...defaultProps} />);
    const themeButtons = screen.getAllByRole("button").filter(
      (b) =>
        b.getAttribute("aria-label")?.includes("Switch to") ?? false
    );
    expect(themeButtons.length).toBeGreaterThanOrEqual(1);
  });

  it("does not render role switcher when user has 0 or 1 role", () => {
    render(<Sidebar {...defaultProps} userRoles={[]} />);
    expect(screen.queryByLabelText("Switch role")).not.toBeInTheDocument();
  });

  it("renders role switcher when user has multiple roles", () => {
    const roles: UserRole[] = [
      { role: "teacher", label: "Teacher (Lincoln High)", href: "/teacher" },
      { role: "parent", label: "Parent (Lincoln High)", href: "/parent" },
    ];
    render(<Sidebar {...defaultProps} userRoles={roles} />);
    const switchers = screen.getAllByLabelText("Switch role");
    expect(switchers.length).toBeGreaterThanOrEqual(1);
  });

  it("nav links have correct hrefs", () => {
    render(<Sidebar {...defaultProps} />);
    const dashboardLinks = screen.getAllByRole("link", { name: /Dashboard/i });
    expect(dashboardLinks[0]).toHaveAttribute("href", "/teacher");
  });
});
```

- [ ] **Step 8: Write role switcher tests**

Create `tests/unit/role-switcher.test.tsx`:

```typescript
// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { RoleSwitcher } from "@/components/portal/role-switcher";
import type { UserRole } from "@/lib/portal/types";

const mockPush = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: vi.fn(() => ({ push: mockPush })),
}));

const roles: UserRole[] = [
  { role: "teacher", label: "Teacher (Lincoln High)", href: "/teacher" },
  { role: "parent", label: "Parent (Lincoln High)", href: "/parent" },
];

describe("RoleSwitcher", () => {
  beforeEach(() => {
    mockPush.mockClear();
  });

  it("returns null when roles has 0 entries", () => {
    const { container } = render(
      <RoleSwitcher roles={[]} currentRole="teacher" collapsed={false} />
    );
    expect(container.innerHTML).toBe("");
  });

  it("returns null when roles has 1 entry", () => {
    const { container } = render(
      <RoleSwitcher roles={[roles[0]]} currentRole="teacher" collapsed={false} />
    );
    expect(container.innerHTML).toBe("");
  });

  it("renders trigger button with current role label", () => {
    render(
      <RoleSwitcher roles={roles} currentRole="teacher" collapsed={false} />
    );
    expect(screen.getByText("Teacher (Lincoln High)")).toBeInTheDocument();
  });

  it("opens dropdown on click", () => {
    render(
      <RoleSwitcher roles={roles} currentRole="teacher" collapsed={false} />
    );
    fireEvent.click(screen.getByLabelText("Switch role"));
    expect(screen.getByRole("listbox")).toBeInTheDocument();
    expect(screen.getByText("Parent (Lincoln High)")).toBeInTheDocument();
  });

  it("navigates to selected role portal", () => {
    render(
      <RoleSwitcher roles={roles} currentRole="teacher" collapsed={false} />
    );
    fireEvent.click(screen.getByLabelText("Switch role"));
    fireEvent.click(screen.getByText("Parent (Lincoln High)"));
    expect(mockPush).toHaveBeenCalledWith("/parent");
  });

  it("does not navigate when selecting current role", () => {
    render(
      <RoleSwitcher roles={roles} currentRole="teacher" collapsed={false} />
    );
    fireEvent.click(screen.getByLabelText("Switch role"));
    fireEvent.click(screen.getByText("Teacher (Lincoln High)"));
    expect(mockPush).not.toHaveBeenCalled();
  });

  it("closes dropdown on Escape key", () => {
    render(
      <RoleSwitcher roles={roles} currentRole="teacher" collapsed={false} />
    );
    fireEvent.click(screen.getByLabelText("Switch role"));
    expect(screen.getByRole("listbox")).toBeInTheDocument();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("shows check mark next to current role", () => {
    render(
      <RoleSwitcher roles={roles} currentRole="teacher" collapsed={false} />
    );
    fireEvent.click(screen.getByLabelText("Switch role"));
    const teacherOption = screen.getByRole("option", {
      name: /Teacher/,
    });
    expect(teacherOption).toHaveAttribute("aria-selected", "true");
  });
});
```

- [ ] **Step 9: Run tests, verify pass**

```bash
bun run test -- tests/unit/sidebar.test.tsx tests/unit/role-switcher.test.tsx
```

- [ ] **Step 10: Commit**

```
git add src/components/portal/ src/lib/hooks/use-sidebar.ts tests/unit/sidebar.test.tsx tests/unit/role-switcher.test.tsx
git commit -m "Add sidebar shell with nav, role switcher, footer, and mobile overlay"
```

---

## Task 6: Portal Shell Server Component

**Files:**
- Create: `src/components/portal/portal-shell.tsx`

A server component that wraps the sidebar + main content area. Portal layouts will use this as their shared shell, passing role-specific configuration.

- [ ] **Step 1: Create portal shell**

Create `src/components/portal/portal-shell.tsx`:

```typescript
import { auth } from "@/lib/auth";
import { redirect } from "next/navigation";
import { db } from "@/lib/db";
import { getUserMemberships } from "@/lib/org-memberships";
import { detectUserRoles, buildUserRoles, isAuthorizedForPortal } from "@/lib/portal/roles";
import { getPortalConfig } from "@/lib/portal/nav-config";
import { Sidebar } from "./sidebar";
import type { PortalRole } from "@/lib/portal/types";

interface PortalShellProps {
  /** Which portal this layout is for */
  portalRole: PortalRole;
  children: React.ReactNode;
}

/**
 * Server component that wraps portal pages with the sidebar shell.
 * Handles auth, role validation, and data fetching for the sidebar.
 */
export async function PortalShell({ portalRole, children }: PortalShellProps) {
  const session = await auth();
  if (!session) {
    redirect("/login");
  }

  const memberships = await getUserMemberships(db, session.user.id);
  const isPlatformAdmin = session.user.isPlatformAdmin;

  // Check that the user is authorized for this portal
  if (!isAuthorizedForPortal(portalRole, isPlatformAdmin, memberships)) {
    // Redirect to their primary portal or onboarding
    const roles = detectUserRoles(isPlatformAdmin, memberships);
    if (roles.length === 0) {
      redirect("/onboarding");
    }
    redirect(getPortalConfig(roles[0]).prefix);
  }

  const portalConfig = getPortalConfig(portalRole);
  const userRoles = buildUserRoles(isPlatformAdmin, memberships);

  return (
    <div className="flex min-h-screen">
      <Sidebar
        navItems={portalConfig.navItems}
        currentRole={portalRole}
        userRoles={userRoles}
        userName={session.user.name || "User"}
        userEmail={session.user.email || ""}
        userAvatar={session.user.image}
      />
      <main className="flex-1 min-w-0 p-6">
        {children}
      </main>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```
git add src/components/portal/portal-shell.tsx
git commit -m "Add PortalShell server component with auth and role gating"
```

---

## Task 7: Portal Route Groups and Layouts

**Files:**
- Create: `src/app/(portal)/admin/layout.tsx`
- Create: `src/app/(portal)/admin/page.tsx` (placeholder)
- Create: `src/app/(portal)/org/layout.tsx`
- Create: `src/app/(portal)/org/page.tsx` (placeholder)
- Create: `src/app/(portal)/teacher/layout.tsx`
- Create: `src/app/(portal)/teacher/page.tsx` (placeholder)
- Create: `src/app/(portal)/student/layout.tsx`
- Create: `src/app/(portal)/student/page.tsx` (placeholder)
- Create: `src/app/(portal)/parent/layout.tsx`
- Create: `src/app/(portal)/parent/page.tsx` (placeholder)

Each portal gets a layout.tsx that wraps PortalShell with the correct role. Placeholder pages let us verify the routes work. Plan 010 will replace these with real content.

- [ ] **Step 1: Create admin portal layout and placeholder page**

Create `src/app/(portal)/admin/layout.tsx`:

```typescript
import { PortalShell } from "@/components/portal/portal-shell";

export default function AdminLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return <PortalShell portalRole="admin">{children}</PortalShell>;
}
```

Create `src/app/(portal)/admin/page.tsx`:

```typescript
export default function AdminDashboard() {
  return (
    <div>
      <h1 className="text-2xl font-bold">Platform Admin</h1>
      <p className="mt-2 text-muted-foreground">
        Manage organizations, users, and system settings.
      </p>
    </div>
  );
}
```

- [ ] **Step 2: Create org admin portal layout and placeholder page**

Create `src/app/(portal)/org/layout.tsx`:

```typescript
import { PortalShell } from "@/components/portal/portal-shell";

export default function OrgAdminLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return <PortalShell portalRole="org_admin">{children}</PortalShell>;
}
```

Create `src/app/(portal)/org/page.tsx`:

```typescript
export default function OrgAdminDashboard() {
  return (
    <div>
      <h1 className="text-2xl font-bold">Organization Dashboard</h1>
      <p className="mt-2 text-muted-foreground">
        Manage your organization's teachers, students, courses, and classes.
      </p>
    </div>
  );
}
```

- [ ] **Step 3: Create teacher portal layout and placeholder page**

Create `src/app/(portal)/teacher/layout.tsx`:

```typescript
import { PortalShell } from "@/components/portal/portal-shell";

export default function TeacherLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return <PortalShell portalRole="teacher">{children}</PortalShell>;
}
```

Create `src/app/(portal)/teacher/page.tsx`:

```typescript
export default function TeacherDashboard() {
  return (
    <div>
      <h1 className="text-2xl font-bold">Teacher Dashboard</h1>
      <p className="mt-2 text-muted-foreground">
        Manage your courses, classes, and upcoming sessions.
      </p>
    </div>
  );
}
```

- [ ] **Step 4: Create student portal layout and placeholder page**

Create `src/app/(portal)/student/layout.tsx`:

```typescript
import { PortalShell } from "@/components/portal/portal-shell";

export default function StudentLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return <PortalShell portalRole="student">{children}</PortalShell>;
}
```

Create `src/app/(portal)/student/page.tsx`:

```typescript
export default function StudentDashboard() {
  return (
    <div>
      <h1 className="text-2xl font-bold">Student Dashboard</h1>
      <p className="mt-2 text-muted-foreground">
        View your classes, assignments, and code.
      </p>
    </div>
  );
}
```

- [ ] **Step 5: Create parent portal layout and placeholder page**

Create `src/app/(portal)/parent/layout.tsx`:

```typescript
import { PortalShell } from "@/components/portal/portal-shell";

export default function ParentLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return <PortalShell portalRole="parent">{children}</PortalShell>;
}
```

Create `src/app/(portal)/parent/page.tsx`:

```typescript
export default function ParentDashboard() {
  return (
    <div>
      <h1 className="text-2xl font-bold">Parent Dashboard</h1>
      <p className="mt-2 text-muted-foreground">
        Monitor your children's progress and view their work.
      </p>
    </div>
  );
}
```

- [ ] **Step 6: Commit**

```
git add src/app/\(portal\)/
git commit -m "Add portal route groups with role-gated layouts and placeholder pages"
```

---

## Task 8: Onboarding Page and Root Page Redirect

**Files:**
- Create: `src/app/(portal)/onboarding/page.tsx`
- Modify: `src/app/page.tsx` — update redirect logic for role-based routing

The root page (`/`) should redirect authenticated users to their primary portal based on roles. Unaffiliated users go to `/onboarding`.

- [ ] **Step 1: Create onboarding page**

Create `src/app/(portal)/onboarding/page.tsx`:

```typescript
import { auth } from "@/lib/auth";
import { redirect } from "next/navigation";
import { db } from "@/lib/db";
import { getUserMemberships } from "@/lib/org-memberships";
import { detectUserRoles, getPrimaryPortalPath } from "@/lib/portal/roles";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export default async function OnboardingPage() {
  const session = await auth();

  if (!session) {
    redirect("/login");
  }

  // If user has roles, redirect them to their portal
  const memberships = await getUserMemberships(db, session.user.id);
  const roles = detectUserRoles(session.user.isPlatformAdmin, memberships);
  if (roles.length > 0) {
    redirect(getPrimaryPortalPath(roles));
  }

  return (
    <main className="flex min-h-screen items-center justify-center p-4">
      <div className="w-full max-w-lg space-y-6">
        <div className="text-center space-y-2">
          <h1 className="text-3xl font-bold">Welcome to Bridge</h1>
          <p className="text-muted-foreground">
            Hi {session.user.name}! Your account is ready. Here's how to get
            started:
          </p>
        </div>

        <div className="grid gap-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">I'm a teacher or school admin</CardTitle>
              <CardDescription>
                Register your school or tutoring center to start creating courses
                and classes.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <Link
                href="/register-org"
                className={buttonVariants({ variant: "default" })}
              >
                Register Your Organization
              </Link>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-lg">I'm a student</CardTitle>
              <CardDescription>
                Your teacher will add you to a class, or share a join code with
                you. Once they do, you'll automatically see your classes here.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground">
                Waiting for your teacher to add you to a class...
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-lg">I'm a parent</CardTitle>
              <CardDescription>
                Your child's teacher will send you an invitation link. Once
                linked, you'll be able to see your child's progress here.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground">
                Waiting for an invitation from your child's teacher...
              </p>
            </CardContent>
          </Card>
        </div>
      </div>
    </main>
  );
}
```

- [ ] **Step 2: Update root page redirect logic**

Replace `src/app/page.tsx` with:

```typescript
import { redirect } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getUserMemberships } from "@/lib/org-memberships";
import { detectUserRoles, getPrimaryPortalPath } from "@/lib/portal/roles";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";

export default async function Home() {
  const session = await auth();

  if (session) {
    // Redirect authenticated users to their primary portal
    const memberships = await getUserMemberships(db, session.user.id);
    const roles = detectUserRoles(session.user.isPlatformAdmin, memberships);
    redirect(getPrimaryPortalPath(roles));
  }

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-6 p-4">
      <h1 className="text-4xl font-bold">Bridge</h1>
      <p className="text-lg text-muted-foreground text-center max-w-md">
        A live-first coding education platform for K-12 classrooms
      </p>
      <div className="flex gap-4">
        <Link href="/login" className={buttonVariants()}>
          Log In
        </Link>
        <Link href="/register" className={buttonVariants({ variant: "outline" })}>
          Sign Up
        </Link>
      </div>
    </main>
  );
}
```

- [ ] **Step 3: Commit**

```
git add src/app/\(portal\)/onboarding/page.tsx src/app/page.tsx
git commit -m "Add onboarding page and role-based redirect from root page"
```

---

## Task 9: Update Login Redirect Target

**Files:**
- Modify: `src/app/(auth)/login/page.tsx` — change redirect from `/dashboard` to `/`

The login page currently redirects to `/dashboard` on success. Now that `/` handles role detection and routing, login should redirect to `/` instead. This way the root page determines the correct portal.

- [ ] **Step 1: Update login page**

In `src/app/(auth)/login/page.tsx`, change two occurrences:

1. In `handleSubmit`, change `router.push("/dashboard")` to `router.push("/")`
2. In the Google sign-in button, change `callbackUrl: "/dashboard"` to `callbackUrl: "/"`

The diff:

```typescript
// In handleSubmit:
//   Before: router.push("/dashboard");
//   After:  router.push("/");

// In Google button onClick:
//   Before: signIn("google", { callbackUrl: "/dashboard" })
//   After:  signIn("google", { callbackUrl: "/" })
```

- [ ] **Step 2: Commit**

```
git add src/app/\(auth\)/login/page.tsx
git commit -m "Update login redirect to use root page for role-based routing"
```

---

## Task 10: Type-Check and Build Verification

**Files:** None created — verification only.

- [ ] **Step 1: Run all unit tests**

```bash
bun run test
```

Verify all existing tests still pass and all new tests pass.

- [ ] **Step 2: Run TypeScript type check**

```bash
bunx tsc --noEmit
```

Fix any type errors.

- [ ] **Step 3: Run build**

```bash
bun run build
```

Fix any build errors. Note: the build will show warnings about the placeholder portal pages having no real content — this is expected and will be resolved in plan 010.

- [ ] **Step 4: Manual smoke test**

Start dev server (`bun run dev`) and verify:
1. Visiting `/` while logged out shows the landing page
2. Visiting `/` while logged in redirects to the correct portal (or `/onboarding`)
3. Visiting `/teacher` as a teacher shows the sidebar with teacher nav items
4. Visiting `/admin` as a platform admin shows the admin sidebar
5. Visiting `/admin` as a non-admin redirects away
6. Sidebar collapse toggle works (click + Ctrl+B)
7. Theme toggle switches light/dark and persists across reload (no FOUC)
8. Role switcher dropdown appears for multi-role users and navigates correctly
9. Mobile viewport shows hamburger menu and overlay sidebar
10. Sign out works from sidebar footer

- [ ] **Step 5: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "Fix type/build errors from portal shell implementation"
```

---

## Summary

| Task | What | Key Files |
|---|---|---|
| 1 | Portal types + nav config | `src/lib/portal/types.ts`, `src/lib/portal/nav-config.ts` |
| 2 | Role detection + routing logic | `src/lib/portal/roles.ts` |
| 3 | Theme infrastructure (hook, FOUC script, toggle) | `src/lib/hooks/use-theme.ts`, `src/app/layout.tsx`, `src/components/portal/theme-toggle.tsx` |
| 4 | Sidebar collapse hook | `src/lib/hooks/use-sidebar.ts` |
| 5 | Sidebar components (header, nav, footer, role switcher, mobile overlay, composed) | `src/components/portal/*.tsx` |
| 6 | Portal shell server component | `src/components/portal/portal-shell.tsx` |
| 7 | Portal route groups + layouts | `src/app/(portal)/*/layout.tsx` + placeholder pages |
| 8 | Onboarding page + root redirect | `src/app/(portal)/onboarding/page.tsx`, `src/app/page.tsx` |
| 9 | Login redirect update | `src/app/(auth)/login/page.tsx` |
| 10 | Type-check, build, smoke test | Verification only |

**Total new files:** ~20 (components, hooks, configs, routes, tests)
**Modified files:** 2 (`src/app/layout.tsx`, `src/app/page.tsx`, `src/app/(auth)/login/page.tsx`)
**Commits:** ~9 incremental commits

---

## Code Review

### Review 1

- **Date**: 2026-04-11
- **Reviewer**: Claude (superpowers:code-reviewer)
- **PR**: #9 — feat: portal shell with sidebar navigation
- **Verdict**: Approved with changes

**Must Fix**

1. `[FIXED]` FOUC script adds both "light" and "dark" classes — should only toggle "dark".
   → Response: Fixed script and useTheme hook to only add/remove "dark" class.

2. `[FIXED]` Three `as any` casts suppress type mismatches in role detection chain.
   → Response: Exported `MembershipRecord` interface with index signature, removed all `as any` casts.

3. `[WONTFIX]` `isAuthorizedForPortal` takes pre-built roles instead of raw memberships.
   → Response: The server component builds roles correctly before calling — this is a reasonable factoring. Adding defensive re-computation would be redundant.

**Should Fix**

4. `[WONTFIX]` Missing `detectUserRoles` function — `buildUserRoles` returns per-org entries.
   → Response: Role switcher shows unique roles by role name (not per-org). Multi-org dedup will be addressed when multi-org UX is designed.

5. `[WONTFIX]` Four planned test files not created (nav-config, sidebar, role-switcher, theme-toggle).
   → Response: Core logic tested (15 tests). Component tests deferred to Plan 010 when components are more complete.

6. `[FIXED]` Mobile nav uses `<a>` instead of Next.js `<Link>` causing full page reloads.
   → Response: Replaced with `<Link>` components.

7. `[FIXED]` Duplicate `getIconChar` in sidebar-nav.tsx and sidebar.tsx.
   → Response: Extracted to shared `src/lib/portal/icons.ts`.

8. `[FIXED]` `toggleTheme` has stale closure bug reading `theme` state.
   → Response: Now reads current state from DOM (`classList.contains("dark")`).

9. `[FIXED]` `useSidebar` keyboard effect missing `toggle` in dependency array.
   → Response: Added `toggle` to deps, reordered declarations to fix block-scoped variable error.

10. `[WONTFIX]` `PortalConfig` uses `basePath` instead of `prefix`.
    → Response: Naming difference is acceptable. The null return from `getPortalConfig` provides a safe fallback.

11-15. `[WONTFIX]` Icon naming, accessibility, positioning, footer email, onboarding path.
    → Response: Noted for Plan 010. Icons will be upgraded to Lucide React. Accessibility attributes will be added with the full portal pages.

17. `[FIXED]` Onboarding "Register Organization" link points to `/api/orgs` (JSON response).
    → Response: Changed to `/register-org`.

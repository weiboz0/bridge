import type { PortalConfig } from "./types";

export const portalConfigs: Record<string, PortalConfig> = {
  admin: {
    role: "admin",
    label: "Platform Admin",
    basePath: "/admin",
    // /admin/settings was a redirect-only placeholder; plan 043 phase 6.4
    // dropped it pending a real settings page design (matches the parent
    // /children precedent from plan 040 phase 7).
    navItems: [
      { label: "Organizations", href: "/admin/orgs", icon: "building-2" },
      { label: "Users", href: "/admin/users", icon: "users" },
      { label: "Units", href: "/admin/units", icon: "file-text" },
    ],
  },
  org_admin: {
    role: "org_admin",
    label: "Organization",
    basePath: "/org",
    navItems: [
      { label: "Dashboard", href: "/org", icon: "layout-dashboard" },
      { label: "Teachers", href: "/org/teachers", icon: "graduation-cap" },
      { label: "Students", href: "/org/students", icon: "users" },
      { label: "Courses", href: "/org/courses", icon: "book-open" },
      { label: "Classes", href: "/org/classes", icon: "school" },
      { label: "Units", href: "/org/units", icon: "file-text" },
      { label: "Parent links", href: "/org/parent-links", icon: "link" },
      { label: "Settings", href: "/org/settings", icon: "settings" },
    ],
  },
  teacher: {
    role: "teacher",
    label: "Teacher",
    basePath: "/teacher",
    navItems: [
      { label: "Dashboard", href: "/teacher", icon: "layout-dashboard" },
      { label: "Units", href: "/teacher/units", icon: "file-text" },
      { label: "Problems", href: "/teacher/problems", icon: "puzzle" },
      { label: "Sessions", href: "/teacher/sessions", icon: "video" },
      { label: "Courses", href: "/teacher/courses", icon: "book-open" },
      { label: "Classes", href: "/teacher/classes", icon: "school" },
    ],
  },
  student: {
    role: "student",
    label: "Student",
    basePath: "/student",
    navItems: [
      { label: "Dashboard", href: "/student", icon: "layout-dashboard" },
      { label: "My Classes", href: "/student/classes", icon: "school" },
      { label: "My Work", href: "/student/code", icon: "code" },
      { label: "Help", href: "/student/help", icon: "help-circle" },
    ],
  },
  parent: {
    role: "parent",
    label: "Parent",
    basePath: "/parent",
    // /parent/children was a redirect-only entry; removed in plan 040
    // phase 7. A real children list view is product work that needs its
    // own design pass (deferred).
    navItems: [
      { label: "Dashboard", href: "/parent", icon: "layout-dashboard" },
    ],
  },
};

export function getPortalConfig(role: string): PortalConfig | null {
  return portalConfigs[role] || null;
}

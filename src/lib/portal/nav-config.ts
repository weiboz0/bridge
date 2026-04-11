import type { PortalConfig } from "./types";

export const portalConfigs: Record<string, PortalConfig> = {
  admin: {
    role: "admin",
    label: "Platform Admin",
    basePath: "/admin",
    navItems: [
      { label: "Organizations", href: "/admin/orgs", icon: "building-2" },
      { label: "Users", href: "/admin/users", icon: "users" },
      { label: "Settings", href: "/admin/settings", icon: "settings" },
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
      { label: "Settings", href: "/org/settings", icon: "settings" },
    ],
  },
  teacher: {
    role: "teacher",
    label: "Teacher",
    basePath: "/teacher",
    navItems: [
      { label: "Dashboard", href: "/teacher", icon: "layout-dashboard" },
      { label: "My Courses", href: "/teacher/courses", icon: "book-open" },
      { label: "My Classes", href: "/teacher/classes", icon: "school" },
      { label: "Schedule", href: "/teacher/schedule", icon: "calendar" },
      { label: "Reports", href: "/teacher/reports", icon: "bar-chart-3" },
    ],
  },
  student: {
    role: "student",
    label: "Student",
    basePath: "/student",
    navItems: [
      { label: "Dashboard", href: "/student", icon: "layout-dashboard" },
      { label: "My Classes", href: "/student/classes", icon: "school" },
      { label: "My Code", href: "/student/code", icon: "code" },
      { label: "Help", href: "/student/help", icon: "help-circle" },
    ],
  },
  parent: {
    role: "parent",
    label: "Parent",
    basePath: "/parent",
    navItems: [
      { label: "Dashboard", href: "/parent", icon: "layout-dashboard" },
      { label: "My Children", href: "/parent/children", icon: "users" },
      { label: "Reports", href: "/parent/reports", icon: "file-text" },
    ],
  },
};

export function getPortalConfig(role: string): PortalConfig | null {
  return portalConfigs[role] || null;
}

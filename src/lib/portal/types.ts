export type PortalRole = "admin" | "org_admin" | "teacher" | "student" | "parent";

export interface NavItem {
  label: string;
  href: string;
  icon: string; // lucide-react icon name
}

export interface PortalConfig {
  role: PortalRole;
  label: string;
  basePath: string;
  navItems: NavItem[];
}

export interface UserRole {
  role: PortalRole;
  orgId?: string;
  orgName?: string;
}

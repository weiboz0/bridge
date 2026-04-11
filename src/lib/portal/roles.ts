import type { UserRole, PortalRole } from "./types";

const ROLE_PRIORITY: PortalRole[] = ["admin", "org_admin", "teacher", "student", "parent"];

export interface MembershipRecord {
  role: string;
  status: string;
  orgId: string;
  orgName: string;
  orgStatus: string;
  [key: string]: unknown; // allow extra fields from DB query
}

/**
 * Build user roles from isPlatformAdmin flag and org memberships.
 * Filters out pending/suspended memberships and inactive orgs.
 */
export function buildUserRoles(
  isPlatformAdmin: boolean,
  memberships: MembershipRecord[]
): UserRole[] {
  const roles: UserRole[] = [];

  if (isPlatformAdmin) {
    roles.push({ role: "admin" });
  }

  for (const m of memberships) {
    // Skip inactive memberships or orgs
    if (m.status !== "active") continue;
    if (m.orgStatus !== "active") continue;

    const role = m.role as PortalRole;
    // Deduplicate — don't add the same role twice
    if (!roles.some((r) => r.role === role && r.orgId === m.orgId)) {
      roles.push({
        role,
        orgId: m.orgId,
        orgName: m.orgName,
      });
    }
  }

  return roles;
}

/**
 * Get the primary (highest priority) role for a user.
 */
export function getPrimaryRole(roles: UserRole[]): UserRole | null {
  if (roles.length === 0) return null;

  for (const priority of ROLE_PRIORITY) {
    const match = roles.find((r) => r.role === priority);
    if (match) return match;
  }

  return roles[0];
}

/**
 * Get the portal path for a role.
 */
export function getPortalPath(role: PortalRole): string {
  const paths: Record<PortalRole, string> = {
    admin: "/admin",
    org_admin: "/org",
    teacher: "/teacher",
    student: "/student",
    parent: "/parent",
  };
  return paths[role];
}

/**
 * Get the primary portal path for a user based on their roles.
 */
export function getPrimaryPortalPath(roles: UserRole[]): string {
  const primary = getPrimaryRole(roles);
  if (!primary) return "/onboarding";
  return getPortalPath(primary.role);
}

/**
 * Check if a user is authorized for a specific portal.
 */
export function isAuthorizedForPortal(
  roles: UserRole[],
  portalRole: PortalRole
): boolean {
  return roles.some((r) => r.role === portalRole);
}

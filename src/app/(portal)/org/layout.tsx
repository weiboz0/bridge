import { PortalShell } from "@/components/portal/portal-shell";
import { api, ApiError } from "@/lib/api-client";
import { OrgSwitcher } from "@/components/portal/org-switcher";

interface OrgMembership {
  orgId: string;
  orgName: string;
  role: string;
  status: string;
  orgStatus: string;
}

/**
 * Plan 043 phase 3: render the OrgSwitcher above the org-portal content
 * when the caller has 2+ active org_admin memberships. Memberships are
 * fetched server-side so the switcher is rendered with real data on
 * first paint (no client-side flicker).
 *
 * The switcher itself is hidden when the user has fewer than 2 options.
 */
export default async function OrgPortalLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  let orgAdminOptions: { orgId: string; orgName: string }[] = [];
  try {
    const memberships = await api<OrgMembership[]>("/api/orgs");
    const seen = new Set<string>();
    for (const m of memberships) {
      if (
        m.role === "org_admin" &&
        m.status === "active" &&
        m.orgStatus === "active" &&
        !seen.has(m.orgId)
      ) {
        seen.add(m.orgId);
        orgAdminOptions.push({ orgId: m.orgId, orgName: m.orgName });
      }
    }
  } catch (e) {
    // Non-fatal — falling through means no switcher renders. The
    // org-page itself will surface a real error if /api/orgs is down.
    if (!(e instanceof ApiError)) throw e;
    orgAdminOptions = [];
  }

  return (
    <PortalShell portalRole="org_admin">
      <OrgSwitcher options={orgAdminOptions} />
      {children}
    </PortalShell>
  );
}

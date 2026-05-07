import { PortalShell } from "@/components/portal/portal-shell";
import { ApiError } from "@/lib/api-client";
import { OrgSwitcher } from "@/components/portal/org-switcher";
import { fetchMyOrgs } from "@/lib/portal/org-context";

/**
 * Plan 043 phase 3: render the OrgSwitcher above the org-portal content
 * when the caller has 2+ active org_admin memberships. Memberships are
 * fetched server-side so the switcher is rendered with real data on
 * first paint (no client-side flicker).
 *
 * The switcher itself is hidden when the user has fewer than 2 options.
 *
 * Plan 077 code-review BLOCKER fix: use the cached `fetchMyOrgs` helper
 * (via React's `cache()`) so this layout fetch and the page-level
 * `resolveOrgContext` fetch share one round-trip per render.
 */
export default async function OrgPortalLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  let orgAdminOptions: { orgId: string; orgName: string }[] = [];
  try {
    const memberships = await fetchMyOrgs();
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

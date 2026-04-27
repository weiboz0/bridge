import { api, ApiError } from "@/lib/api-client";
import { OrgSettingsCard, type OrgSettingsData } from "@/components/org/org-settings-card";
import type { OrgListError } from "@/components/org/org-list-state";
import {
  parseOrgIdFromSearchParams,
  appendOrgId,
} from "@/lib/portal/org-context";

interface DashboardPayload {
  org: OrgSettingsData;
}

// Reuses /api/org/dashboard since the org payload it returns is exactly
// what settings needs to render. Adding a separate /api/org/settings
// would be a contract-clarity nicety but trivially cheap to add later
// — the dashboard endpoint already authorizes org_admin.
export default async function OrgSettingsPage({
  searchParams,
}: {
  searchParams?: Promise<{ orgId?: string }>;
}) {
  const orgId = parseOrgIdFromSearchParams(await searchParams);
  let org: OrgSettingsData | null = null;
  let error: OrgListError | null = null;
  try {
    const payload = await api<DashboardPayload>(
      appendOrgId("/api/org/dashboard", orgId)
    );
    org = payload.org;
  } catch (e) {
    if (e instanceof ApiError) {
      error = { status: e.status, message: e.message };
    } else {
      error = { status: null, message: e instanceof Error ? e.message : String(e) };
    }
  }

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">Settings</h1>
      <OrgSettingsCard org={org} error={error} />
    </div>
  );
}

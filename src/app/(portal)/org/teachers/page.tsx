import { api, ApiError } from "@/lib/api-client";
import { TeachersList, type OrgMemberRow } from "@/components/org/teachers-list";
import type { OrgListError } from "@/components/org/org-list-state";
import {
  appendOrgId,
  resolveOrgIdServerSide,
} from "@/lib/portal/org-context";
import { InviteMemberButton } from "@/components/org/invite-member-button";
import { getIdentity } from "@/lib/identity";

export default async function OrgTeachersPage({
  searchParams,
}: {
  searchParams?: Promise<{ orgId?: string }>;
}) {
  // Codex post-impl Q5: row actions need a real orgId to construct
  // /api/orgs/{orgId}/members/... URLs. The legacy query-fallback
  // (no `?orgId=`) lets the list endpoint auto-resolve, but row
  // actions would otherwise build broken `/api/orgs//members/...`.
  // Resolve to the caller's first active org_admin membership when
  // the query is absent.
  const sp = await searchParams;
  const orgId = await resolveOrgIdServerSide(sp);
  const identity = await getIdentity();
  const currentUserId = identity?.userId ?? "";

  if (!orgId) {
    return (
      <div className="p-6 max-w-2xl space-y-2">
        <h1 className="text-2xl font-bold">Teachers</h1>
        <p className="text-muted-foreground">
          You need to be an org admin in at least one active organization to
          manage teachers.
        </p>
      </div>
    );
  }

  let data: OrgMemberRow[] | null = null;
  let error: OrgListError | null = null;
  try {
    data = await api<OrgMemberRow[]>(appendOrgId("/api/org/teachers", orgId));
  } catch (e) {
    if (e instanceof ApiError) {
      error = { status: e.status, message: e.message };
    } else {
      error = { status: null, message: e instanceof Error ? e.message : String(e) };
    }
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between gap-4">
        <h1 className="text-2xl font-bold">
          Teachers{data ? ` (${data.length})` : ""}
        </h1>
        <InviteMemberButton orgId={orgId} role="teacher" />
      </div>
      <TeachersList
        data={data}
        error={error}
        orgId={orgId}
        currentUserId={currentUserId}
      />
    </div>
  );
}

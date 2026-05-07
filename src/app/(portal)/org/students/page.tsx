import { api, ApiError } from "@/lib/api-client";
import { StudentsList } from "@/components/org/students-list";
import type { OrgMemberRow } from "@/components/org/teachers-list";
import type { OrgListError } from "@/components/org/org-list-state";
import { resolveOrgContext, appendOrgId } from "@/lib/portal/org-context";
import { handleOrgContext } from "@/components/portal/org-context-guard";
import { InviteMemberButton } from "@/components/org/invite-member-button";
import { getIdentity } from "@/lib/identity";

export default async function OrgStudentsPage({
  searchParams,
}: {
  searchParams?: Promise<{ orgId?: string }>;
}) {
  const sp = await searchParams;
  const ctx = await resolveOrgContext(sp);
  const handled = handleOrgContext(ctx);
  if (handled.kind === "guard") return handled.element;
  const { orgId, orgName } = handled;

  const identity = await getIdentity();
  const currentUserId = identity?.userId ?? "";

  let data: OrgMemberRow[] | null = null;
  let error: OrgListError | null = null;
  try {
    data = await api<OrgMemberRow[]>(appendOrgId("/api/org/students", orgId));
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
          {orgName} — Students{data ? ` (${data.length})` : ""}
        </h1>
        <InviteMemberButton orgId={orgId} role="student" />
      </div>
      <StudentsList
        data={data}
        error={error}
        orgId={orgId}
        currentUserId={currentUserId}
      />
    </div>
  );
}

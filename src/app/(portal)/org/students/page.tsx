import { api, ApiError } from "@/lib/api-client";
import { StudentsList } from "@/components/org/students-list";
import type { OrgMemberRow } from "@/components/org/teachers-list";
import type { OrgListError } from "@/components/org/org-list-state";
import {
  appendOrgId,
  resolveOrgIdServerSide,
} from "@/lib/portal/org-context";
import { InviteMemberButton } from "@/components/org/invite-member-button";
import { getIdentity } from "@/lib/identity";

export default async function OrgStudentsPage({
  searchParams,
}: {
  searchParams?: Promise<{ orgId?: string }>;
}) {
  // Codex post-impl Q5: row actions need a real orgId. See the
  // matching note on /org/teachers/page.tsx for the rationale.
  const sp = await searchParams;
  const orgId = await resolveOrgIdServerSide(sp);
  const identity = await getIdentity();
  const currentUserId = identity?.userId ?? "";

  if (!orgId) {
    return (
      <div className="p-6 max-w-2xl space-y-2">
        <h1 className="text-2xl font-bold">Students</h1>
        <p className="text-muted-foreground">
          You need to be an org admin in at least one active organization to
          manage students.
        </p>
      </div>
    );
  }

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
          Students{data ? ` (${data.length})` : ""}
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

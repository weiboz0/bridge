import { api, ApiError } from "@/lib/api-client";
import { TeachersList, type OrgMemberRow } from "@/components/org/teachers-list";
import type { OrgListError } from "@/components/org/org-list-state";

export default async function OrgTeachersPage() {
  let data: OrgMemberRow[] | null = null;
  let error: OrgListError | null = null;
  try {
    data = await api<OrgMemberRow[]>("/api/org/teachers");
  } catch (e) {
    if (e instanceof ApiError) {
      error = { status: e.status, message: e.message };
    } else {
      error = { status: null, message: e instanceof Error ? e.message : String(e) };
    }
  }

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">Teachers{data ? ` (${data.length})` : ""}</h1>
      <TeachersList data={data} error={error} />
    </div>
  );
}

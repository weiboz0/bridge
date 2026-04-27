import { api, ApiError } from "@/lib/api-client";
import { StudentsList } from "@/components/org/students-list";
import type { OrgMemberRow } from "@/components/org/teachers-list";
import type { OrgListError } from "@/components/org/org-list-state";

export default async function OrgStudentsPage() {
  let data: OrgMemberRow[] | null = null;
  let error: OrgListError | null = null;
  try {
    data = await api<OrgMemberRow[]>("/api/org/students");
  } catch (e) {
    if (e instanceof ApiError) {
      error = { status: e.status, message: e.message };
    } else {
      error = { status: null, message: e instanceof Error ? e.message : String(e) };
    }
  }

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">Students{data ? ` (${data.length})` : ""}</h1>
      <StudentsList data={data} error={error} />
    </div>
  );
}

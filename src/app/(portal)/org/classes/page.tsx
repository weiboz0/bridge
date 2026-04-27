import { api, ApiError } from "@/lib/api-client";
import { ClassesList, type OrgClassRow } from "@/components/org/classes-list";
import type { OrgListError } from "@/components/org/org-list-state";

export default async function OrgClassesPage() {
  let data: OrgClassRow[] | null = null;
  let error: OrgListError | null = null;
  try {
    data = await api<OrgClassRow[]>("/api/org/classes");
  } catch (e) {
    if (e instanceof ApiError) {
      error = { status: e.status, message: e.message };
    } else {
      error = { status: null, message: e instanceof Error ? e.message : String(e) };
    }
  }

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">Classes{data ? ` (${data.length})` : ""}</h1>
      <ClassesList data={data} error={error} />
    </div>
  );
}

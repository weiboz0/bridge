import { api, ApiError } from "@/lib/api-client";
import { ClassesList, type OrgClassRow } from "@/components/org/classes-list";
import type { OrgListError } from "@/components/org/org-list-state";
import {
  parseOrgIdFromSearchParams,
  appendOrgId,
} from "@/lib/portal/org-context";

export default async function OrgClassesPage({
  searchParams,
}: {
  searchParams?: Promise<{ orgId?: string }>;
}) {
  const orgId = parseOrgIdFromSearchParams(await searchParams);
  let data: OrgClassRow[] | null = null;
  let error: OrgListError | null = null;
  try {
    data = await api<OrgClassRow[]>(appendOrgId("/api/org/classes", orgId));
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

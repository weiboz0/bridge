import { api, ApiError } from "@/lib/api-client";
import { ClassesList, type OrgClassRow } from "@/components/org/classes-list";
import type { OrgListError } from "@/components/org/org-list-state";
import { resolveOrgContext, appendOrgId } from "@/lib/portal/org-context";
import { handleOrgContext } from "@/components/portal/org-context-guard";

export default async function OrgClassesPage({
  searchParams,
}: {
  searchParams?: Promise<{ orgId?: string }>;
}) {
  const sp = await searchParams;
  const ctx = await resolveOrgContext(sp);
  const handled = handleOrgContext(ctx);
  if (handled.kind === "guard") return handled.element;
  const { orgId, orgName } = handled;

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
      <h1 className="text-2xl font-bold">{orgName} — Classes{data ? ` (${data.length})` : ""}</h1>
      <ClassesList data={data} error={error} orgId={orgId} />
    </div>
  );
}

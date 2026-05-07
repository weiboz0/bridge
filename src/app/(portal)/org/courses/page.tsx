import { api, ApiError } from "@/lib/api-client";
import { CoursesList, type OrgCourseRow } from "@/components/org/courses-list";
import type { OrgListError } from "@/components/org/org-list-state";
import { resolveOrgContext, appendOrgId } from "@/lib/portal/org-context";
import { handleOrgContext } from "@/components/portal/org-context-guard";

export default async function OrgCoursesPage({
  searchParams,
}: {
  searchParams?: Promise<{ orgId?: string }>;
}) {
  const sp = await searchParams;
  const ctx = await resolveOrgContext(sp);
  const handled = handleOrgContext(ctx);
  if (handled.kind === "guard") return handled.element;
  const { orgId } = handled;

  let data: OrgCourseRow[] | null = null;
  let error: OrgListError | null = null;
  try {
    data = await api<OrgCourseRow[]>(appendOrgId("/api/org/courses", orgId));
  } catch (e) {
    if (e instanceof ApiError) {
      error = { status: e.status, message: e.message };
    } else {
      error = { status: null, message: e instanceof Error ? e.message : String(e) };
    }
  }

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">Courses{data ? ` (${data.length})` : ""}</h1>
      <CoursesList data={data} error={error} />
    </div>
  );
}

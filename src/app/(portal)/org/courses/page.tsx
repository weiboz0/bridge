import { api, ApiError } from "@/lib/api-client";
import { CoursesList, type OrgCourseRow } from "@/components/org/courses-list";
import type { OrgListError } from "@/components/org/org-list-state";

export default async function OrgCoursesPage() {
  let data: OrgCourseRow[] | null = null;
  let error: OrgListError | null = null;
  try {
    data = await api<OrgCourseRow[]>("/api/org/courses");
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

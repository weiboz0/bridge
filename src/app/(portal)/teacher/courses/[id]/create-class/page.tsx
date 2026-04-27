import { notFound } from "next/navigation";
import { api, ApiError } from "@/lib/api-client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { CreateClassForm } from "@/components/teacher/create-class-form";
import { isValidUUID } from "@/lib/utils";

interface Course {
  id: string;
  title: string;
  orgId: string;
}

export default async function CreateClassPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  // Plan 043 phase 6.2: malformed course IDs used to surface as a blank
  // page + console "API 400" error. Mirror the parent course-detail
  // page's UUID guard so bad URLs map to a stable not-found state.
  if (!isValidUUID(id)) notFound();

  let course: Course;
  try {
    course = await api<Course>(`/api/courses/${id}`);
  } catch (e) {
    if (e instanceof ApiError && (e.status === 404 || e.status === 403)) notFound();
    throw e;
  }

  return (
    <div className="p-6 max-w-lg">
      <Card>
        <CardHeader>
          <CardTitle>Create Class from &ldquo;{course.title}&rdquo;</CardTitle>
        </CardHeader>
        <CardContent>
          <CreateClassForm courseId={id} orgId={course.orgId} />
        </CardContent>
      </Card>
    </div>
  );
}

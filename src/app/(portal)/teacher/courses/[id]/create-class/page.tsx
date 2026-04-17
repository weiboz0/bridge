import { notFound } from "next/navigation";
import { api, ApiError } from "@/lib/api-client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { CreateClassForm } from "@/components/teacher/create-class-form";

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

import { notFound } from "next/navigation";
import { api, ApiError } from "@/lib/api-client";
import { Card, CardContent } from "@/components/ui/card";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";
import { AddTopicForm } from "@/components/teacher/add-topic-form";
import { DeleteTopicButton } from "@/components/teacher/delete-topic-button";

interface Course {
  id: string;
  title: string;
  gradeLevel: string;
  language: string;
  createdBy: string;
}

interface Topic {
  id: string;
  title: string;
  description: string | null;
}

interface ClassItem {
  id: string;
  title: string;
  term: string | null;
  status: string;
  courseId: string;
  memberRole: string;
}

export default async function TeacherCourseDetailPage({
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

  const [topicList, allClasses] = await Promise.all([
    api<Topic[]>(`/api/courses/${id}/topics`),
    api<ClassItem[]>("/api/classes/mine"),
  ]);

  const classList = allClasses.filter((c) => c.courseId === id);

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{course.title}</h1>
          <p className="text-muted-foreground">{course.gradeLevel} · {course.language}</p>
        </div>
        <Link
          href={`/teacher/courses/${id}/create-class`}
          className={buttonVariants()}
        >
          Create Class
        </Link>
      </div>

      <div className="grid gap-6 md:grid-cols-2">
        <div className="space-y-4">
          <h2 className="text-lg font-semibold">Topics ({topicList.length})</h2>

          <AddTopicForm courseId={id} />

          {topicList.length === 0 ? (
            <p className="text-sm text-muted-foreground">No topics yet. Add your first topic above.</p>
          ) : (
            <div className="space-y-2">
              {topicList.map((topic, i) => (
                <Card key={topic.id}>
                  <CardContent className="py-3 flex items-center justify-between">
                    <Link href={`/teacher/courses/${id}/topics/${topic.id}`} className="flex-1">
                      <p className="font-medium hover:text-primary">{i + 1}. {topic.title}</p>
                      {topic.description && (
                        <p className="text-sm text-muted-foreground mt-1">{topic.description}</p>
                      )}
                    </Link>
                    <DeleteTopicButton courseId={id} topicId={topic.id} />
                  </CardContent>
                </Card>
              ))}
            </div>
          )}
        </div>

        <div className="space-y-4">
          <h2 className="text-lg font-semibold">Classes ({classList.length})</h2>
          {classList.length === 0 ? (
            <p className="text-sm text-muted-foreground">No classes yet. Create one to start teaching.</p>
          ) : (
            <div className="space-y-2">
              {classList.map((cls) => (
                <Link key={cls.id} href={`/teacher/classes/${cls.id}`}>
                  <Card className="hover:border-primary transition-colors cursor-pointer mb-2">
                    <CardContent className="py-3">
                      <p className="font-medium">{cls.title}</p>
                      <p className="text-sm text-muted-foreground">{cls.term || "No term"} · {cls.status}</p>
                    </CardContent>
                  </Card>
                </Link>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

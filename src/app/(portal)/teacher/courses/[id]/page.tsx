import { notFound } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getCourse } from "@/lib/courses";
import { listTopicsByCourse } from "@/lib/topics";
import { listClassesByCourse } from "@/lib/classes";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";

export default async function TeacherCourseDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const session = await auth();
  const { id } = await params;
  const course = await getCourse(db, id);
  if (!course) notFound();

  // Verify teacher is the course creator
  if (course.createdBy !== session!.user.id && !session!.user.isPlatformAdmin) notFound();

  const [topicList, classList] = await Promise.all([
    listTopicsByCourse(db, id),
    listClassesByCourse(db, id),
  ]);

  return (
    <div className="p-6 space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{course.title}</h1>
        <p className="text-muted-foreground">{course.gradeLevel} · {course.language}</p>
        {course.description && <p className="mt-2">{course.description}</p>}
      </div>

      <div className="grid gap-6 md:grid-cols-2">
        <div>
          <h2 className="text-lg font-semibold mb-3">Topics ({topicList.length})</h2>
          {topicList.length === 0 ? (
            <p className="text-sm text-muted-foreground">No topics yet.</p>
          ) : (
            <div className="space-y-2">
              {topicList.map((topic, i) => (
                <Card key={topic.id}>
                  <CardContent className="py-3">
                    <p className="font-medium">{i + 1}. {topic.title}</p>
                    {topic.description && (
                      <p className="text-sm text-muted-foreground mt-1">{topic.description}</p>
                    )}
                  </CardContent>
                </Card>
              ))}
            </div>
          )}
        </div>

        <div>
          <h2 className="text-lg font-semibold mb-3">Classes ({classList.length})</h2>
          {classList.length === 0 ? (
            <p className="text-sm text-muted-foreground">No classes created from this course yet.</p>
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

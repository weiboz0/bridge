import { notFound } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getClass } from "@/lib/classes";
import { getCourse } from "@/lib/courses";
import { listTopicsByCourse } from "@/lib/topics";
import { listClassMembers } from "@/lib/class-memberships";
import { parseLessonContent } from "@/lib/lesson-content";
import { LessonRenderer } from "@/components/lesson/lesson-renderer";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";

export default async function StudentClassDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const session = await auth();
  const { id } = await params;
  const cls = await getClass(db, id);
  if (!cls) notFound();

  const members = await listClassMembers(db, id);
  const isEnrolled = members.some((m) => m.userId === session!.user.id);
  if (!isEnrolled && !session!.user.isPlatformAdmin) notFound();

  const course = await getCourse(db, cls.courseId);
  const topics = course ? await listTopicsByCourse(db, course.id) : [];

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{cls.title}</h1>
          <p className="text-muted-foreground">
            {course?.title || ""} · {cls.term || "No term"}
          </p>
        </div>
        <Link
          href={`/dashboard/classrooms/${id}/editor`}
          className={buttonVariants()}
        >
          Open Editor
        </Link>
      </div>

      {topics.length > 0 && (
        <div className="space-y-4">
          <h2 className="text-lg font-semibold">Topics</h2>
          {topics.map((topic, i) => {
            const content = parseLessonContent(topic.lessonContent);
            return (
              <Card key={topic.id}>
                <CardHeader>
                  <CardTitle className="text-base">{i + 1}. {topic.title}</CardTitle>
                  {topic.description && (
                    <CardDescription>{topic.description}</CardDescription>
                  )}
                </CardHeader>
                {content.blocks.length > 0 && (
                  <CardContent>
                    <LessonRenderer content={content} />
                  </CardContent>
                )}
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}

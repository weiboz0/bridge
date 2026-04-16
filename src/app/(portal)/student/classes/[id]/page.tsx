import { notFound } from "next/navigation";
import { api } from "@/lib/api-client";
import { parseLessonContent } from "@/lib/lesson-content";
import { LessonRenderer } from "@/components/lesson/lesson-renderer";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";

interface ClassDetail {
  id: string;
  title: string;
  term: string;
  courseId: string;
}

interface CourseDetail {
  id: string;
  title: string;
}

interface TopicItem {
  id: string;
  title: string;
  description: string;
  lessonContent: string;
}

interface SessionItem {
  id: string;
  status: string;
  startedAt: string;
  participantCount: number;
}

export default async function StudentClassDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  let cls: ClassDetail;
  try {
    cls = await api<ClassDetail>(`/api/classes/${id}`);
  } catch {
    notFound();
  }

  const [course, topics, sessions] = await Promise.all([
    api<CourseDetail>(`/api/courses/${cls.courseId}`).catch(() => null),
    api<TopicItem[]>(`/api/courses/${cls.courseId}/topics`).catch(() => []),
    api<SessionItem[]>(`/api/sessions/by-class/${id}`).catch(() => []),
  ]);

  const activeSession = sessions.find((s) => s.status === "active");

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{cls.title}</h1>
          <p className="text-muted-foreground">
            {course?.title || ""} · {cls.term || "No term"}
          </p>
        </div>
      </div>

      {activeSession && (
        <Link href={`/student/classes/${id}/session/${activeSession.id}`}>
          <Card className="border-green-500 bg-green-50 dark:bg-green-950/20 hover:border-green-600 transition-colors cursor-pointer">
            <CardHeader>
              <CardTitle className="text-lg text-green-700 dark:text-green-400">
                Live Session — Join Now
              </CardTitle>
              <CardDescription>
                Started {new Date(activeSession.startedAt).toLocaleTimeString()} · {activeSession.participantCount} students online
              </CardDescription>
            </CardHeader>
          </Card>
        </Link>
      )}

      {!activeSession && (
        <Card>
          <CardContent className="py-6 text-center text-muted-foreground">
            <p>No live session right now. Your teacher will start one soon.</p>
          </CardContent>
        </Card>
      )}

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

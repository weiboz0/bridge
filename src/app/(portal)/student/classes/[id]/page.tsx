import { notFound } from "next/navigation";
import { api } from "@/lib/api-client";
import { isValidUUID } from "@/lib/utils";
import { db } from "@/lib/db";
import { listLinkedUnitsByTopicIds } from "@/lib/session-topics";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import Link from "next/link";

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
}

interface ProblemItem {
  id: string;
  title: string;
  language: string;
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
  if (!isValidUUID(id)) notFound();

  let cls: ClassDetail;
  try {
    cls = await api<ClassDetail>(`/api/classes/${id}`);
  } catch {
    notFound();
  }

  const logAndDefault = async <T,>(p: Promise<T>, fallback: T, label: string): Promise<T> => {
    try {
      return await p;
    } catch (e) {
      console.warn(`[student/class] ${label} failed:`, e);
      return fallback;
    }
  };
  const [course, topics, sessions] = await Promise.all([
    logAndDefault(api<CourseDetail>(`/api/courses/${cls.courseId}`), null, "course"),
    logAndDefault(api<TopicItem[]>(`/api/courses/${cls.courseId}/topics`), [], "topics"),
    logAndDefault(api<SessionItem[]>(`/api/sessions/by-class/${id}`), [], "sessions"),
  ]);

  const activeSession = sessions.find((s) => s.status === "live");

  const problemsByTopic = new Map<string, ProblemItem[]>();
  await Promise.all(
    topics.map(async (t) => {
      const list = await logAndDefault(
        api<ProblemItem[]>(`/api/topics/${t.id}/problems`),
        [] as ProblemItem[],
        `problems for topic ${t.id}`
      );
      problemsByTopic.set(t.id, list);
    })
  );

  // Plan 044 phase 2: render the linked teaching_unit per topic instead
  // of topic.lessonContent. Single bulk query — no N+1.
  const linkedUnits = topics.length > 0
    ? await listLinkedUnitsByTopicIds(db, topics.map((t) => t.id))
    : {};

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
        <Link href={`/student/sessions/${activeSession.id}`}>
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

      <div className="space-y-4">
        <h2 className="text-lg font-semibold">Topics</h2>
        {topics.length === 0 && (
          <Card>
            <CardContent className="py-6 text-center text-muted-foreground">
              <p>No topics yet.</p>
            </CardContent>
          </Card>
        )}
        {topics.length > 0 && topics.map((topic, i) => {
            const problems = problemsByTopic.get(topic.id) ?? [];
            const linkedUnit = linkedUnits[topic.id];
            return (
              <Card key={topic.id}>
                <CardHeader>
                  <CardTitle className="text-base">{i + 1}. {topic.title}</CardTitle>
                  {topic.description && (
                    <CardDescription>{topic.description}</CardDescription>
                  )}
                </CardHeader>
                {/* Linked teaching_unit reference, or empty-state. */}
                <CardContent>
                  {linkedUnit ? (
                    <Link
                      href={`/student/units/${linkedUnit.unitId}`}
                      className="inline-flex items-center gap-2 rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm hover:border-amber-400 hover:text-amber-800"
                    >
                      <span className="font-medium">{linkedUnit.unitTitle}</span>
                      <span className="font-mono text-[10px] uppercase tracking-[0.16em] text-muted-foreground">
                        {linkedUnit.unitMaterialType}
                      </span>
                    </Link>
                  ) : (
                    <p className="text-sm text-muted-foreground italic">
                      No material yet for this topic.
                    </p>
                  )}
                </CardContent>
                {problems.length > 0 && (
                  <CardContent className="border-t pt-3">
                    <p className="mb-2 font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
                      Problems · {problems.length}
                    </p>
                    <div className="flex flex-wrap gap-1.5">
                      {problems.map((p) => (
                        <Link
                          key={p.id}
                          href={`/student/classes/${id}/problems/${p.id}`}
                          className="inline-flex items-center gap-1.5 rounded-md border border-zinc-200 bg-white px-2.5 py-1 text-sm hover:border-amber-400 hover:text-amber-800"
                        >
                          <span className="font-medium tracking-tight">{p.title}</span>
                          <span className="font-mono text-[10px] uppercase tracking-[0.16em] text-muted-foreground">
                            {p.language}
                          </span>
                        </Link>
                      ))}
                    </div>
                  </CardContent>
                )}
              </Card>
            );
          })}
      </div>
    </div>
  );
}

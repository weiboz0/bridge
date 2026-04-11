import { notFound, redirect } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getCourse } from "@/lib/courses";
import { listTopicsByCourse, createTopic, deleteTopic, reorderTopics } from "@/lib/topics";
import { listClassesByCourse } from "@/lib/classes";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";
import { revalidatePath } from "next/cache";

export default async function TeacherCourseDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const session = await auth();
  const { id } = await params;
  const course = await getCourse(db, id);
  if (!course) notFound();
  if (course.createdBy !== session!.user.id && !session!.user.isPlatformAdmin) notFound();

  const [topicList, classList] = await Promise.all([
    listTopicsByCourse(db, id),
    listClassesByCourse(db, id),
  ]);

  async function handleAddTopic(formData: FormData) {
    "use server";
    const { auth: getAuth } = await import("@/lib/auth");
    const { db: database } = await import("@/lib/db");
    const { getCourse: get } = await import("@/lib/courses");
    const { createTopic: create } = await import("@/lib/topics");
    const sess = await getAuth();
    if (!sess?.user?.id) return;
    const c = await get(database, id);
    if (!c || (c.createdBy !== sess.user.id && !sess.user.isPlatformAdmin)) return;
    const title = formData.get("title") as string;
    if (!title) return;
    await create(database, { courseId: id, title });
    revalidatePath(`/teacher/courses/${id}`);
  }

  async function handleDeleteTopic(formData: FormData) {
    "use server";
    const { auth: getAuth } = await import("@/lib/auth");
    const { db: database } = await import("@/lib/db");
    const { getCourse: get } = await import("@/lib/courses");
    const { deleteTopic: del } = await import("@/lib/topics");
    const sess = await getAuth();
    if (!sess?.user?.id) return;
    const c = await get(database, id);
    if (!c || (c.createdBy !== sess.user.id && !sess.user.isPlatformAdmin)) return;
    const topicId = formData.get("topicId") as string;
    if (!topicId) return;
    await del(database, topicId);
    revalidatePath(`/teacher/courses/${id}`);
  }

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

          <form action={handleAddTopic} className="flex gap-2">
            <Input name="title" placeholder="New topic title" required className="flex-1" />
            <Button type="submit" size="sm">Add Topic</Button>
          </form>

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
                    <form action={handleDeleteTopic}>
                      <input type="hidden" name="topicId" value={topic.id} />
                      <Button type="submit" variant="ghost" size="sm" className="text-destructive">×</Button>
                    </form>
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

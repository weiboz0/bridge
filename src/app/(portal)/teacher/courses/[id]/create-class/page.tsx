import { notFound, redirect } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getCourse } from "@/lib/courses";
import { createClass } from "@/lib/classes";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { revalidatePath } from "next/cache";

export default async function CreateClassPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const session = await auth();
  const { id } = await params;
  const course = await getCourse(db, id);
  if (!course) notFound();
  if (course.createdBy !== session!.user.id && !session!.user.isPlatformAdmin) notFound();

  async function handleCreate(formData: FormData) {
    "use server";
    const { auth: getAuth } = await import("@/lib/auth");
    const { db: database } = await import("@/lib/db");
    const { createClass: create } = await import("@/lib/classes");
    const { getCourse: get } = await import("@/lib/courses");
    const sess = await getAuth();
    if (!sess?.user?.id) return;

    const title = formData.get("title") as string;
    const term = formData.get("term") as string;
    if (!title) return;

    const c = await get(database, id);
    if (!c) return;

    const cls = await create(database, {
      courseId: id,
      orgId: c.orgId,
      title,
      term,
      createdBy: sess.user.id,
    });

    redirect(`/teacher/classes/${cls.id}`);
  }

  return (
    <div className="p-6 max-w-lg">
      <Card>
        <CardHeader>
          <CardTitle>Create Class from "{course.title}"</CardTitle>
        </CardHeader>
        <CardContent>
          <form action={handleCreate} className="space-y-4">
            <div>
              <Label>Class Title</Label>
              <Input name="title" placeholder="e.g., Fall 2026 Period 3" required />
            </div>
            <div>
              <Label>Term (optional)</Label>
              <Input name="term" placeholder="e.g., Fall 2026" />
            </div>
            <Button type="submit">Create Class</Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}

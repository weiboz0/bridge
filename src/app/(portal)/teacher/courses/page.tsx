import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { listCoursesByCreator, createCourse } from "@/lib/courses";
import { getUserMemberships } from "@/lib/org-memberships";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import Link from "next/link";
import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";

export default async function TeacherCoursesPage() {
  const session = await auth();
  const courses = await listCoursesByCreator(db, session!.user.id);
  const memberships = await getUserMemberships(db, session!.user.id);
  const teacherOrgs = memberships.filter(
    (m) => (m.role === "teacher" || m.role === "org_admin") && m.status === "active"
  );

  async function handleCreateCourse(formData: FormData) {
    "use server";
    const { auth: getAuth } = await import("@/lib/auth");
    const { db: database } = await import("@/lib/db");
    const { createCourse: create } = await import("@/lib/courses");
    const sess = await getAuth();
    if (!sess?.user?.id) return;

    const title = formData.get("title") as string;
    const orgId = formData.get("orgId") as string;
    const gradeLevel = formData.get("gradeLevel") as string;
    if (!title || !orgId || !gradeLevel) return;

    const course = await create(database, {
      orgId,
      createdBy: sess.user.id,
      title,
      gradeLevel: gradeLevel as "K-5" | "6-8" | "9-12",
    });

    redirect(`/teacher/courses/${course.id}`);
  }

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">My Courses</h1>

      {teacherOrgs.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Create Course</CardTitle>
          </CardHeader>
          <CardContent>
            <form action={handleCreateCourse} className="flex gap-3 items-end flex-wrap">
              <div>
                <Label className="text-xs">Title</Label>
                <Input name="title" placeholder="e.g., Intro to Python" required className="w-48" />
              </div>
              <div>
                <Label className="text-xs">Organization</Label>
                <select name="orgId" className="border rounded px-2 py-1.5 text-sm bg-background" required>
                  {teacherOrgs.map((m) => (
                    <option key={m.orgId} value={m.orgId}>{m.orgName}</option>
                  ))}
                </select>
              </div>
              <div>
                <Label className="text-xs">Grade Level</Label>
                <select name="gradeLevel" className="border rounded px-2 py-1.5 text-sm bg-background" required>
                  <option value="K-5">K-5</option>
                  <option value="6-8">6-8</option>
                  <option value="9-12">9-12</option>
                </select>
              </div>
              <Button type="submit" size="sm">Create</Button>
            </form>
          </CardContent>
        </Card>
      )}

      {courses.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            <p>No courses yet. Create your first course above.</p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {courses.map((course) => (
            <Link key={course.id} href={`/teacher/courses/${course.id}`}>
              <Card className="hover:border-primary transition-colors cursor-pointer">
                <CardHeader>
                  <CardTitle className="text-lg">{course.title}</CardTitle>
                  <CardDescription>
                    {course.gradeLevel} · {course.language}
                    {course.isPublished && " · Published"}
                  </CardDescription>
                </CardHeader>
                {course.description && (
                  <CardContent>
                    <p className="text-sm text-muted-foreground line-clamp-2">{course.description}</p>
                  </CardContent>
                )}
              </Card>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}

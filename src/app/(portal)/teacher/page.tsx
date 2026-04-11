import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { listCoursesByCreator } from "@/lib/courses";
import { listClassesByUser } from "@/lib/classes";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";

export default async function TeacherDashboard() {
  const session = await auth();
  const [courses, classes] = await Promise.all([
    listCoursesByCreator(db, session!.user.id),
    listClassesByUser(db, session!.user.id),
  ]);

  const myClasses = classes.filter((c) => c.memberRole === "instructor");

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Teacher Dashboard</h1>
        <Link href="/teacher/courses" className={buttonVariants()}>
          Create Course
        </Link>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm text-muted-foreground">My Courses</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-3xl font-bold">{courses.length}</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-sm text-muted-foreground">My Classes</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-3xl font-bold">{myClasses.length}</p>
          </CardContent>
        </Card>
      </div>

      {myClasses.length > 0 && (
        <div>
          <h2 className="text-lg font-semibold mb-3">Active Classes</h2>
          <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
            {myClasses.filter((c) => c.status === "active").map((cls) => (
              <Link key={cls.id} href={`/teacher/classes/${cls.id}`}>
                <Card className="hover:border-primary transition-colors cursor-pointer">
                  <CardHeader>
                    <CardTitle className="text-lg">{cls.title}</CardTitle>
                    <CardDescription>{cls.term || "No term"}</CardDescription>
                  </CardHeader>
                </Card>
              </Link>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

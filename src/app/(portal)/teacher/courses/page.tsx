import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { listCoursesByCreator } from "@/lib/courses";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";

export default async function TeacherCoursesPage() {
  const session = await auth();
  const courses = await listCoursesByCreator(db, session!.user.id);

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">My Courses</h1>
      </div>

      {courses.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            <p>No courses yet. Create your first course to get started.</p>
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

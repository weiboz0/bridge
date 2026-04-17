import { api } from "@/lib/api-client";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import Link from "next/link";
import { CreateCourseForm } from "@/components/teacher/create-course-form";

interface Course {
  id: string;
  title: string;
  description: string | null;
  gradeLevel: string;
  language: string;
  isPublished: boolean;
}

interface TeacherOrg {
  orgId: string;
  orgName: string;
}

interface TeacherCoursesData {
  courses: Course[];
  teacherOrgs: TeacherOrg[];
}

export default async function TeacherCoursesPage() {
  const data = await api<TeacherCoursesData>("/api/teacher/courses");
  const { courses, teacherOrgs } = data;

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">My Courses</h1>

      {teacherOrgs.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Create Course</CardTitle>
          </CardHeader>
          <CardContent>
            <CreateCourseForm teacherOrgs={teacherOrgs} />
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

import { api } from "@/lib/api-client";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";

interface CourseItem { id: string; title: string }
interface ClassItem { id: string; title: string; term: string; status: string; memberRole: string }
interface SessionItem { id: string; status: string; participantCount: number }

export default async function TeacherDashboard() {
  const [courses, allClasses] = await Promise.all([
    api<CourseItem[]>("/api/teacher/courses").then((d) => (d as any).courses ?? []),
    api<ClassItem[]>("/api/classes/mine"),
  ]);

  const myClasses = allClasses.filter((c) => c.memberRole === "instructor");

  // Check for active sessions per class
  const activeSessions = new Map<string, SessionItem>();
  await Promise.all(
    myClasses.map(async (cls) => {
      const session = await api<SessionItem | null>(`/api/sessions/active/${cls.id}`).catch(() => null);
      if (session) activeSessions.set(cls.id, session);
    })
  );

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
          <h2 className="text-lg font-semibold mb-3">My Classes</h2>
          <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
            {myClasses.map((cls) => {
              const active = activeSessions.get(cls.id);
              return (
                <Link key={cls.id} href={active
                  ? `/teacher/classes/${cls.id}/session/${active.id}/dashboard`
                  : `/teacher/classes/${cls.id}`
                }>
                  <Card className={`hover:border-primary transition-colors cursor-pointer ${active ? "border-green-500" : ""}`}>
                    <CardHeader>
                      <div className="flex items-center justify-between">
                        <CardTitle className="text-lg">{cls.title}</CardTitle>
                        {active && (
                          <span className="text-xs font-medium text-green-600 bg-green-100 dark:bg-green-900/30 dark:text-green-400 px-2 py-0.5 rounded-full">
                            Live · {active.participantCount}
                          </span>
                        )}
                      </div>
                      <CardDescription>{cls.term || "No term"}</CardDescription>
                    </CardHeader>
                  </Card>
                </Link>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}

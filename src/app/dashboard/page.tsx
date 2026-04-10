import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { listClassrooms } from "@/lib/classrooms";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export default async function DashboardPage() {
  const session = await auth();
  const classroomList = await listClassrooms(db, session!.user.id);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Dashboard</h1>
        <div className="flex gap-2">
          {session!.user.role === "teacher" && (
            <Link
              href="/dashboard/classrooms/new"
              className={buttonVariants()}
            >
              Create Classroom
            </Link>
          )}
          <Link
            href="/dashboard/classrooms/join"
            className={buttonVariants({ variant: "outline" })}
          >
            Join Classroom
          </Link>
        </div>
      </div>

      {classroomList.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            <p>No classrooms yet.</p>
            <p className="text-sm mt-2">
              {session!.user.role === "teacher"
                ? "Create your first classroom to get started."
                : "Ask your teacher for a join code."}
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {classroomList.map((classroom) => (
            <Link key={classroom.id} href={`/dashboard/classrooms/${classroom.id}`}>
              <Card className="hover:border-primary transition-colors cursor-pointer">
                <CardHeader>
                  <CardTitle className="text-lg">{classroom.name}</CardTitle>
                  <CardDescription>
                    {classroom.gradeLevel} &middot; {classroom.editorMode}
                  </CardDescription>
                </CardHeader>
                {classroom.description && (
                  <CardContent>
                    <p className="text-sm text-muted-foreground line-clamp-2">
                      {classroom.description}
                    </p>
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

import { notFound } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getClassroom, getClassroomMembers } from "@/lib/classrooms";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export default async function ClassroomDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const session = await auth();
  const { id } = await params;
  const classroom = await getClassroom(db, id);

  if (!classroom) {
    notFound();
  }

  const members = await getClassroomMembers(db, id);
  const isTeacher = classroom.teacherId === session!.user.id;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{classroom.name}</h1>
        <p className="text-muted-foreground">
          {classroom.gradeLevel} &middot; {classroom.editorMode}
        </p>
        {classroom.description && (
          <p className="mt-2">{classroom.description}</p>
        )}
      </div>

      <div className="flex gap-2">
        <Link
          href={`/dashboard/classrooms/${id}/editor`}
          className={buttonVariants()}
        >
          Open Editor
        </Link>
      </div>

      {isTeacher && (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Join Code</CardTitle>
            <CardDescription>Share this code with your students</CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-3xl font-mono tracking-widest font-bold text-center">
              {classroom.joinCode}
            </p>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">
            Students ({members.length})
          </CardTitle>
        </CardHeader>
        <CardContent>
          {members.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No students have joined yet.
            </p>
          ) : (
            <ul className="space-y-2">
              {members.map((member) => (
                <li
                  key={member.userId}
                  className="flex items-center justify-between py-2 border-b last:border-0"
                >
                  <span className="text-sm font-medium">{member.name}</span>
                  <span className="text-xs text-muted-foreground">
                    {member.email}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

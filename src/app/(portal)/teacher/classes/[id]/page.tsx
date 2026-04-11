import { notFound } from "next/navigation";
import { db } from "@/lib/db";
import { getClass } from "@/lib/classes";
import { listClassMembers } from "@/lib/class-memberships";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export default async function TeacherClassDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const cls = await getClass(db, id);
  if (!cls) notFound();

  const members = await listClassMembers(db, id);
  const students = members.filter((m) => m.role === "student");
  const instructors = members.filter((m) => m.role === "instructor" || m.role === "ta");

  return (
    <div className="p-6 space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{cls.title}</h1>
        <p className="text-muted-foreground">{cls.term || "No term"} · {cls.status}</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Join Code</CardTitle>
          <CardDescription>Share with students to join this class</CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-3xl font-mono tracking-widest font-bold text-center">{cls.joinCode}</p>
        </CardContent>
      </Card>

      <div className="grid gap-6 md:grid-cols-2">
        <div>
          <h2 className="text-lg font-semibold mb-3">Students ({students.length})</h2>
          {students.length === 0 ? (
            <p className="text-sm text-muted-foreground">No students have joined yet.</p>
          ) : (
            <div className="space-y-2">
              {students.map((m) => (
                <div key={m.id} className="flex items-center justify-between py-2 border-b last:border-0">
                  <span className="text-sm font-medium">{m.name}</span>
                  <span className="text-xs text-muted-foreground">{m.email}</span>
                </div>
              ))}
            </div>
          )}
        </div>

        <div>
          <h2 className="text-lg font-semibold mb-3">Instructors & TAs ({instructors.length})</h2>
          <div className="space-y-2">
            {instructors.map((m) => (
              <div key={m.id} className="flex items-center justify-between py-2 border-b last:border-0">
                <span className="text-sm font-medium">{m.name}</span>
                <span className="text-xs text-muted-foreground">{m.role}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

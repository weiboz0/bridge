import { notFound } from "next/navigation";
import { api } from "@/lib/api-client";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { StartSessionButton } from "@/components/teacher/start-session-button";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";

interface ClassDetail {
  id: string;
  title: string;
  term: string;
  status: string;
  joinCode: string;
}

interface ClassMember {
  id: string;
  userId: string;
  role: string;
  name: string;
  email: string;
}

interface SessionItem {
  id: string;
  classId: string;
  teacherId: string;
  status: string;
  startedAt: string;
  endedAt: string | null;
  participantCount: number;
}

export default async function TeacherClassDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  let cls: ClassDetail;
  try {
    cls = await api<ClassDetail>(`/api/classes/${id}`);
  } catch {
    notFound();
  }

  const [members, sessions] = await Promise.all([
    api<ClassMember[]>(`/api/classes/${id}/members`),
    api<SessionItem[]>(`/api/sessions/by-class/${id}`),
  ]);

  const students = members.filter((m) => m.role === "student");
  const instructors = members.filter((m) => m.role === "instructor" || m.role === "ta");
  const activeSession = sessions.find((s) => s.status === "active");
  const pastSessions = sessions.filter((s) => s.status === "ended");

  function formatDuration(start: string, end: string | null) {
    if (!end) return "In progress";
    const ms = new Date(end).getTime() - new Date(start).getTime();
    const mins = Math.round(ms / 60000);
    if (mins < 60) return `${mins} min`;
    return `${Math.floor(mins / 60)}h ${mins % 60}m`;
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{cls.title}</h1>
          <p className="text-muted-foreground">{cls.term || "No term"} · {cls.status}</p>
        </div>
        {activeSession ? (
          <Link
            href={`/teacher/classes/${id}/session/${activeSession.id}/dashboard`}
            className={buttonVariants()}
          >
            Resume Session ({activeSession.participantCount} students)
          </Link>
        ) : (
          <StartSessionButton classId={id} />
        )}
      </div>

      {activeSession && (
        <Card className="border-green-500 bg-green-50 dark:bg-green-950/20">
          <CardHeader>
            <CardTitle className="text-lg text-green-700 dark:text-green-400">Live Session Active</CardTitle>
            <CardDescription>
              Started {new Date(activeSession.startedAt).toLocaleTimeString()} · {activeSession.participantCount} students
            </CardDescription>
          </CardHeader>
        </Card>
      )}

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

      {pastSessions.length > 0 && (
        <div>
          <h2 className="text-lg font-semibold mb-3">Past Sessions ({pastSessions.length})</h2>
          <div className="space-y-2">
            {pastSessions.map((s) => (
              <Link
                key={s.id}
                href={`/teacher/classes/${id}/session/${s.id}/dashboard`}
                className="block"
              >
                <div className="flex items-center justify-between py-3 px-4 border rounded-lg hover:bg-muted/50 transition-colors">
                  <div>
                    <p className="text-sm font-medium">
                      {new Date(s.startedAt).toLocaleDateString()} at {new Date(s.startedAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
                    </p>
                    <p className="text-xs text-muted-foreground">
                      {formatDuration(s.startedAt, s.endedAt)} · {s.participantCount} students
                    </p>
                  </div>
                  <span className="text-xs text-muted-foreground">View →</span>
                </div>
              </Link>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

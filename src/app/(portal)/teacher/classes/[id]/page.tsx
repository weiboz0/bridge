import { notFound } from "next/navigation";
import { api, ApiError } from "@/lib/api-client";
import { isValidUUID } from "@/lib/utils";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { StartSessionButton } from "@/components/teacher/start-session-button";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";
import { ClassStudentsList } from "@/components/teacher/class-students-list";

export interface TeacherParentLinkRow {
  linkId: string;
  studentUserId: string;
  studentName: string;
  parentUserId: string;
  parentEmail: string;
  parentName: string;
  createdAt: string;
}

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
  if (!isValidUUID(id)) notFound();

  let cls: ClassDetail;
  try {
    cls = await api<ClassDetail>(`/api/classes/${id}`);
  } catch {
    notFound();
  }

  const [members, sessions, parentLinks] = await Promise.all([
    api<ClassMember[]>(`/api/classes/${id}/members`),
    api<SessionItem[]>(`/api/sessions/by-class/${id}`),
    // Plan 070 phase 3 — class-detail Parents popover. Backend
    // gates by roster authority (instructor / TA / org_admin /
    // platform admin); a student viewer wouldn't even reach this
    // page, but the .catch keeps the rest of the page loading if
    // the endpoint is missing in older envs.
    api<TeacherParentLinkRow[]>(`/api/teacher/classes/${id}/parent-links`).catch(
      (e: unknown) => {
        if (e instanceof ApiError && (e.status === 404 || e.status === 403)) {
          return [] as TeacherParentLinkRow[];
        }
        throw e;
      },
    ),
  ]);

  const students = members.filter((m) => m.role === "student");
  const instructors = members.filter((m) => m.role === "instructor" || m.role === "ta");
  const activeSession = sessions.find((s) => s.status === "live");
  const pastSessions = sessions.filter((s) => s.status === "ended");

  // Group parent links by student id so the popover renders the
  // right rows on each click without an extra round-trip.
  const parentsByStudent = new Map<string, TeacherParentLinkRow[]>();
  for (const link of parentLinks) {
    const list = parentsByStudent.get(link.studentUserId) ?? [];
    list.push(link);
    parentsByStudent.set(link.studentUserId, list);
  }

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
            <ClassStudentsList
              students={students.map((m) => ({
                id: m.id,
                userId: m.userId,
                name: m.name,
                email: m.email,
                parents: parentsByStudent.get(m.userId) ?? [],
              }))}
            />
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
            {/*
              Plan 048 phase 6: ended sessions render as non-clickable rows.
              Pre-048 they linked to a "Session ended" placeholder dashboard
              that has no review content. Until a read-only review surface
              ships, no link is better than a link to nothing useful.
            */}
            {pastSessions.map((s) => {
              const isLive = s.status === "live";
              const meta = (
                <div>
                  <p className="text-sm font-medium">
                    {new Date(s.startedAt).toLocaleDateString()} at {new Date(s.startedAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {formatDuration(s.startedAt, s.endedAt)} · {s.participantCount} students
                  </p>
                </div>
              );
              if (isLive) {
                return (
                  <Link
                    key={s.id}
                    href={`/teacher/classes/${id}/session/${s.id}/dashboard`}
                    className="block"
                  >
                    <div className="flex items-center justify-between py-3 px-4 border rounded-lg hover:bg-muted/50 transition-colors">
                      {meta}
                      <span className="text-xs text-muted-foreground">View →</span>
                    </div>
                  </Link>
                );
              }
              return (
                <div
                  key={s.id}
                  className="flex items-center justify-between py-3 px-4 border rounded-lg"
                >
                  {meta}
                  <span className="rounded-full bg-zinc-100 px-2.5 py-1 text-xs font-medium text-zinc-600">
                    Ended
                  </span>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}

import Link from "next/link";
import { api } from "@/lib/api-client";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { StartSessionButton } from "@/components/teacher/start-session-button";
import { buttonVariants } from "@/components/ui/button";

interface CourseItem {
  id: string;
  title: string;
}

interface ClassItem {
  id: string;
  title: string;
  term: string;
  status: string;
  memberRole: string;
}

interface SessionItem {
  id: string;
  classId: string | null;
  title: string;
  status: string;
  startedAt: string;
  endedAt: string | null;
}

interface SessionListResponse {
  items: SessionItem[];
  nextCursor?: string | null;
}

function formatTimestamp(value: string) {
  return new Date(value).toLocaleString([], {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

export default async function TeacherDashboard() {
  const [coursesResponse, allClasses, sessionList] = await Promise.all([
    api<{ courses?: CourseItem[] }>("/api/teacher/courses").catch(() => ({ courses: [] })),
    api<ClassItem[]>("/api/classes/mine"),
    api<SessionListResponse>("/api/sessions?limit=20"),
  ]);

  const courses = coursesResponse.courses ?? [];
  const myClasses = allClasses.filter((classItem) => classItem.memberRole === "instructor");
  const mySessions = sessionList.items ?? [];

  const activeSessions = new Map<string, SessionItem>();
  for (const session of mySessions) {
    if (session.classId && session.status === "live" && !activeSessions.has(session.classId)) {
      activeSessions.set(session.classId, session);
    }
  }

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-zinc-900">Teacher Dashboard</h1>
        <Link
          href="/teacher/courses"
          className={buttonVariants({
            className: "bg-zinc-900 text-white hover:bg-zinc-800",
          })}
        >
          Create Course
        </Link>
      </div>

      <div className="grid gap-4 md:grid-cols-3">
        <Card className="border-zinc-200 bg-white">
          <CardHeader>
            <CardTitle className="text-sm text-zinc-500">My Courses</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-3xl font-bold text-zinc-900">{courses.length}</p>
          </CardContent>
        </Card>
        <Card className="border-zinc-200 bg-white">
          <CardHeader>
            <CardTitle className="text-sm text-zinc-500">My Classes</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-3xl font-bold text-zinc-900">{myClasses.length}</p>
          </CardContent>
        </Card>
        <Card className="border-zinc-200 bg-white">
          <CardHeader>
            <CardTitle className="text-sm text-zinc-500">My Sessions</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-3xl font-bold text-zinc-900">{mySessions.length}</p>
          </CardContent>
        </Card>
      </div>

      <Link href="/teacher/units">
        <Card className="border-zinc-200 bg-white transition-colors hover:border-zinc-300 hover:bg-zinc-50 cursor-pointer">
          <CardHeader>
            <div className="flex items-center justify-between">
              <div className="space-y-1">
                <CardTitle className="text-lg text-zinc-900">Unit Library</CardTitle>
                <CardDescription className="text-zinc-500">
                  Browse, search, and manage your teaching units across personal, org, and platform scopes.
                </CardDescription>
              </div>
              <span className="text-sm text-zinc-400">View all</span>
            </div>
          </CardHeader>
        </Card>
      </Link>

      <Card className="border-zinc-200 bg-white">
        <CardHeader className="gap-4 md:flex-row md:items-center md:justify-between">
          <div className="space-y-1">
            <CardTitle className="text-lg text-zinc-900">Sessions</CardTitle>
            <CardDescription className="text-zinc-500">
              Start an independent session or reopen one of your recent sessions.
            </CardDescription>
          </div>
          <StartSessionButton
            mode="orphan"
            buttonLabel="Start Session"
            defaultTitle="Office hours"
          />
        </CardHeader>
        <CardContent>
          {mySessions.length === 0 ? (
            <div className="rounded-lg border border-dashed border-zinc-200 bg-zinc-50 px-4 py-6 text-sm text-zinc-500">
              No sessions yet. Start a standalone session for office hours, small-group support,
              or ad hoc help.
            </div>
          ) : (
            <div className="space-y-2">
              {mySessions.map((session) => {
                const isLive = session.status === "live";
                const sessionLabel = session.classId ? "Class session" : "Independent session";

                // Plan 048 phase 6: ended sessions render as a non-clickable row
                // (matches SessionRow in /teacher/sessions/page.tsx). The
                // dedicated /teacher/sessions/{id} dashboard isn't built for
                // ended-state read-only viewing yet; linking to a "Session
                // ended" placeholder is worse UX than no link at all.
                const meta = (
                  <div className="space-y-1">
                    <p className="font-medium text-zinc-900">{session.title}</p>
                    <p className="text-sm text-zinc-500">
                      {sessionLabel} · Started {formatTimestamp(session.startedAt)}
                      {session.endedAt ? ` · Ended ${formatTimestamp(session.endedAt)}` : ""}
                    </p>
                  </div>
                );
                const badge = (
                  <span
                    className={
                      isLive
                        ? "rounded-full bg-emerald-50 px-2.5 py-1 text-xs font-medium text-emerald-700"
                        : "rounded-full bg-zinc-100 px-2.5 py-1 text-xs font-medium text-zinc-600"
                    }
                  >
                    {isLive ? "Live" : "Ended"}
                  </span>
                );

                if (isLive) {
                  return (
                    <Link key={session.id} href={`/teacher/sessions/${session.id}`} className="block">
                      <div className="flex items-center justify-between rounded-lg border border-zinc-200 bg-white px-4 py-3 transition-colors hover:bg-zinc-50">
                        {meta}
                        {badge}
                      </div>
                    </Link>
                  );
                }
                return (
                  <div
                    key={session.id}
                    className="flex items-center justify-between rounded-lg border border-zinc-200 bg-white px-4 py-3"
                  >
                    {meta}
                    {badge}
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>

      {myClasses.length > 0 && (
        <div>
          <h2 className="mb-3 text-lg font-semibold text-zinc-900">My Classes</h2>
          <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
            {myClasses.map((classItem) => {
              const activeSession = activeSessions.get(classItem.id);

              return (
                <Link
                  key={classItem.id}
                  href={
                    activeSession
                      ? `/teacher/sessions/${activeSession.id}`
                      : `/teacher/classes/${classItem.id}`
                  }
                >
                  <Card
                    className={`cursor-pointer border-zinc-200 bg-white transition-colors hover:border-zinc-300 ${
                      activeSession ? "border-emerald-400" : ""
                    }`}
                  >
                    <CardHeader>
                      <div className="flex items-center justify-between gap-3">
                        <CardTitle className="text-lg text-zinc-900">{classItem.title}</CardTitle>
                        {activeSession && (
                          <span className="rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700">
                            Live
                          </span>
                        )}
                      </div>
                      <CardDescription className="text-zinc-500">
                        {classItem.term || "No term"}
                      </CardDescription>
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

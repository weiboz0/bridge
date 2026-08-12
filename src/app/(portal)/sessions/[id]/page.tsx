import { notFound, redirect } from "next/navigation";
import Link from "next/link";
import { api } from "@/lib/api-client";
import { ApiError } from "@/lib/api-error";
import { isValidUUID } from "@/lib/utils";
import { TeacherDashboard } from "@/components/session/teacher/teacher-dashboard";
import { StudentSession } from "@/components/session/student/student-session";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

type EditorMode = "python" | "javascript" | "blockly";

interface SessionInfo {
  id: string;
  classId: string | null;
  teacherId: string;
  status: string;
  inviteToken?: string | null;
  inviteExpiresAt?: string | null;
}

interface TeacherPagePayload {
  session: SessionInfo;
  classId: string | null;
  returnPath: string;
  editorMode: string;
  courseTopics: Array<{
    topicId: string;
    title: string;
    unitId: string | null;
    unitTitle: string | null;
    unitMaterialType: string | null;
  }>;
}

interface StudentPagePayload {
  session: { id: string; classId: string | null; status: string };
  classId: string | null;
  returnPath: string;
  editorMode: string;
}

// Plan 090 phase 5 — role-neutral session room dispatcher.
//
// The neutral /sessions route serves both hosts and participants of
// class-less (ad-hoc) sessions with a single URL, so a share link works
// regardless of which side of the session the recipient is on. Auth is
// entirely Go's: fetch teacher-page first (200 means the caller is the
// host — or an authorized instructor/admin for a class-bound session);
// a 403/404 falls back to student-page. This mirrors, and reuses without
// forking, the existing /teacher/sessions/[sessionId] and
// /student/sessions/[sessionId] pages.
export default async function SessionRoomPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id: sessionId } = await params;
  if (!isValidUUID(sessionId)) notFound();

  let teacherPayload: TeacherPagePayload | null = null;
  try {
    teacherPayload = await api<TeacherPagePayload>(`/api/sessions/${sessionId}/teacher-page`);
  } catch (err) {
    if (!(err instanceof ApiError && (err.status === 403 || err.status === 404))) {
      throw err;
    }
  }

  if (teacherPayload) {
    // Plan 043 phase 2.2 parity: TeacherDashboard is a live-only surface.
    // For ended sessions, render a notice instead (same as
    // /teacher/sessions/[sessionId]).
    if (teacherPayload.session.status !== "live") {
      return (
        <div className="mx-auto max-w-2xl px-4 py-12">
          <Card>
            <CardHeader>
              <CardTitle>Session ended</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3 text-sm">
            <p className="text-muted-foreground">
                This session is no longer live. Its whiteboards remain available
                in the read-only archive.
              </p>
              <Link href={`/sessions/${sessionId}/whiteboards`} className="text-primary underline">
                View whiteboard archive
              </Link>
              <Link href="/sessions" className="text-primary underline">
                Back to sessions
              </Link>
            </CardContent>
          </Card>
        </div>
      );
    }

    return (
      <TeacherDashboard
        sessionId={sessionId}
        editorMode={(teacherPayload.editorMode as EditorMode) ?? "python"}
        courseTopics={teacherPayload.courseTopics}
        inviteToken={teacherPayload.session.inviteToken ?? null}
        inviteExpiresAt={teacherPayload.session.inviteExpiresAt ?? null}
      />
    );
  }

  let studentPayload: StudentPagePayload;
  try {
    studentPayload = await api<StudentPagePayload>(`/api/sessions/${sessionId}/student-page`);
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) {
      // GetStudentPage returns 404 for two distinct cases with the same HTTP
      // status: a session that truly does not exist ("Not found") and an
      // existing session that has ended ("Session has ended"), including for
      // former participants. Only the latter has an archive to redirect to —
      // a genuinely missing session must still 404 at the page level. The
      // real Go handler puts the distinguishing text in the JSON body; check
      // both it and `message` so a caller that only sets one still resolves.
      const body = err.body as { error?: string } | undefined;
      const sessionEnded = body?.error === "Session has ended" || /session (has )?ended/i.test(err.message);
      if (sessionEnded) {
        redirect(`/sessions/${sessionId}/whiteboards`);
      }
      notFound();
    }
    if (err instanceof ApiError && err.status === 403) {
      notFound();
    }
    throw err;
  }

  // Side-effect: record participation, same pattern as
  // /student/sessions/[sessionId]/page.tsx. The GET above already
  // validated access; join is a separate POST so the page still renders
  // if joining transiently fails.
  try {
    await api(`/api/sessions/${sessionId}/join`, { method: "POST" });
  } catch {
    // Non-fatal.
  }

  const returnPath = studentPayload.classId ? studentPayload.returnPath : "/sessions";

  return (
    <StudentSession
      sessionId={sessionId}
      classId={studentPayload.classId}
      returnPath={returnPath}
      editorMode={(studentPayload.editorMode as EditorMode) ?? "python"}
    />
  );
}

import { notFound } from "next/navigation";
import Link from "next/link";
import { api, ApiError } from "@/lib/api-client";
import { isValidUUID } from "@/lib/utils";
import { TeacherDashboard } from "@/components/session/teacher/teacher-dashboard";
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
  courseTopics: Array<{ topicId: string; title: string; lessonContent: string }>;
}

export default async function TeacherSessionPage({
  params,
}: {
  params: Promise<{ sessionId: string }>;
}) {
  const { sessionId } = await params;
  if (!isValidUUID(sessionId)) notFound();

  let payload: TeacherPagePayload;
  try {
    payload = await api<TeacherPagePayload>(`/api/sessions/${sessionId}/teacher-page`);
  } catch (err) {
    // Go authorizes; trust its 403/404. We do not compare IDs locally.
    if (err instanceof ApiError && (err.status === 404 || err.status === 403)) {
      notFound();
    }
    throw err;
  }

  // Plan 043 phase 2.2: TeacherDashboard is a live-only surface (Yjs,
  // broadcast, end-session action). For ended sessions, render a notice
  // instead of the live workspace. Guards against bookmarks and direct
  // URL access — the listing-page link from Phase 2.1 is the other half.
  if (payload.session.status !== "live") {
    return (
      <div className="mx-auto max-w-2xl px-4 py-12">
        <Card>
          <CardHeader>
            <CardTitle>Session ended</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3 text-sm">
            <p className="text-muted-foreground">
              This session is no longer live. A read-only review surface for
              ended sessions is coming in a future update.
            </p>
            <Link href="/teacher/sessions" className="text-primary underline">
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
      classId={payload.classId}
      returnPath={payload.returnPath}
      editorMode={(payload.editorMode as EditorMode) ?? "python"}
      courseTopics={payload.courseTopics}
      inviteToken={payload.session.inviteToken ?? null}
      inviteExpiresAt={payload.session.inviteExpiresAt ?? null}
    />
  );
}

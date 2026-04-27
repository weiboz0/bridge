import { notFound } from "next/navigation";
import { api, ApiError } from "@/lib/api-client";
import { isValidUUID } from "@/lib/utils";
import { TeacherDashboard } from "@/components/session/teacher/teacher-dashboard";

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

import { notFound } from "next/navigation";
import { api, ApiError } from "@/lib/api-client";
import { isValidUUID } from "@/lib/utils";
import { StudentSession } from "@/components/session/student/student-session";

type EditorMode = "python" | "javascript" | "blockly";

interface StudentPagePayload {
  session: { id: string; classId: string | null; status: string };
  classId: string | null;
  returnPath: string;
  editorMode: string;
}

export default async function StudentSessionPage({
  params,
}: {
  params: Promise<{ sessionId: string }>;
}) {
  const { sessionId } = await params;
  if (!isValidUUID(sessionId)) notFound();

  let payload: StudentPagePayload;
  try {
    payload = await api<StudentPagePayload>(`/api/sessions/${sessionId}/student-page`);
  } catch (err) {
    if (err instanceof ApiError && (err.status === 404 || err.status === 403)) {
      notFound();
    }
    throw err;
  }

  // Side-effect: record the student as a participant. The student-page GET
  // is purely read; participation is its own POST so the page can render
  // even if joining transiently fails (and so a kicked student leaving the
  // session doesn't 404 their workspace).
  try {
    await api(`/api/sessions/${sessionId}/join`, { method: "POST" });
  } catch {
    // Non-fatal — Go's join handler will reject if truly unauthorized,
    // which the student-page GET above would have caught first.
  }

  return (
    <StudentSession
      sessionId={sessionId}
      classId={payload.classId}
      returnPath={payload.returnPath}
      editorMode={(payload.editorMode as EditorMode) ?? "python"}
    />
  );
}

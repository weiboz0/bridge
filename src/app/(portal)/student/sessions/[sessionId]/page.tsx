import { notFound } from "next/navigation";
import { api } from "@/lib/api-client";
import { getClassSettings } from "@/lib/classes";
import { db } from "@/lib/db";
import { StudentSession } from "@/components/session/student/student-session";

type EditorMode = "python" | "javascript" | "blockly";

interface SessionDetail {
  id: string;
  classId: string | null;
  status: string;
}

async function loadStudentSessionPageData(sessionId: string) {
  let liveSession: SessionDetail;
  try {
    liveSession = await api<SessionDetail>(`/api/sessions/${sessionId}`);
  } catch {
    notFound();
  }

  if (liveSession.status !== "live") {
    notFound();
  }

  try {
    await api(`/api/sessions/${sessionId}/join`, { method: "POST" });
  } catch {
    notFound();
  }

  if (!liveSession.classId) {
    return {
      classId: null,
      returnPath: "/student",
      editorMode: "python" as EditorMode,
    };
  }

  const settings = await getClassSettings(db, liveSession.classId);

  return {
    classId: liveSession.classId,
    returnPath: `/student/classes/${liveSession.classId}`,
    editorMode:
      (settings?.editorMode as EditorMode | undefined) ?? ("python" as EditorMode),
  };
}

export default async function StudentSessionPage({
  params,
}: {
  params: Promise<{ sessionId: string }>;
}) {
  const { sessionId } = await params;
  const pageData = await loadStudentSessionPageData(sessionId);

  return (
    <StudentSession
      sessionId={sessionId}
      classId={pageData.classId}
      returnPath={pageData.returnPath}
      editorMode={pageData.editorMode}
    />
  );
}

import { notFound } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSession, joinSession } from "@/lib/sessions";
import { getClass, getClassroom } from "@/lib/classes";
import { listClassMembers } from "@/lib/class-memberships";
import { StudentSession } from "@/components/session/student/student-session";

export default async function StudentSessionPage({
  params,
}: {
  params: Promise<{ id: string; sessionId: string }>;
}) {
  const session = await auth();
  const { id: classId, sessionId } = await params;

  const cls = await getClass(db, classId);
  if (!cls) notFound();

  // Verify student is enrolled
  const members = await listClassMembers(db, classId);
  const isEnrolled = members.some((m) => m.userId === session!.user.id);
  if (!isEnrolled && !session!.user.isPlatformAdmin) notFound();

  const liveSession = await getSession(db, sessionId);
  if (!liveSession || liveSession.status !== "active") notFound();

  // Auto-join session (server-side, no HTTP round-trip)
  await joinSession(db, sessionId, session!.user.id);

  const classroom = await getClassroom(db, classId);

  return (
    <StudentSession
      sessionId={sessionId}
      classId={classId}
      editorMode={(classroom?.editorMode as "python" | "javascript" | "blockly") || "python"}
    />
  );
}

import { notFound } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSession } from "@/lib/sessions";
import { getClass, getClassSettings } from "@/lib/classes";
import { getCourse } from "@/lib/courses";
import { listTopicsByCourse } from "@/lib/topics";
import { listClassMembers } from "@/lib/class-memberships";
import { TeacherDashboard } from "@/components/session/teacher/teacher-dashboard";

export default async function TeacherSessionDashboardPage({
  params,
}: {
  params: Promise<{ id: string; sessionId: string }>;
}) {
  const session = await auth();
  const { id: classId, sessionId } = await params;

  const cls = await getClass(db, classId);
  if (!cls) notFound();

  // Verify instructor
  const members = await listClassMembers(db, classId);
  const isInstructor = members.some(
    (m) => m.userId === session!.user.id && (m.role === "instructor" || m.role === "ta")
  );
  if (!isInstructor && !session!.user.isPlatformAdmin) notFound();

  const liveSession = await getSession(db, sessionId);
  if (!liveSession) notFound();

  const settings = await getClassSettings(db, classId);
  const course = await getCourse(db, cls.courseId);
  const courseTopics = course ? await listTopicsByCourse(db, course.id) : [];

  return (
    <TeacherDashboard
      sessionId={sessionId}
      classId={classId}
      editorMode={(settings?.editorMode as "python" | "javascript" | "blockly") || "python"}
      courseTopics={courseTopics.map((t) => ({
        topicId: t.id,
        title: t.title,
        lessonContent: t.lessonContent,
      }))}
      inviteToken={liveSession.inviteToken ?? null}
      inviteExpiresAt={liveSession.inviteExpiresAt?.toISOString() ?? null}
    />
  );
}

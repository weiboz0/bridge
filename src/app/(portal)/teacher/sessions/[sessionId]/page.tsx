import { notFound } from "next/navigation";
import { auth } from "@/lib/auth";
import { api } from "@/lib/api-client";
import { isValidUUID } from "@/lib/utils";
import { db } from "@/lib/db";
import { getClass, getClassSettings } from "@/lib/classes";
import { getCourse } from "@/lib/courses";
import { listTopicsByCourse } from "@/lib/topics";
import { listClassMembers } from "@/lib/class-memberships";
import { TeacherDashboard } from "@/components/session/teacher/teacher-dashboard";

type EditorMode = "python" | "javascript" | "blockly";

interface SessionDetail {
  id: string;
  classId: string | null;
  teacherId: string;
  status: string;
  inviteToken?: string | null;
  inviteExpiresAt?: string | null;
}

async function loadTeacherSessionPageData(
  sessionId: string,
  viewerId: string,
  isPlatformAdmin: boolean
) {
  let liveSession: SessionDetail;
  try {
    liveSession = await api<SessionDetail>(`/api/sessions/${sessionId}`);
  } catch {
    notFound();
  }

  if (!liveSession.classId) {
    if (liveSession.teacherId !== viewerId && !isPlatformAdmin) {
      notFound();
    }

    return {
      liveSession,
      classId: null,
      returnPath: "/teacher",
      editorMode: "python" as EditorMode,
      courseTopics: [] as Array<{ topicId: string; title: string; lessonContent: unknown }>,
    };
  }

  const cls = await getClass(db, liveSession.classId);
  if (!cls) {
    notFound();
  }

  const members = await listClassMembers(db, liveSession.classId);
  const isInstructor = members.some(
    (member) =>
      member.userId === viewerId &&
      (member.role === "instructor" || member.role === "ta")
  );
  if (!isInstructor && !isPlatformAdmin) {
    notFound();
  }

  const settings = await getClassSettings(db, liveSession.classId);
  const course = await getCourse(db, cls.courseId);
  const courseTopics = course ? await listTopicsByCourse(db, course.id) : [];

  return {
    liveSession,
    classId: liveSession.classId,
    returnPath: `/teacher/classes/${liveSession.classId}`,
    editorMode:
      (settings?.editorMode as EditorMode | undefined) ?? ("python" as EditorMode),
    courseTopics: courseTopics.map((topic) => ({
      topicId: topic.id,
      title: topic.title,
      lessonContent: topic.lessonContent,
    })),
  };
}

export default async function TeacherSessionPage({
  params,
}: {
  params: Promise<{ sessionId: string }>;
}) {
  const session = await auth();
  if (!session?.user?.id) {
    notFound();
  }

  const { sessionId } = await params;
  if (!isValidUUID(sessionId)) notFound();

  const pageData = await loadTeacherSessionPageData(
    sessionId,
    session.user.id,
    session.user.isPlatformAdmin
  );

  return (
    <TeacherDashboard
      sessionId={sessionId}
      classId={pageData.classId}
      returnPath={pageData.returnPath}
      editorMode={pageData.editorMode}
      courseTopics={pageData.courseTopics}
      inviteToken={pageData.liveSession.inviteToken ?? null}
      inviteExpiresAt={pageData.liveSession.inviteExpiresAt ?? null}
    />
  );
}

import { redirect } from "next/navigation";

export default async function TeacherSessionDashboardRedirect({
  params,
}: {
  params: Promise<{ id: string; sessionId: string }>;
}) {
  const { sessionId } = await params;
  redirect(`/teacher/sessions/${sessionId}`);
}

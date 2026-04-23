import { redirect } from "next/navigation";

export default async function StudentSessionRedirect({
  params,
}: {
  params: Promise<{ id: string; sessionId: string }>;
}) {
  const { sessionId } = await params;
  redirect(`/student/sessions/${sessionId}`);
}

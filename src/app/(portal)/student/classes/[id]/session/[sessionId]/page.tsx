import { redirect, notFound } from "next/navigation";
import { isValidUUID } from "@/lib/utils";

export default async function StudentSessionRedirect({
  params,
}: {
  params: Promise<{ id: string; sessionId: string }>;
}) {
  const { sessionId } = await params;
  if (!isValidUUID(sessionId)) notFound();
  redirect(`/student/sessions/${sessionId}`);
}

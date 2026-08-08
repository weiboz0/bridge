import { notFound } from "next/navigation";
import { WhiteboardArchive } from "@/components/session/whiteboard/whiteboard-archive";
import { isValidUUID } from "@/lib/utils";

/**
 * Deliberately does not call the ordinary session-page endpoints: they reject
 * every ended session, including teachers and former participants. The client
 * archive asks the canvas list endpoint, whose archive branch is the metadata
 * authorization boundary; selecting an item then mints its own scoped token.
 */
export default async function WhiteboardArchivePage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id: sessionId } = await params;
  if (!isValidUUID(sessionId)) notFound();
  return <WhiteboardArchive sessionId={sessionId} />;
}

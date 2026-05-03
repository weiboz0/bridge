import { notFound } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getLinkedChildren } from "@/lib/parent-links";
import { getActiveSessionForStudent } from "@/lib/attendance";
import { LiveSessionViewer } from "@/components/parent/live-session-viewer";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";

export default async function ParentLiveViewPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const session = await auth();
  const { id: childId } = await params;

  // Verify parent-child link
  const children = await getLinkedChildren(db, session!.user.id);
  if (!children.some((c) => c.userId === childId)) notFound();

  const activeSession = await getActiveSessionForStudent(db, childId);

  if (!activeSession) {
    return (
      <div className="p-6 space-y-4">
        <h1 className="text-2xl font-bold">Live Session</h1>
        <p className="text-muted-foreground">
          Your child is not in a live session right now.
        </p>
        <Link href={`/parent/children/${childId}`} className={buttonVariants({ variant: "outline" })}>
          Back to Profile
        </Link>
      </div>
    );
  }

  return (
    <div className="h-screen flex flex-col">
      <div className="flex items-center justify-between px-4 py-2 border-b">
        <div className="flex items-center gap-2">
          <span className="w-2 h-2 rounded-full bg-green-500 animate-pulse" />
          <span className="text-sm font-medium">Watching Live — Read Only</span>
        </div>
        <Link href={`/parent/children/${childId}`} className={buttonVariants({ size: "sm", variant: "ghost" })}>
          Back
        </Link>
      </div>
      <div className="flex-1 min-h-0">
        <LiveSessionViewer
          sessionId={activeSession.sessionId}
          studentId={childId}
          editorMode="python"
        />
      </div>
    </div>
  );
}

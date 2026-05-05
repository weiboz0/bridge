import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getLinkedChildren } from "@/lib/parent-links";
import { getActiveSessionForStudent } from "@/lib/attendance";
import { LiveNowBadge } from "@/components/parent/live-now-badge";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";

export default async function ParentDashboard() {
  const session = await auth();
  const children = await getLinkedChildren(db, session!.user.id);

  // Check live status for each child
  const childrenWithStatus = await Promise.all(
    children.map(async (child) => {
      const activeSession = await getActiveSessionForStudent(db, child.userId);
      return { ...child, isLive: !!activeSession, sessionId: activeSession?.sessionId };
    })
  );

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">Parent Dashboard</h1>

      {childrenWithStatus.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            <p>No children linked yet.</p>
            <p className="text-sm mt-2">
              Ask your child&apos;s teacher or school administrator to link your account. Once linked, your child&apos;s classes will appear here.
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {childrenWithStatus.map((child) => (
            <Card key={child.userId} className="hover:border-primary transition-colors">
              <CardHeader>
                <div className="flex items-center justify-between">
                  <CardTitle className="text-lg">{child.name}</CardTitle>
                  {child.isLive && <LiveNowBadge />}
                </div>
                <CardDescription>
                  {child.classCount} class{child.classCount !== 1 ? "es" : ""}
                </CardDescription>
              </CardHeader>
              <CardFooter className="flex gap-2">
                <Link
                  href={`/parent/children/${child.userId}`}
                  className={buttonVariants({ size: "sm", variant: "outline" })}
                >
                  View Profile
                </Link>
                {child.isLive && (
                  <Link
                    href={`/parent/children/${child.userId}/live`}
                    className={buttonVariants({ size: "sm" })}
                  >
                    Watch Live
                  </Link>
                )}
              </CardFooter>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}

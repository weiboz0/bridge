import { notFound } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getClass } from "@/lib/classes";
import { listClassMembers } from "@/lib/class-memberships";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";

export default async function StudentClassDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const session = await auth();
  const { id } = await params;
  const cls = await getClass(db, id);
  if (!cls) notFound();

  // Verify student is enrolled in this class
  const members = await listClassMembers(db, id);
  const isEnrolled = members.some((m) => m.userId === session!.user.id);
  if (!isEnrolled && !session!.user.isPlatformAdmin) notFound();

  return (
    <div className="p-6 space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{cls.title}</h1>
        <p className="text-muted-foreground">{cls.term || "No term"}</p>
      </div>

      <div className="flex gap-3">
        <Link
          href={`/dashboard/classrooms/${id}/editor`}
          className={buttonVariants()}
        >
          Open Editor
        </Link>
      </div>
    </div>
  );
}

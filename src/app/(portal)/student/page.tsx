import { api } from "@/lib/api-client";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import Link from "next/link";
import { JoinClassDialog } from "@/components/student/join-class-dialog";

interface ClassItem {
  id: string;
  title: string;
  term: string;
  status: string;
}

export default async function StudentDashboard({
  searchParams,
}: {
  searchParams?: Promise<{ invite?: string }>;
}) {
  const allClasses = await api<(ClassItem & { memberRole: string })[]>("/api/classes/mine");
  const myClasses = allClasses.filter((c) => c.memberRole === "student" && c.status === "active");
  // ?invite=<code> set by the post-register redirect when the user signed
  // up from a class invite link. Pass it to the dialog so it auto-opens
  // with the code prefilled — one click to enroll instead of asking the
  // student to retype it.
  const params = await searchParams;
  const initialInviteCode = params?.invite;

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">My Dashboard</h1>
        <JoinClassDialog initialInviteCode={initialInviteCode} />
      </div>

      {myClasses.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            <p>No classes yet. Ask your teacher for a join code.</p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {myClasses.map((cls) => (
            <Link key={cls.id} href={`/student/classes/${cls.id}`}>
              <Card className="hover:border-primary transition-colors cursor-pointer">
                <CardHeader>
                  <CardTitle className="text-lg">{cls.title}</CardTitle>
                  <CardDescription>{cls.term || "No term"}</CardDescription>
                </CardHeader>
              </Card>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}

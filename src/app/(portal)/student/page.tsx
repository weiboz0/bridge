import { api } from "@/lib/api-client";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";

interface ClassItem {
  id: string;
  title: string;
  term: string;
  status: string;
}

export default async function StudentDashboard() {
  const allClasses = await api<(ClassItem & { memberRole: string })[]>("/api/classes/mine");
  const myClasses = allClasses.filter((c) => c.memberRole === "student" && c.status === "active");

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">My Dashboard</h1>
        <Link href="/student/classes" className={buttonVariants({ variant: "outline" })}>
          Join a Class
        </Link>
      </div>

      {myClasses.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            <p>No classes yet. Ask your teacher for a join code.</p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {myClasses.filter((c) => c.status === "active").map((cls) => (
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

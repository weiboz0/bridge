import { api } from "@/lib/api-client";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import Link from "next/link";

interface ClassItem {
  id: string;
  title: string;
  term: string | null;
  status: string;
  memberRole: string;
}

export default async function StudentClassesPage() {
  const classes = await api<ClassItem[]>("/api/classes/mine");
  const myClasses = classes.filter((c) => c.memberRole === "student");

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">My Classes</h1>

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
                  <CardDescription>{cls.term || "No term"} · {cls.status}</CardDescription>
                </CardHeader>
              </Card>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}

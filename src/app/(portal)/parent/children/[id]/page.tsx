import { db } from "@/lib/db";
import { users } from "@/lib/db/schema";
import { eq } from "drizzle-orm";
import { listClassesByUser } from "@/lib/classes";
import { listDocuments } from "@/lib/documents";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { notFound } from "next/navigation";

export default async function ChildDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const [child] = await db.select().from(users).where(eq(users.id, id));
  if (!child) notFound();

  const classes = await listClassesByUser(db, id);
  const docs = await listDocuments(db, { ownerId: id });

  return (
    <div className="p-6 space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{child.name}</h1>
        <p className="text-muted-foreground">{child.email}</p>
      </div>

      <div>
        <h2 className="text-lg font-semibold mb-3">Classes ({classes.length})</h2>
        {classes.length === 0 ? (
          <p className="text-sm text-muted-foreground">Not enrolled in any classes.</p>
        ) : (
          <div className="space-y-2">
            {classes.map((cls) => (
              <Card key={cls.id}>
                <CardContent className="py-3">
                  <p className="font-medium">{cls.title}</p>
                  <p className="text-sm text-muted-foreground">{cls.term || "No term"} · {cls.memberRole}</p>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </div>

      <div>
        <h2 className="text-lg font-semibold mb-3">Recent Code ({docs.length})</h2>
        {docs.length === 0 ? (
          <p className="text-sm text-muted-foreground">No code saved yet.</p>
        ) : (
          <div className="space-y-2">
            {docs.slice(0, 10).map((doc) => (
              <Card key={doc.id}>
                <CardContent className="py-3">
                  <div className="flex justify-between">
                    <span className="text-sm">{doc.language}</span>
                    <span className="text-xs text-muted-foreground">
                      {new Date(doc.updatedAt).toLocaleString()}
                    </span>
                  </div>
                  {doc.plainText && (
                    <pre className="mt-2 text-xs text-muted-foreground bg-muted/50 rounded p-2 overflow-hidden max-h-16">
                      {doc.plainText.slice(0, 150)}
                    </pre>
                  )}
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

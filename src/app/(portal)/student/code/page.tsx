import { api } from "@/lib/api-client";
import { Card, CardContent } from "@/components/ui/card";

interface DocumentItem {
  id: string;
  language: string;
  plainText: string | null;
  updatedAt: string;
}

export default async function StudentCodePage() {
  const docs = await api<DocumentItem[]>("/api/documents");

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">My Code</h1>

      {docs.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            <p>No code saved yet. Join a class and start coding!</p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {docs.map((doc) => (
            <Card key={doc.id}>
              <CardContent className="py-3">
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium">{doc.language}</span>
                  <span className="text-xs text-muted-foreground">
                    {new Date(doc.updatedAt).toLocaleString()}
                  </span>
                </div>
                {doc.plainText && (
                  <pre className="mt-2 text-xs text-muted-foreground bg-muted/50 rounded p-2 overflow-hidden max-h-20">
                    {doc.plainText.slice(0, 200)}
                  </pre>
                )}
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}

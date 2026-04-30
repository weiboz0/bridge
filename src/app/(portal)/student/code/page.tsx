import Link from "next/link";
import { api } from "@/lib/api-client";
import { Card, CardContent } from "@/components/ui/card";

interface DocumentItem {
  id: string;
  language: string;
  plainText: string | null;
  updatedAt: string;
  // Plan 048 phase 7: nav metadata resolved server-side via LEFT JOIN
  // sessions on documents.session_id. Both fields are nullable. The
  // branch ladder below picks the right href (or non-clickable).
  sessionId?: string | null;
  topicId?: string | null;
  classId?: string | null;
  sessionStatus?: string | null;
}

/**
 * Plan 048 phase 7: target href branch ladder, in order:
 *   1. classId is null/empty → null (non-clickable). Catches both
 *      "no session, no class" AND "dangling session_id" (session
 *      row hard-deleted; documents.session_id has no FK constraint
 *      per drizzle/0005_code-persistence.sql).
 *   2. sessionId set AND sessionStatus === "live" → live session route.
 *   3. Otherwise (ended session OR class-only doc) → class detail.
 *      Ended sessions can NOT link to /student/sessions/{id} because
 *      that page returns 404 for non-live sessions
 *      (platform/internal/handlers/sessions.go:1365-1386).
 */
function navigationHref(doc: DocumentItem): string | null {
  if (!doc.classId) return null;
  if (doc.sessionId && doc.sessionStatus === "live") {
    return `/student/sessions/${doc.sessionId}`;
  }
  return `/student/classes/${doc.classId}`;
}

export default async function StudentWorkPage() {
  const docs = await api<DocumentItem[]>("/api/documents");

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">My Work</h1>

      {docs.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            <p>No code saved yet. Join a class and start coding!</p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {docs.map((doc) => {
            const href = navigationHref(doc);
            const card = (
              <Card key={doc.id} className={href ? "hover:bg-muted/30 transition-colors" : ""}>
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
                  {href && (
                    <p className="mt-2 text-[11px] uppercase tracking-wider text-muted-foreground">
                      {doc.sessionStatus === "live" ? "Open live session →" : "Open class →"}
                    </p>
                  )}
                </CardContent>
              </Card>
            );
            if (href) {
              return (
                <Link key={doc.id} href={href} className="block">
                  {card}
                </Link>
              );
            }
            return card;
          })}
        </div>
      )}
    </div>
  );
}

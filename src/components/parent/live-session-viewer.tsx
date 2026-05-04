"use client";

import Link from "next/link";
import { useState, useEffect } from "react";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { useRealtimeToken } from "@/lib/realtime/use-realtime-token";
import { RealtimeConfigBanner } from "@/components/realtime/realtime-config-banner";
import { CodeEditor } from "@/components/editor/code-editor";

interface SessionTopic {
  topicId: string;
  title: string;
  // Parent view: read-only linked teaching_unit reference.
  unitId: string | null;
  unitTitle: string | null;
  unitMaterialType: string | null;
}

interface LiveSessionViewerProps {
  sessionId: string;
  studentId: string;
  /**
   * @deprecated Plan 053b dropped the legacy `${parentId}:parent`
   * token construction. The mint endpoint now signs tokens server-
   * side after verifying the parent_link. Kept on the props
   * interface for backward-compat with existing callers; remove
   * after consumers stop passing it.
   */
  parentId?: string;
  editorMode: string;
}

export function LiveSessionViewer({
  sessionId,
  studentId,
  editorMode,
}: LiveSessionViewerProps) {
  const [topics, setTopics] = useState<SessionTopic[]>([]);
  // Plan 053b Phase 4 — migrated to useRealtimeToken. The Go
  // `authorizeSessionDoc` now has a parent path that requires:
  //   1. Active parent_link from caller → studentId, AND
  //   2. studentId is a participant in THIS session (so a parent
  //      of one student can't mint tokens for unrelated sessions).
  // Both gates ship in plan 064 (parent_links table) + this PR.
  const documentName = `session:${sessionId}:user:${studentId}`;
  const { token: realtimeToken, unavailable: realtimeUnavailable } = useRealtimeToken(documentName);

  const { yText, provider, connected } = useYjsProvider({
    documentName,
    token: realtimeToken,
  });

  useEffect(() => {
    async function fetchTopics() {
      const res = await fetch(`/api/sessions/${sessionId}/topics`);
      if (res.ok) setTopics(await res.json());
    }
    fetchTopics();
  }, [sessionId]);

  return (
    <>
      <RealtimeConfigBanner unavailable={realtimeUnavailable} />
    <div className="flex h-full">
      {topics.length > 0 && (
        <div className="w-1/3 border-r overflow-auto p-4 space-y-4">
          {topics.map((t) => (
            <div key={t.topicId}>
              <h3 className="text-base font-semibold mb-2">{t.title}</h3>
              {t.unitId ? (
                <Link
                  href={`/student/units/${t.unitId}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-2 rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm hover:border-amber-400 hover:text-amber-800"
                >
                  <span className="font-medium">{t.unitTitle}</span>
                  {t.unitMaterialType && (
                    <span className="font-mono text-[10px] uppercase tracking-[0.16em] text-muted-foreground">
                      {t.unitMaterialType}
                    </span>
                  )}
                </Link>
              ) : (
                <p className="text-sm text-muted-foreground italic">
                  No material yet for this focus area.
                </p>
              )}
            </div>
          ))}
        </div>
      )}

      {/* Read-only code view */}
      <div className="flex-1 flex flex-col">
        <div className="flex items-center gap-2 px-4 py-2 border-b">
          <span className="text-sm text-muted-foreground">Watching live — read only</span>
          <span className={`w-2 h-2 rounded-full ${connected ? "bg-green-500" : "bg-red-500"}`} />
        </div>
        <div className="flex-1 min-h-0">
          <CodeEditor
            yText={yText}
            provider={provider}
            readOnly
            language={editorMode}
          />
        </div>
      </div>
    </div>
    </>
  );
}

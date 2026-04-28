"use client";

import Link from "next/link";
import { useState, useEffect } from "react";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { CodeEditor } from "@/components/editor/code-editor";

interface SessionTopic {
  topicId: string;
  title: string;
  // Plan 044 phase 2: linked teaching_unit identity replaces inline
  // lessonContent rendering. Parent view doesn't need authoring,
  // just visibility — clicking a Unit opens it for read.
  unitId: string | null;
  unitTitle: string | null;
  unitMaterialType: string | null;
}

interface LiveSessionViewerProps {
  sessionId: string;
  studentId: string;
  parentId: string;
  editorMode: string;
}

export function LiveSessionViewer({
  sessionId,
  studentId,
  parentId,
  editorMode,
}: LiveSessionViewerProps) {
  const [topics, setTopics] = useState<SessionTopic[]>([]);
  const documentName = `session:${sessionId}:user:${studentId}`;
  const token = `${parentId}:parent`;

  const { yText, provider, connected } = useYjsProvider({
    documentName,
    token,
  });

  useEffect(() => {
    async function fetchTopics() {
      const res = await fetch(`/api/sessions/${sessionId}/topics`);
      if (res.ok) setTopics(await res.json());
    }
    fetchTopics();
  }, [sessionId]);

  return (
    <div className="flex h-full">
      {/* Plan 044 phase 2: linked teaching_unit references replace the
          inline lessonContent rendering. */}
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
                  No material yet for this topic.
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
  );
}

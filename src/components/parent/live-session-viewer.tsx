"use client";

import { useState, useEffect } from "react";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { CodeEditor } from "@/components/editor/code-editor";
import { LessonRenderer } from "@/components/lesson/lesson-renderer";
import { parseLessonContent } from "@/lib/lesson-content";

interface SessionTopic {
  title: string;
  lessonContent: unknown;
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
      {/* Lesson content */}
      {topics.length > 0 && (
        <div className="w-1/3 border-r overflow-auto p-4 space-y-4">
          {topics.map((t, i) => (
            <div key={i}>
              <h3 className="text-base font-semibold mb-2">{t.title}</h3>
              <LessonRenderer content={parseLessonContent(t.lessonContent)} />
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

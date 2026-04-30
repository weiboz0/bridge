"use client";

import Link from "next/link";
import { useState, useEffect } from "react";
import { useSession } from "next-auth/react";
import { useStudentLayout } from "@/lib/hooks/use-student-layout";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { EditorSwitcher } from "@/components/editor/editor-switcher";
import { AiChatPanel } from "@/components/ai/ai-chat-panel";
import { RaiseHandButton } from "@/components/help-queue/raise-hand-button";
import { CodeEditor } from "@/components/editor/code-editor";
import { Button } from "@/components/ui/button";

interface SessionTopic {
  topicId: string;
  title: string;
  // Plan 044 phase 2: lessonContent + starterCode kept on the response
  // for one release as a safety net (plan 046 drops them); UI no longer
  // reads them. The linked teaching_unit identity is the canonical
  // source of teaching material.
  unitId: string | null;
  unitTitle: string | null;
  unitMaterialType: string | null;
}

interface StudentSessionProps {
  sessionId: string;
  classId: string | null;
  returnPath?: string;
  editorMode: "python" | "javascript" | "blockly";
  starterCode?: string;
}

export function StudentSession({
  sessionId,
  classId,
  returnPath,
  editorMode,
  starterCode = "",
}: StudentSessionProps) {
  const { data: session } = useSession();
  const { mode, toggle } = useStudentLayout();
  const [topics, setTopics] = useState<SessionTopic[]>([]);
  const [aiEnabled, setAiEnabled] = useState(false);
  const [showAi, setShowAi] = useState(false);
  const [sidePanelMinimized, setSidePanelMinimized] = useState(false);
  const [broadcastActive, setBroadcastActive] = useState(false);

  const userId = session?.user?.id || "";
  const documentName = `session:${sessionId}:user:${userId}`;
  const token = `${userId}:user`;

  const { yText, provider, connected } = useYjsProvider({
    documentName,
    token,
  });

  // Broadcast document
  const broadcastDocName = `broadcast:${sessionId}`;
  const { yText: broadcastYText, provider: broadcastProvider } = useYjsProvider({
    documentName: broadcastActive ? broadcastDocName : "noop",
    token,
  });

  // Fetch session topics
  useEffect(() => {
    async function fetchTopics() {
      const res = await fetch(`/api/sessions/${sessionId}/topics`);
      if (res.ok) setTopics(await res.json());
    }
    fetchTopics();
  }, [sessionId]);

  // SSE events
  useEffect(() => {
    const eventSource = new EventSource(`/api/sessions/${sessionId}/events`);
    eventSource.addEventListener("ai_toggled", (e) => {
      const data = JSON.parse(e.data);
      if (data.studentId === userId) {
        setAiEnabled(data.enabled);
        if (data.enabled) setShowAi(true);
      }
    });
    eventSource.addEventListener("broadcast_started", () => setBroadcastActive(true));
    eventSource.addEventListener("broadcast_ended", () => setBroadcastActive(false));
    eventSource.addEventListener("session_ended", () => {
      window.location.href = returnPath ?? (classId ? `/student/classes/${classId}` : "/student");
    });
    return () => eventSource.close();
  }, [sessionId, classId, returnPath, userId]);

  // Plan 044 phase 2: render the linked teaching_unit per topic.
  // Click-through opens the Unit's projected view at /student/units/<id>
  // in a new tab so the editor pane isn't disturbed.
  const lessonPanel = topics.length > 0 ? (
    <div className="overflow-auto p-4 space-y-4">
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
  ) : null;

  const isStacked = mode === "stacked";

  return (
    <div className="flex h-screen">
      <div className={`flex ${isStacked ? "flex-col" : "flex-row"} flex-1 min-h-0`}>
        {/* Broadcast overlay */}
        {broadcastActive && (
          <div className={isStacked ? "h-40 shrink-0" : "w-1/3 shrink-0"}>
            <div className="h-full border-b border-r">
              <div className="bg-blue-50 dark:bg-blue-950 px-3 py-1 text-xs font-medium text-blue-700 dark:text-blue-300 border-b">
                Teacher is broadcasting
              </div>
              <div className="h-[calc(100%-28px)]">
                <CodeEditor yText={broadcastYText} provider={broadcastProvider} readOnly language={editorMode} />
              </div>
            </div>
          </div>
        )}

        {/* Lesson content */}
        {lessonPanel && (
          <div className={isStacked ? "h-1/3 shrink-0 border-b overflow-auto" : "w-1/3 shrink-0 border-r overflow-auto"}>
            {lessonPanel}
          </div>
        )}

        {/* Editor + output */}
        <div className="flex-1 min-h-0">
          <div className="flex items-center justify-between px-4 py-2 border-b">
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">
                {editorMode === "blockly" ? "Blockly" : editorMode}
              </span>
              <span className={`w-2 h-2 rounded-full ${connected ? "bg-green-500" : "bg-red-500"}`} />
            </div>
            <div className="flex gap-2">
              <Button variant="ghost" size="sm" onClick={toggle}>
                {isStacked ? "Side-by-side" : "Stacked"}
              </Button>
              <RaiseHandButton sessionId={sessionId} />
              {aiEnabled && (
                <Button variant="ghost" size="sm" onClick={() => setShowAi(!showAi)}>
                  {showAi ? "Hide AI" : "Ask AI"}
                </Button>
              )}
            </div>
          </div>
          <div className="h-[calc(100%-44px)]">
            <EditorSwitcher
              editorMode={editorMode}
              initialCode={starterCode}
              yText={yText}
              provider={provider}
            />
          </div>
        </div>
      </div>

      {/* Side panel: AI Chat */}
      {showAi && !sidePanelMinimized && (
        <div className="w-80 border-l flex flex-col">
          <div className="flex items-center justify-between px-3 py-2 border-b">
            <span className="text-sm font-medium">AI Tutor</span>
            <Button variant="ghost" size="sm" onClick={() => setSidePanelMinimized(true)}>−</Button>
          </div>
          <div className="flex-1 min-h-0">
            <AiChatPanel
              sessionId={sessionId}
              code={yText?.toString() || ""}
              enabled={true}
            />
          </div>
        </div>
      )}
      {showAi && sidePanelMinimized && (
        <div
          className="w-10 border-l flex flex-col items-center py-2 cursor-pointer hover:bg-muted/50"
          onClick={() => setSidePanelMinimized(false)}
        >
          <span className="text-xs writing-mode-vertical">AI</span>
        </div>
      )}
    </div>
  );
}

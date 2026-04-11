"use client";

import { useState, useEffect } from "react";
import { useParams } from "next/navigation";
import { useSession } from "next-auth/react";
import { CodeEditor } from "@/components/editor/code-editor";
import { OutputPanel } from "@/components/editor/output-panel";
import { RunButton } from "@/components/editor/run-button";
import { AiChatPanel } from "@/components/ai/ai-chat-panel";
import { RaiseHandButton } from "@/components/help-queue/raise-hand-button";
import { usePyodide } from "@/lib/pyodide/use-pyodide";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { Button } from "@/components/ui/button";
import { DiffViewer } from "@/components/editor/diff-viewer";

export default function StudentSessionPage() {
  const params = useParams<{ id: string; sessionId: string }>();
  const { data: session } = useSession();
  const [code, setCode] = useState("");
  const [aiEnabled, setAiEnabled] = useState(false);
  const [showAi, setShowAi] = useState(false);
  const [broadcastActive, setBroadcastActive] = useState(false);
  const [codeBeforeRun, setCodeBeforeRun] = useState<string | null>(null);
  const [showDiff, setShowDiff] = useState(false);
  const { ready, running, output, runCode, clearOutput } = usePyodide();

  const userId = session?.user?.id || "";
  const documentName = `session:${params.sessionId}:user:${userId}`;
  const token = `${userId}:user`;

  const { yText, provider, connected } = useYjsProvider({
    documentName,
    token,
  });

  // Broadcast document (read-only view of teacher's code)
  const broadcastDocName = `broadcast:${params.sessionId}`;
  const { yText: broadcastYText, provider: broadcastProvider } = useYjsProvider({
    documentName: broadcastActive ? broadcastDocName : "noop",
    token,
  });

  // Listen for SSE events (AI toggle, broadcast, session end)
  useEffect(() => {
    const eventSource = new EventSource(
      `/api/sessions/${params.sessionId}/events`
    );

    eventSource.addEventListener("ai_toggled", (e) => {
      const data = JSON.parse(e.data);
      if (data.studentId === userId) {
        setAiEnabled(data.enabled);
        if (data.enabled && !showAi) {
          setShowAi(true);
        }
      }
    });

    eventSource.addEventListener("broadcast_started", () => {
      setBroadcastActive(true);
    });

    eventSource.addEventListener("broadcast_ended", () => {
      setBroadcastActive(false);
    });

    eventSource.addEventListener("session_ended", () => {
      window.location.href = `/dashboard/classrooms/${params.id}`;
    });

    return () => eventSource.close();
  }, [params.sessionId, params.id, userId, showAi]);

  function handleRun() {
    const currentCode = yText?.toString() || code;
    setCodeBeforeRun(currentCode);
    setShowDiff(false);
    runCode(currentCode);
  }

  return (
    <div className="flex h-[calc(100vh-3.5rem)]">
      <div className="flex flex-col flex-1 gap-2 p-0">
        <div className="flex items-center justify-between px-4 pt-2">
          <div className="flex items-center gap-2">
            <h2 className="text-sm font-medium text-muted-foreground">
              Live Session
            </h2>
            <span
              className={`w-2 h-2 rounded-full ${
                connected ? "bg-green-500" : "bg-red-500"
              }`}
            />
          </div>
          <div className="flex gap-2">
            <RaiseHandButton sessionId={params.sessionId} />
            {aiEnabled && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setShowAi(!showAi)}
              >
                {showAi ? "Hide AI" : "Ask AI"}
              </Button>
            )}
            {codeBeforeRun !== null && !running && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setShowDiff(!showDiff)}
              >
                {showDiff ? "Editor" : "View Changes"}
              </Button>
            )}
            <Button
              variant="ghost"
              size="sm"
              onClick={clearOutput}
              disabled={running}
            >
              Clear
            </Button>
            <RunButton
              onRun={handleRun}
              running={running}
              ready={ready}
            />
          </div>
        </div>

        {broadcastActive && (
          <div className="mx-4 mb-2 border rounded-lg overflow-hidden">
            <div className="bg-blue-50 px-3 py-1 text-xs font-medium text-blue-700 border-b">
              Teacher is broadcasting
            </div>
            <div className="h-40">
              <CodeEditor yText={broadcastYText} provider={broadcastProvider} readOnly />
            </div>
          </div>
        )}

        <div className="flex-1 min-h-0 px-4">
          {showDiff && codeBeforeRun !== null ? (
            <DiffViewer
              original={codeBeforeRun}
              modified={yText?.toString() || code}
              readOnly
            />
          ) : (
            <CodeEditor
              onChange={setCode}
              yText={yText}
              provider={provider}
            />
          )}
        </div>

        <div className="h-[200px] shrink-0 px-4 pb-4">
          <OutputPanel output={output} running={running} />
        </div>
      </div>

      {showAi && (
        <div className="w-80 border-l p-2">
          <AiChatPanel
            sessionId={params.sessionId}
            code={yText?.toString() || code}
            enabled={true}
          />
        </div>
      )}
    </div>
  );
}

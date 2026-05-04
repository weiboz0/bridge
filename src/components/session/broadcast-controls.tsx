"use client";

import { useState } from "react";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { useRealtimeToken } from "@/lib/realtime/use-realtime-token";
import { CodeEditor } from "@/components/editor/code-editor";
import { Button } from "@/components/ui/button";

interface BroadcastControlsProps {
  sessionId: string;
}

export function BroadcastControls({ sessionId }: BroadcastControlsProps) {
  const [broadcasting, setBroadcasting] = useState(false);

  const documentName = broadcasting ? `broadcast:${sessionId}` : "noop";
  // Plan 053 phase 3 — JWT minted per-doc; helper returns "" until
  // the mint resolves. useYjsProvider's shouldConnect guard handles
  // empty-token by skipping the WS open.
  // Plan 068 phase 4 — banner lives on teacher-dashboard parent page,
  // not per-control (BroadcastControls is part of the dashboard
  // toolbar). Destructure { token } only.
  const { token: realtimeToken } = useRealtimeToken(documentName);
  const { yText, provider, connected } = useYjsProvider({
    documentName,
    token: realtimeToken,
  });

  function toggleBroadcast() {
    setBroadcasting(!broadcasting);
  }

  if (!broadcasting) {
    return (
      <Button variant="outline" size="sm" onClick={toggleBroadcast}>
        Start Broadcasting
      </Button>
    );
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium">Broadcasting Live</span>
          <span className={`w-2 h-2 rounded-full ${connected ? "bg-green-500 animate-pulse" : "bg-red-500"}`} />
        </div>
        <Button variant="destructive" size="sm" onClick={toggleBroadcast}>
          Stop
        </Button>
      </div>
      <div className="h-64 border rounded-lg">
        <CodeEditor yText={yText} provider={provider} />
      </div>
    </div>
  );
}

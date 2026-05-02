"use client";

import { useEffect, useRef, useState } from "react";
import * as Y from "yjs";
import { HocuspocusProvider } from "@hocuspocus/provider";
import { codeToHtml } from "shiki";
import { AiToggleButton } from "@/components/ai/ai-toggle-button";
import { useRealtimeToken } from "@/lib/realtime/use-realtime-token";

interface StudentTileProps {
  sessionId: string;
  studentId: string;
  studentName: string;
  status: string;
  helpRequestedAt?: string | null;
  onClick: () => void;
}

export function StudentTile({
  sessionId,
  studentId,
  studentName,
  status,
  helpRequestedAt,
  onClick,
}: StudentTileProps) {
  const [html, setHtml] = useState("");
  const providerRef = useRef<HocuspocusProvider | null>(null);
  const yDocRef = useRef<Y.Doc | null>(null);

  // Plan 053 phase 3 — each tile mints its own JWT scoped to the
  // student's session doc. The cache in `getRealtimeToken` collapses
  // duplicate fetches when many tiles render simultaneously.
  const documentName = `session:${sessionId}:user:${studentId}`;
  const realtimeToken = useRealtimeToken(documentName);

  useEffect(() => {
    if (!realtimeToken) return;
    const yDoc = new Y.Doc();
    const yText = yDoc.getText("content");

    const provider = new HocuspocusProvider({
      url: process.env.NEXT_PUBLIC_HOCUSPOCUS_URL || `ws://${window.location.hostname}:4000`,
      name: documentName,
      document: yDoc,
      token: realtimeToken,
    });

    yDocRef.current = yDoc;
    providerRef.current = provider;

    async function highlight(code: string) {
      if (!code.trim()) {
        setHtml("");
        return;
      }
      const result = await codeToHtml(code, {
        lang: "python",
        theme: "github-light",
      });
      setHtml(result);
    }

    // Initial render
    highlight(yText.toString());

    // Subscribe to changes with debounce to avoid flooding Shiki
    // when many students type simultaneously
    let debounceTimer: ReturnType<typeof setTimeout>;
    const observer = () => {
      clearTimeout(debounceTimer);
      debounceTimer = setTimeout(() => highlight(yText.toString()), 300);
    };
    yText.observe(observer);

    return () => {
      clearTimeout(debounceTimer);
      yText.unobserve(observer);
      provider.destroy();
      yDoc.destroy();
    };
  }, [sessionId, studentId, documentName, realtimeToken]);

  const statusColor = {
    present: "bg-green-500",
    invited: "bg-blue-500",
    left: "bg-zinc-400",
  }[status] || "bg-gray-500";
  const dotColor = helpRequestedAt ? "bg-red-500" : statusColor;

  return (
    <div
      onClick={onClick}
      className="border rounded-lg p-2 cursor-pointer hover:border-primary transition-colors"
    >
      <div className="flex items-center justify-between mb-1">
        <span className="text-xs font-medium truncate">{studentName}</span>
        <div className="flex items-center gap-1">
          <AiToggleButton sessionId={sessionId} studentId={studentId} />
          <span className={`w-2 h-2 rounded-full ${dotColor}`} />
        </div>
      </div>
      <div
        className="h-24 overflow-hidden rounded text-[9px] font-mono leading-tight"
        dangerouslySetInnerHTML={{ __html: html }}
      />
    </div>
  );
}

"use client";

import { useEffect, useRef, useState } from "react";
import * as Y from "yjs";
import { HocuspocusProvider } from "@hocuspocus/provider";
import { codeToHtml } from "shiki";
import { AiToggleButton } from "@/components/ai/ai-toggle-button";

interface StudentTileProps {
  sessionId: string;
  studentId: string;
  studentName: string;
  status: string;
  token: string;
  onClick: () => void;
}

export function StudentTile({
  sessionId,
  studentId,
  studentName,
  status,
  token,
  onClick,
}: StudentTileProps) {
  const [html, setHtml] = useState("");
  const providerRef = useRef<HocuspocusProvider | null>(null);
  const yDocRef = useRef<Y.Doc | null>(null);

  useEffect(() => {
    const yDoc = new Y.Doc();
    const yText = yDoc.getText("content");
    const documentName = `session:${sessionId}:user:${studentId}`;

    const provider = new HocuspocusProvider({
      url: process.env.NEXT_PUBLIC_HOCUSPOCUS_URL || `ws://${window.location.hostname}:4000`,
      name: documentName,
      document: yDoc,
      token,
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

    // Subscribe to changes
    const observer = () => {
      highlight(yText.toString());
    };
    yText.observe(observer);

    return () => {
      yText.unobserve(observer);
      provider.destroy();
      yDoc.destroy();
    };
  }, [sessionId, studentId, token]);

  const statusColor = {
    active: "bg-green-500",
    idle: "bg-yellow-500",
    needs_help: "bg-red-500",
  }[status] || "bg-gray-500";

  return (
    <div
      onClick={onClick}
      className="border rounded-lg p-2 cursor-pointer hover:border-primary transition-colors"
    >
      <div className="flex items-center justify-between mb-1">
        <span className="text-xs font-medium truncate">{studentName}</span>
        <div className="flex items-center gap-1">
          <AiToggleButton sessionId={sessionId} studentId={studentId} />
          <span className={`w-2 h-2 rounded-full ${statusColor}`} />
        </div>
      </div>
      <div
        className="h-24 overflow-hidden rounded text-[9px] font-mono leading-tight"
        dangerouslySetInnerHTML={{ __html: html }}
      />
    </div>
  );
}

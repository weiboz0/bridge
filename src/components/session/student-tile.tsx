"use client";

import { useEffect, useRef } from "react";
import { EditorView, lineNumbers } from "@codemirror/view";
import { EditorState } from "@codemirror/state";
import { python } from "@codemirror/lang-python";
import { syntaxHighlighting, defaultHighlightStyle } from "@codemirror/language";
import * as Y from "yjs";
import { HocuspocusProvider } from "@hocuspocus/provider";
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
  const containerRef = useRef<HTMLDivElement>(null);
  const providerRef = useRef<HocuspocusProvider | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    const yDoc = new Y.Doc();
    const yText = yDoc.getText("content");
    const documentName = `session:${sessionId}:user:${studentId}`;

    const provider = new HocuspocusProvider({
      url: process.env.NEXT_PUBLIC_HOCUSPOCUS_URL || `ws://${window.location.hostname}:4000`,
      name: documentName,
      document: yDoc,
      token,
    });
    providerRef.current = provider;

    import("y-codemirror.next").then(({ yCollab }) => {
      if (!containerRef.current) return;

      const state = EditorState.create({
        extensions: [
          lineNumbers(),
          python(),
          syntaxHighlighting(defaultHighlightStyle),
          EditorView.editable.of(false),
          EditorView.theme({
            "&": { height: "100%", fontSize: "9px" },
            ".cm-scroller": { overflow: "hidden", fontFamily: "monospace" },
            ".cm-gutters": { display: "none" },
          }),
          yCollab(yText, provider.awareness!),
        ],
      });

      new EditorView({ state, parent: containerRef.current });
    });

    return () => {
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
      <div ref={containerRef} className="h-24 overflow-hidden rounded" />
    </div>
  );
}

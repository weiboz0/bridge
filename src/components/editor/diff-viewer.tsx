"use client";

import { DiffEditor } from "@monaco-editor/react";
import { setupMonaco } from "@/lib/monaco/setup";
import { useTheme } from "@/lib/hooks/use-theme";

setupMonaco();

interface DiffViewerProps {
  original: string;
  modified: string;
  readOnly?: boolean;
}

export function DiffViewer({
  original,
  modified,
  readOnly = false,
}: DiffViewerProps) {
  const { theme } = useTheme();

  return (
    <div className="border rounded-lg overflow-hidden h-full">
      <DiffEditor
        original={original}
        modified={modified}
        language="python"
        theme={theme === "dark" ? "bridge-dark" : "bridge-light"}
        options={{
          readOnly,
          renderSideBySide: true,
          minimap: { enabled: false },
          fontSize: 14,
          fontFamily: "var(--font-geist-mono), monospace",
          lineNumbers: "on",
          scrollBeyondLastLine: false,
          automaticLayout: true,
          contextmenu: false,
        }}
      />
    </div>
  );
}

"use client";

import { useRef, useEffect, useId } from "react";
import { Editor, type OnMount } from "@monaco-editor/react";
import type * as monacoTypes from "monaco-editor";
import type * as Y from "yjs";
import type { HocuspocusProvider } from "@hocuspocus/provider";
import { setupMonaco } from "@/lib/monaco/setup";
import { useTheme } from "@/lib/hooks/use-theme";

setupMonaco();

interface CodeEditorProps {
  initialCode?: string;
  onChange?: (code: string) => void;
  readOnly?: boolean;
  yText?: Y.Text | null;
  provider?: HocuspocusProvider | null;
}

export function CodeEditor({
  initialCode = "",
  onChange,
  readOnly = false,
  yText,
  provider,
}: CodeEditorProps) {
  const editorRef = useRef<monacoTypes.editor.IStandaloneCodeEditor | null>(null);
  const bindingRef = useRef<any>(null);
  const { theme } = useTheme();
  const instanceId = useId();

  // Clean up Yjs binding when yText/provider change or unmount
  useEffect(() => {
    return () => {
      bindingRef.current?.destroy();
      bindingRef.current = null;
    };
  }, [yText, provider]);

  const handleMount: OnMount = (editor, monaco) => {
    editorRef.current = editor;

    // Register themes
    import("@/lib/monaco/themes").then(({ bridgeLightTheme, bridgeDarkTheme }) => {
      monaco.editor.defineTheme("bridge-light", bridgeLightTheme);
      monaco.editor.defineTheme("bridge-dark", bridgeDarkTheme);
      monaco.editor.setTheme(theme === "dark" ? "bridge-dark" : "bridge-light");
    });

    // Yjs collaborative binding
    if (yText && provider) {
      import("y-monaco").then(({ MonacoBinding }) => {
        bindingRef.current = new MonacoBinding(
          yText,
          editor.getModel()!,
          new Set([editor]),
          provider.awareness!
        );
      });
    }

    // onChange listener (works in both collaborative and standalone modes)
    if (onChange) {
      editor.onDidChangeModelContent(() => {
        onChange(editor.getValue());
      });
    }
  };

  const modelUri = `inmemory://editor/${instanceId}`;

  return (
    <div className="border rounded-lg overflow-hidden h-full">
      <Editor
        defaultValue={yText ? undefined : initialCode}
        language="python"
        theme={theme === "dark" ? "bridge-dark" : "bridge-light"}
        path={modelUri}
        onMount={handleMount}
        options={{
          readOnly,
          minimap: { enabled: true },
          fontSize: 14,
          fontFamily: "var(--font-geist-mono), monospace",
          lineNumbers: "on",
          bracketPairColorization: { enabled: true },
          autoClosingBrackets: "always",
          matchBrackets: "always",
          folding: true,
          wordWrap: "on",
          scrollBeyondLastLine: false,
          automaticLayout: true,
          tabSize: 4,
          insertSpaces: true,
          quickSuggestions: true,
          suggestOnTriggerCharacters: true,
          parameterHints: { enabled: true },
          // Disable features that confuse K-12 students
          contextmenu: false,
          overviewRulerBorder: false,
        }}
      />
    </div>
  );
}

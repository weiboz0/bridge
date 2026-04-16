"use client";

import { useRef, useEffect, useId, useState } from "react";
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
  language?: string;
  yText?: Y.Text | null;
  provider?: HocuspocusProvider | null;
}

export function CodeEditor({
  initialCode = "",
  onChange,
  readOnly = false,
  language = "python",
  yText,
  provider,
}: CodeEditorProps) {
  const editorRef = useRef<monacoTypes.editor.IStandaloneCodeEditor | null>(null);
  const bindingRef = useRef<any>(null);
  const [editorReady, setEditorReady] = useState(false);
  const { theme } = useTheme();
  const instanceId = useId();

  // Create/recreate Yjs binding when editor, yText, or provider become available
  useEffect(() => {
    const editor = editorRef.current;
    if (!editorReady || !editor || !yText || !provider) return;

    let binding: any;
    import("y-monaco").then(({ MonacoBinding }) => {
      binding = new MonacoBinding(
        yText,
        editor.getModel()!,
        new Set([editor]),
        provider.awareness!
      );
      bindingRef.current = binding;
    });

    return () => {
      binding?.destroy();
      bindingRef.current = null;
    };
  }, [editorReady, yText, provider]);

  const handleMount: OnMount = (editor, monaco) => {
    editorRef.current = editor;
    setEditorReady(true);

    // Register themes
    import("@/lib/monaco/themes").then(({ bridgeLightTheme, bridgeDarkTheme }) => {
      monaco.editor.defineTheme("bridge-light", bridgeLightTheme);
      monaco.editor.defineTheme("bridge-dark", bridgeDarkTheme);
      monaco.editor.setTheme(theme === "dark" ? "bridge-dark" : "bridge-light");
    });

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
        language={language}
        theme={theme === "dark" ? "bridge-dark" : "bridge-light"}
        path={modelUri}
        onMount={handleMount}
        options={{
          readOnly,
          minimap: { enabled: true },
          fontSize: 14,
          fontFamily: "'JetBrains Mono', var(--font-mono), monospace",
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

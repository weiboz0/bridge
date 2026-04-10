"use client";

import { useRef, useEffect } from "react";
import { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter } from "@codemirror/view";
import { EditorState, type Extension } from "@codemirror/state";
import { defaultKeymap, indentWithTab, history, historyKeymap } from "@codemirror/commands";
import { python } from "@codemirror/lang-python";
import { syntaxHighlighting, defaultHighlightStyle, bracketMatching, indentOnInput } from "@codemirror/language";
import { autocompletion, closeBrackets } from "@codemirror/autocomplete";
import type * as Y from "yjs";
import type { HocuspocusProvider } from "@hocuspocus/provider";

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
  const containerRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    // If Yjs binding is provided, wait for it to be ready
    if (yText !== undefined && !yText) return;

    const extensions: Extension[] = [
      lineNumbers(),
      highlightActiveLine(),
      highlightActiveLineGutter(),
      bracketMatching(),
      closeBrackets(),
      indentOnInput(),
      autocompletion(),
      python(),
      syntaxHighlighting(defaultHighlightStyle),
      keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
      EditorView.editable.of(!readOnly),
      EditorView.theme({
        "&": { height: "100%", fontSize: "14px" },
        ".cm-scroller": { overflow: "auto", fontFamily: "var(--font-geist-mono), monospace" },
        ".cm-content": { minHeight: "200px" },
      }),
    ];

    if (yText && provider) {
      // Yjs collaborative mode — import dynamically to avoid SSR issues
      import("y-codemirror.next").then(({ yCollab }) => {
        if (!containerRef.current) return;

        extensions.push(yCollab(yText, provider.awareness!));

        const state = EditorState.create({ extensions });
        const view = new EditorView({ state, parent: containerRef.current });
        viewRef.current = view;
      });
      return () => {
        viewRef.current?.destroy();
        viewRef.current = null;
      };
    }

    // Non-collaborative mode
    extensions.push(history());

    if (onChange) {
      extensions.push(
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            onChange(update.state.doc.toString());
          }
        })
      );
    }

    const state = EditorState.create({
      doc: initialCode,
      extensions,
    });

    const view = new EditorView({
      state,
      parent: containerRef.current,
    });

    viewRef.current = view;

    return () => {
      view.destroy();
      viewRef.current = null;
    };
  }, [yText, provider]);

  return (
    <div
      ref={containerRef}
      className="border rounded-lg overflow-hidden h-full"
    />
  );
}

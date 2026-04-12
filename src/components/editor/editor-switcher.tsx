"use client";

import { useState, useCallback } from "react";
import { CodeEditor } from "./code-editor";
import { OutputPanel } from "./output-panel";
import { RunButton } from "./run-button";
import { HtmlPreview } from "./html-preview";
import { BlocklyEditor } from "./blockly/blockly-editor";
import { usePyodide } from "@/lib/pyodide/use-pyodide";
import { useJsRunner } from "@/lib/js-runner/use-js-runner";
import { Button } from "@/components/ui/button";
import type * as Y from "yjs";
import type { HocuspocusProvider } from "@hocuspocus/provider";

type EditorMode = "python" | "javascript" | "blockly";

interface EditorSwitcherProps {
  editorMode: EditorMode;
  initialCode?: string;
  yText?: Y.Text | null;
  provider?: HocuspocusProvider | null;
}

export function EditorSwitcher({
  editorMode,
  initialCode = "",
  yText,
  provider,
}: EditorSwitcherProps) {
  const [code, setCode] = useState(initialCode);
  const [blocklyCode, setBlocklyCode] = useState("");

  const pyodide = usePyodide();
  const jsRunner = useJsRunner();

  const isPython = editorMode === "python";
  const isJs = editorMode === "javascript";
  const isBlockly = editorMode === "blockly";

  const runner = isPython ? pyodide : jsRunner;

  const handleRun = useCallback(() => {
    if (isBlockly) {
      jsRunner.runCode(blocklyCode);
    } else if (isPython) {
      const currentCode = yText?.toString() || code;
      pyodide.runCode(currentCode);
    } else {
      const currentCode = yText?.toString() || code;
      jsRunner.runCode(currentCode);
    }
  }, [isBlockly, isPython, blocklyCode, code, yText, pyodide, jsRunner]);

  const handleClear = useCallback(() => {
    if (isPython) {
      pyodide.clearOutput();
    } else {
      jsRunner.clearOutput();
    }
  }, [isPython, pyodide, jsRunner]);

  const output = isBlockly ? jsRunner.output : runner.output;
  const running = isBlockly ? jsRunner.running : runner.running;
  const ready = isBlockly ? jsRunner.ready : runner.ready;

  return (
    <div className="flex flex-col h-full gap-2">
      {/* Toolbar */}
      <div className="flex items-center justify-between px-1">
        <span className="text-xs text-muted-foreground font-mono">
          {isBlockly ? "Blockly → JavaScript" : editorMode}
        </span>
        <div className="flex gap-2">
          <Button variant="ghost" size="sm" onClick={handleClear} disabled={running}>
            Clear
          </Button>
          <RunButton
            onRun={handleRun}
            running={running}
            ready={ready}
          />
        </div>
      </div>

      {/* Editor area */}
      <div className="flex-1 min-h-0">
        {isBlockly ? (
          <BlocklyEditor
            onChange={(state, generatedCode) => {
              setCode(state);
              setBlocklyCode(generatedCode);
            }}
          />
        ) : (
          <div className={isJs ? "flex gap-2 h-full" : "h-full"}>
            <div className={isJs ? "flex-1" : "h-full"}>
              <CodeEditor
                initialCode={initialCode}
                onChange={setCode}
                language={editorMode}
                yText={yText}
                provider={provider}
              />
            </div>
            {isJs && (
              <div className="w-1/2">
                <HtmlPreview html={code} />
              </div>
            )}
          </div>
        )}
      </div>

      {/* Output */}
      <div className="h-[180px] shrink-0">
        <OutputPanel output={output} running={running} />
      </div>
    </div>
  );
}

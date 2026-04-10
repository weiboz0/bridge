"use client";

import { useState } from "react";
import { CodeEditor } from "@/components/editor/code-editor";
import { OutputPanel } from "@/components/editor/output-panel";
import { RunButton } from "@/components/editor/run-button";
import { usePyodide } from "@/lib/pyodide/use-pyodide";
import { Button } from "@/components/ui/button";

const STARTER_CODE = `# Welcome to Bridge!
# Write your Python code here and click Run.

print("Hello, world!")
`;

export default function EditorPage() {
  const [code, setCode] = useState(STARTER_CODE);
  const { ready, running, output, runCode, clearOutput } = usePyodide();

  return (
    <div className="flex flex-col h-[calc(100vh-3.5rem)] gap-2 p-0">
      <div className="flex items-center justify-between px-4 pt-2">
        <h2 className="text-sm font-medium text-muted-foreground">Python Editor</h2>
        <div className="flex gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={clearOutput}
            disabled={running}
          >
            Clear
          </Button>
          <RunButton onRun={() => runCode(code)} running={running} ready={ready} />
        </div>
      </div>

      <div className="flex-1 min-h-0 px-4">
        <CodeEditor initialCode={STARTER_CODE} onChange={setCode} />
      </div>

      <div className="h-[200px] shrink-0 px-4 pb-4">
        <OutputPanel output={output} running={running} />
      </div>
    </div>
  );
}

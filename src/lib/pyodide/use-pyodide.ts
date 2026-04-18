"use client";

import { useState, useRef, useCallback, useEffect } from "react";
import type { OutputLine } from "@/components/editor/output-panel";

interface UsePyodideReturn {
  ready: boolean;
  running: boolean;
  output: OutputLine[];
  runCode: (code: string, options?: { stdin?: string }) => void;
  clearOutput: () => void;
}

export function usePyodide(): UsePyodideReturn {
  const [ready, setReady] = useState(false);
  const [running, setRunning] = useState(false);
  const [output, setOutput] = useState<OutputLine[]>([]);
  const workerRef = useRef<Worker | null>(null);

  useEffect(() => {
    const worker = new Worker(
      new URL("@/workers/pyodide-worker.ts", import.meta.url),
      { type: "classic" }
    );

    worker.onmessage = (event) => {
      const { data } = event;

      switch (data.type) {
        case "ready":
          setReady(true);
          break;
        case "stdout":
          setOutput((prev) => [...prev, { type: "stdout", text: data.text }]);
          break;
        case "stderr":
          setOutput((prev) => [...prev, { type: "stderr", text: data.text }]);
          break;
        case "done":
          setRunning(false);
          break;
      }
    };

    worker.postMessage({ type: "init" });
    workerRef.current = worker;

    return () => {
      worker.terminate();
      workerRef.current = null;
    };
  }, []);

  const runCode = useCallback(
    (code: string, options?: { stdin?: string }) => {
      if (!workerRef.current || !ready) return;
      setRunning(true);
      setOutput([]);
      const id = crypto.randomUUID();
      workerRef.current.postMessage({ type: "run", code, id, stdin: options?.stdin });
    },
    [ready]
  );

  const clearOutput = useCallback(() => {
    setOutput([]);
  }, []);

  return { ready, running, output, runCode, clearOutput };
}

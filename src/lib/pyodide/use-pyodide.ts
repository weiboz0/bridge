"use client";

import { useState, useRef, useCallback, useEffect } from "react";
import type { OutputLine } from "@/components/editor/output-panel";

interface UsePyodideReturn {
  ready: boolean;
  running: boolean;
  output: OutputLine[];
  /** True when the running program has called input() and is waiting for a line. */
  awaitingInput: boolean;
  runCode: (code: string, options?: { stdin?: string }) => void;
  /** Feed one line into the running program. No-op when not awaitingInput. */
  provideStdin: (line: string) => void;
  clearOutput: () => void;
}

const STDIN_BUFFER_BYTES = 64 * 1024;

export function usePyodide(): UsePyodideReturn {
  const [ready, setReady] = useState(false);
  const [running, setRunning] = useState(false);
  const [output, setOutput] = useState<OutputLine[]>([]);
  const [awaitingInput, setAwaitingInput] = useState(false);

  const workerRef = useRef<Worker | null>(null);
  // SAB control + buf live for the worker's lifetime.
  const sabControlRef = useRef<Int32Array | null>(null);
  const sabBufRef = useRef<Uint8Array | null>(null);
  // Pre-fed stdin queue drained as the worker requests lines.
  const queueRef = useRef<string[]>([]);

  const writeStdinLine = useCallback((line: string) => {
    const control = sabControlRef.current;
    const buf = sabBufRef.current;
    if (!control || !buf) return; // SAB not available — legacy path will be used by the worker
    const bytes = new TextEncoder().encode(line);
    if (bytes.length > buf.length) {
      console.warn("[pyodide] stdin line exceeds SAB buffer; truncating");
    }
    const writeLen = Math.min(bytes.length, buf.length);
    buf.set(bytes.subarray(0, writeLen));
    Atomics.store(control, 1, writeLen);
    Atomics.store(control, 0, 1);
    Atomics.notify(control, 0, 1);
  }, []);

  useEffect(() => {
    const worker = new Worker(
      new URL("@/workers/pyodide-worker.ts", import.meta.url),
      { type: "classic" }
    );

    // Allocate SAB only when crossOriginIsolated is true (COOP/COEP set).
    // In non-isolated dev contexts the worker falls back to its legacy
    // pre-fed buffer.
    if (typeof crossOriginIsolated !== "undefined" && crossOriginIsolated) {
      try {
        const sab = new SharedArrayBuffer(8 + STDIN_BUFFER_BYTES);
        sabControlRef.current = new Int32Array(sab, 0, 2);
        sabBufRef.current = new Uint8Array(sab, 8);
        worker.postMessage({ type: "init-stdin", sab });
      } catch (e) {
        console.warn("[pyodide] SAB allocation failed; falling back to buffered stdin", e);
      }
    }

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
        case "input_request":
          // Drain pre-fed queue if it has lines; otherwise prompt the user.
          if (queueRef.current.length > 0) {
            writeStdinLine(queueRef.current.shift()!);
          } else {
            setAwaitingInput(true);
          }
          break;
        case "done":
          setRunning(false);
          setAwaitingInput(false);
          // Drain any leftover queued lines on completion.
          queueRef.current = [];
          break;
      }
    };

    worker.postMessage({ type: "init" });
    workerRef.current = worker;

    return () => {
      worker.terminate();
      workerRef.current = null;
    };
  }, [writeStdinLine]);

  const runCode = useCallback(
    (code: string, options?: { stdin?: string }) => {
      if (!workerRef.current || !ready) return;
      setRunning(true);
      setOutput([]);
      setAwaitingInput(false);
      // Seed the queue from the pre-supplied stdin (one entry per line, drop
      // a trailing empty so split("\n") doesn't yield a phantom blank input).
      const lines = options?.stdin ? options.stdin.split("\n") : [];
      if (lines.length > 0 && lines[lines.length - 1] === "") lines.pop();
      queueRef.current = lines;

      const id = crypto.randomUUID();
      // Pass `stdin` through too — used by the worker's legacy path when SAB
      // isn't wired (dev without COOP/COEP).
      workerRef.current.postMessage({ type: "run", code, id, stdin: options?.stdin });
    },
    [ready]
  );

  const provideStdin = useCallback(
    (line: string) => {
      if (!awaitingInput) return;
      writeStdinLine(line);
      setAwaitingInput(false);
    },
    [awaitingInput, writeStdinLine]
  );

  const clearOutput = useCallback(() => {
    setOutput([]);
  }, []);

  return { ready, running, output, awaitingInput, runCode, provideStdin, clearOutput };
}

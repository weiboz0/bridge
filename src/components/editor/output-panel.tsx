"use client";

import { useState, useEffect, useRef, type KeyboardEvent } from "react";

export interface OutputLine {
  type: "stdout" | "stderr";
  text: string;
}

interface OutputPanelProps {
  output: OutputLine[];
  running: boolean;
  /** When true, renders an inline stdin field. On Enter the value is sent
   *  via `onStdin` and the field clears. */
  awaitingInput?: boolean;
  onStdin?: (line: string) => void;
}

export function OutputPanel({
  output,
  running,
  awaitingInput = false,
  onStdin,
}: OutputPanelProps) {
  const [draft, setDraft] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  // Autofocus the stdin field whenever the program starts asking for input.
  useEffect(() => {
    if (awaitingInput && inputRef.current) {
      inputRef.current.focus();
    }
  }, [awaitingInput]);

  function handleKey(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key !== "Enter") return;
    e.preventDefault();
    onStdin?.(draft);
    setDraft("");
  }

  return (
    <div
      data-testid="output-panel"
      className="bg-zinc-50 text-zinc-900 border border-zinc-200 font-mono text-sm p-3 rounded-lg overflow-auto h-full min-h-[120px]"
    >
      {running && !awaitingInput && (
        <div className="text-amber-700 mb-1">Running...</div>
      )}
      {output.map((line, i) => (
        <div
          key={i}
          className={
            line.type === "stderr"
              ? "text-red-700 whitespace-pre-wrap"
              : "whitespace-pre-wrap"
          }
        >
          {line.text}
        </div>
      ))}
      {awaitingInput && onStdin && (
        <div className="mt-1 flex items-center gap-1">
          <span className="text-amber-700">›</span>
          <input
            ref={inputRef}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={handleKey}
            placeholder="press Enter to submit"
            className="flex-1 bg-transparent text-zinc-900 outline-none placeholder:text-zinc-400"
            aria-label="stdin"
          />
        </div>
      )}
    </div>
  );
}

"use client";

export interface OutputLine {
  type: "stdout" | "stderr";
  text: string;
}

interface OutputPanelProps {
  output: OutputLine[];
  running: boolean;
}

export function OutputPanel({ output, running }: OutputPanelProps) {
  return (
    <div
      data-testid="output-panel"
      className="bg-zinc-950 text-zinc-100 font-mono text-sm p-3 rounded-lg overflow-auto h-full min-h-[120px]"
    >
      {running && (
        <div className="text-yellow-400 mb-1">Running...</div>
      )}
      {output.map((line, i) => (
        <div
          key={i}
          className={
            line.type === "stderr"
              ? "text-red-400 whitespace-pre-wrap"
              : "whitespace-pre-wrap"
          }
        >
          {line.text}
        </div>
      ))}
    </div>
  );
}

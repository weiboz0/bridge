"use client";

import { Button } from "@/components/ui/button";

interface RunButtonProps {
  onRun: () => void;
  running: boolean;
  ready: boolean;
  language?: string;
}

export function RunButton({ onRun, running, ready, language = "python" }: RunButtonProps) {
  const loadingText = language === "python" ? "Loading Python..." : "Loading...";

  return (
    <Button
      onClick={onRun}
      disabled={running || !ready}
      size="sm"
      variant={running ? "outline" : "default"}
    >
      {!ready ? loadingText : running ? "Running..." : "Run"}
    </Button>
  );
}

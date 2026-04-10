"use client";

import { Button } from "@/components/ui/button";

interface RunButtonProps {
  onRun: () => void;
  running: boolean;
  ready: boolean;
}

export function RunButton({ onRun, running, ready }: RunButtonProps) {
  return (
    <Button
      onClick={onRun}
      disabled={running || !ready}
      size="sm"
      variant={running ? "outline" : "default"}
    >
      {!ready ? "Loading Python..." : running ? "Running..." : "Run"}
    </Button>
  );
}

"use client";

import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";

interface TeacherHeaderProps {
  sessionId: string;
  studentCount: number;
  onEndSession: () => void;
  onToggleLeft: () => void;
  onToggleRight: () => void;
  leftVisible: boolean;
  rightVisible: boolean;
}

export function TeacherHeader({
  sessionId,
  studentCount,
  onEndSession,
  onToggleLeft,
  onToggleRight,
  leftVisible,
  rightVisible,
}: TeacherHeaderProps) {
  const [elapsed, setElapsed] = useState(0);
  const [ending, setEnding] = useState(false);

  useEffect(() => {
    const interval = setInterval(() => setElapsed((e) => e + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const minutes = Math.floor(elapsed / 60);
  const seconds = elapsed % 60;

  return (
    <div className="flex items-center justify-between px-4 py-2 border-b bg-muted/20">
      <div className="flex items-center gap-4">
        <span className="text-sm font-medium">Live Session</span>
        <span className="text-xs text-muted-foreground font-mono">
          {minutes}:{seconds.toString().padStart(2, "0")}
        </span>
        <span className="text-xs text-muted-foreground">
          {studentCount} student{studentCount !== 1 ? "s" : ""}
        </span>
      </div>
      <div className="flex items-center gap-2">
        <Button
          variant="ghost"
          size="sm"
          onClick={onToggleLeft}
          className={leftVisible ? "" : "opacity-50"}
        >
          ☰ Students
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={onToggleRight}
          className={rightVisible ? "" : "opacity-50"}
        >
          AI ☰
        </Button>
        <Button
          variant="destructive"
          size="sm"
          onClick={() => { setEnding(true); onEndSession(); }}
          disabled={ending}
        >
          {ending ? "Ending..." : "End Session"}
        </Button>
      </div>
    </div>
  );
}

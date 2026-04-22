"use client";

import { AiToggleButton } from "@/components/ai/ai-toggle-button";

interface Participant {
  studentId: string;
  name: string;
  status: string;
  helpRequestedAt?: string | null;
}

interface StudentListPanelProps {
  sessionId: string;
  participants: Participant[];
  selectedStudentId: string | null;
  onSelectStudent: (id: string) => void;
}

export function StudentListPanel({
  sessionId,
  participants,
  selectedStudentId,
  onSelectStudent,
}: StudentListPanelProps) {
  // Sort: hand raised first, then present, invited, and left.
  const sorted = [...participants].sort((a, b) => {
    const priority = (p: Participant) => {
      if (p.helpRequestedAt) return 0;
      if (p.status === "present") return 1;
      if (p.status === "invited") return 2;
      if (p.status === "left") return 3;
      return 4;
    };
    return priority(a) - priority(b);
  });

  return (
    <div className="h-full flex flex-col">
      <div className="px-3 py-2 border-b">
        <h3 className="text-sm font-medium">Students ({participants.length})</h3>
      </div>
      <div className="flex-1 overflow-auto">
        {sorted.map((p) => {
          const statusColor = {
            present: "bg-green-500",
            invited: "bg-blue-500",
            left: "bg-zinc-400",
          }[p.status] || "bg-gray-500";
          const dotColor = p.helpRequestedAt ? "bg-red-500" : statusColor;

          return (
            <div
              key={p.studentId}
              onClick={() => onSelectStudent(p.studentId)}
              className={`flex items-center justify-between px-3 py-2 cursor-pointer hover:bg-muted/50 transition-colors border-b border-border/30 ${
                selectedStudentId === p.studentId ? "bg-primary/10" : ""
              }`}
            >
              <div className="flex items-center gap-2">
                <span className={`w-2 h-2 rounded-full ${dotColor}`} />
                <span className="text-sm truncate">{p.name}</span>
                {p.helpRequestedAt && <span className="text-xs">✋</span>}
              </div>
              <AiToggleButton sessionId={sessionId} studentId={p.studentId} />
            </div>
          );
        })}
      </div>
    </div>
  );
}

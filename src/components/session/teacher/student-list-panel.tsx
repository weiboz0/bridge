"use client";

import { AiToggleButton } from "@/components/ai/ai-toggle-button";

interface Participant {
  studentId: string;
  name: string;
  status: string;
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
  // Sort: needs_help first, then active, then idle
  const sorted = [...participants].sort((a, b) => {
    const priority: Record<string, number> = { needs_help: 0, active: 1, idle: 2 };
    return (priority[a.status] ?? 3) - (priority[b.status] ?? 3);
  });

  return (
    <div className="h-full flex flex-col">
      <div className="px-3 py-2 border-b">
        <h3 className="text-sm font-medium">Students ({participants.length})</h3>
      </div>
      <div className="flex-1 overflow-auto">
        {sorted.map((p) => {
          const statusColor = {
            active: "bg-green-500",
            idle: "bg-yellow-500",
            needs_help: "bg-red-500",
          }[p.status] || "bg-gray-500";

          return (
            <div
              key={p.studentId}
              onClick={() => onSelectStudent(p.studentId)}
              className={`flex items-center justify-between px-3 py-2 cursor-pointer hover:bg-muted/50 transition-colors border-b border-border/30 ${
                selectedStudentId === p.studentId ? "bg-primary/10" : ""
              }`}
            >
              <div className="flex items-center gap-2">
                <span className={`w-2 h-2 rounded-full ${statusColor}`} />
                <span className="text-sm truncate">{p.name}</span>
                {p.status === "needs_help" && <span className="text-xs">✋</span>}
              </div>
              <AiToggleButton sessionId={sessionId} studentId={p.studentId} />
            </div>
          );
        })}
      </div>
    </div>
  );
}

"use client";

import { useState, type ReactNode } from "react";
import { AiToggleButton } from "@/components/ai/ai-toggle-button";
import { Button } from "@/components/ui/button";

interface Participant {
  userId: string;
  studentId: string;
  name: string;
  status: string;
  invitedBy?: string | null;
  helpRequestedAt?: string | null;
}

interface StudentListPanelProps {
  sessionId: string;
  participants: Participant[];
  selectedStudentId: string | null;
  onSelectStudent: (id: string) => void;
  onParticipantsChanged: () => Promise<unknown>;
}

function Badge({
  children,
  className = "",
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <span
      className={`inline-flex items-center rounded-md border px-2 py-0.5 text-[11px] font-medium ${className}`}
    >
      {children}
    </span>
  );
}

export function StudentListPanel({
  sessionId,
  participants,
  selectedStudentId,
  onSelectStudent,
  onParticipantsChanged,
}: StudentListPanelProps) {
  const [removingUserId, setRemovingUserId] = useState<string | null>(null);

  const sorted = [...participants].sort((a, b) => {
    const priority = (participant: Participant) => {
      if (participant.helpRequestedAt) return 0;
      if (participant.status === "present") return 1;
      if (participant.status === "invited") return 2;
      if (participant.status === "left") return 3;
      return 4;
    };

    return priority(a) - priority(b);
  });

  async function handleRemoveParticipant(userId: string) {
    setRemovingUserId(userId);

    try {
      const res = await fetch(`/api/sessions/${sessionId}/participants/${userId}`, {
        method: "DELETE",
      });

      if (res.ok) {
        await onParticipantsChanged();
      }
    } finally {
      setRemovingUserId(null);
    }
  }

  return (
    <div className="flex h-full flex-col">
      <div className="border-b px-3 py-2">
        <h3 className="text-sm font-medium">Students ({participants.length})</h3>
      </div>
      <div className="flex-1 overflow-auto">
        {sorted.map((participant) => {
          const participantId = participant.userId || participant.studentId;
          const statusColor = {
            invited: "bg-amber-500",
            present: "bg-green-500",
            left: "bg-zinc-400",
          }[participant.status] || "bg-zinc-400";
          const statusBadgeClass = {
            invited: "border-amber-200 bg-amber-50 text-amber-700",
            present: "border-emerald-200 bg-emerald-50 text-emerald-700",
            left: "border-zinc-200 bg-zinc-100 text-zinc-600",
          }[participant.status] || "border-zinc-200 bg-zinc-100 text-zinc-600";
          const dotColor = participant.helpRequestedAt ? "bg-red-500" : statusColor;

          return (
            <div
              key={participantId}
              onClick={() => onSelectStudent(participantId)}
              className={`flex cursor-pointer items-center justify-between border-b border-border/30 px-3 py-2 transition-colors hover:bg-muted/50 ${
                selectedStudentId === participantId ? "bg-primary/10" : ""
              }`}
            >
              <div className="flex min-w-0 items-center gap-2">
                <span className={`h-2 w-2 rounded-full ${dotColor}`} />
                <span className="truncate text-sm">{participant.name}</span>
              </div>
              <div className="flex items-center gap-2">
                <Badge className={statusBadgeClass}>{participant.status}</Badge>
                {participant.invitedBy && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="border-zinc-200 bg-white text-zinc-700 hover:bg-zinc-50 hover:text-zinc-900"
                    disabled={removingUserId === participantId}
                    onClick={(event) => {
                      event.stopPropagation();
                      void handleRemoveParticipant(participantId);
                    }}
                  >
                    Remove
                  </Button>
                )}
                <AiToggleButton sessionId={sessionId} studentId={participantId} />
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

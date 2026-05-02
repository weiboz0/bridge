"use client";

import { StudentTile } from "./student-tile";

interface Participant {
  studentId: string;
  name: string;
  status: string;
  helpRequestedAt?: string | null;
}

interface StudentGridProps {
  sessionId: string;
  participants: Participant[];
  onSelectStudent: (studentId: string) => void;
}

export function StudentGrid({
  sessionId,
  participants,
  onSelectStudent,
}: StudentGridProps) {
  if (participants.length === 0) {
    return (
      <div className="text-center text-muted-foreground py-8">
        No students have joined yet.
      </div>
    );
  }

  return (
    <div className="grid gap-3 grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
      {participants.map((p) => (
        <StudentTile
          key={p.studentId}
          sessionId={sessionId}
          studentId={p.studentId}
          studentName={p.name}
          status={p.status}
          helpRequestedAt={p.helpRequestedAt}
          onClick={() => onSelectStudent(p.studentId)}
        />
      ))}
    </div>
  );
}

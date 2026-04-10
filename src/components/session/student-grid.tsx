"use client";

import { StudentTile } from "./student-tile";

interface Participant {
  studentId: string;
  name: string;
  status: string;
}

interface StudentGridProps {
  sessionId: string;
  participants: Participant[];
  token: string;
  onSelectStudent: (studentId: string) => void;
}

export function StudentGrid({
  sessionId,
  participants,
  token,
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
          token={token}
          onClick={() => onSelectStudent(p.studentId)}
        />
      ))}
    </div>
  );
}

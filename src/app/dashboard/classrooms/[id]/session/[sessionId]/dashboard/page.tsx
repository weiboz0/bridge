"use client";

import { useState, useEffect, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import { useSession } from "next-auth/react";
import { StudentGrid } from "@/components/session/student-grid";
import { CodeEditor } from "@/components/editor/code-editor";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { Button } from "@/components/ui/button";

interface Participant {
  studentId: string;
  name: string;
  status: string;
}

export default function TeacherDashboardPage() {
  const params = useParams<{ id: string; sessionId: string }>();
  const router = useRouter();
  const { data: session } = useSession();
  const [participants, setParticipants] = useState<Participant[]>([]);
  const [selectedStudent, setSelectedStudent] = useState<string | null>(null);
  const [ending, setEnding] = useState(false);

  const userId = session?.user?.id || "";
  const token = `${userId}:teacher`;

  // When a student is selected, connect to their Yjs document
  const selectedDocName = selectedStudent
    ? `session:${params.sessionId}:user:${selectedStudent}`
    : "";

  const { yText, provider, connected } = useYjsProvider({
    documentName: selectedDocName || "noop",
    token,
  });

  // Poll participants
  useEffect(() => {
    async function fetchParticipants() {
      const res = await fetch(`/api/sessions/${params.sessionId}/participants`);
      if (res.ok) {
        const data = await res.json();
        setParticipants(data);
      }
    }
    fetchParticipants();
    const interval = setInterval(fetchParticipants, 3000);
    return () => clearInterval(interval);
  }, [params.sessionId]);

  // SSE events
  useEffect(() => {
    const eventSource = new EventSource(
      `/api/sessions/${params.sessionId}/events`
    );

    eventSource.addEventListener("student_joined", (e) => {
      const data = JSON.parse(e.data);
      setParticipants((prev) => {
        if (prev.some((p) => p.studentId === data.studentId)) return prev;
        return [...prev, { studentId: data.studentId, name: data.name, status: "active" }];
      });
    });

    eventSource.addEventListener("student_left", (e) => {
      const data = JSON.parse(e.data);
      setParticipants((prev) =>
        prev.filter((p) => p.studentId !== data.studentId)
      );
    });

    return () => eventSource.close();
  }, [params.sessionId]);

  const endSession = useCallback(async () => {
    setEnding(true);
    await fetch(`/api/sessions/${params.sessionId}`, { method: "PATCH" });
    router.push(`/dashboard/classrooms/${params.id}`);
  }, [params.sessionId, params.id, router]);

  if (selectedStudent) {
    const student = participants.find((p) => p.studentId === selectedStudent);
    return (
      <div className="flex flex-col h-[calc(100vh-3.5rem)]">
        <div className="flex items-center justify-between px-4 py-2 border-b">
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="sm" onClick={() => setSelectedStudent(null)}>
              Back
            </Button>
            <span className="font-medium">{student?.name || "Student"}</span>
            <span className={`w-2 h-2 rounded-full ${connected ? "bg-green-500" : "bg-red-500"}`} />
          </div>
        </div>
        <div className="flex-1 min-h-0 p-4">
          {selectedDocName && (
            <CodeEditor yText={yText} provider={provider} />
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4 p-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold">Live Dashboard</h1>
          <p className="text-sm text-muted-foreground">
            {participants.length} student{participants.length !== 1 ? "s" : ""} connected
          </p>
        </div>
        <Button
          variant="destructive"
          onClick={endSession}
          disabled={ending}
        >
          {ending ? "Ending..." : "End Session"}
        </Button>
      </div>

      <StudentGrid
        sessionId={params.sessionId}
        participants={participants}
        token={token}
        onSelectStudent={setSelectedStudent}
      />
    </div>
  );
}

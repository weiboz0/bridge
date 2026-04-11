"use client";

import { useState, useEffect, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import { useSession } from "next-auth/react";
import { StudentGrid } from "@/components/session/student-grid";
import { CodeEditor } from "@/components/editor/code-editor";
import { HelpQueuePanel } from "@/components/help-queue/help-queue-panel";
import { AiToggleButton } from "@/components/ai/ai-toggle-button";
import { AiActivityFeed } from "@/components/ai/ai-activity-feed";
import { AnnotationForm } from "@/components/annotations/annotation-form";
import { AnnotationList } from "@/components/annotations/annotation-list";
import { BroadcastControls } from "@/components/session/broadcast-controls";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { Button } from "@/components/ui/button";
import { DiffViewer } from "@/components/editor/diff-viewer";

const STARTER_CODE = `# Welcome to Bridge!
# Write your Python code here and click Run.

print("Hello, world!")
`;

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
  const [annotations, setAnnotations] = useState<any[]>([]);
  const [aiInteractions, setAiInteractions] = useState<any[]>([]);
  const [showDiff, setShowDiff] = useState(false);

  const userId = session?.user?.id || "";
  const token = `${userId}:teacher`;

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
        setParticipants(await res.json());
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

  // Poll AI interactions
  useEffect(() => {
    async function fetchInteractions() {
      const res = await fetch(`/api/ai/interactions?sessionId=${params.sessionId}`);
      if (res.ok) {
        const data = await res.json();
        const summaries = data.map((i: any) => ({
          id: i.id,
          studentId: i.studentId,
          studentName: participants.find((p) => p.studentId === i.studentId)?.name || "Unknown",
          messageCount: Array.isArray(i.messages) ? i.messages.length : 0,
          createdAt: i.createdAt,
        }));
        setAiInteractions(summaries);
      }
    }
    if (participants.length > 0) {
      fetchInteractions();
      const interval = setInterval(fetchInteractions, 5000);
      return () => clearInterval(interval);
    }
  }, [params.sessionId, participants]);

  // Fetch annotations for selected student
  const fetchAnnotations = useCallback(async () => {
    if (!selectedStudent) return;
    const docId = `session:${params.sessionId}:user:${selectedStudent}`;
    const res = await fetch(`/api/annotations?documentId=${encodeURIComponent(docId)}`);
    if (res.ok) {
      setAnnotations(await res.json());
    }
  }, [selectedStudent, params.sessionId]);

  useEffect(() => {
    fetchAnnotations();
  }, [fetchAnnotations]);

  useEffect(() => {
    setShowDiff(false);
  }, [selectedStudent]);

  async function deleteAnnotation(id: string) {
    await fetch(`/api/annotations/${id}`, { method: "DELETE" });
    fetchAnnotations();
  }

  const endSession = useCallback(async () => {
    setEnding(true);
    await fetch(`/api/sessions/${params.sessionId}`, { method: "PATCH" });
    router.push(`/dashboard/classrooms/${params.id}`);
  }, [params.sessionId, params.id, router]);

  // Collaborative editing view
  if (selectedStudent) {
    const student = participants.find((p) => p.studentId === selectedStudent);
    const docId = `session:${params.sessionId}:user:${selectedStudent}`;
    return (
      <div className="flex h-[calc(100vh-3.5rem)]">
        <div className="flex flex-col flex-1">
          <div className="flex items-center justify-between px-4 py-2 border-b">
            <div className="flex items-center gap-2">
              <Button variant="ghost" size="sm" onClick={() => setSelectedStudent(null)}>
                Back
              </Button>
              <span className="font-medium">{student?.name || "Student"}</span>
              <span className={`w-2 h-2 rounded-full ${connected ? "bg-green-500" : "bg-red-500"}`} />
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setShowDiff(!showDiff)}
              >
                {showDiff ? "Editor" : "View Diff"}
              </Button>
              <AiToggleButton sessionId={params.sessionId} studentId={selectedStudent} />
            </div>
          </div>
          <div className="flex-1 min-h-0 p-4">
            {selectedDocName && (
              showDiff ? (
                <DiffViewer
                  original={STARTER_CODE}
                  modified={yText?.toString() || ""}
                  readOnly
                />
              ) : (
                <CodeEditor yText={yText} provider={provider} />
              )
            )}
          </div>
        </div>
        <div className="w-72 border-l p-3 space-y-3 overflow-auto">
          <h3 className="text-sm font-medium">Annotations</h3>
          <AnnotationForm documentId={docId} onCreated={fetchAnnotations} />
          <AnnotationList annotations={annotations} onDelete={deleteAnnotation} />
        </div>
      </div>
    );
  }

  // Main dashboard view
  return (
    <div className="space-y-4 p-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold">Live Dashboard</h1>
          <p className="text-sm text-muted-foreground">
            {participants.length} student{participants.length !== 1 ? "s" : ""} connected
          </p>
        </div>
        <div className="flex gap-2">
          <Button
            variant="destructive"
            onClick={endSession}
            disabled={ending}
          >
            {ending ? "Ending..." : "End Session"}
          </Button>
        </div>
      </div>

      <BroadcastControls sessionId={params.sessionId} token={token} />

      <div className="flex gap-4">
        <div className="flex-1">
          <StudentGrid
            sessionId={params.sessionId}
            participants={participants}
            token={token}
            onSelectStudent={setSelectedStudent}
          />
        </div>
        <div className="w-64 space-y-4">
          <HelpQueuePanel sessionId={params.sessionId} />
          <AiActivityFeed interactions={aiInteractions} />
        </div>
      </div>
    </div>
  );
}

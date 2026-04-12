"use client";

import { useState, useEffect, useCallback } from "react";
import { useRouter } from "next/navigation";
import { useSession } from "next-auth/react";
import { usePanelLayout } from "@/lib/hooks/use-panel-layout";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { TeacherHeader } from "./teacher-header";
import { StudentListPanel } from "./student-list-panel";
import { ModeToolbar, type DashboardMode } from "./mode-toolbar";
import { AiAssistantPanel } from "./ai-assistant-panel";
import { StudentGrid } from "@/components/session/student-grid";
import { CodeEditor } from "@/components/editor/code-editor";
import { LessonRenderer } from "@/components/lesson/lesson-renderer";
import { AnnotationForm } from "@/components/annotations/annotation-form";
import { AnnotationList } from "@/components/annotations/annotation-list";
import { EditorSwitcher } from "@/components/editor/editor-switcher";
import { parseLessonContent } from "@/lib/lesson-content";
import { Button } from "@/components/ui/button";

interface TeacherDashboardProps {
  sessionId: string;
  classId: string;
  classroomId: string;
  editorMode: "python" | "javascript" | "blockly";
  courseTopics: Array<{ topicId: string; title: string; lessonContent: unknown }>;
}

interface Participant {
  studentId: string;
  name: string;
  status: string;
}

export function TeacherDashboard({
  sessionId,
  classId,
  classroomId,
  editorMode,
  courseTopics,
}: TeacherDashboardProps) {
  const router = useRouter();
  const { data: session } = useSession();
  const { layout, toggleLeft, toggleRight } = usePanelLayout();
  const [mode, setMode] = useState<DashboardMode>("grid");
  const [participants, setParticipants] = useState<Participant[]>([]);
  const [selectedStudent, setSelectedStudent] = useState<string | null>(null);
  const [aiInteractions, setAiInteractions] = useState<any[]>([]);
  const [annotations, setAnnotations] = useState<any[]>([]);

  const userId = session?.user?.id || "";
  const token = `${userId}:teacher`;

  // Selected student Yjs document
  const selectedDocName = selectedStudent
    ? `session:${sessionId}:user:${selectedStudent}`
    : "";
  const { yText, provider, connected } = useYjsProvider({
    documentName: selectedDocName || "noop",
    token,
  });

  // Poll participants
  useEffect(() => {
    async function fetchParticipants() {
      const res = await fetch(`/api/sessions/${sessionId}/participants`);
      if (res.ok) setParticipants(await res.json());
    }
    fetchParticipants();
    const interval = setInterval(fetchParticipants, 3000);
    return () => clearInterval(interval);
  }, [sessionId]);

  // SSE events
  useEffect(() => {
    const eventSource = new EventSource(`/api/sessions/${sessionId}/events`);
    eventSource.addEventListener("student_joined", (e) => {
      const data = JSON.parse(e.data);
      setParticipants((prev) => {
        if (prev.some((p) => p.studentId === data.studentId)) return prev;
        return [...prev, { studentId: data.studentId, name: data.name, status: "active" }];
      });
    });
    eventSource.addEventListener("student_left", (e) => {
      const data = JSON.parse(e.data);
      setParticipants((prev) => prev.filter((p) => p.studentId !== data.studentId));
    });
    return () => eventSource.close();
  }, [sessionId]);

  // Poll AI interactions
  useEffect(() => {
    async function fetchAI() {
      const res = await fetch(`/api/ai/interactions?sessionId=${sessionId}`);
      if (res.ok) {
        const data = await res.json();
        setAiInteractions(data.map((i: any) => ({
          id: i.id,
          studentId: i.studentId,
          studentName: participants.find((p) => p.studentId === i.studentId)?.name || "Unknown",
          messageCount: Array.isArray(i.messages) ? i.messages.length : 0,
          createdAt: i.createdAt,
        })));
      }
    }
    if (participants.length > 0) {
      fetchAI();
      const interval = setInterval(fetchAI, 5000);
      return () => clearInterval(interval);
    }
  }, [sessionId, participants]);

  // Fetch annotations for selected student
  const fetchAnnotations = useCallback(async () => {
    if (!selectedStudent) return;
    const docId = `session:${sessionId}:user:${selectedStudent}`;
    const res = await fetch(`/api/annotations?documentId=${encodeURIComponent(docId)}`);
    if (res.ok) setAnnotations(await res.json());
  }, [selectedStudent, sessionId]);

  useEffect(() => { fetchAnnotations(); }, [fetchAnnotations]);

  async function deleteAnnotation(id: string) {
    await fetch(`/api/annotations/${id}`, { method: "DELETE" });
    fetchAnnotations();
  }

  const endSession = useCallback(async () => {
    await fetch(`/api/sessions/${sessionId}`, { method: "PATCH" });
    router.push(`/teacher/classes/${classId}`);
  }, [sessionId, classId, router]);

  // When student is selected, switch to collaborate mode
  function handleSelectStudent(id: string) {
    setSelectedStudent(id);
    setMode("collaborate");
  }

  // Render main area based on mode
  function renderMainArea() {
    switch (mode) {
      case "presentation": {
        const topicContents = courseTopics.map((t) => parseLessonContent(t.lessonContent));
        return (
          <div className="p-4 overflow-auto h-full space-y-6">
            {courseTopics.map((t, i) => (
              <div key={t.topicId}>
                <h3 className="text-lg font-semibold mb-2">{t.title}</h3>
                <LessonRenderer content={topicContents[i]} />
              </div>
            ))}
            {courseTopics.length === 0 && (
              <p className="text-muted-foreground text-center py-8">
                No topics linked to this session yet.
              </p>
            )}
          </div>
        );
      }

      case "grid":
        return (
          <div className="p-4 overflow-auto h-full">
            <StudentGrid
              sessionId={sessionId}
              participants={participants}
              token={token}
              onSelectStudent={handleSelectStudent}
            />
          </div>
        );

      case "collaborate": {
        if (!selectedStudent) {
          return (
            <div className="flex items-center justify-center h-full text-muted-foreground">
              <p>Select a student from the list to collaborate.</p>
            </div>
          );
        }
        const student = participants.find((p) => p.studentId === selectedStudent);
        const docId = `session:${sessionId}:user:${selectedStudent}`;
        return (
          <div className="flex h-full">
            <div className="flex-1 flex flex-col">
              <div className="flex items-center gap-2 px-4 py-2 border-b">
                <Button variant="ghost" size="sm" onClick={() => { setSelectedStudent(null); setMode("grid"); }}>
                  ← Back
                </Button>
                <span className="font-medium">{student?.name || "Student"}</span>
                <span className={`w-2 h-2 rounded-full ${connected ? "bg-green-500" : "bg-red-500"}`} />
              </div>
              <div className="flex-1 min-h-0 p-4">
                <CodeEditor yText={yText} provider={provider} language={editorMode} />
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

      case "broadcast":
        return (
          <div className="p-4 h-full">
            <EditorSwitcher editorMode={editorMode} initialCode="# Broadcast your code to all students" />
          </div>
        );
    }
  }

  return (
    <div className="flex flex-col h-screen">
      <TeacherHeader
        sessionId={sessionId}
        studentCount={participants.length}
        onEndSession={endSession}
        onToggleLeft={toggleLeft}
        onToggleRight={toggleRight}
        leftVisible={layout.leftVisible}
        rightVisible={layout.rightVisible}
      />

      <div className="flex flex-1 min-h-0">
        {/* Left: Student List */}
        {layout.leftVisible && (
          <div className="border-r" style={{ width: `${layout.leftWidth}%` }}>
            <StudentListPanel
              sessionId={sessionId}
              participants={participants}
              selectedStudentId={selectedStudent}
              onSelectStudent={handleSelectStudent}
            />
          </div>
        )}

        {/* Center: Main Area */}
        <div className="flex-1 flex flex-col min-h-0">
          <div className="flex-1 min-h-0">
            {renderMainArea()}
          </div>
          <ModeToolbar activeMode={mode} onModeChange={setMode} />
        </div>

        {/* Right: AI Assistant */}
        {layout.rightVisible && (
          <div className="border-l" style={{ width: `${layout.rightWidth}%` }}>
            <AiAssistantPanel sessionId={sessionId} aiInteractions={aiInteractions} />
          </div>
        )}
      </div>
    </div>
  );
}

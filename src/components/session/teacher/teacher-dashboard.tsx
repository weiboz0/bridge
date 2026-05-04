"use client";

import Link from "next/link";
import { useState, useEffect, useCallback, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { useSession } from "next-auth/react";
import { usePanelLayout } from "@/lib/hooks/use-panel-layout";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { useRealtimeToken } from "@/lib/realtime/use-realtime-token";
import { RealtimeConfigBanner } from "@/components/realtime/realtime-config-banner";
import { TeacherHeader } from "./teacher-header";
import { StudentListPanel } from "./student-list-panel";
import { ModeToolbar, type DashboardMode } from "./mode-toolbar";
import { AiAssistantPanel } from "./ai-assistant-panel";
import { StudentGrid } from "@/components/session/student-grid";
import { CodeEditor } from "@/components/editor/code-editor";
import { AnnotationForm } from "@/components/annotations/annotation-form";
import { AnnotationList } from "@/components/annotations/annotation-list";
import { EditorSwitcher } from "@/components/editor/editor-switcher";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

interface TeacherDashboardProps {
  sessionId: string;
  classId: string | null;
  returnPath?: string;
  editorMode: "python" | "javascript" | "blockly";
  // Plan 044 phase 2: presentation mode renders the linked
  // teaching_unit per topic.
  courseTopics: Array<{
    topicId: string;
    title: string;
    unitId: string | null;
    unitTitle: string | null;
    unitMaterialType: string | null;
  }>;
  inviteToken?: string | null;
  inviteExpiresAt?: string | null;
}

interface Participant {
  userId: string;
  studentId: string;
  name: string;
  status: string;
  invitedBy?: string | null;
  helpRequestedAt?: string | null;
}

interface ParticipantResponse {
  userId?: string;
  studentId?: string;
  name?: string | null;
  email?: string | null;
  status?: string;
  invitedBy?: string | null;
  helpRequestedAt?: string | null;
}

interface AiInteractionResponse {
  id: string;
  studentId: string;
  messages?: unknown[];
  createdAt: string;
}

interface AiInteractionSummary {
  id: string;
  studentId: string;
  studentName: string;
  messageCount: number;
  createdAt: string;
}

interface AnnotationItem {
  id: string;
  lineStart: string;
  lineEnd: string;
  content: string;
  authorType: "teacher" | "ai";
  createdAt: string;
}

const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function normalizeParticipantStatus(status?: string) {
  if (status === "present" || status === "invited" || status === "left") {
    return status;
  }
  if (status === "active" || status === "needs_help") {
    return "present";
  }
  if (status === "idle") {
    return "invited";
  }
  return "present";
}

function normalizeParticipant(participant: ParticipantResponse): Participant | null {
  const userId = participant.userId ?? participant.studentId;
  if (!userId) {
    return null;
  }

  return {
    userId,
    studentId: userId,
    name: participant.name?.trim() || participant.email?.trim() || userId,
    status: normalizeParticipantStatus(participant.status),
    invitedBy: participant.invitedBy ?? null,
    helpRequestedAt: participant.helpRequestedAt ?? null,
  };
}

export function TeacherDashboard({
  sessionId,
  classId,
  returnPath,
  editorMode,
  courseTopics,
  inviteToken,
  inviteExpiresAt,
}: TeacherDashboardProps) {
  const router = useRouter();
  const { data: session } = useSession();
  const { layout, toggleLeft, toggleRight } = usePanelLayout();
  const [mode, setMode] = useState<DashboardMode>("grid");
  const [participants, setParticipants] = useState<Participant[]>([]);
  const [selectedStudent, setSelectedStudent] = useState<string | null>(null);
  const [aiInteractions, setAiInteractions] = useState<AiInteractionSummary[]>([]);
  const [annotations, setAnnotations] = useState<AnnotationItem[]>([]);
  const [participantLookup, setParticipantLookup] = useState("");
  const [participantLookupError, setParticipantLookupError] = useState<string | null>(null);
  const [isAddingParticipant, setIsAddingParticipant] = useState(false);

  const userId = session?.user?.id || "";

  const selectedDocName = selectedStudent
    ? `session:${sessionId}:user:${selectedStudent}`
    : "noop";
  // Plan 053 phase 3 — mint a JWT for the selected student's doc.
  // The cache in `getRealtimeToken` keeps this to one mint per
  // (doc, tab); switching students re-fetches.
  const { token: realtimeToken, unavailable: selectedUnavailable } = useRealtimeToken(selectedDocName);

  // Plan 068 phase 4 — sentinel probe. Codex pass-1 caught that
  // selectedDocName is "noop" until a teacher clicks a student tile,
  // which means the realtime banner would never fire on the initial
  // dashboard load even if HOCUSPOCUS_TOKEN_SECRET is unset. The
  // 503 path in realtime_token.go fires BEFORE any document-level
  // checks, so any non-noop doc-name reveals the misconfiguration.
  // Probe the teacher's own session doc — always real, always
  // mountable, doubles as a cache pre-warm for if the teacher later
  // wants to view their own typing.
  const sentinelDocName = userId ? `session:${sessionId}:user:${userId}` : "noop";
  const { unavailable: sentinelUnavailable } = useRealtimeToken(sentinelDocName);
  const realtimeUnavailable = selectedUnavailable || sentinelUnavailable;
  const { yText, provider, connected } = useYjsProvider({
    documentName: selectedDocName,
    token: realtimeToken,
  });

  const fetchParticipants = useCallback(async () => {
    const res = await fetch(`/api/sessions/${sessionId}/participants`);
    if (!res.ok) {
      return [];
    }

    const data = await res.json();
    const nextParticipants = Array.isArray(data)
      ? data
          .map((participant) => normalizeParticipant(participant as ParticipantResponse))
          .filter((participant): participant is Participant => participant !== null)
      : [];

    setParticipants(nextParticipants);
    return nextParticipants;
  }, [sessionId]);

  useEffect(() => {
    void fetchParticipants();
    const interval = setInterval(fetchParticipants, 3000);
    return () => clearInterval(interval);
  }, [fetchParticipants]);

  useEffect(() => {
    const eventSource = new EventSource(`/api/sessions/${sessionId}/events`);
    const refreshParticipants = () => {
      void fetchParticipants();
    };

    eventSource.addEventListener("student_joined", refreshParticipants);
    eventSource.addEventListener("student_left", refreshParticipants);

    return () => eventSource.close();
  }, [fetchParticipants, sessionId]);

  useEffect(() => {
    if (selectedStudent && !participants.some((participant) => participant.userId === selectedStudent)) {
      setSelectedStudent(null);
      setMode((currentMode) => (currentMode === "collaborate" ? "grid" : currentMode));
    }
  }, [participants, selectedStudent]);

  useEffect(() => {
    async function fetchAI() {
      const res = await fetch(`/api/ai/interactions?sessionId=${sessionId}`);
      if (res.ok) {
        const data = (await res.json()) as AiInteractionResponse[];
        setAiInteractions(data.map((interaction) => ({
          id: interaction.id,
          studentId: interaction.studentId,
          studentName:
            participants.find((participant) => participant.userId === interaction.studentId)?.name ||
            "Unknown",
          messageCount: Array.isArray(interaction.messages) ? interaction.messages.length : 0,
          createdAt: interaction.createdAt,
        })));
      }
    }

    if (participants.length > 0) {
      void fetchAI();
      const interval = setInterval(fetchAI, 5000);
      return () => clearInterval(interval);
    }
  }, [sessionId, participants]);

  const fetchAnnotations = useCallback(async () => {
    if (!selectedStudent) return;

    const docId = `session:${sessionId}:user:${selectedStudent}`;
    const res = await fetch(`/api/annotations?documentId=${encodeURIComponent(docId)}`);
    if (res.ok) {
      setAnnotations((await res.json()) as AnnotationItem[]);
    }
  }, [selectedStudent, sessionId]);

  useEffect(() => {
    void fetchAnnotations();
  }, [fetchAnnotations]);

  async function deleteAnnotation(id: string) {
    await fetch(`/api/annotations/${id}`, { method: "DELETE" });
    void fetchAnnotations();
  }

  const endSession = useCallback(async () => {
    await fetch(`/api/sessions/${sessionId}/end`, { method: "POST" });
    router.push(returnPath ?? (classId ? `/teacher/classes/${classId}` : "/teacher"));
  }, [sessionId, classId, returnPath, router]);

  function handleSelectStudent(id: string) {
    setSelectedStudent(id);
    setMode("collaborate");
  }

  async function handleAddParticipant(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const lookupValue = participantLookup.trim();
    if (!lookupValue) {
      return;
    }

    setIsAddingParticipant(true);
    setParticipantLookupError(null);

    try {
      const res = await fetch(`/api/sessions/${sessionId}/participants`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(
          UUID_PATTERN.test(lookupValue)
            ? { userId: lookupValue }
            : { email: lookupValue }
        ),
      });

      if (res.ok) {
        setParticipantLookup("");
        await fetchParticipants();
        return;
      }

      if (res.status === 404) {
        setParticipantLookupError("User not found");
        return;
      }

      setParticipantLookupError("Unable to add participant");
    } finally {
      setIsAddingParticipant(false);
    }
  }

  function renderMainArea() {
    switch (mode) {
      case "presentation": {
        // Plan 044 phase 2: render linked Unit per topic (1:1). Per Codex
        // correction #7, each topic without a Unit shows its own empty
        // state, not just a single all-topics-empty banner.
        return (
          <div className="h-full space-y-6 overflow-auto p-4">
            {courseTopics.map((topic) => (
              <div key={topic.topicId}>
                <h3 className="mb-2 text-lg font-semibold">{topic.title}</h3>
                {topic.unitId ? (
                  <Link
                    href={`/teacher/units/${topic.unitId}/edit`}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-2 rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm hover:border-amber-400 hover:text-amber-800"
                  >
                    <span className="font-medium">{topic.unitTitle}</span>
                    {topic.unitMaterialType && (
                      <span className="font-mono text-[10px] uppercase tracking-[0.16em] text-muted-foreground">
                        {topic.unitMaterialType}
                      </span>
                    )}
                  </Link>
                ) : (
                  <p className="text-sm text-muted-foreground italic">
                    No teaching unit linked.
                  </p>
                )}
              </div>
            ))}
            {courseTopics.length === 0 && (
              <p className="py-8 text-center text-muted-foreground">
                No focus areas linked to this session yet.
              </p>
            )}
          </div>
        );
      }

      case "grid":
        return (
          <div className="h-full overflow-auto p-4">
            <StudentGrid
              sessionId={sessionId}
              participants={participants}
              onSelectStudent={handleSelectStudent}
            />
          </div>
        );

      case "collaborate": {
        if (!selectedStudent) {
          return (
            <div className="flex h-full items-center justify-center text-muted-foreground">
              <p>Select a student from the list to collaborate.</p>
            </div>
          );
        }

        const student = participants.find((participant) => participant.userId === selectedStudent);
        const docId = `session:${sessionId}:user:${selectedStudent}`;

        return (
          <div className="flex h-full">
            <div className="flex flex-1 flex-col">
              <div className="flex items-center gap-2 border-b px-4 py-2">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    setSelectedStudent(null);
                    setMode("grid");
                  }}
                >
                  ← Back
                </Button>
                <span className="font-medium">{student?.name || "Student"}</span>
                <span className={`h-2 w-2 rounded-full ${connected ? "bg-green-500" : "bg-red-500"}`} />
              </div>
              <div className="min-h-0 flex-1 p-4">
                <CodeEditor yText={yText} provider={provider} language={editorMode} />
              </div>
            </div>
            <div className="w-72 space-y-3 overflow-auto border-l p-3">
              <h3 className="text-sm font-medium">Annotations</h3>
              <AnnotationForm documentId={docId} onCreated={fetchAnnotations} />
              <AnnotationList annotations={annotations} onDelete={deleteAnnotation} />
            </div>
          </div>
        );
      }

      case "broadcast":
        return (
          <div className="h-full p-4">
            <EditorSwitcher
              editorMode={editorMode}
              initialCode="# Broadcast your code to all students"
            />
          </div>
        );
    }
  }

  return (
    <>
      <RealtimeConfigBanner unavailable={realtimeUnavailable} />
    <div className="flex h-screen flex-col" data-testid="teacher-dashboard">
      <TeacherHeader
        sessionId={sessionId}
        studentCount={participants.length}
        inviteToken={inviteToken}
        inviteExpiresAt={inviteExpiresAt}
        onEndSession={endSession}
        onToggleLeft={toggleLeft}
        onToggleRight={toggleRight}
        leftVisible={layout.leftVisible}
        rightVisible={layout.rightVisible}
      />

      <div className="flex min-h-0 flex-1">
        {layout.leftVisible && (
          <div
            className="flex min-h-0 flex-col border-r"
            style={{ width: `${layout.leftWidth}%` }}
          >
            <div className="min-h-0 flex-1">
              <StudentListPanel
                sessionId={sessionId}
                participants={participants}
                selectedStudentId={selectedStudent}
                onSelectStudent={handleSelectStudent}
                onParticipantsChanged={fetchParticipants}
              />
            </div>
            <div className="border-t border-zinc-200 bg-zinc-50/40 p-3">
              <form className="space-y-2" onSubmit={handleAddParticipant}>
                <div className="flex items-center gap-2">
                  <Input
                    value={participantLookup}
                    onChange={(event) => {
                      setParticipantLookup(event.target.value);
                      if (participantLookupError) {
                        setParticipantLookupError(null);
                      }
                    }}
                    placeholder="Email or user ID"
                    className="border-zinc-200 bg-white text-zinc-900 placeholder:text-zinc-400"
                  />
                  <Button
                    type="submit"
                    variant="outline"
                    className="border-zinc-200 bg-white text-zinc-700 hover:bg-zinc-50 hover:text-zinc-900"
                    disabled={isAddingParticipant || participantLookup.trim().length === 0}
                  >
                    Add
                  </Button>
                </div>
                {participantLookupError && (
                  <p className="text-xs text-red-600">{participantLookupError}</p>
                )}
              </form>
            </div>
          </div>
        )}

        <div className="flex min-h-0 flex-1 flex-col">
          <div className="min-h-0 flex-1">{renderMainArea()}</div>
          <ModeToolbar activeMode={mode} onModeChange={setMode} />
        </div>

        {layout.rightVisible && (
          <div className="border-l" style={{ width: `${layout.rightWidth}%` }}>
            <AiAssistantPanel sessionId={sessionId} aiInteractions={aiInteractions} />
          </div>
        )}
      </div>
    </div>
    </>
  );
}

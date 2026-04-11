# MVP Gaps Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the 4 critical MVP gaps: AI toggle SSE integration, annotation UI for CodeMirror, teacher AI activity feed, and teacher broadcast mode.

**Architecture:** All backend APIs exist — this is purely frontend wiring. AI toggle uses the existing SSE event bus to notify students in real-time. Annotations render as CodeMirror gutter markers with click-to-view popovers. AI activity feed polls the existing interactions API. Broadcast mode reuses the Yjs infrastructure with a broadcast document.

**Tech Stack:** React, CodeMirror 6 (decorations/gutters), SSE (EventSource), Yjs

**Depends on:** All 4 prior plans (Foundation, Live Editor, Real-time Sessions, AI & Interaction)

---

## File Structure

```
src/
├── components/
│   ├── ai/
│   │   └── ai-activity-feed.tsx                    # Teacher view of all AI conversations
│   ├── annotations/
│   │   ├── annotation-form.tsx                     # Inline form for creating annotations
│   │   └── annotation-list.tsx                     # List of annotations for a document
│   └── session/
│       └── broadcast-controls.tsx                  # Teacher broadcast start/stop + student banner
├── app/
│   └── dashboard/
│       └── classrooms/
│           └── [id]/
│               └── session/
│                   └── [sessionId]/
│                       ├── page.tsx                # Modify: add SSE listener for ai_toggled, annotations
│                       └── dashboard/
│                           └── page.tsx            # Modify: add AI activity feed, AI toggle on tiles, broadcast
tests/
└── unit/
    ├── ai-activity-feed.test.tsx                   # Activity feed rendering tests
    └── annotation-list.test.tsx                    # Annotation list rendering tests
```

---

## Task 1: AI Toggle SSE Integration

**Files:**
- Modify: `src/app/dashboard/classrooms/[id]/session/[sessionId]/page.tsx`

The student session page already has an SSE connection placeholder (line 32-34). Wire it up so when the teacher toggles AI, the student gets notified in real-time.

- [ ] **Step 1: Add SSE listener for ai_toggled events**

In `src/app/dashboard/classrooms/[id]/session/[sessionId]/page.tsx`, add a `useEffect` that listens for `ai_toggled` events via SSE and updates the `aiEnabled` state. Add this after the `useYjsProvider` hook:

```tsx
  // Listen for AI toggle events via SSE
  useEffect(() => {
    const eventSource = new EventSource(
      `/api/sessions/${params.sessionId}/events`
    );

    eventSource.addEventListener("ai_toggled", (e) => {
      const data = JSON.parse(e.data);
      if (data.studentId === userId) {
        setAiEnabled(data.enabled);
        if (data.enabled && !showAi) {
          setShowAi(true);
        }
      }
    });

    eventSource.addEventListener("session_ended", () => {
      // Redirect back to classroom when session ends
      window.location.href = `/dashboard/classrooms/${params.id}`;
    });

    return () => eventSource.close();
  }, [params.sessionId, params.id, userId, showAi]);
```

Also remove the old placeholder comment (lines 32-34) and the unused `aiEnabled` state initialization on line 19 (change to `false` — it's already `false`, just remove the comment above it).

Update the "Ask AI" button to only show when `aiEnabled` is true:

Replace:
```tsx
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setShowAi(!showAi)}
            >
              {showAi ? "Hide AI" : "Ask AI"}
            </Button>
```

With:
```tsx
            {aiEnabled && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setShowAi(!showAi)}
              >
                {showAi ? "Hide AI" : "Ask AI"}
              </Button>
            )}
```

- [ ] **Step 2: Verify build**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun run build
```

Expected: Build passes.

- [ ] **Step 3: Commit**

```bash
git add src/app/dashboard/classrooms/\[id\]/session/\[sessionId\]/page.tsx
git commit -m "feat: wire AI toggle SSE to student session page

Student receives real-time notification when teacher enables/disables
AI. Chat panel auto-opens when AI is enabled. Session end redirects
student back to classroom."
```

---

## Task 2: AI Toggle Button on Student Tiles

**Files:**
- Modify: `src/components/session/student-tile.tsx`
- Modify: `src/components/session/student-grid.tsx`
- Modify: `src/app/dashboard/classrooms/[id]/session/[sessionId]/dashboard/page.tsx`

The `AiToggleButton` component exists but isn't rendered on the teacher dashboard. Add it to each student tile.

- [ ] **Step 1: Add AI toggle to StudentTile**

In `src/components/session/student-tile.tsx`, add the `AiToggleButton` import and render it in the tile header. Add to imports:

```tsx
import { AiToggleButton } from "@/components/ai/ai-toggle-button";
```

Add `sessionId` is already a prop. Add the button in the header div, after the status indicator:

Replace the header div:
```tsx
      <div className="flex items-center justify-between mb-1">
        <span className="text-xs font-medium truncate">{studentName}</span>
        <span className={`w-2 h-2 rounded-full ${statusColor}`} />
      </div>
```

With:
```tsx
      <div className="flex items-center justify-between mb-1">
        <span className="text-xs font-medium truncate">{studentName}</span>
        <div className="flex items-center gap-1">
          <AiToggleButton sessionId={sessionId} studentId={studentId} />
          <span className={`w-2 h-2 rounded-full ${statusColor}`} />
        </div>
      </div>
```

- [ ] **Step 2: Verify build**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun run build
```

- [ ] **Step 3: Commit**

```bash
git add src/components/session/student-tile.tsx
git commit -m "feat: add AI toggle button to student tiles on teacher dashboard"
```

---

## Task 3: AI Activity Feed

**Files:**
- Create: `src/components/ai/ai-activity-feed.tsx`
- Test: `tests/unit/ai-activity-feed.test.tsx`
- Modify: `src/app/dashboard/classrooms/[id]/session/[sessionId]/dashboard/page.tsx`

- [ ] **Step 1: Write failing test**

Create `tests/unit/ai-activity-feed.test.tsx`:

```tsx
// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { AiActivityFeed } from "@/components/ai/ai-activity-feed";

describe("AiActivityFeed", () => {
  it("shows empty state when no interactions", () => {
    render(<AiActivityFeed interactions={[]} />);
    expect(screen.getByText("No AI interactions yet.")).toBeInTheDocument();
  });

  it("renders interaction entries", () => {
    const interactions = [
      {
        id: "1",
        studentId: "s1",
        studentName: "Alice",
        messageCount: 3,
        createdAt: "2026-04-10T12:00:00Z",
      },
      {
        id: "2",
        studentId: "s2",
        studentName: "Bob",
        messageCount: 1,
        createdAt: "2026-04-10T12:05:00Z",
      },
    ];
    render(<AiActivityFeed interactions={interactions} />);
    expect(screen.getByText("Alice")).toBeInTheDocument();
    expect(screen.getByText("Bob")).toBeInTheDocument();
    expect(screen.getByText("3 messages")).toBeInTheDocument();
    expect(screen.getByText("1 message")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify failure**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/ai-activity-feed.test.tsx
```

Expected: FAIL — cannot resolve `@/components/ai/ai-activity-feed`.

- [ ] **Step 3: Implement AiActivityFeed**

Create `src/components/ai/ai-activity-feed.tsx`:

```tsx
"use client";

interface InteractionSummary {
  id: string;
  studentId: string;
  studentName: string;
  messageCount: number;
  createdAt: string;
}

interface AiActivityFeedProps {
  interactions: InteractionSummary[];
}

export function AiActivityFeed({ interactions }: AiActivityFeedProps) {
  if (interactions.length === 0) {
    return (
      <p className="text-sm text-muted-foreground text-center py-4">
        No AI interactions yet.
      </p>
    );
  }

  return (
    <div className="space-y-2">
      <h3 className="text-sm font-medium">AI Activity</h3>
      {interactions.map((interaction) => (
        <div
          key={interaction.id}
          className="flex items-center justify-between p-2 border rounded-lg text-sm"
        >
          <div>
            <span className="font-medium">{interaction.studentName}</span>
            <span className="text-muted-foreground ml-2">
              {interaction.messageCount} message{interaction.messageCount !== 1 ? "s" : ""}
            </span>
          </div>
          <span className="text-xs text-muted-foreground">
            {new Date(interaction.createdAt).toLocaleTimeString()}
          </span>
        </div>
      ))}
    </div>
  );
}
```

- [ ] **Step 4: Run test**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/ai-activity-feed.test.tsx
```

Expected: All tests pass.

- [ ] **Step 5: Wire into teacher dashboard**

In `src/app/dashboard/classrooms/[id]/session/[sessionId]/dashboard/page.tsx`, add the activity feed to the sidebar. Add import:

```tsx
import { AiActivityFeed } from "@/components/ai/ai-activity-feed";
```

Add state and polling effect after the participants polling effect:

```tsx
  const [aiInteractions, setAiInteractions] = useState<any[]>([]);

  useEffect(() => {
    async function fetchInteractions() {
      const res = await fetch(`/api/ai/interactions?sessionId=${params.sessionId}`);
      if (res.ok) {
        const data = await res.json();
        const summaries = data.map((i: any) => ({
          id: i.id,
          studentId: i.studentId,
          studentName: participants.find((p: any) => p.studentId === i.studentId)?.name || "Unknown",
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
```

Add the feed below the HelpQueuePanel in the sidebar:

```tsx
        <div className="w-64 space-y-4">
          <HelpQueuePanel sessionId={params.sessionId} />
          <AiActivityFeed interactions={aiInteractions} />
        </div>
```

- [ ] **Step 6: Commit**

```bash
git add src/components/ai/ai-activity-feed.tsx tests/unit/ai-activity-feed.test.tsx src/app/dashboard/classrooms/\[id\]/session/\[sessionId\]/dashboard/page.tsx
git commit -m "feat: add AI activity feed to teacher dashboard

Shows all AI interactions in the session with student name and
message count. Polls every 5 seconds for updates."
```

---

## Task 4: Annotation UI

**Files:**
- Create: `src/components/annotations/annotation-form.tsx`
- Create: `src/components/annotations/annotation-list.tsx`
- Test: `tests/unit/annotation-list.test.tsx`
- Modify: `src/app/dashboard/classrooms/[id]/session/[sessionId]/dashboard/page.tsx` (collaborative view)

- [ ] **Step 1: Write failing test for AnnotationList**

Create `tests/unit/annotation-list.test.tsx`:

```tsx
// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { AnnotationList } from "@/components/annotations/annotation-list";

describe("AnnotationList", () => {
  it("shows empty state when no annotations", () => {
    render(<AnnotationList annotations={[]} onDelete={() => {}} />);
    expect(screen.getByText("No annotations yet.")).toBeInTheDocument();
  });

  it("renders annotations with line numbers and content", () => {
    const annotations = [
      {
        id: "1",
        lineStart: "5",
        lineEnd: "5",
        content: "Good use of a loop!",
        authorType: "teacher" as const,
        createdAt: "2026-04-10T12:00:00Z",
      },
      {
        id: "2",
        lineStart: "10",
        lineEnd: "12",
        content: "Consider using a function here",
        authorType: "ai" as const,
        createdAt: "2026-04-10T12:05:00Z",
      },
    ];
    render(<AnnotationList annotations={annotations} onDelete={() => {}} />);
    expect(screen.getByText("Line 5")).toBeInTheDocument();
    expect(screen.getByText("Good use of a loop!")).toBeInTheDocument();
    expect(screen.getByText("Lines 10-12")).toBeInTheDocument();
    expect(screen.getByText("Consider using a function here")).toBeInTheDocument();
  });

  it("shows author type badge", () => {
    const annotations = [
      {
        id: "1",
        lineStart: "1",
        lineEnd: "1",
        content: "test",
        authorType: "teacher" as const,
        createdAt: "2026-04-10T12:00:00Z",
      },
    ];
    render(<AnnotationList annotations={annotations} onDelete={() => {}} />);
    expect(screen.getByText("Teacher")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify failure**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/annotation-list.test.tsx
```

- [ ] **Step 3: Create AnnotationList component**

Create `src/components/annotations/annotation-list.tsx`:

```tsx
"use client";

import { Button } from "@/components/ui/button";

interface Annotation {
  id: string;
  lineStart: string;
  lineEnd: string;
  content: string;
  authorType: "teacher" | "ai";
  createdAt: string;
}

interface AnnotationListProps {
  annotations: Annotation[];
  onDelete: (id: string) => void;
}

export function AnnotationList({ annotations, onDelete }: AnnotationListProps) {
  if (annotations.length === 0) {
    return (
      <p className="text-sm text-muted-foreground text-center py-4">
        No annotations yet.
      </p>
    );
  }

  return (
    <div className="space-y-2">
      {annotations.map((annotation) => (
        <div key={annotation.id} className="border rounded-lg p-2 text-sm space-y-1">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <span className="font-mono text-xs text-muted-foreground">
                {annotation.lineStart === annotation.lineEnd
                  ? `Line ${annotation.lineStart}`
                  : `Lines ${annotation.lineStart}-${annotation.lineEnd}`}
              </span>
              <span className={`text-xs px-1.5 py-0.5 rounded ${
                annotation.authorType === "teacher"
                  ? "bg-blue-100 text-blue-700"
                  : "bg-purple-100 text-purple-700"
              }`}>
                {annotation.authorType === "teacher" ? "Teacher" : "AI"}
              </span>
            </div>
            <Button
              variant="ghost"
              size="sm"
              className="h-6 w-6 p-0 text-muted-foreground"
              onClick={() => onDelete(annotation.id)}
            >
              ×
            </Button>
          </div>
          <p className="whitespace-pre-wrap">{annotation.content}</p>
        </div>
      ))}
    </div>
  );
}
```

- [ ] **Step 4: Run test**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/annotation-list.test.tsx
```

Expected: All tests pass.

- [ ] **Step 5: Create AnnotationForm component**

Create `src/components/annotations/annotation-form.tsx`:

```tsx
"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

interface AnnotationFormProps {
  documentId: string;
  onCreated: () => void;
}

export function AnnotationForm({ documentId, onCreated }: AnnotationFormProps) {
  const [lineStart, setLineStart] = useState("");
  const [lineEnd, setLineEnd] = useState("");
  const [content, setContent] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!lineStart || !content) return;

    setLoading(true);
    const res = await fetch("/api/annotations", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        documentId,
        lineStart,
        lineEnd: lineEnd || lineStart,
        content,
      }),
    });

    if (res.ok) {
      setLineStart("");
      setLineEnd("");
      setContent("");
      onCreated();
    }
    setLoading(false);
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-2 border rounded-lg p-2">
      <div className="flex gap-2">
        <Input
          placeholder="Line"
          value={lineStart}
          onChange={(e) => setLineStart(e.target.value)}
          className="w-16 text-sm"
          required
        />
        <span className="text-muted-foreground self-center">-</span>
        <Input
          placeholder="End"
          value={lineEnd}
          onChange={(e) => setLineEnd(e.target.value)}
          className="w-16 text-sm"
        />
      </div>
      <Input
        placeholder="Add a comment..."
        value={content}
        onChange={(e) => setContent(e.target.value)}
        className="text-sm"
        required
      />
      <Button type="submit" size="sm" disabled={loading} className="w-full">
        {loading ? "Adding..." : "Add Annotation"}
      </Button>
    </form>
  );
}
```

- [ ] **Step 6: Add annotation panel to teacher's collaborative view**

In the teacher dashboard's collaborative editing view (when `selectedStudent` is set), add an annotation sidebar. This section of `src/app/dashboard/classrooms/[id]/session/[sessionId]/dashboard/page.tsx` currently shows just the editor. Modify the `selectedStudent` branch to include annotations.

Add imports:

```tsx
import { AnnotationForm } from "@/components/annotations/annotation-form";
import { AnnotationList } from "@/components/annotations/annotation-list";
```

Add state for annotations after the existing state declarations:

```tsx
  const [annotations, setAnnotations] = useState<any[]>([]);
```

Add a fetch function and effect (inside the component, after the SSE effect):

```tsx
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

  async function deleteAnnotation(id: string) {
    await fetch(`/api/annotations/${id}`, { method: "DELETE" });
    fetchAnnotations();
  }
```

Replace the collaborative editing return block (the `if (selectedStudent)` branch) with:

```tsx
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
            <AiToggleButton sessionId={params.sessionId} studentId={selectedStudent} />
          </div>
          <div className="flex-1 min-h-0 p-4">
            {selectedDocName && (
              <CodeEditor yText={yText} provider={provider} />
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
```

- [ ] **Step 7: Commit**

```bash
git add src/components/annotations/ tests/unit/annotation-list.test.tsx src/app/dashboard/classrooms/\[id\]/session/\[sessionId\]/dashboard/page.tsx
git commit -m "feat: add annotation UI for teacher collaborative editing

Annotation form to comment on specific lines, annotation list with
teacher/AI badges, delete support. Rendered in sidebar when teacher
clicks into a student's code."
```

---

## Task 5: Broadcast Mode

**Files:**
- Create: `src/components/session/broadcast-controls.tsx`
- Modify: `src/app/dashboard/classrooms/[id]/session/[sessionId]/dashboard/page.tsx`
- Modify: `src/app/dashboard/classrooms/[id]/session/[sessionId]/page.tsx`

- [ ] **Step 1: Create BroadcastControls component for teacher**

Create `src/components/session/broadcast-controls.tsx`:

```tsx
"use client";

import { useState } from "react";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { CodeEditor } from "@/components/editor/code-editor";
import { Button } from "@/components/ui/button";
import { sessionEventBus } from "@/lib/sse";

interface BroadcastControlsProps {
  sessionId: string;
  token: string;
}

export function BroadcastControls({ sessionId, token }: BroadcastControlsProps) {
  const [broadcasting, setBroadcasting] = useState(false);

  const documentName = `broadcast:${sessionId}`;
  const { yText, provider, connected } = useYjsProvider({
    documentName: broadcasting ? documentName : "noop",
    token,
  });

  async function toggleBroadcast() {
    const newState = !broadcasting;
    setBroadcasting(newState);

    // Notify students via API (SSE event)
    await fetch(`/api/sessions/${sessionId}/events`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        event: newState ? "broadcast_started" : "broadcast_ended",
      }),
    }).catch(() => {});
  }

  if (!broadcasting) {
    return (
      <Button variant="outline" size="sm" onClick={toggleBroadcast}>
        Start Broadcasting
      </Button>
    );
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium">Broadcasting Live</span>
          <span className={`w-2 h-2 rounded-full ${connected ? "bg-green-500 animate-pulse" : "bg-red-500"}`} />
        </div>
        <Button variant="destructive" size="sm" onClick={toggleBroadcast}>
          Stop
        </Button>
      </div>
      <div className="h-64 border rounded-lg">
        <CodeEditor yText={yText} provider={provider} />
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Add broadcast controls to teacher dashboard**

In `src/app/dashboard/classrooms/[id]/session/[sessionId]/dashboard/page.tsx`, import and add:

```tsx
import { BroadcastControls } from "@/components/session/broadcast-controls";
```

Add the broadcast controls between the header and the student grid, in the main dashboard view (not the selectedStudent view):

After the header `</div>` and before `<div className="flex gap-4">`:

```tsx
      <BroadcastControls sessionId={params.sessionId} token={token} />
```

- [ ] **Step 3: Add broadcast listener to student session page**

In `src/app/dashboard/classrooms/[id]/session/[sessionId]/page.tsx`, add broadcast state and display. Add state:

```tsx
  const [broadcastActive, setBroadcastActive] = useState(false);
```

Add SSE listeners for broadcast events (inside the existing SSE useEffect, before `return () => eventSource.close()`):

```tsx
    eventSource.addEventListener("broadcast_started", () => {
      setBroadcastActive(true);
    });

    eventSource.addEventListener("broadcast_ended", () => {
      setBroadcastActive(false);
    });
```

Add a broadcast view provider:

```tsx
  const broadcastDocName = `broadcast:${params.sessionId}`;
  const { yText: broadcastYText, provider: broadcastProvider } = useYjsProvider({
    documentName: broadcastActive ? broadcastDocName : "noop",
    token,
  });
```

Add a broadcast banner at the top of the editor area (inside the main content div, before the editor):

```tsx
        {broadcastActive && (
          <div className="mx-4 mb-2 border rounded-lg overflow-hidden">
            <div className="bg-blue-50 px-3 py-1 text-xs font-medium text-blue-700 border-b">
              Teacher is broadcasting
            </div>
            <div className="h-40">
              <CodeEditor yText={broadcastYText} provider={broadcastProvider} readOnly />
            </div>
          </div>
        )}
```

- [ ] **Step 4: Verify build**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun run build
```

- [ ] **Step 5: Commit**

```bash
git add src/components/session/broadcast-controls.tsx src/app/dashboard/classrooms/\[id\]/session/\[sessionId\]/dashboard/page.tsx src/app/dashboard/classrooms/\[id\]/session/\[sessionId\]/page.tsx
git commit -m "feat: add teacher broadcast mode

Teacher can broadcast their editor to all students in real-time.
Uses Yjs broadcast document. Students see a read-only banner with
the teacher's live code."
```

---

## Task 6: Full Verification and TODO Update

- [ ] **Step 1: Run full test suite**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test
```

Expected: All tests pass.

- [ ] **Step 2: Verify build**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun run build
```

- [ ] **Step 3: Update TODO.md**

Mark the 4 completed items:

```markdown
- [x] ~~**AI toggle SSE integration**~~
- [x] ~~**Annotation UI**~~
- [x] ~~**AI activity feed**~~
- [x] ~~**Broadcast mode**~~
```

- [ ] **Step 4: Commit**

```bash
git add TODO.md
git commit -m "docs: mark completed MVP gap items in TODO.md"
```

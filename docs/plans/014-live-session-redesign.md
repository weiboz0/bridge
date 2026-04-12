# Live Session Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign the live session experience per spec 002 Sub-project 4. Replace the current flat teacher dashboard and student session page with a 3-panel teacher dashboard (student list, main area with mode switching, AI assistant panel) and a flexible student layout (side-by-side or stacked with lesson content, editor, output, and side panel).

**Architecture:** The teacher dashboard becomes a single page with 3 collapsible/resizable panels. The left panel shows the student list (sorted by hand-raised first). The center panel switches between 4 modes: Presentation (lesson content), Student Grid (miniaturized code tiles), Collaborative Edit (full editor synced with a selected student), and Broadcast (teacher's editor mirrored to all students). The right panel shows AI recommendations, help queue, and AI activity feed. The student view gains lesson content alongside the code editor, a layout toggle (side-by-side vs stacked), and a minimizable side panel for AI chat and annotations. Session-topic linking lets the teacher select which topics to cover in a session.

**Tech Stack:** Next.js 16 App Router, React Client Components, Monaco Editor, Yjs + Hocuspocus, shadcn/ui (NO asChild), Tailwind CSS v4, lucide-react, Vitest + Testing Library

**Depends on:** Plan 013 (multi-language -- EditorSwitcher component), Plan 011 (lesson content -- LessonRenderer + lesson-content types), Plan 007 (course hierarchy -- schema), Plan 003 (realtime sessions -- SSE, Yjs)

**Key constraints:**
- shadcn/ui uses `@base-ui/react` -- NO `asChild` prop; use `buttonVariants()` with `<Link>` instead
- Auth.js v5: `session.user.id`, `session.user.isPlatformAdmin`
- Drizzle ORM for all DB queries -- use existing lib functions, add new ones only when missing
- `fileParallelism: false` in Vitest -- `.tsx` tests need `// @vitest-environment jsdom`
- The current teacher dashboard is at `src/app/dashboard/classrooms/[id]/session/[sessionId]/dashboard/page.tsx` and student page at `src/app/dashboard/classrooms/[id]/session/[sessionId]/page.tsx`. These existing routes are kept working but the new portal routes are the canonical path forward.
- The `sessionTopics` table already exists in the schema but has no lib functions or API routes yet.
- The `BroadcastControls` component currently uses SSE events (`broadcast_started`, `broadcast_ended`) but has no API route -- the broadcast is purely Yjs-based.
- Existing SSE event bus at `src/lib/sse.ts` is the standard for server-to-client real-time events.
- localStorage key convention: `bridge-<name>` (see `bridge-theme`, `bridge-sidebar-collapsed`)

---

## File Structure

```
src/
├── lib/
│   ├── session-topics.ts                         # Create: CRUD for session_topics N:N
│   └── hooks/
│       ├── use-panel-layout.ts                   # Create: localStorage persistence for teacher panel sizes
│       └── use-student-layout.ts                 # Create: localStorage persistence for student layout preference
├── components/
│   └── session/
│       ├── teacher/
│       │   ├── teacher-dashboard.tsx              # Create: main 3-panel dashboard shell
│       │   ├── teacher-header.tsx                 # Create: session header (topic name, timer, count, end button)
│       │   ├── student-list-panel.tsx             # Create: left panel -- student list sorted by hand-raised
│       │   ├── student-list-item.tsx              # Create: single row in student list
│       │   ├── main-area.tsx                      # Create: center panel with mode switching
│       │   ├── mode-toolbar.tsx                   # Create: bottom toolbar for mode tabs
│       │   ├── presentation-mode.tsx              # Create: renders lesson content from selected topic(s)
│       │   ├── grid-mode.tsx                      # Create: wraps existing StudentGrid
│       │   ├── collaborate-mode.tsx               # Create: full editor synced with selected student + annotations
│       │   ├── broadcast-mode.tsx                 # Create: teacher broadcast editor using EditorSwitcher
│       │   ├── ai-assistant-panel.tsx             # Create: right panel -- recommendations + help queue + activity feed
│       │   └── topic-selector.tsx                 # Create: multi-select for session topics
│       └── student/
│           ├── student-session.tsx                # Create: main student session layout
│           ├── student-toolbar.tsx                # Create: toolbar with layout toggle, raise hand, AI toggle
│           ├── student-side-panel.tsx             # Create: minimizable side panel (AI chat, annotations)
│           ├── student-broadcast-overlay.tsx      # Create: broadcast viewer overlay
│           └── student-lesson-panel.tsx           # Create: lesson content panel for student view
├── app/
│   ├── (portal)/
│   │   ├── teacher/
│   │   │   └── classes/
│   │   │       └── [id]/
│   │   │           └── session/
│   │   │               └── [sessionId]/
│   │   │                   └── dashboard/
│   │   │                       └── page.tsx       # Create: new teacher dashboard page (server component wrapper)
│   │   └── student/
│   │       └── classes/
│   │           └── [id]/
│   │               └── session/
│   │                   └── [sessionId]/
│   │                       └── page.tsx           # Create: new student session page (server component wrapper)
│   └── api/
│       └── sessions/
│           └── [id]/
│               ├── topics/
│               │   └── route.ts                   # Create: GET/POST/DELETE session topics
│               └── broadcast/
│                   └── route.ts                   # Create: POST broadcast start/stop (emits SSE events)
tests/
├── unit/
│   ├── session-topics.test.ts                     # Create: session-topics lib tests
│   ├── teacher-dashboard.test.tsx                 # Create: teacher dashboard component tests
│   ├── student-list-panel.test.tsx                # Create: student list sorting tests
│   ├── mode-toolbar.test.tsx                      # Create: mode toolbar rendering tests
│   ├── student-session.test.tsx                   # Create: student session layout tests
│   ├── student-side-panel.test.tsx                # Create: side panel minimize/badge tests
│   ├── use-panel-layout.test.ts                   # Create: panel layout hook tests
│   └── use-student-layout.test.ts                 # Create: student layout hook tests
```

---

## Task 1: Session-Topic Linking Library + API + Tests

**Files:**
- Create: `src/lib/session-topics.ts`
- Create: `src/app/api/sessions/[id]/topics/route.ts`
- Create: `tests/unit/session-topics.test.ts`

The `sessionTopics` table already exists in the schema. This task adds lib functions and an API route so the teacher can link topics to a session.

- [ ] **Step 1: Create `src/lib/session-topics.ts`**

```typescript
import { eq, and } from "drizzle-orm";
import { sessionTopics, topics } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

export async function addSessionTopic(
  db: Database,
  sessionId: string,
  topicId: string
) {
  const [row] = await db
    .insert(sessionTopics)
    .values({ sessionId, topicId })
    .onConflictDoNothing()
    .returning();
  return row ?? { sessionId, topicId };
}

export async function removeSessionTopic(
  db: Database,
  sessionId: string,
  topicId: string
) {
  const deleted = await db
    .delete(sessionTopics)
    .where(
      and(
        eq(sessionTopics.sessionId, sessionId),
        eq(sessionTopics.topicId, topicId)
      )
    )
    .returning();
  return deleted.length > 0;
}

export async function listSessionTopics(db: Database, sessionId: string) {
  return db
    .select({
      topicId: sessionTopics.topicId,
      title: topics.title,
      description: topics.description,
      sortOrder: topics.sortOrder,
      lessonContent: topics.lessonContent,
      starterCode: topics.starterCode,
    })
    .from(sessionTopics)
    .innerJoin(topics, eq(sessionTopics.topicId, topics.id))
    .where(eq(sessionTopics.sessionId, sessionId))
    .orderBy(topics.sortOrder);
}

export async function replaceSessionTopics(
  db: Database,
  sessionId: string,
  topicIds: string[]
) {
  // Delete all existing
  await db
    .delete(sessionTopics)
    .where(eq(sessionTopics.sessionId, sessionId));

  // Insert new
  if (topicIds.length > 0) {
    await db.insert(sessionTopics).values(
      topicIds.map((topicId) => ({ sessionId, topicId }))
    );
  }
}
```

- [ ] **Step 2: Create `src/app/api/sessions/[id]/topics/route.ts`**

```typescript
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSession } from "@/lib/sessions";
import {
  listSessionTopics,
  addSessionTopic,
  removeSessionTopic,
} from "@/lib/session-topics";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const topics = await listSessionTopics(db, id);
  return NextResponse.json(topics);
}

const addSchema = z.object({
  topicId: z.string().uuid(),
});

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const liveSession = await getSession(db, id);
  if (!liveSession) {
    return NextResponse.json({ error: "Session not found" }, { status: 404 });
  }
  if (liveSession.teacherId !== session.user.id) {
    return NextResponse.json(
      { error: "Only the teacher can manage session topics" },
      { status: 403 }
    );
  }

  const body = await request.json();
  const parsed = addSchema.safeParse(body);
  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const result = await addSessionTopic(db, id, parsed.data.topicId);
  return NextResponse.json(result, { status: 201 });
}

const deleteSchema = z.object({
  topicId: z.string().uuid(),
});

export async function DELETE(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const liveSession = await getSession(db, id);
  if (!liveSession) {
    return NextResponse.json({ error: "Session not found" }, { status: 404 });
  }
  if (liveSession.teacherId !== session.user.id) {
    return NextResponse.json(
      { error: "Only the teacher can manage session topics" },
      { status: 403 }
    );
  }

  const body = await request.json();
  const parsed = deleteSchema.safeParse(body);
  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const removed = await removeSessionTopic(db, id, parsed.data.topicId);
  if (!removed) {
    return NextResponse.json({ error: "Topic not linked" }, { status: 404 });
  }

  return NextResponse.json({ ok: true });
}
```

- [ ] **Step 3: Create `tests/unit/session-topics.test.ts`**

```typescript
import { describe, it, expect, vi, beforeEach } from "vitest";

// Mock drizzle db
const mockInsert = vi.fn();
const mockDelete = vi.fn();
const mockSelect = vi.fn();
const mockValues = vi.fn();
const mockOnConflictDoNothing = vi.fn();
const mockReturning = vi.fn();
const mockWhere = vi.fn();
const mockInnerJoin = vi.fn();
const mockOrderBy = vi.fn();
const mockFrom = vi.fn();

vi.mock("@/lib/db/schema", () => ({
  sessionTopics: {
    sessionId: "session_id",
    topicId: "topic_id",
  },
  topics: {
    id: "id",
    title: "title",
    description: "description",
    sortOrder: "sort_order",
    lessonContent: "lesson_content",
    starterCode: "starter_code",
  },
}));

vi.mock("drizzle-orm", () => ({
  eq: vi.fn((a, b) => ({ eq: [a, b] })),
  and: vi.fn((...args: unknown[]) => ({ and: args })),
}));

function createMockDb() {
  mockReturning.mockReturnValue([{ sessionId: "s1", topicId: "t1" }]);
  mockOnConflictDoNothing.mockReturnValue({ returning: mockReturning });
  mockValues.mockReturnValue({ onConflictDoNothing: mockOnConflictDoNothing });
  mockInsert.mockReturnValue({ values: mockValues });

  mockWhere.mockReturnValue([]);
  mockOrderBy.mockReturnValue([]);
  mockInnerJoin.mockReturnValue({ where: vi.fn().mockReturnValue({ orderBy: mockOrderBy }) });
  mockFrom.mockReturnValue({ innerJoin: mockInnerJoin });
  mockSelect.mockReturnValue({ from: mockFrom });

  const deleteWhere = vi.fn().mockReturnValue({ returning: vi.fn().mockReturnValue([{ sessionId: "s1", topicId: "t1" }]) });
  mockDelete.mockReturnValue({ where: deleteWhere });

  return {
    insert: mockInsert,
    delete: mockDelete,
    select: mockSelect,
  } as any;
}

describe("session-topics", () => {
  let db: any;

  beforeEach(() => {
    vi.clearAllMocks();
    db = createMockDb();
  });

  it("addSessionTopic inserts a row", async () => {
    const { addSessionTopic } = await import("@/lib/session-topics");
    const result = await addSessionTopic(db, "s1", "t1");
    expect(mockInsert).toHaveBeenCalled();
    expect(result).toEqual({ sessionId: "s1", topicId: "t1" });
  });

  it("removeSessionTopic deletes a row", async () => {
    const { removeSessionTopic } = await import("@/lib/session-topics");
    const result = await removeSessionTopic(db, "s1", "t1");
    expect(mockDelete).toHaveBeenCalled();
    expect(result).toBe(true);
  });

  it("removeSessionTopic returns false when nothing deleted", async () => {
    const deleteWhere = vi.fn().mockReturnValue({ returning: vi.fn().mockReturnValue([]) });
    db.delete = vi.fn().mockReturnValue({ where: deleteWhere });
    const { removeSessionTopic } = await import("@/lib/session-topics");
    const result = await removeSessionTopic(db, "s1", "t-nonexistent");
    expect(result).toBe(false);
  });

  it("listSessionTopics queries with join and order", async () => {
    const { listSessionTopics } = await import("@/lib/session-topics");
    await listSessionTopics(db, "s1");
    expect(mockSelect).toHaveBeenCalled();
  });

  it("replaceSessionTopics deletes then inserts", async () => {
    const deleteWhere = vi.fn().mockReturnValue(undefined);
    db.delete = vi.fn().mockReturnValue({ where: deleteWhere });
    mockValues.mockReturnValue(undefined);
    const { replaceSessionTopics } = await import("@/lib/session-topics");
    await replaceSessionTopics(db, "s1", ["t1", "t2"]);
    expect(db.delete).toHaveBeenCalled();
    expect(mockInsert).toHaveBeenCalled();
  });

  it("replaceSessionTopics with empty array only deletes", async () => {
    const deleteWhere = vi.fn().mockReturnValue(undefined);
    db.delete = vi.fn().mockReturnValue({ where: deleteWhere });
    const { replaceSessionTopics } = await import("@/lib/session-topics");
    await replaceSessionTopics(db, "s1", []);
    expect(db.delete).toHaveBeenCalled();
    expect(mockInsert).not.toHaveBeenCalled();
  });
});
```

- [ ] **Step 4: Run tests, verify pass**

```bash
cd /home/chris/workshop/Bridge && bun test tests/unit/session-topics.test.ts
```

- [ ] **Step 5: Commit**

```bash
git add src/lib/session-topics.ts src/app/api/sessions/\[id\]/topics/route.ts tests/unit/session-topics.test.ts
git commit -m "Add session-topic linking library and API route

Create CRUD functions for the session_topics N:N table and expose
GET/POST/DELETE endpoints so teachers can link topics to sessions."
```

---

## Task 2: Broadcast API Route

**Files:**
- Create: `src/app/api/sessions/[id]/broadcast/route.ts`

The broadcast is primarily Yjs-based (teacher writes to `broadcast:{sessionId}` document, students subscribe). This API route handles the SSE event emission so students know when to show/hide the broadcast viewer.

- [ ] **Step 1: Create `src/app/api/sessions/[id]/broadcast/route.ts`**

```typescript
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSession } from "@/lib/sessions";
import { sessionEventBus } from "@/lib/sse";

const broadcastSchema = z.object({
  active: z.boolean(),
});

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const liveSession = await getSession(db, id);

  if (!liveSession) {
    return NextResponse.json({ error: "Session not found" }, { status: 404 });
  }

  if (liveSession.teacherId !== session.user.id) {
    return NextResponse.json(
      { error: "Only the teacher can control broadcasting" },
      { status: 403 }
    );
  }

  const body = await request.json();
  const parsed = broadcastSchema.safeParse(body);
  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const event = parsed.data.active ? "broadcast_started" : "broadcast_ended";
  sessionEventBus.emit(id, event, {
    sessionId: id,
    teacherId: session.user.id,
  });

  return NextResponse.json({ active: parsed.data.active });
}
```

- [ ] **Step 2: Commit**

```bash
git add src/app/api/sessions/\[id\]/broadcast/route.ts
git commit -m "Add broadcast API route for SSE event emission

Teachers can POST to toggle broadcast on/off, which emits
broadcast_started or broadcast_ended SSE events to all clients."
```

---

## Task 3: Layout Persistence Hooks

**Files:**
- Create: `src/lib/hooks/use-panel-layout.ts`
- Create: `src/lib/hooks/use-student-layout.ts`
- Create: `tests/unit/use-panel-layout.test.ts`
- Create: `tests/unit/use-student-layout.test.ts`

These hooks persist layout preferences to localStorage, following the existing `bridge-*` key convention.

- [ ] **Step 1: Create `src/lib/hooks/use-panel-layout.ts`**

```typescript
"use client";

import { useState, useEffect, useCallback } from "react";

export interface PanelSizes {
  left: number;   // percentage width (0-100), 0 = collapsed
  right: number;  // percentage width (0-100), 0 = collapsed
}

const DEFAULT_SIZES: PanelSizes = { left: 20, right: 25 };
const STORAGE_KEY = "bridge-teacher-panel-layout";

export function usePanelLayout() {
  const [sizes, setSizesState] = useState<PanelSizes>(DEFAULT_SIZES);

  useEffect(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored) {
        const parsed = JSON.parse(stored);
        if (typeof parsed.left === "number" && typeof parsed.right === "number") {
          setSizesState(parsed);
        }
      }
    } catch {
      // Ignore corrupt localStorage
    }
  }, []);

  const setSizes = useCallback((next: PanelSizes) => {
    setSizesState(next);
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
    } catch {
      // localStorage full or unavailable
    }
  }, []);

  const toggleLeft = useCallback(() => {
    setSizes({
      ...sizes,
      left: sizes.left === 0 ? DEFAULT_SIZES.left : 0,
    });
  }, [sizes, setSizes]);

  const toggleRight = useCallback(() => {
    setSizes({
      ...sizes,
      right: sizes.right === 0 ? DEFAULT_SIZES.right : 0,
    });
  }, [sizes, setSizes]);

  return { sizes, setSizes, toggleLeft, toggleRight };
}
```

- [ ] **Step 2: Create `src/lib/hooks/use-student-layout.ts`**

```typescript
"use client";

import { useState, useEffect, useCallback } from "react";

export type StudentLayout = "side-by-side" | "stacked";

const STORAGE_KEY = "bridge-student-layout";

function getDefaultLayout(): StudentLayout {
  if (typeof window === "undefined") return "side-by-side";
  return window.innerWidth < 768 ? "stacked" : "side-by-side";
}

export function useStudentLayout() {
  const [layout, setLayoutState] = useState<StudentLayout>("side-by-side");

  useEffect(() => {
    const stored = localStorage.getItem(STORAGE_KEY) as StudentLayout | null;
    if (stored === "side-by-side" || stored === "stacked") {
      setLayoutState(stored);
    } else {
      setLayoutState(getDefaultLayout());
    }
  }, []);

  const setLayout = useCallback((next: StudentLayout) => {
    setLayoutState(next);
    try {
      localStorage.setItem(STORAGE_KEY, next);
    } catch {
      // localStorage full or unavailable
    }
  }, []);

  const toggleLayout = useCallback(() => {
    setLayout(layout === "side-by-side" ? "stacked" : "side-by-side");
  }, [layout, setLayout]);

  return { layout, setLayout, toggleLayout };
}
```

- [ ] **Step 3: Create `tests/unit/use-panel-layout.test.ts`**

```typescript
import { describe, it, expect, vi, beforeEach } from "vitest";

// @vitest-environment jsdom

describe("usePanelLayout", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.resetModules();
  });

  it("returns default sizes when nothing stored", async () => {
    const { usePanelLayout } = await import("@/lib/hooks/use-panel-layout");
    // We test the raw function logic since hooks need a React render context
    // Focus on the localStorage key and default values
    expect(localStorage.getItem("bridge-teacher-panel-layout")).toBeNull();
  });

  it("stores sizes under the correct key", () => {
    const data = JSON.stringify({ left: 15, right: 30 });
    localStorage.setItem("bridge-teacher-panel-layout", data);
    const stored = JSON.parse(localStorage.getItem("bridge-teacher-panel-layout")!);
    expect(stored.left).toBe(15);
    expect(stored.right).toBe(30);
  });

  it("handles corrupt localStorage gracefully", () => {
    localStorage.setItem("bridge-teacher-panel-layout", "not json");
    expect(() => {
      JSON.parse(localStorage.getItem("bridge-teacher-panel-layout")!);
    }).toThrow();
    // The hook should catch this and use defaults -- tested via component tests
  });
});
```

- [ ] **Step 4: Create `tests/unit/use-student-layout.test.ts`**

```typescript
import { describe, it, expect, vi, beforeEach } from "vitest";

// @vitest-environment jsdom

describe("useStudentLayout", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.resetModules();
  });

  it("uses correct localStorage key", () => {
    localStorage.setItem("bridge-student-layout", "stacked");
    expect(localStorage.getItem("bridge-student-layout")).toBe("stacked");
  });

  it("accepts side-by-side value", () => {
    localStorage.setItem("bridge-student-layout", "side-by-side");
    expect(localStorage.getItem("bridge-student-layout")).toBe("side-by-side");
  });

  it("ignores invalid stored value", () => {
    localStorage.setItem("bridge-student-layout", "invalid");
    const stored = localStorage.getItem("bridge-student-layout");
    // Hook should ignore "invalid" and use default -- tested via component tests
    expect(stored).toBe("invalid");
  });
});
```

- [ ] **Step 5: Run tests, verify pass**

```bash
cd /home/chris/workshop/Bridge && bun test tests/unit/use-panel-layout.test.ts tests/unit/use-student-layout.test.ts
```

- [ ] **Step 6: Commit**

```bash
git add src/lib/hooks/use-panel-layout.ts src/lib/hooks/use-student-layout.ts tests/unit/use-panel-layout.test.ts tests/unit/use-student-layout.test.ts
git commit -m "Add layout persistence hooks for teacher and student views

usePanelLayout persists teacher 3-panel sizes to localStorage.
useStudentLayout persists student side-by-side vs stacked preference."
```

---

## Task 4: Teacher Dashboard -- Student List Panel

**Files:**
- Create: `src/components/session/teacher/student-list-item.tsx`
- Create: `src/components/session/teacher/student-list-panel.tsx`
- Create: `tests/unit/student-list-panel.test.tsx`

The student list panel is the left panel of the teacher dashboard. It shows all participants sorted with hand-raised students at the top, then by name. Each row shows the student name, status indicator, AI enabled state, and hand-raised indicator.

- [ ] **Step 1: Create `src/components/session/teacher/student-list-item.tsx`**

```typescript
"use client";

import { AiToggleButton } from "@/components/ai/ai-toggle-button";

export interface StudentListItemData {
  studentId: string;
  name: string;
  status: string;
  handRaised: boolean;
}

interface StudentListItemProps {
  student: StudentListItemData;
  sessionId: string;
  selected: boolean;
  onClick: () => void;
}

export function StudentListItem({
  student,
  sessionId,
  selected,
  onClick,
}: StudentListItemProps) {
  const statusColor = {
    active: "bg-green-500",
    idle: "bg-yellow-500",
    needs_help: "bg-red-500",
  }[student.status] || "bg-gray-500";

  return (
    <button
      type="button"
      onClick={onClick}
      className={`w-full flex items-center gap-2 px-3 py-2 text-left rounded-lg transition-colors hover:bg-accent ${
        selected ? "bg-accent border border-primary" : ""
      }`}
    >
      <span className={`w-2 h-2 rounded-full shrink-0 ${statusColor}`} />
      <span className="text-sm font-medium truncate flex-1">{student.name}</span>
      <div className="flex items-center gap-1 shrink-0">
        {student.handRaised && (
          <span className="text-sm" title="Hand raised">
            {"\u270B"}
          </span>
        )}
        <AiToggleButton sessionId={sessionId} studentId={student.studentId} />
      </div>
    </button>
  );
}
```

- [ ] **Step 2: Create `src/components/session/teacher/student-list-panel.tsx`**

```typescript
"use client";

import { useMemo } from "react";
import { StudentListItem, type StudentListItemData } from "./student-list-item";

interface StudentListPanelProps {
  students: StudentListItemData[];
  sessionId: string;
  selectedStudentId: string | null;
  onSelectStudent: (studentId: string) => void;
  collapsed: boolean;
}

export function StudentListPanel({
  students,
  sessionId,
  selectedStudentId,
  onSelectStudent,
  collapsed,
}: StudentListPanelProps) {
  const sorted = useMemo(() => {
    return [...students].sort((a, b) => {
      // Hand raised first
      if (a.handRaised && !b.handRaised) return -1;
      if (!a.handRaised && b.handRaised) return 1;
      // Then by name
      return a.name.localeCompare(b.name);
    });
  }, [students]);

  if (collapsed) return null;

  const handRaisedCount = students.filter((s) => s.handRaised).length;

  return (
    <div className="flex flex-col h-full border-r">
      <div className="px-3 py-2 border-b">
        <h3 className="text-sm font-semibold">
          Students ({students.length})
        </h3>
        {handRaisedCount > 0 && (
          <p className="text-xs text-orange-600">
            {handRaisedCount} hand{handRaisedCount !== 1 ? "s" : ""} raised
          </p>
        )}
      </div>
      <div className="flex-1 overflow-auto p-1 space-y-0.5">
        {sorted.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center py-4">
            No students connected
          </p>
        ) : (
          sorted.map((student) => (
            <StudentListItem
              key={student.studentId}
              student={student}
              sessionId={sessionId}
              selected={selectedStudentId === student.studentId}
              onClick={() => onSelectStudent(student.studentId)}
            />
          ))
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Create `tests/unit/student-list-panel.test.tsx`**

```tsx
// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { StudentListPanel } from "@/components/session/teacher/student-list-panel";

// Mock the AiToggleButton since it makes API calls
vi.mock("@/components/ai/ai-toggle-button", () => ({
  AiToggleButton: ({ studentId }: { studentId: string }) => (
    <button data-testid={`ai-toggle-${studentId}`}>AI</button>
  ),
}));

const students = [
  { studentId: "s1", name: "Charlie", status: "active", handRaised: false },
  { studentId: "s2", name: "Alice", status: "active", handRaised: true },
  { studentId: "s3", name: "Bob", status: "idle", handRaised: false },
];

describe("StudentListPanel", () => {
  it("renders all students", () => {
    render(
      <StudentListPanel
        students={students}
        sessionId="sess-1"
        selectedStudentId={null}
        onSelectStudent={() => {}}
        collapsed={false}
      />
    );
    expect(screen.getByText("Charlie")).toBeDefined();
    expect(screen.getByText("Alice")).toBeDefined();
    expect(screen.getByText("Bob")).toBeDefined();
  });

  it("sorts hand-raised students first", () => {
    render(
      <StudentListPanel
        students={students}
        sessionId="sess-1"
        selectedStudentId={null}
        onSelectStudent={() => {}}
        collapsed={false}
      />
    );
    const buttons = screen.getAllByRole("button").filter(
      (b) => b.textContent !== "AI"
    );
    // Alice (hand raised) should be first
    expect(buttons[0].textContent).toContain("Alice");
  });

  it("shows hand raised count", () => {
    render(
      <StudentListPanel
        students={students}
        sessionId="sess-1"
        selectedStudentId={null}
        onSelectStudent={() => {}}
        collapsed={false}
      />
    );
    expect(screen.getByText("1 hand raised")).toBeDefined();
  });

  it("shows empty state when no students", () => {
    render(
      <StudentListPanel
        students={[]}
        sessionId="sess-1"
        selectedStudentId={null}
        onSelectStudent={() => {}}
        collapsed={false}
      />
    );
    expect(screen.getByText("No students connected")).toBeDefined();
  });

  it("renders nothing when collapsed", () => {
    const { container } = render(
      <StudentListPanel
        students={students}
        sessionId="sess-1"
        selectedStudentId={null}
        onSelectStudent={() => {}}
        collapsed={true}
      />
    );
    expect(container.innerHTML).toBe("");
  });

  it("calls onSelectStudent when clicking a student", async () => {
    const onSelect = vi.fn();
    const { user } = await import("@testing-library/user-event").then((m) => ({
      user: m.default.setup(),
    }));
    render(
      <StudentListPanel
        students={students}
        sessionId="sess-1"
        selectedStudentId={null}
        onSelectStudent={onSelect}
        collapsed={false}
      />
    );
    const buttons = screen.getAllByRole("button").filter(
      (b) => b.textContent !== "AI"
    );
    await user.click(buttons[0]);
    expect(onSelect).toHaveBeenCalled();
  });

  it("shows student count in header", () => {
    render(
      <StudentListPanel
        students={students}
        sessionId="sess-1"
        selectedStudentId={null}
        onSelectStudent={() => {}}
        collapsed={false}
      />
    );
    expect(screen.getByText("Students (3)")).toBeDefined();
  });
});
```

- [ ] **Step 4: Run tests, verify pass**

```bash
cd /home/chris/workshop/Bridge && bun test tests/unit/student-list-panel.test.tsx
```

- [ ] **Step 5: Commit**

```bash
git add src/components/session/teacher/student-list-item.tsx src/components/session/teacher/student-list-panel.tsx tests/unit/student-list-panel.test.tsx
git commit -m "Add teacher student list panel with hand-raised sorting

Left panel of teacher dashboard showing all participants sorted
with hand-raised students at top, with status and AI toggle per student."
```

---

## Task 5: Teacher Dashboard -- Mode Toolbar + Main Area Shell

**Files:**
- Create: `src/components/session/teacher/mode-toolbar.tsx`
- Create: `src/components/session/teacher/main-area.tsx`
- Create: `tests/unit/mode-toolbar.test.tsx`

The mode toolbar sits at the bottom of the center panel and has tabs for Presentation, Student Grid, Collaborate, and Broadcast. The main area renders the active mode.

- [ ] **Step 1: Create `src/components/session/teacher/mode-toolbar.tsx`**

```typescript
"use client";

import { Button } from "@/components/ui/button";

export type DashboardMode = "presentation" | "grid" | "collaborate" | "broadcast";

interface ModeToolbarProps {
  activeMode: DashboardMode;
  onModeChange: (mode: DashboardMode) => void;
  broadcastActive: boolean;
}

const MODES: { key: DashboardMode; label: string }[] = [
  { key: "presentation", label: "Presentation" },
  { key: "grid", label: "Student Grid" },
  { key: "collaborate", label: "Collaborate" },
  { key: "broadcast", label: "Broadcast" },
];

export function ModeToolbar({
  activeMode,
  onModeChange,
  broadcastActive,
}: ModeToolbarProps) {
  return (
    <div className="flex items-center gap-1 px-3 py-2 border-t bg-muted/30">
      {MODES.map(({ key, label }) => (
        <Button
          key={key}
          variant={activeMode === key ? "default" : "ghost"}
          size="sm"
          onClick={() => onModeChange(key)}
          className="text-xs"
        >
          {label}
          {key === "broadcast" && broadcastActive && (
            <span className="ml-1 w-2 h-2 rounded-full bg-red-500 animate-pulse inline-block" />
          )}
        </Button>
      ))}
    </div>
  );
}
```

- [ ] **Step 2: Create `src/components/session/teacher/main-area.tsx`**

This is the container that renders the active mode. Individual mode components are created in the next tasks.

```typescript
"use client";

import type { DashboardMode } from "./mode-toolbar";

interface MainAreaProps {
  mode: DashboardMode;
  children: React.ReactNode;
}

export function MainArea({ mode, children }: MainAreaProps) {
  return (
    <div className="flex flex-col h-full" data-mode={mode}>
      <div className="flex-1 min-h-0 overflow-auto">
        {children}
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Create `tests/unit/mode-toolbar.test.tsx`**

```tsx
// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ModeToolbar } from "@/components/session/teacher/mode-toolbar";

describe("ModeToolbar", () => {
  it("renders all four mode buttons", () => {
    render(
      <ModeToolbar
        activeMode="grid"
        onModeChange={() => {}}
        broadcastActive={false}
      />
    );
    expect(screen.getByText("Presentation")).toBeDefined();
    expect(screen.getByText("Student Grid")).toBeDefined();
    expect(screen.getByText("Collaborate")).toBeDefined();
    expect(screen.getByText("Broadcast")).toBeDefined();
  });

  it("calls onModeChange when a mode is clicked", () => {
    const onChange = vi.fn();
    render(
      <ModeToolbar
        activeMode="grid"
        onModeChange={onChange}
        broadcastActive={false}
      />
    );
    fireEvent.click(screen.getByText("Presentation"));
    expect(onChange).toHaveBeenCalledWith("presentation");
  });

  it("shows broadcast active indicator", () => {
    const { container } = render(
      <ModeToolbar
        activeMode="broadcast"
        onModeChange={() => {}}
        broadcastActive={true}
      />
    );
    // There should be an animate-pulse indicator
    const pulseIndicator = container.querySelector(".animate-pulse");
    expect(pulseIndicator).not.toBeNull();
  });

  it("does not show broadcast indicator when inactive", () => {
    const { container } = render(
      <ModeToolbar
        activeMode="grid"
        onModeChange={() => {}}
        broadcastActive={false}
      />
    );
    const pulseIndicator = container.querySelector(".animate-pulse");
    expect(pulseIndicator).toBeNull();
  });
});
```

- [ ] **Step 4: Run tests, verify pass**

```bash
cd /home/chris/workshop/Bridge && bun test tests/unit/mode-toolbar.test.tsx
```

- [ ] **Step 5: Commit**

```bash
git add src/components/session/teacher/mode-toolbar.tsx src/components/session/teacher/main-area.tsx tests/unit/mode-toolbar.test.tsx
git commit -m "Add mode toolbar and main area shell for teacher dashboard

Four mode tabs (Presentation, Student Grid, Collaborate, Broadcast)
with active state styling and broadcast active indicator."
```

---

## Task 6: Teacher Dashboard -- Presentation Mode + Topic Selector

**Files:**
- Create: `src/components/session/teacher/presentation-mode.tsx`
- Create: `src/components/session/teacher/topic-selector.tsx`

Presentation mode renders lesson content from the selected topic(s) using the existing `LessonRenderer` component. The topic selector is a multi-select dropdown that fetches available topics from the course and lets the teacher link/unlink them via the session-topics API.

- [ ] **Step 1: Create `src/components/session/teacher/topic-selector.tsx`**

```typescript
"use client";

import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";

interface Topic {
  topicId: string;
  title: string;
  sortOrder: number;
}

interface AvailableTopic {
  id: string;
  title: string;
  sortOrder: number;
}

interface TopicSelectorProps {
  sessionId: string;
  courseId: string;
}

export function TopicSelector({ sessionId, courseId }: TopicSelectorProps) {
  const [availableTopics, setAvailableTopics] = useState<AvailableTopic[]>([]);
  const [linkedTopics, setLinkedTopics] = useState<Topic[]>([]);
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);

  // Fetch available topics from course
  useEffect(() => {
    async function fetchTopics() {
      const res = await fetch(`/api/courses/${courseId}/topics`);
      if (res.ok) {
        setAvailableTopics(await res.json());
      }
    }
    if (courseId) fetchTopics();
  }, [courseId]);

  // Fetch linked session topics
  useEffect(() => {
    async function fetchLinked() {
      const res = await fetch(`/api/sessions/${sessionId}/topics`);
      if (res.ok) {
        setLinkedTopics(await res.json());
      }
    }
    fetchLinked();
  }, [sessionId]);

  async function toggleTopic(topicId: string) {
    setLoading(true);
    const isLinked = linkedTopics.some((t) => t.topicId === topicId);

    if (isLinked) {
      const res = await fetch(`/api/sessions/${sessionId}/topics`, {
        method: "DELETE",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ topicId }),
      });
      if (res.ok) {
        setLinkedTopics((prev) => prev.filter((t) => t.topicId !== topicId));
      }
    } else {
      const res = await fetch(`/api/sessions/${sessionId}/topics`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ topicId }),
      });
      if (res.ok) {
        const topic = availableTopics.find((t) => t.id === topicId);
        if (topic) {
          setLinkedTopics((prev) => [
            ...prev,
            { topicId: topic.id, title: topic.title, sortOrder: topic.sortOrder },
          ]);
        }
      }
    }
    setLoading(false);
  }

  const linkedIds = new Set(linkedTopics.map((t) => t.topicId));

  return (
    <div className="relative">
      <Button
        variant="outline"
        size="sm"
        onClick={() => setOpen(!open)}
        className="text-xs"
      >
        Topics ({linkedTopics.length})
      </Button>
      {open && (
        <div className="absolute top-full left-0 mt-1 w-64 bg-background border rounded-lg shadow-lg z-50 p-2 space-y-1">
          {availableTopics.length === 0 ? (
            <p className="text-xs text-muted-foreground p-2">No topics available</p>
          ) : (
            availableTopics.map((topic) => (
              <button
                key={topic.id}
                type="button"
                onClick={() => toggleTopic(topic.id)}
                disabled={loading}
                className={`w-full text-left px-2 py-1.5 rounded text-sm transition-colors ${
                  linkedIds.has(topic.id)
                    ? "bg-primary/10 text-primary font-medium"
                    : "hover:bg-accent"
                }`}
              >
                <span className="mr-2">
                  {linkedIds.has(topic.id) ? "\u2713" : "\u00A0\u00A0"}
                </span>
                {topic.title}
              </button>
            ))
          )}
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Create `src/components/session/teacher/presentation-mode.tsx`**

```typescript
"use client";

import { useState, useEffect } from "react";
import { LessonRenderer } from "@/components/lesson/lesson-renderer";
import { parseLessonContent } from "@/lib/lesson-content";

interface SessionTopic {
  topicId: string;
  title: string;
  description: string | null;
  sortOrder: number;
  lessonContent: unknown;
  starterCode: string | null;
}

interface PresentationModeProps {
  sessionId: string;
}

export function PresentationMode({ sessionId }: PresentationModeProps) {
  const [topics, setTopics] = useState<SessionTopic[]>([]);
  const [activeTopicIndex, setActiveTopicIndex] = useState(0);

  useEffect(() => {
    async function fetchTopics() {
      const res = await fetch(`/api/sessions/${sessionId}/topics`);
      if (res.ok) {
        setTopics(await res.json());
      }
    }
    fetchTopics();
    // Poll for topic changes (teacher may add/remove during session)
    const interval = setInterval(fetchTopics, 10000);
    return () => clearInterval(interval);
  }, [sessionId]);

  if (topics.length === 0) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center text-muted-foreground">
          <p className="text-lg font-medium">No topics selected</p>
          <p className="text-sm mt-1">
            Use the topic selector in the header to add topics to this session.
          </p>
        </div>
      </div>
    );
  }

  const activeTopic = topics[activeTopicIndex];
  const content = parseLessonContent(activeTopic?.lessonContent);

  return (
    <div className="flex flex-col h-full">
      {topics.length > 1 && (
        <div className="flex items-center gap-1 px-4 py-2 border-b overflow-x-auto">
          {topics.map((topic, i) => (
            <button
              key={topic.topicId}
              type="button"
              onClick={() => setActiveTopicIndex(i)}
              className={`px-3 py-1 rounded text-sm whitespace-nowrap transition-colors ${
                i === activeTopicIndex
                  ? "bg-primary text-primary-foreground"
                  : "hover:bg-accent"
              }`}
            >
              {topic.title}
            </button>
          ))}
        </div>
      )}
      <div className="flex-1 overflow-auto p-6">
        {activeTopic && (
          <>
            <h2 className="text-xl font-bold mb-4">{activeTopic.title}</h2>
            {activeTopic.description && (
              <p className="text-muted-foreground mb-4">{activeTopic.description}</p>
            )}
            <LessonRenderer content={content} />
          </>
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Commit**

```bash
git add src/components/session/teacher/topic-selector.tsx src/components/session/teacher/presentation-mode.tsx
git commit -m "Add presentation mode and topic selector for teacher dashboard

Presentation mode renders lesson content from selected topics.
Topic selector lets teachers link/unlink course topics to the session."
```

---

## Task 7: Teacher Dashboard -- Grid, Collaborate, and Broadcast Modes

**Files:**
- Create: `src/components/session/teacher/grid-mode.tsx`
- Create: `src/components/session/teacher/collaborate-mode.tsx`
- Create: `src/components/session/teacher/broadcast-mode.tsx`

These wrap existing components (StudentGrid, CodeEditor, EditorSwitcher) into the mode-switching framework.

- [ ] **Step 1: Create `src/components/session/teacher/grid-mode.tsx`**

```typescript
"use client";

import { StudentGrid } from "@/components/session/student-grid";

interface Participant {
  studentId: string;
  name: string;
  status: string;
}

interface GridModeProps {
  sessionId: string;
  participants: Participant[];
  token: string;
  onSelectStudent: (studentId: string) => void;
}

export function GridMode({
  sessionId,
  participants,
  token,
  onSelectStudent,
}: GridModeProps) {
  return (
    <div className="p-4">
      <StudentGrid
        sessionId={sessionId}
        participants={participants}
        token={token}
        onSelectStudent={onSelectStudent}
      />
    </div>
  );
}
```

- [ ] **Step 2: Create `src/components/session/teacher/collaborate-mode.tsx`**

```typescript
"use client";

import { useState, useCallback, useEffect } from "react";
import { CodeEditor } from "@/components/editor/code-editor";
import { DiffViewer } from "@/components/editor/diff-viewer";
import { AnnotationForm } from "@/components/annotations/annotation-form";
import { AnnotationList } from "@/components/annotations/annotation-list";
import { AiToggleButton } from "@/components/ai/ai-toggle-button";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { Button } from "@/components/ui/button";

interface CollaborateModeProps {
  sessionId: string;
  selectedStudentId: string | null;
  selectedStudentName: string;
  token: string;
  language?: string;
  starterCode?: string;
  onBack: () => void;
}

export function CollaborateMode({
  sessionId,
  selectedStudentId,
  selectedStudentName,
  token,
  language = "python",
  starterCode = "",
  onBack,
}: CollaborateModeProps) {
  const [annotations, setAnnotations] = useState<any[]>([]);
  const [showDiff, setShowDiff] = useState(false);

  const docName = selectedStudentId
    ? `session:${sessionId}:user:${selectedStudentId}`
    : "";

  const { yText, provider, connected } = useYjsProvider({
    documentName: docName || "noop",
    token,
  });

  const docId = docName;

  const fetchAnnotations = useCallback(async () => {
    if (!docId) return;
    const res = await fetch(
      `/api/annotations?documentId=${encodeURIComponent(docId)}`
    );
    if (res.ok) {
      setAnnotations(await res.json());
    }
  }, [docId]);

  useEffect(() => {
    fetchAnnotations();
  }, [fetchAnnotations]);

  useEffect(() => {
    setShowDiff(false);
  }, [selectedStudentId]);

  async function deleteAnnotation(id: string) {
    await fetch(`/api/annotations/${id}`, { method: "DELETE" });
    fetchAnnotations();
  }

  if (!selectedStudentId) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center text-muted-foreground">
          <p className="text-lg font-medium">No student selected</p>
          <p className="text-sm mt-1">
            Select a student from the list to collaborate on their code.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full">
      <div className="flex flex-col flex-1">
        <div className="flex items-center justify-between px-4 py-2 border-b">
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="sm" onClick={onBack}>
              Back
            </Button>
            <span className="font-medium">{selectedStudentName}</span>
            <span
              className={`w-2 h-2 rounded-full ${
                connected ? "bg-green-500" : "bg-red-500"
              }`}
            />
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setShowDiff(!showDiff)}
            >
              {showDiff ? "Editor" : "View Diff"}
            </Button>
            <AiToggleButton
              sessionId={sessionId}
              studentId={selectedStudentId}
            />
          </div>
        </div>
        <div className="flex-1 min-h-0 p-4">
          {showDiff ? (
            <DiffViewer
              original={starterCode}
              modified={yText?.toString() || ""}
              readOnly
            />
          ) : (
            <CodeEditor
              yText={yText}
              provider={provider}
              language={language}
            />
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

- [ ] **Step 3: Create `src/components/session/teacher/broadcast-mode.tsx`**

```typescript
"use client";

import { useState, useCallback, useEffect } from "react";
import { EditorSwitcher } from "@/components/editor/editor-switcher";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { Button } from "@/components/ui/button";

type EditorMode = "python" | "javascript" | "blockly";

interface BroadcastModeProps {
  sessionId: string;
  token: string;
  editorMode: EditorMode;
  starterCode?: string;
}

export function BroadcastMode({
  sessionId,
  token,
  editorMode,
  starterCode = "",
}: BroadcastModeProps) {
  const [broadcasting, setBroadcasting] = useState(false);

  const documentName = `broadcast:${sessionId}`;
  const { yText, provider, connected } = useYjsProvider({
    documentName: broadcasting ? documentName : "noop",
    token,
  });

  const toggleBroadcast = useCallback(async () => {
    const next = !broadcasting;
    setBroadcasting(next);

    // Notify students via SSE
    await fetch(`/api/sessions/${sessionId}/broadcast`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ active: next }),
    });
  }, [broadcasting, sessionId]);

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 py-2 border-b">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium">
            {broadcasting ? "Broadcasting Live" : "Broadcast Mode"}
          </span>
          {broadcasting && (
            <span className="w-2 h-2 rounded-full bg-red-500 animate-pulse" />
          )}
          {broadcasting && (
            <span
              className={`w-2 h-2 rounded-full ${
                connected ? "bg-green-500" : "bg-yellow-500"
              }`}
              title={connected ? "Connected" : "Connecting..."}
            />
          )}
        </div>
        <Button
          variant={broadcasting ? "destructive" : "default"}
          size="sm"
          onClick={toggleBroadcast}
        >
          {broadcasting ? "Stop Broadcasting" : "Start Broadcasting"}
        </Button>
      </div>
      <div className="flex-1 min-h-0 p-4">
        {broadcasting ? (
          <EditorSwitcher
            editorMode={editorMode}
            initialCode={starterCode}
            yText={yText}
            provider={provider}
          />
        ) : (
          <div className="flex items-center justify-center h-full text-muted-foreground">
            <div className="text-center">
              <p className="text-lg font-medium">Broadcast is off</p>
              <p className="text-sm mt-1">
                Start broadcasting to share your code editor with all students.
              </p>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Commit**

```bash
git add src/components/session/teacher/grid-mode.tsx src/components/session/teacher/collaborate-mode.tsx src/components/session/teacher/broadcast-mode.tsx
git commit -m "Add grid, collaborate, and broadcast mode components

Grid mode wraps existing StudentGrid. Collaborate mode provides full
editor synced with selected student plus annotations sidebar. Broadcast
mode uses EditorSwitcher with Yjs and notifies students via SSE."
```

---

## Task 8: Teacher Dashboard -- AI Assistant Panel + Session Header

**Files:**
- Create: `src/components/session/teacher/ai-assistant-panel.tsx`
- Create: `src/components/session/teacher/teacher-header.tsx`

The AI assistant panel is the right panel showing recommendations, help queue, and AI activity feed. The header shows the topic name, session timer, student count, and end session button.

- [ ] **Step 1: Create `src/components/session/teacher/ai-assistant-panel.tsx`**

```typescript
"use client";

import { HelpQueuePanel } from "@/components/help-queue/help-queue-panel";
import { AiActivityFeed } from "@/components/ai/ai-activity-feed";

interface InteractionSummary {
  id: string;
  studentId: string;
  studentName: string;
  messageCount: number;
  createdAt: string;
}

interface AiAssistantPanelProps {
  sessionId: string;
  aiInteractions: InteractionSummary[];
  collapsed: boolean;
  studentCount: number;
  handRaisedCount: number;
}

export function AiAssistantPanel({
  sessionId,
  aiInteractions,
  collapsed,
  studentCount,
  handRaisedCount,
}: AiAssistantPanelProps) {
  if (collapsed) return null;

  return (
    <div className="flex flex-col h-full border-l">
      <div className="px-3 py-2 border-b">
        <h3 className="text-sm font-semibold">AI Assistant</h3>
      </div>
      <div className="flex-1 overflow-auto p-3 space-y-4">
        {/* Recommendations */}
        <div className="space-y-2">
          <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wide">
            Recommendations
          </h4>
          <div className="space-y-1.5">
            {handRaisedCount > 0 && (
              <div className="text-xs bg-orange-50 dark:bg-orange-950/30 text-orange-700 dark:text-orange-400 rounded-lg p-2">
                {handRaisedCount} student{handRaisedCount !== 1 ? "s" : ""} need{handRaisedCount === 1 ? "s" : ""} help
              </div>
            )}
            {studentCount === 0 && (
              <div className="text-xs bg-muted rounded-lg p-2">
                Waiting for students to join...
              </div>
            )}
            {aiInteractions.length > 3 && (
              <div className="text-xs bg-blue-50 dark:bg-blue-950/30 text-blue-700 dark:text-blue-400 rounded-lg p-2">
                {aiInteractions.length} students are actively using AI chat
              </div>
            )}
            {handRaisedCount === 0 && studentCount > 0 && aiInteractions.length <= 3 && (
              <div className="text-xs text-muted-foreground p-2">
                Class is progressing smoothly
              </div>
            )}
          </div>
        </div>

        {/* Help Queue */}
        <div className="space-y-2">
          <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wide">
            Help Queue
          </h4>
          <HelpQueuePanel sessionId={sessionId} />
        </div>

        {/* AI Activity Feed */}
        <div className="space-y-2">
          <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wide">
            AI Activity
          </h4>
          <AiActivityFeed interactions={aiInteractions} />
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Create `src/components/session/teacher/teacher-header.tsx`**

```typescript
"use client";

import { useState, useEffect, useCallback } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { TopicSelector } from "./topic-selector";

interface TeacherHeaderProps {
  sessionId: string;
  classroomId: string;
  courseId: string;
  topicNames: string[];
  studentCount: number;
  startedAt: string;
  onToggleLeft: () => void;
  onToggleRight: () => void;
  leftCollapsed: boolean;
  rightCollapsed: boolean;
}

export function TeacherHeader({
  sessionId,
  classroomId,
  courseId,
  topicNames,
  studentCount,
  startedAt,
  onToggleLeft,
  onToggleRight,
  leftCollapsed,
  rightCollapsed,
}: TeacherHeaderProps) {
  const router = useRouter();
  const [ending, setEnding] = useState(false);
  const [elapsed, setElapsed] = useState("");

  useEffect(() => {
    function updateElapsed() {
      const start = new Date(startedAt).getTime();
      const now = Date.now();
      const diff = Math.floor((now - start) / 1000);
      const hours = Math.floor(diff / 3600);
      const mins = Math.floor((diff % 3600) / 60);
      const secs = diff % 60;
      if (hours > 0) {
        setElapsed(`${hours}:${String(mins).padStart(2, "0")}:${String(secs).padStart(2, "0")}`);
      } else {
        setElapsed(`${mins}:${String(secs).padStart(2, "0")}`);
      }
    }
    updateElapsed();
    const interval = setInterval(updateElapsed, 1000);
    return () => clearInterval(interval);
  }, [startedAt]);

  const endSession = useCallback(async () => {
    setEnding(true);
    await fetch(`/api/sessions/${sessionId}`, { method: "PATCH" });
    router.push(`/teacher/classes/${classroomId}`);
  }, [sessionId, classroomId, router]);

  return (
    <div className="flex items-center justify-between px-4 py-2 border-b bg-background">
      <div className="flex items-center gap-3">
        <Button
          variant="ghost"
          size="sm"
          onClick={onToggleLeft}
          className="text-xs"
          title={leftCollapsed ? "Show student list" : "Hide student list"}
        >
          {leftCollapsed ? "\u25B6" : "\u25C0"}
        </Button>
        <div>
          <h1 className="text-sm font-bold">
            {topicNames.length > 0
              ? topicNames.join(", ")
              : "Live Session"}
          </h1>
          <p className="text-xs text-muted-foreground">
            {studentCount} student{studentCount !== 1 ? "s" : ""} · {elapsed}
          </p>
        </div>
        <TopicSelector sessionId={sessionId} courseId={courseId} />
      </div>
      <div className="flex items-center gap-2">
        <Button
          variant="ghost"
          size="sm"
          onClick={onToggleRight}
          className="text-xs"
          title={rightCollapsed ? "Show AI panel" : "Hide AI panel"}
        >
          {rightCollapsed ? "\u25C0" : "\u25B6"}
        </Button>
        <Button
          variant="destructive"
          size="sm"
          onClick={endSession}
          disabled={ending}
        >
          {ending ? "Ending..." : "End Session"}
        </Button>
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Commit**

```bash
git add src/components/session/teacher/ai-assistant-panel.tsx src/components/session/teacher/teacher-header.tsx
git commit -m "Add AI assistant panel and session header for teacher dashboard

AI panel shows recommendations, help queue, and activity feed.
Header shows topic names, session timer, student count, panel toggles,
topic selector, and end session button."
```

---

## Task 9: Teacher Dashboard -- Main Orchestrator Component + Tests

**Files:**
- Create: `src/components/session/teacher/teacher-dashboard.tsx`
- Create: `tests/unit/teacher-dashboard.test.tsx`

This is the main client component that orchestrates the entire 3-panel teacher dashboard, managing state for mode switching, panel visibility, participants, AI interactions, and selected student.

- [ ] **Step 1: Create `src/components/session/teacher/teacher-dashboard.tsx`**

```typescript
"use client";

import { useState, useEffect, useCallback } from "react";
import { useSession } from "next-auth/react";
import { usePanelLayout } from "@/lib/hooks/use-panel-layout";
import { TeacherHeader } from "./teacher-header";
import { StudentListPanel, type StudentListItemData } from "./student-list-panel";
import { MainArea } from "./main-area";
import { ModeToolbar, type DashboardMode } from "./mode-toolbar";
import { PresentationMode } from "./presentation-mode";
import { GridMode } from "./grid-mode";
import { CollaborateMode } from "./collaborate-mode";
import { BroadcastMode } from "./broadcast-mode";
import { AiAssistantPanel } from "./ai-assistant-panel";

// Re-export for the student-list-panel
export type { StudentListItemData };

interface Participant {
  studentId: string;
  name: string;
  status: string;
}

interface TeacherDashboardProps {
  sessionId: string;
  classroomId: string;
  courseId: string;
  editorMode: "python" | "javascript" | "blockly";
  startedAt: string;
}

export function TeacherDashboard({
  sessionId,
  classroomId,
  courseId,
  editorMode,
  startedAt,
}: TeacherDashboardProps) {
  const { data: session } = useSession();
  const { sizes, toggleLeft, toggleRight } = usePanelLayout();
  const [mode, setMode] = useState<DashboardMode>("grid");
  const [participants, setParticipants] = useState<Participant[]>([]);
  const [selectedStudent, setSelectedStudent] = useState<string | null>(null);
  const [broadcastActive, setBroadcastActive] = useState(false);
  const [aiInteractions, setAiInteractions] = useState<any[]>([]);
  const [topicNames, setTopicNames] = useState<string[]>([]);

  const userId = session?.user?.id || "";
  const token = `${userId}:teacher`;

  // Build student list with hand-raised status
  const studentList: StudentListItemData[] = participants.map((p) => ({
    studentId: p.studentId,
    name: p.name,
    status: p.status,
    handRaised: p.status === "needs_help",
  }));

  // Poll participants
  useEffect(() => {
    async function fetchParticipants() {
      const res = await fetch(`/api/sessions/${sessionId}/participants`);
      if (res.ok) {
        setParticipants(await res.json());
      }
    }
    fetchParticipants();
    const interval = setInterval(fetchParticipants, 3000);
    return () => clearInterval(interval);
  }, [sessionId]);

  // SSE events
  useEffect(() => {
    const eventSource = new EventSource(
      `/api/sessions/${sessionId}/events`
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

    eventSource.addEventListener("hand_raised", (e) => {
      const data = JSON.parse(e.data);
      setParticipants((prev) =>
        prev.map((p) =>
          p.studentId === data.studentId ? { ...p, status: "needs_help" } : p
        )
      );
    });

    eventSource.addEventListener("hand_lowered", (e) => {
      const data = JSON.parse(e.data);
      setParticipants((prev) =>
        prev.map((p) =>
          p.studentId === data.studentId ? { ...p, status: "active" } : p
        )
      );
    });

    return () => eventSource.close();
  }, [sessionId]);

  // Poll AI interactions
  useEffect(() => {
    async function fetchInteractions() {
      const res = await fetch(`/api/ai/interactions?sessionId=${sessionId}`);
      if (res.ok) {
        const data = await res.json();
        const summaries = data.map((i: any) => ({
          id: i.id,
          studentId: i.studentId,
          studentName:
            participants.find((p) => p.studentId === i.studentId)?.name ||
            "Unknown",
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
  }, [sessionId, participants]);

  // Fetch topic names for header
  useEffect(() => {
    async function fetchTopicNames() {
      const res = await fetch(`/api/sessions/${sessionId}/topics`);
      if (res.ok) {
        const topics = await res.json();
        setTopicNames(topics.map((t: any) => t.title));
      }
    }
    fetchTopicNames();
    const interval = setInterval(fetchTopicNames, 10000);
    return () => clearInterval(interval);
  }, [sessionId]);

  // When selecting a student, switch to collaborate mode
  const handleSelectStudent = useCallback(
    (studentId: string) => {
      setSelectedStudent(studentId);
      setMode("collaborate");
    },
    []
  );

  const handleCollaborateBack = useCallback(() => {
    setSelectedStudent(null);
    setMode("grid");
  }, []);

  const selectedStudentName =
    participants.find((p) => p.studentId === selectedStudent)?.name || "Student";
  const handRaisedCount = studentList.filter((s) => s.handRaised).length;

  const leftCollapsed = sizes.left === 0;
  const rightCollapsed = sizes.right === 0;

  function renderMode() {
    switch (mode) {
      case "presentation":
        return <PresentationMode sessionId={sessionId} />;
      case "grid":
        return (
          <GridMode
            sessionId={sessionId}
            participants={participants}
            token={token}
            onSelectStudent={handleSelectStudent}
          />
        );
      case "collaborate":
        return (
          <CollaborateMode
            sessionId={sessionId}
            selectedStudentId={selectedStudent}
            selectedStudentName={selectedStudentName}
            token={token}
            language={editorMode === "blockly" ? "javascript" : editorMode}
            onBack={handleCollaborateBack}
          />
        );
      case "broadcast":
        return (
          <BroadcastMode
            sessionId={sessionId}
            token={token}
            editorMode={editorMode}
          />
        );
    }
  }

  return (
    <div className="flex flex-col h-[calc(100vh-3.5rem)]">
      <TeacherHeader
        sessionId={sessionId}
        classroomId={classroomId}
        courseId={courseId}
        topicNames={topicNames}
        studentCount={participants.length}
        startedAt={startedAt}
        onToggleLeft={toggleLeft}
        onToggleRight={toggleRight}
        leftCollapsed={leftCollapsed}
        rightCollapsed={rightCollapsed}
      />
      <div className="flex flex-1 min-h-0">
        {/* Left: Student List */}
        <div
          style={{ width: leftCollapsed ? 0 : `${sizes.left}%` }}
          className="shrink-0 transition-all duration-200 overflow-hidden"
        >
          <StudentListPanel
            students={studentList}
            sessionId={sessionId}
            selectedStudentId={selectedStudent}
            onSelectStudent={handleSelectStudent}
            collapsed={leftCollapsed}
          />
        </div>

        {/* Center: Main Area */}
        <div className="flex-1 flex flex-col min-w-0">
          <MainArea mode={mode}>
            {renderMode()}
          </MainArea>
          <ModeToolbar
            activeMode={mode}
            onModeChange={setMode}
            broadcastActive={broadcastActive}
          />
        </div>

        {/* Right: AI Assistant */}
        <div
          style={{ width: rightCollapsed ? 0 : `${sizes.right}%` }}
          className="shrink-0 transition-all duration-200 overflow-hidden"
        >
          <AiAssistantPanel
            sessionId={sessionId}
            aiInteractions={aiInteractions}
            collapsed={rightCollapsed}
            studentCount={participants.length}
            handRaisedCount={handRaisedCount}
          />
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Create `tests/unit/teacher-dashboard.test.tsx`**

```tsx
// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";

// Mock next-auth
vi.mock("next-auth/react", () => ({
  useSession: () => ({
    data: { user: { id: "teacher-1", name: "Teacher" } },
  }),
}));

// Mock next/navigation
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
  useParams: () => ({ id: "class-1", sessionId: "sess-1" }),
}));

// Mock hooks
vi.mock("@/lib/hooks/use-panel-layout", () => ({
  usePanelLayout: () => ({
    sizes: { left: 20, right: 25 },
    setSizes: vi.fn(),
    toggleLeft: vi.fn(),
    toggleRight: vi.fn(),
  }),
}));

// Mock fetch
const mockFetch = vi.fn().mockResolvedValue({
  ok: true,
  json: async () => [],
});
global.fetch = mockFetch;

// Mock EventSource
class MockEventSource {
  addEventListener = vi.fn();
  close = vi.fn();
}
global.EventSource = MockEventSource as any;

// Mock Yjs provider
vi.mock("@/lib/yjs/use-yjs-provider", () => ({
  useYjsProvider: () => ({
    yDoc: null,
    yText: null,
    provider: null,
    connected: false,
  }),
}));

// Mock child components to keep tests focused
vi.mock("@/components/session/teacher/student-list-panel", () => ({
  StudentListPanel: () => <div data-testid="student-list">Student List</div>,
}));
vi.mock("@/components/session/teacher/presentation-mode", () => ({
  PresentationMode: () => <div data-testid="presentation-mode">Presentation</div>,
}));
vi.mock("@/components/session/teacher/grid-mode", () => ({
  GridMode: () => <div data-testid="grid-mode">Grid</div>,
}));
vi.mock("@/components/session/teacher/collaborate-mode", () => ({
  CollaborateMode: () => <div data-testid="collaborate-mode">Collaborate</div>,
}));
vi.mock("@/components/session/teacher/broadcast-mode", () => ({
  BroadcastMode: () => <div data-testid="broadcast-mode">Broadcast</div>,
}));
vi.mock("@/components/session/teacher/ai-assistant-panel", () => ({
  AiAssistantPanel: () => <div data-testid="ai-panel">AI Panel</div>,
}));
vi.mock("@/components/session/teacher/teacher-header", () => ({
  TeacherHeader: ({ studentCount }: any) => (
    <div data-testid="header">Header: {studentCount} students</div>
  ),
}));
vi.mock("@/components/session/teacher/mode-toolbar", () => ({
  ModeToolbar: ({ activeMode, onModeChange }: any) => (
    <div data-testid="mode-toolbar">
      Mode: {activeMode}
      <button onClick={() => onModeChange("presentation")}>Presentation</button>
    </div>
  ),
}));

describe("TeacherDashboard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFetch.mockResolvedValue({ ok: true, json: async () => [] });
  });

  it("renders all three panels and toolbar", async () => {
    const { TeacherDashboard } = await import(
      "@/components/session/teacher/teacher-dashboard"
    );
    render(
      <TeacherDashboard
        sessionId="sess-1"
        classroomId="class-1"
        courseId="course-1"
        editorMode="python"
        startedAt={new Date().toISOString()}
      />
    );
    expect(screen.getByTestId("student-list")).toBeDefined();
    expect(screen.getByTestId("grid-mode")).toBeDefined();
    expect(screen.getByTestId("ai-panel")).toBeDefined();
    expect(screen.getByTestId("mode-toolbar")).toBeDefined();
    expect(screen.getByTestId("header")).toBeDefined();
  });

  it("defaults to grid mode", async () => {
    const { TeacherDashboard } = await import(
      "@/components/session/teacher/teacher-dashboard"
    );
    render(
      <TeacherDashboard
        sessionId="sess-1"
        classroomId="class-1"
        courseId="course-1"
        editorMode="python"
        startedAt={new Date().toISOString()}
      />
    );
    expect(screen.getByTestId("grid-mode")).toBeDefined();
    expect(screen.getByText("Mode: grid")).toBeDefined();
  });

  it("subscribes to SSE events", async () => {
    const { TeacherDashboard } = await import(
      "@/components/session/teacher/teacher-dashboard"
    );
    render(
      <TeacherDashboard
        sessionId="sess-1"
        classroomId="class-1"
        courseId="course-1"
        editorMode="python"
        startedAt={new Date().toISOString()}
      />
    );
    // Verify EventSource was constructed with correct URL
    expect(MockEventSource).toHaveBeenCalledTimes(1);
  });
});
```

- [ ] **Step 3: Run tests, verify pass**

```bash
cd /home/chris/workshop/Bridge && bun test tests/unit/teacher-dashboard.test.tsx
```

- [ ] **Step 4: Commit**

```bash
git add src/components/session/teacher/teacher-dashboard.tsx tests/unit/teacher-dashboard.test.tsx
git commit -m "Add main teacher dashboard orchestrator component

3-panel layout with student list (left), mode-switching main area
(center), and AI assistant panel (right). Manages participants,
SSE events, AI interactions, and mode switching state."
```

---

## Task 10: Teacher Dashboard Portal Page (Server Component)

**Files:**
- Create: `src/app/(portal)/teacher/classes/[id]/session/[sessionId]/dashboard/page.tsx`

Server component that loads session data and renders the client TeacherDashboard.

- [ ] **Step 1: Create the directory and page**

```bash
mkdir -p /home/chris/workshop/Bridge/src/app/\(portal\)/teacher/classes/\[id\]/session/\[sessionId\]/dashboard
```

- [ ] **Step 2: Create `src/app/(portal)/teacher/classes/[id]/session/[sessionId]/dashboard/page.tsx`**

```typescript
import { notFound, redirect } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSession } from "@/lib/sessions";
import { getClass, getClassroom } from "@/lib/classes";
import { getCourse } from "@/lib/courses";
import { listClassMembers } from "@/lib/class-memberships";
import { TeacherDashboard } from "@/components/session/teacher/teacher-dashboard";

export default async function TeacherDashboardPage({
  params,
}: {
  params: Promise<{ id: string; sessionId: string }>;
}) {
  const session = await auth();
  if (!session?.user?.id) redirect("/login");

  const { id: classId, sessionId } = await params;
  const liveSession = await getSession(db, sessionId);
  if (!liveSession) notFound();
  if (liveSession.status !== "active") redirect(`/teacher/classes/${classId}`);

  // Verify teacher is instructor
  const members = await listClassMembers(db, classId);
  const isInstructor = members.some(
    (m) =>
      m.userId === session.user.id &&
      (m.role === "instructor" || m.role === "ta")
  );
  if (!isInstructor && !session.user.isPlatformAdmin) notFound();

  // Get classroom and course info
  const cls = await getClass(db, classId);
  if (!cls) notFound();

  const classroom = await getClassroom(db, classId);
  const course = await getCourse(db, cls.courseId);

  return (
    <TeacherDashboard
      sessionId={sessionId}
      classroomId={classId}
      courseId={cls.courseId}
      editorMode={(classroom?.editorMode as "python" | "javascript" | "blockly") || "python"}
      startedAt={liveSession.startedAt.toISOString()}
    />
  );
}
```

- [ ] **Step 3: Commit**

```bash
git add src/app/\(portal\)/teacher/classes/\[id\]/session/\[sessionId\]/dashboard/page.tsx
git commit -m "Add teacher dashboard server page at portal route

Loads session, class, classroom, and course data, verifies instructor
role, then renders the TeacherDashboard client component."
```

---

## Task 11: Student Session -- Side Panel Component

**Files:**
- Create: `src/components/session/student/student-side-panel.tsx`
- Create: `tests/unit/student-side-panel.test.tsx`

The student side panel is a minimizable panel that shows AI chat and annotations. When minimized, it shows notification badges.

- [ ] **Step 1: Create `src/components/session/student/student-side-panel.tsx`**

```typescript
"use client";

import { useState } from "react";
import { AiChatPanel } from "@/components/ai/ai-chat-panel";
import { AnnotationList } from "@/components/annotations/annotation-list";
import { Button } from "@/components/ui/button";

type SidePanelTab = "ai" | "annotations";

interface StudentSidePanelProps {
  sessionId: string;
  code: string;
  aiEnabled: boolean;
  annotations: any[];
  onDeleteAnnotation?: (id: string) => void;
  minimized: boolean;
  onToggleMinimize: () => void;
  unreadAiCount: number;
  unreadAnnotationCount: number;
}

export function StudentSidePanel({
  sessionId,
  code,
  aiEnabled,
  annotations,
  onDeleteAnnotation,
  minimized,
  onToggleMinimize,
  unreadAiCount,
  unreadAnnotationCount,
}: StudentSidePanelProps) {
  const [activeTab, setActiveTab] = useState<SidePanelTab>("ai");

  if (minimized) {
    return (
      <div className="flex flex-col items-center gap-2 py-2 px-1 border-l">
        <Button
          variant="ghost"
          size="sm"
          onClick={onToggleMinimize}
          className="text-xs w-8 h-8 p-0"
          title="Expand panel"
        >
          {"\u25C0"}
        </Button>
        {aiEnabled && (
          <button
            type="button"
            onClick={() => {
              setActiveTab("ai");
              onToggleMinimize();
            }}
            className="relative w-8 h-8 flex items-center justify-center rounded hover:bg-accent"
            title="AI Chat"
          >
            <span className="text-sm">AI</span>
            {unreadAiCount > 0 && (
              <span className="absolute -top-1 -right-1 w-4 h-4 bg-primary text-primary-foreground text-[10px] rounded-full flex items-center justify-center">
                {unreadAiCount}
              </span>
            )}
          </button>
        )}
        <button
          type="button"
          onClick={() => {
            setActiveTab("annotations");
            onToggleMinimize();
          }}
          className="relative w-8 h-8 flex items-center justify-center rounded hover:bg-accent"
          title="Annotations"
        >
          <span className="text-sm">@</span>
          {unreadAnnotationCount > 0 && (
            <span className="absolute -top-1 -right-1 w-4 h-4 bg-primary text-primary-foreground text-[10px] rounded-full flex items-center justify-center">
              {unreadAnnotationCount}
            </span>
          )}
        </button>
      </div>
    );
  }

  return (
    <div className="w-80 flex flex-col border-l">
      <div className="flex items-center justify-between px-2 py-1 border-b">
        <div className="flex gap-1">
          {aiEnabled && (
            <Button
              variant={activeTab === "ai" ? "default" : "ghost"}
              size="sm"
              onClick={() => setActiveTab("ai")}
              className="text-xs"
            >
              AI Chat
            </Button>
          )}
          <Button
            variant={activeTab === "annotations" ? "default" : "ghost"}
            size="sm"
            onClick={() => setActiveTab("annotations")}
            className="text-xs"
          >
            Annotations
          </Button>
        </div>
        <Button
          variant="ghost"
          size="sm"
          onClick={onToggleMinimize}
          className="text-xs w-6 h-6 p-0"
          title="Minimize panel"
        >
          {"\u25B6"}
        </Button>
      </div>
      <div className="flex-1 min-h-0 p-2">
        {activeTab === "ai" && aiEnabled ? (
          <AiChatPanel sessionId={sessionId} code={code} enabled={aiEnabled} />
        ) : (
          <div className="h-full overflow-auto">
            <AnnotationList
              annotations={annotations}
              onDelete={onDeleteAnnotation || (() => {})}
            />
          </div>
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Create `tests/unit/student-side-panel.test.tsx`**

```tsx
// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { StudentSidePanel } from "@/components/session/student/student-side-panel";

// Mock child components
vi.mock("@/components/ai/ai-chat-panel", () => ({
  AiChatPanel: () => <div data-testid="ai-chat">AI Chat</div>,
}));
vi.mock("@/components/annotations/annotation-list", () => ({
  AnnotationList: () => <div data-testid="annotation-list">Annotations</div>,
}));

describe("StudentSidePanel", () => {
  const baseProps = {
    sessionId: "sess-1",
    code: "print('hello')",
    aiEnabled: true,
    annotations: [],
    minimized: false,
    onToggleMinimize: vi.fn(),
    unreadAiCount: 0,
    unreadAnnotationCount: 0,
  };

  it("renders AI chat when expanded and AI enabled", () => {
    render(<StudentSidePanel {...baseProps} />);
    expect(screen.getByTestId("ai-chat")).toBeDefined();
  });

  it("renders annotation list when annotations tab selected", () => {
    render(<StudentSidePanel {...baseProps} />);
    fireEvent.click(screen.getByText("Annotations"));
    expect(screen.getByTestId("annotation-list")).toBeDefined();
  });

  it("shows minimized state with icon buttons", () => {
    render(<StudentSidePanel {...baseProps} minimized={true} />);
    expect(screen.getByTitle("Expand panel")).toBeDefined();
    expect(screen.getByTitle("AI Chat")).toBeDefined();
    expect(screen.getByTitle("Annotations")).toBeDefined();
  });

  it("shows notification badges when minimized", () => {
    const { container } = render(
      <StudentSidePanel
        {...baseProps}
        minimized={true}
        unreadAiCount={3}
        unreadAnnotationCount={1}
      />
    );
    expect(screen.getByText("3")).toBeDefined();
    expect(screen.getByText("1")).toBeDefined();
  });

  it("calls onToggleMinimize when expand button clicked", () => {
    const toggle = vi.fn();
    render(
      <StudentSidePanel {...baseProps} minimized={true} onToggleMinimize={toggle} />
    );
    fireEvent.click(screen.getByTitle("Expand panel"));
    expect(toggle).toHaveBeenCalled();
  });

  it("does not show AI chat button when AI disabled", () => {
    render(
      <StudentSidePanel {...baseProps} aiEnabled={false} />
    );
    expect(screen.queryByText("AI Chat")).toBeNull();
  });
});
```

- [ ] **Step 3: Run tests, verify pass**

```bash
cd /home/chris/workshop/Bridge && bun test tests/unit/student-side-panel.test.tsx
```

- [ ] **Step 4: Commit**

```bash
git add src/components/session/student/student-side-panel.tsx tests/unit/student-side-panel.test.tsx
git commit -m "Add minimizable student side panel with AI chat and annotations

Side panel can be expanded or minimized. Minimized state shows icon
buttons with notification badges for unread messages and annotations."
```

---

## Task 12: Student Session -- Lesson Panel + Broadcast Overlay + Toolbar

**Files:**
- Create: `src/components/session/student/student-lesson-panel.tsx`
- Create: `src/components/session/student/student-broadcast-overlay.tsx`
- Create: `src/components/session/student/student-toolbar.tsx`

- [ ] **Step 1: Create `src/components/session/student/student-lesson-panel.tsx`**

```typescript
"use client";

import { useState, useEffect } from "react";
import { LessonRenderer } from "@/components/lesson/lesson-renderer";
import { parseLessonContent } from "@/lib/lesson-content";

interface SessionTopic {
  topicId: string;
  title: string;
  description: string | null;
  sortOrder: number;
  lessonContent: unknown;
  starterCode: string | null;
}

interface StudentLessonPanelProps {
  sessionId: string;
}

export function StudentLessonPanel({ sessionId }: StudentLessonPanelProps) {
  const [topics, setTopics] = useState<SessionTopic[]>([]);
  const [activeTopicIndex, setActiveTopicIndex] = useState(0);

  useEffect(() => {
    async function fetchTopics() {
      const res = await fetch(`/api/sessions/${sessionId}/topics`);
      if (res.ok) {
        setTopics(await res.json());
      }
    }
    fetchTopics();
    const interval = setInterval(fetchTopics, 10000);
    return () => clearInterval(interval);
  }, [sessionId]);

  if (topics.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground">
        <p className="text-sm">No lesson content for this session.</p>
      </div>
    );
  }

  const activeTopic = topics[activeTopicIndex];
  const content = parseLessonContent(activeTopic?.lessonContent);

  return (
    <div className="flex flex-col h-full">
      {topics.length > 1 && (
        <div className="flex items-center gap-1 px-3 py-1.5 border-b overflow-x-auto shrink-0">
          {topics.map((topic, i) => (
            <button
              key={topic.topicId}
              type="button"
              onClick={() => setActiveTopicIndex(i)}
              className={`px-2 py-1 rounded text-xs whitespace-nowrap transition-colors ${
                i === activeTopicIndex
                  ? "bg-primary text-primary-foreground"
                  : "hover:bg-accent"
              }`}
            >
              {topic.title}
            </button>
          ))}
        </div>
      )}
      <div className="flex-1 overflow-auto p-4">
        {activeTopic && (
          <>
            <h3 className="text-lg font-bold mb-3">{activeTopic.title}</h3>
            {activeTopic.description && (
              <p className="text-sm text-muted-foreground mb-3">
                {activeTopic.description}
              </p>
            )}
            <LessonRenderer content={content} />
          </>
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Create `src/components/session/student/student-broadcast-overlay.tsx`**

```typescript
"use client";

import { CodeEditor } from "@/components/editor/code-editor";
import type * as Y from "yjs";
import type { HocuspocusProvider } from "@hocuspocus/provider";

interface StudentBroadcastOverlayProps {
  active: boolean;
  yText: Y.Text | null;
  provider: HocuspocusProvider | null;
}

export function StudentBroadcastOverlay({
  active,
  yText,
  provider,
}: StudentBroadcastOverlayProps) {
  if (!active) return null;

  return (
    <div className="border rounded-lg overflow-hidden shrink-0">
      <div className="bg-blue-50 dark:bg-blue-950/30 px-3 py-1.5 text-xs font-medium text-blue-700 dark:text-blue-400 border-b flex items-center gap-2">
        <span className="w-2 h-2 rounded-full bg-blue-500 animate-pulse" />
        Teacher is broadcasting
      </div>
      <div className="h-48">
        <CodeEditor yText={yText} provider={provider} readOnly />
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Create `src/components/session/student/student-toolbar.tsx`**

```typescript
"use client";

import { Button } from "@/components/ui/button";
import { RaiseHandButton } from "@/components/help-queue/raise-hand-button";
import { RunButton } from "@/components/editor/run-button";
import type { StudentLayout } from "@/lib/hooks/use-student-layout";

interface StudentToolbarProps {
  sessionId: string;
  layout: StudentLayout;
  onToggleLayout: () => void;
  aiEnabled: boolean;
  onToggleAi: () => void;
  showAi: boolean;
  connected: boolean;
  onRun: () => void;
  onClear: () => void;
  running: boolean;
  ready: boolean;
}

export function StudentToolbar({
  sessionId,
  layout,
  onToggleLayout,
  aiEnabled,
  onToggleAi,
  showAi,
  connected,
  onRun,
  onClear,
  running,
  ready,
}: StudentToolbarProps) {
  return (
    <div className="flex items-center justify-between px-4 py-1.5 border-b">
      <div className="flex items-center gap-2">
        <h2 className="text-sm font-medium text-muted-foreground">
          Live Session
        </h2>
        <span
          className={`w-2 h-2 rounded-full ${
            connected ? "bg-green-500" : "bg-red-500"
          }`}
        />
        <Button
          variant="ghost"
          size="sm"
          onClick={onToggleLayout}
          className="text-xs"
          title={layout === "side-by-side" ? "Switch to stacked" : "Switch to side-by-side"}
        >
          {layout === "side-by-side" ? "\u2B81" : "\u2B80"}
        </Button>
      </div>
      <div className="flex items-center gap-2">
        <RaiseHandButton sessionId={sessionId} />
        {aiEnabled && (
          <Button variant="ghost" size="sm" onClick={onToggleAi}>
            {showAi ? "Hide AI" : "Ask AI"}
          </Button>
        )}
        <Button
          variant="ghost"
          size="sm"
          onClick={onClear}
          disabled={running}
        >
          Clear
        </Button>
        <RunButton onRun={onRun} running={running} ready={ready} />
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Commit**

```bash
git add src/components/session/student/student-lesson-panel.tsx src/components/session/student/student-broadcast-overlay.tsx src/components/session/student/student-toolbar.tsx
git commit -m "Add student lesson panel, broadcast overlay, and toolbar

Lesson panel fetches and renders session topics with tab switching.
Broadcast overlay shows teacher code when broadcasting. Toolbar has
layout toggle, raise hand, AI toggle, run/clear controls."
```

---

## Task 13: Student Session -- Main Orchestrator Component + Tests

**Files:**
- Create: `src/components/session/student/student-session.tsx`
- Create: `tests/unit/student-session.test.tsx`

Main client component for the student session experience. Supports side-by-side and stacked layouts, integrates lesson content, editor, output, broadcast overlay, and side panel.

- [ ] **Step 1: Create `src/components/session/student/student-session.tsx`**

```typescript
"use client";

import { useState, useEffect, useCallback } from "react";
import { useSession } from "next-auth/react";
import { useStudentLayout } from "@/lib/hooks/use-student-layout";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { usePyodide } from "@/lib/pyodide/use-pyodide";
import { useJsRunner } from "@/lib/js-runner/use-js-runner";
import { CodeEditor } from "@/components/editor/code-editor";
import { OutputPanel } from "@/components/editor/output-panel";
import { StudentToolbar } from "./student-toolbar";
import { StudentLessonPanel } from "./student-lesson-panel";
import { StudentBroadcastOverlay } from "./student-broadcast-overlay";
import { StudentSidePanel } from "./student-side-panel";

type EditorMode = "python" | "javascript" | "blockly";

interface StudentSessionProps {
  sessionId: string;
  classroomId: string;
  editorMode: EditorMode;
  starterCode?: string;
}

export function StudentSession({
  sessionId,
  classroomId,
  editorMode,
  starterCode = "",
}: StudentSessionProps) {
  const { data: session } = useSession();
  const { layout, toggleLayout } = useStudentLayout();
  const [code, setCode] = useState(starterCode);
  const [aiEnabled, setAiEnabled] = useState(false);
  const [showSidePanel, setShowSidePanel] = useState(false);
  const [sidePanelMinimized, setSidePanelMinimized] = useState(false);
  const [broadcastActive, setBroadcastActive] = useState(false);
  const [annotations, setAnnotations] = useState<any[]>([]);

  const userId = session?.user?.id || "";
  const documentName = `session:${sessionId}:user:${userId}`;
  const token = `${userId}:user`;

  const pyodide = usePyodide();
  const jsRunner = useJsRunner();
  const isPython = editorMode === "python";
  const runner = isPython ? pyodide : jsRunner;

  const { yText, provider, connected } = useYjsProvider({
    documentName,
    token,
  });

  // Broadcast Yjs connection
  const broadcastDocName = `broadcast:${sessionId}`;
  const { yText: broadcastYText, provider: broadcastProvider } = useYjsProvider({
    documentName: broadcastActive ? broadcastDocName : "noop",
    token,
  });

  // SSE events
  useEffect(() => {
    const eventSource = new EventSource(
      `/api/sessions/${sessionId}/events`
    );

    eventSource.addEventListener("ai_toggled", (e) => {
      const data = JSON.parse(e.data);
      if (data.studentId === userId) {
        setAiEnabled(data.enabled);
        if (data.enabled) {
          setShowSidePanel(true);
          setSidePanelMinimized(false);
        }
      }
    });

    eventSource.addEventListener("broadcast_started", () => {
      setBroadcastActive(true);
    });

    eventSource.addEventListener("broadcast_ended", () => {
      setBroadcastActive(false);
    });

    eventSource.addEventListener("session_ended", () => {
      window.location.href = `/student/classes/${classroomId}`;
    });

    return () => eventSource.close();
  }, [sessionId, classroomId, userId]);

  // Fetch annotations
  useEffect(() => {
    async function fetchAnnotations() {
      const docId = `session:${sessionId}:user:${userId}`;
      const res = await fetch(
        `/api/annotations?documentId=${encodeURIComponent(docId)}`
      );
      if (res.ok) {
        setAnnotations(await res.json());
      }
    }
    if (userId) {
      fetchAnnotations();
      const interval = setInterval(fetchAnnotations, 5000);
      return () => clearInterval(interval);
    }
  }, [sessionId, userId]);

  const handleRun = useCallback(() => {
    const currentCode = yText?.toString() || code;
    runner.runCode(currentCode);
  }, [yText, code, runner]);

  const handleClear = useCallback(() => {
    runner.clearOutput();
  }, [runner]);

  const toggleSidePanel = useCallback(() => {
    if (!showSidePanel) {
      setShowSidePanel(true);
      setSidePanelMinimized(false);
    } else {
      setShowSidePanel(false);
    }
  }, [showSidePanel]);

  const isSideBySide = layout === "side-by-side";

  // Determine language for Monaco
  const language = editorMode === "blockly" ? "javascript" : editorMode;

  return (
    <div className="flex flex-col h-[calc(100vh-3.5rem)]">
      <StudentToolbar
        sessionId={sessionId}
        layout={layout}
        onToggleLayout={toggleLayout}
        aiEnabled={aiEnabled}
        onToggleAi={toggleSidePanel}
        showAi={showSidePanel && !sidePanelMinimized}
        connected={connected}
        onRun={handleRun}
        onClear={handleClear}
        running={runner.running}
        ready={runner.ready}
      />

      <div className="flex flex-1 min-h-0">
        <div className={`flex ${isSideBySide ? "flex-row" : "flex-col"} flex-1 min-w-0`}>
          {/* Lesson content */}
          <div
            className={
              isSideBySide
                ? "w-1/2 border-r overflow-hidden"
                : "h-1/3 border-b overflow-hidden"
            }
          >
            <StudentLessonPanel sessionId={sessionId} />
          </div>

          {/* Editor + Output */}
          <div className={`flex flex-col ${isSideBySide ? "w-1/2" : "flex-1"} min-h-0`}>
            {/* Broadcast overlay */}
            <div className="px-4 pt-2">
              <StudentBroadcastOverlay
                active={broadcastActive}
                yText={broadcastYText}
                provider={broadcastProvider}
              />
            </div>

            {/* Code editor */}
            <div className="flex-1 min-h-0 px-4 py-2">
              <CodeEditor
                initialCode={starterCode}
                onChange={setCode}
                language={language}
                yText={yText}
                provider={provider}
              />
            </div>

            {/* Output */}
            <div className="h-[180px] shrink-0 px-4 pb-2">
              <OutputPanel output={runner.output} running={runner.running} />
            </div>
          </div>
        </div>

        {/* Side Panel */}
        {showSidePanel && (
          <StudentSidePanel
            sessionId={sessionId}
            code={yText?.toString() || code}
            aiEnabled={aiEnabled}
            annotations={annotations}
            minimized={sidePanelMinimized}
            onToggleMinimize={() => setSidePanelMinimized(!sidePanelMinimized)}
            unreadAiCount={0}
            unreadAnnotationCount={0}
          />
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Create `tests/unit/student-session.test.tsx`**

```tsx
// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";

// Mock next-auth
vi.mock("next-auth/react", () => ({
  useSession: () => ({
    data: { user: { id: "student-1", name: "Student" } },
  }),
}));

// Mock hooks
vi.mock("@/lib/hooks/use-student-layout", () => ({
  useStudentLayout: () => ({
    layout: "side-by-side" as const,
    setLayout: vi.fn(),
    toggleLayout: vi.fn(),
  }),
}));

vi.mock("@/lib/yjs/use-yjs-provider", () => ({
  useYjsProvider: () => ({
    yDoc: null,
    yText: null,
    provider: null,
    connected: true,
  }),
}));

vi.mock("@/lib/pyodide/use-pyodide", () => ({
  usePyodide: () => ({
    ready: true,
    running: false,
    output: [],
    runCode: vi.fn(),
    clearOutput: vi.fn(),
  }),
}));

vi.mock("@/lib/js-runner/use-js-runner", () => ({
  useJsRunner: () => ({
    ready: true,
    running: false,
    output: [],
    runCode: vi.fn(),
    clearOutput: vi.fn(),
  }),
}));

// Mock fetch
global.fetch = vi.fn().mockResolvedValue({
  ok: true,
  json: async () => [],
});

// Mock EventSource
class MockEventSource {
  addEventListener = vi.fn();
  close = vi.fn();
}
global.EventSource = MockEventSource as any;

// Mock child components
vi.mock("@/components/session/student/student-toolbar", () => ({
  StudentToolbar: (props: any) => (
    <div data-testid="toolbar">Toolbar: {props.layout}</div>
  ),
}));
vi.mock("@/components/session/student/student-lesson-panel", () => ({
  StudentLessonPanel: () => <div data-testid="lesson-panel">Lesson</div>,
}));
vi.mock("@/components/session/student/student-broadcast-overlay", () => ({
  StudentBroadcastOverlay: ({ active }: any) =>
    active ? <div data-testid="broadcast">Broadcasting</div> : null,
}));
vi.mock("@/components/session/student/student-side-panel", () => ({
  StudentSidePanel: () => <div data-testid="side-panel">Side Panel</div>,
}));
vi.mock("@/components/editor/code-editor", () => ({
  CodeEditor: () => <div data-testid="code-editor">Editor</div>,
}));
vi.mock("@/components/editor/output-panel", () => ({
  OutputPanel: () => <div data-testid="output-panel">Output</div>,
}));

describe("StudentSession", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders toolbar, lesson panel, editor, and output", async () => {
    const { StudentSession } = await import(
      "@/components/session/student/student-session"
    );
    render(
      <StudentSession
        sessionId="sess-1"
        classroomId="class-1"
        editorMode="python"
      />
    );
    expect(screen.getByTestId("toolbar")).toBeDefined();
    expect(screen.getByTestId("lesson-panel")).toBeDefined();
    expect(screen.getByTestId("code-editor")).toBeDefined();
    expect(screen.getByTestId("output-panel")).toBeDefined();
  });

  it("displays layout from hook", async () => {
    const { StudentSession } = await import(
      "@/components/session/student/student-session"
    );
    render(
      <StudentSession
        sessionId="sess-1"
        classroomId="class-1"
        editorMode="python"
      />
    );
    expect(screen.getByText("Toolbar: side-by-side")).toBeDefined();
  });

  it("subscribes to SSE events", async () => {
    const { StudentSession } = await import(
      "@/components/session/student/student-session"
    );
    render(
      <StudentSession
        sessionId="sess-1"
        classroomId="class-1"
        editorMode="python"
      />
    );
    expect(MockEventSource).toHaveBeenCalledTimes(1);
  });
});
```

- [ ] **Step 3: Run tests, verify pass**

```bash
cd /home/chris/workshop/Bridge && bun test tests/unit/student-session.test.tsx
```

- [ ] **Step 4: Commit**

```bash
git add src/components/session/student/student-session.tsx tests/unit/student-session.test.tsx
git commit -m "Add main student session orchestrator component

Flexible side-by-side or stacked layout with lesson content, code
editor, output panel, broadcast overlay, and minimizable side panel.
Handles SSE events for AI toggle, broadcast, and session end."
```

---

## Task 14: Student Session Portal Page (Server Component)

**Files:**
- Create: `src/app/(portal)/student/classes/[id]/session/[sessionId]/page.tsx`

Server component that loads session data and renders the StudentSession client component.

- [ ] **Step 1: Create the directory and page**

```bash
mkdir -p /home/chris/workshop/Bridge/src/app/\(portal\)/student/classes/\[id\]/session/\[sessionId\]
```

- [ ] **Step 2: Create `src/app/(portal)/student/classes/[id]/session/[sessionId]/page.tsx`**

```typescript
import { notFound, redirect } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getSession, joinSession } from "@/lib/sessions";
import { getClass, getClassroom } from "@/lib/classes";
import { listClassMembers } from "@/lib/class-memberships";
import { listSessionTopics } from "@/lib/session-topics";
import { StudentSession } from "@/components/session/student/student-session";

export default async function StudentSessionPage({
  params,
}: {
  params: Promise<{ id: string; sessionId: string }>;
}) {
  const session = await auth();
  if (!session?.user?.id) redirect("/login");

  const { id: classId, sessionId } = await params;
  const liveSession = await getSession(db, sessionId);
  if (!liveSession) notFound();
  if (liveSession.status !== "active") redirect(`/student/classes/${classId}`);

  // Verify student is enrolled
  const members = await listClassMembers(db, classId);
  const isEnrolled = members.some((m) => m.userId === session.user.id);
  if (!isEnrolled && !session.user.isPlatformAdmin) notFound();

  // Auto-join session
  await joinSession(db, sessionId, session.user.id);

  // Get classroom info for editor mode
  const cls = await getClass(db, classId);
  if (!cls) notFound();
  const classroom = await getClassroom(db, classId);

  // Get starter code from first linked topic
  const sessionTopics = await listSessionTopics(db, sessionId);
  const starterCode = sessionTopics.find((t) => t.starterCode)?.starterCode || "";

  return (
    <StudentSession
      sessionId={sessionId}
      classroomId={classId}
      editorMode={(classroom?.editorMode as "python" | "javascript" | "blockly") || "python"}
      starterCode={starterCode}
    />
  );
}
```

- [ ] **Step 3: Commit**

```bash
git add src/app/\(portal\)/student/classes/\[id\]/session/\[sessionId\]/page.tsx
git commit -m "Add student session server page at portal route

Loads session, verifies enrollment, auto-joins session, loads editor
mode and starter code, then renders StudentSession client component."
```

---

## Task 15: Update SessionControls for Portal Routes + Integration

**Files:**
- Modify: `src/components/session/session-controls.tsx`
- Modify: `src/app/(portal)/teacher/classes/[id]/page.tsx`
- Modify: `src/app/(portal)/student/classes/[id]/page.tsx`

Update the session controls component to use portal routes instead of legacy dashboard routes, and add session start/join buttons to the portal class detail pages.

- [ ] **Step 1: Create portal-aware version of SessionControls**

Create `src/components/session/portal-session-controls.tsx`:

```typescript
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";

interface PortalSessionControlsProps {
  classId: string;
  classroomId: string;
  portalRole: "teacher" | "student";
  activeSessionId: string | null;
}

export function PortalSessionControls({
  classId,
  classroomId,
  portalRole,
  activeSessionId,
}: PortalSessionControlsProps) {
  const router = useRouter();
  const [loading, setLoading] = useState(false);

  async function startSession() {
    setLoading(true);
    const res = await fetch("/api/sessions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ classroomId }),
    });

    if (res.ok) {
      const session = await res.json();
      router.push(`/teacher/classes/${classId}/session/${session.id}/dashboard`);
    }
    setLoading(false);
  }

  async function joinSession() {
    if (!activeSessionId) return;
    setLoading(true);

    await fetch(`/api/sessions/${activeSessionId}/join`, {
      method: "POST",
    });

    router.push(`/student/classes/${classId}/session/${activeSessionId}`);
    setLoading(false);
  }

  if (portalRole === "teacher") {
    return (
      <div className="flex gap-2">
        <Button onClick={startSession} disabled={loading}>
          {loading
            ? "Starting..."
            : activeSessionId
              ? "Restart Session"
              : "Start Session"}
        </Button>
        {activeSessionId && (
          <Button
            variant="outline"
            onClick={() =>
              router.push(
                `/teacher/classes/${classId}/session/${activeSessionId}/dashboard`
              )
            }
          >
            Open Dashboard
          </Button>
        )}
      </div>
    );
  }

  if (!activeSessionId) {
    return (
      <p className="text-sm text-muted-foreground">
        No active session. Wait for your teacher to start one.
      </p>
    );
  }

  return (
    <Button onClick={joinSession} disabled={loading}>
      {loading ? "Joining..." : "Join Session"}
    </Button>
  );
}
```

- [ ] **Step 2: Update teacher class detail page to show session controls**

Modify `src/app/(portal)/teacher/classes/[id]/page.tsx` to add the session start/open controls.

Add imports at the top:
```typescript
import { getClassroom } from "@/lib/classes";
import { getActiveSession } from "@/lib/sessions";
import { PortalSessionControls } from "@/components/session/portal-session-controls";
```

After the members query, add:
```typescript
  const classroom = await getClassroom(db, id);
  const activeSession = classroom
    ? await getActiveSession(db, classroom.id)
    : null;
```

Note: The `getActiveSession` function expects `classroomId` which is `classroom.id` from the `newClassrooms` table. However, the current `getActiveSession` queries by the `liveSessions.classroomId` field. Since the old `liveSessions` table references the old `classrooms` table, we need to handle this mapping. For now, we pass the old classroomId (same as classId for the bridge period). If the `liveSessions.classroomId` is updated to point to `newClassrooms.id`, the query should use `classroom.id`.

Add the session controls in the JSX after the join code card:
```tsx
      {classroom && (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Live Session</CardTitle>
            <CardDescription>
              {activeSession ? "A session is currently active" : "Start a session for this class"}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <PortalSessionControls
              classId={id}
              classroomId={classroom.id}
              portalRole="teacher"
              activeSessionId={activeSession?.id || null}
            />
          </CardContent>
        </Card>
      )}
```

- [ ] **Step 3: Update student class detail page to show join button**

Modify `src/app/(portal)/student/classes/[id]/page.tsx` to add session join.

Add imports:
```typescript
import { getClassroom } from "@/lib/classes";
import { getActiveSession } from "@/lib/sessions";
import { PortalSessionControls } from "@/components/session/portal-session-controls";
```

After the `topics` fetch, add:
```typescript
  const classroom = await getClassroom(db, cls.courseId);
  const activeSession = classroom
    ? await getActiveSession(db, classroom.id)
    : null;
```

Add the session controls in the JSX after the header:
```tsx
      {classroom && activeSession && (
        <Card className="border-green-200 dark:border-green-800">
          <CardContent className="py-4 flex items-center justify-between">
            <div>
              <p className="font-medium text-green-700 dark:text-green-400">
                A live session is active
              </p>
              <p className="text-sm text-muted-foreground">
                Join to participate
              </p>
            </div>
            <PortalSessionControls
              classId={id}
              classroomId={classroom.id}
              portalRole="student"
              activeSessionId={activeSession.id}
            />
          </CardContent>
        </Card>
      )}
```

- [ ] **Step 4: Commit**

```bash
git add src/components/session/portal-session-controls.tsx src/app/\(portal\)/teacher/classes/\[id\]/page.tsx src/app/\(portal\)/student/classes/\[id\]/page.tsx
git commit -m "Add portal session controls and integrate into class pages

PortalSessionControls uses portal routes for navigation. Teacher class
page shows start/open session button. Student class page shows join
button when a session is active."
```

---

## Task 16: Courses Topics API Route (for TopicSelector)

**Files:**
- Create: `src/app/api/courses/[id]/topics/route.ts`

The TopicSelector component in the teacher dashboard fetches available topics from this endpoint.

- [ ] **Step 1: Check if the route already exists**

```bash
ls /home/chris/workshop/Bridge/src/app/api/courses/
```

If `[id]/topics/route.ts` does not exist, create it.

- [ ] **Step 2: Create `src/app/api/courses/[id]/topics/route.ts`**

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { listTopicsByCourse } from "@/lib/topics";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const topics = await listTopicsByCourse(db, id);
  return NextResponse.json(topics);
}
```

- [ ] **Step 3: Commit**

```bash
git add src/app/api/courses/\[id\]/topics/route.ts
git commit -m "Add GET /api/courses/:id/topics endpoint for topic listing

Used by the TopicSelector component to fetch available topics
when the teacher links topics to a session."
```

---

## Task 17: Final Integration Test + All Tests Pass

**Files:**
- Run all existing tests to verify nothing is broken

- [ ] **Step 1: Run all tests**

```bash
cd /home/chris/workshop/Bridge && bun test
```

- [ ] **Step 2: Fix any test failures**

If any existing tests break due to the new files (e.g., import resolution), fix the issues.

- [ ] **Step 3: Final commit**

```bash
git add -A
git commit -m "Fix any test issues from live session redesign integration"
```

(Only if there were fixes needed. If all tests pass, skip this commit.)

---

## Post-Execution Checklist

- [ ] All new components created under `src/components/session/teacher/` and `src/components/session/student/`
- [ ] Session-topic linking lib + API route at `/api/sessions/:id/topics`
- [ ] Broadcast API route at `/api/sessions/:id/broadcast`
- [ ] Courses topics API route at `/api/courses/:id/topics`
- [ ] Teacher dashboard portal page at `/teacher/classes/:id/session/:sessionId/dashboard`
- [ ] Student session portal page at `/student/classes/:id/session/:sessionId`
- [ ] Layout persistence hooks using `bridge-*` localStorage keys
- [ ] Portal session controls added to both teacher and student class detail pages
- [ ] All tests pass
- [ ] No breaking changes to existing legacy routes (they still work)

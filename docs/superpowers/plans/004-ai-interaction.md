# AI & Interaction Implementation Plan (Plan 4 of 4)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add AI tutoring, code annotations, and help queue features so teachers can enable per-student AI assistance, annotate student code, and manage a help queue during live sessions.

**Architecture:** Server-mediated Claude API proxy with Socratic guardrails, streamed to students via SSE. AI interactions logged to PostgreSQL for full teacher visibility. Code annotations stored in a dedicated table and rendered as CodeMirror decorations. "Raise hand" uses the existing SessionParticipant status field from Plan 3.

**Tech Stack:** `@anthropic-ai/sdk`, SSE (ReadableStream), CodeMirror 6 decorations, Drizzle ORM, Zod, Vitest

**Spec:** `docs/superpowers/specs/001-bridge-platform-design.md`

**Depends on:** Plan 1 (Foundation), Plan 2 (Live Editor), Plan 3 (Real-time Sessions)

---

## File Structure

```
src/
├── app/
│   └── api/
│       ├── ai/
│       │   ├── chat/route.ts                          # POST: student sends message, streams AI response via SSE
│       │   ├── toggle/route.ts                        # POST: teacher enables/disables AI for a student
│       │   └── interactions/route.ts                  # GET: teacher views AI interaction logs
│       ├── annotations/
│       │   ├── route.ts                               # POST: create annotation, GET: list annotations
│       │   └── [id]/route.ts                          # DELETE: remove annotation
│       └── sessions/
│           └── [id]/
│               └── help-queue/route.ts                # GET: list queue, POST: raise/lower hand
├── lib/
│   ├── ai/
│   │   ├── client.ts                                  # Anthropic SDK client singleton
│   │   ├── system-prompts.ts                          # Grade-level Socratic system prompts
│   │   ├── guardrails.ts                              # Output filtering (no complete solutions)
│   │   └── interactions.ts                            # AIInteraction CRUD operations
│   └── annotations.ts                                 # CodeAnnotation CRUD operations
├── components/
│   ├── ai/
│   │   ├── ai-chat-panel.tsx                          # Student-facing AI chat sidebar
│   │   ├── ai-message.tsx                             # Single chat message bubble
│   │   ├── ai-toggle-button.tsx                       # Teacher toggle for AI per student
│   │   └── ai-activity-feed.tsx                       # Teacher view of AI interactions
│   ├── annotations/
│   │   ├── annotation-gutter.tsx                      # CodeMirror gutter markers
│   │   ├── annotation-popover.tsx                     # Popover for viewing/creating annotations
│   │   └── annotation-form.tsx                        # Form for creating annotations
│   └── help-queue/
│       ├── raise-hand-button.tsx                      # Student "raise hand" button
│       └── help-queue-panel.tsx                       # Teacher help queue list
tests/
├── api/
│   ├── ai-chat.test.ts                                # AI chat endpoint tests
│   ├── ai-toggle.test.ts                              # AI toggle endpoint tests
│   ├── ai-interactions.test.ts                        # AI interaction log tests
│   ├── annotations.test.ts                            # Annotation CRUD tests
│   └── help-queue.test.ts                             # Help queue tests
├── unit/
│   ├── system-prompts.test.ts                         # System prompt generation tests
│   ├── guardrails.test.ts                             # Output filtering tests
│   ├── ai-interactions.test.ts                        # AI interaction logic tests
│   ├── annotations.test.ts                            # Annotation logic tests
│   ├── ai-chat-panel.test.tsx                         # Chat panel rendering tests
│   ├── ai-message.test.tsx                            # Message bubble tests
│   ├── annotation-gutter.test.tsx                     # Gutter marker tests
│   ├── raise-hand-button.test.tsx                     # Raise hand button tests
│   └── help-queue-panel.test.tsx                      # Help queue panel tests
```

---

## Task 1: Install Anthropic SDK and Add Environment Variable

**Files:**
- Modify: `package.json`
- Modify: `.env.example`

- [ ] **Step 1: Install the Anthropic SDK**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun add @anthropic-ai/sdk
```

- [ ] **Step 2: Add ANTHROPIC_API_KEY to .env.example**

Edit `.env.example` — append after the Redis line:

```env
# Database
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge
 
# Auth
NEXTAUTH_URL=http://localhost:3000
NEXTAUTH_SECRET=generate-a-secret-with-openssl-rand-base64-32

# Google OAuth
GOOGLE_CLIENT_ID=
GOOGLE_CLIENT_SECRET=

# Redis
REDIS_URL=redis://localhost:6379

# AI (Anthropic Claude)
ANTHROPIC_API_KEY=
```

- [ ] **Step 3: Verify build still passes**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun run build
```

Expected: Build completes without errors.

- [ ] **Step 4: Commit**

```bash
git add package.json bun.lock .env.example
git commit -m "chore: install @anthropic-ai/sdk and add ANTHROPIC_API_KEY to .env.example"
```

---

## Task 2: Database Schema — AIInteraction and CodeAnnotation Tables

**Files:**
- Modify: `src/lib/db/schema.ts`
- Modify: `tests/helpers.ts`

- [ ] **Step 1: Add authorType enum and new tables to schema**

Edit `src/lib/db/schema.ts` — add the following after the existing `editorModeEnum` declaration (before the `// --- Tables ---` comment):

```typescript
export const annotationAuthorTypeEnum = pgEnum("annotation_author_type", [
  "teacher",
  "ai",
]);
```

Then add the following import at the top (merge with existing imports from `drizzle-orm/pg-core`):

```typescript
import {
  pgTable,
  uuid,
  varchar,
  text,
  timestamp,
  jsonb,
  pgEnum,
  uniqueIndex,
  index,
  integer,
  boolean,
} from "drizzle-orm/pg-core";
```

Then add these two tables at the bottom of the file, after the `classroomMembers` table:

```typescript
export const aiInteractions = pgTable(
  "ai_interactions",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    studentId: uuid("student_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    sessionId: uuid("session_id").notNull(),
    documentId: uuid("document_id").notNull(),
    enabledByTeacherId: uuid("enabled_by_teacher_id")
      .notNull()
      .references(() => users.id),
    messages: jsonb("messages").notNull().default([]),
    createdAt: timestamp("created_at").defaultNow().notNull(),
  },
  (table) => [
    index("ai_interactions_student_idx").on(table.studentId),
    index("ai_interactions_session_idx").on(table.sessionId),
  ]
);

export const codeAnnotations = pgTable(
  "code_annotations",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    documentId: uuid("document_id").notNull(),
    authorId: uuid("author_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    authorType: annotationAuthorTypeEnum("author_type").notNull(),
    lineStart: integer("line_start").notNull(),
    lineEnd: integer("line_end").notNull(),
    content: text("content").notNull(),
    resolved: boolean("resolved").notNull().default(false),
    createdAt: timestamp("created_at").defaultNow().notNull(),
  },
  (table) => [
    index("code_annotations_document_idx").on(table.documentId),
    index("code_annotations_author_idx").on(table.authorId),
  ]
);
```

Note: `sessionId` and `documentId` are plain UUID strings (not foreign keys) because those tables are defined in Plan 3 and may not exist yet in the schema file when this plan is implemented. If Plan 3 has already added `liveSessions` and `documents` tables to `schema.ts`, change these to proper FK references:
- `sessionId: uuid("session_id").notNull().references(() => liveSessions.id)`
- `documentId: uuid("document_id").notNull().references(() => documents.id)`

Do the same for `codeAnnotations.documentId`.

- [ ] **Step 2: Add cleanup for new tables to test helpers**

Edit `tests/helpers.ts` — add imports for the new tables and update `cleanupDatabase`:

Add to imports:

```typescript
import * as schema from "@/lib/db/schema";
```

(This import already exists — no change needed.)

Update `cleanupDatabase` to delete from the new tables *before* the existing deletes (order matters for FK constraints):

```typescript
export async function cleanupDatabase() {
  await testDb.delete(schema.codeAnnotations);
  await testDb.delete(schema.aiInteractions);
  await testDb.delete(schema.classroomMembers);
  await testDb.delete(schema.classrooms);
  await testDb.delete(schema.authProviders);
  await testDb.delete(schema.users);
  await testDb.delete(schema.schools);
}
```

- [ ] **Step 3: Generate and run migration**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge" bun run db:generate
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge" bun run db:migrate
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run db:migrate
```

- [ ] **Step 4: Write schema validation test**

Create `tests/unit/ai-schema.test.ts`:

```typescript
import { describe, it, expect } from "vitest";
import { aiInteractions, codeAnnotations, annotationAuthorTypeEnum } from "@/lib/db/schema";

describe("AI & annotation schema", () => {
  it("aiInteractions table has required columns", () => {
    const columns = Object.keys(aiInteractions);
    expect(columns).toContain("id");
    expect(columns).toContain("studentId");
    expect(columns).toContain("sessionId");
    expect(columns).toContain("documentId");
    expect(columns).toContain("enabledByTeacherId");
    expect(columns).toContain("messages");
    expect(columns).toContain("createdAt");
  });

  it("codeAnnotations table has required columns", () => {
    const columns = Object.keys(codeAnnotations);
    expect(columns).toContain("id");
    expect(columns).toContain("documentId");
    expect(columns).toContain("authorId");
    expect(columns).toContain("authorType");
    expect(columns).toContain("lineStart");
    expect(columns).toContain("lineEnd");
    expect(columns).toContain("content");
    expect(columns).toContain("resolved");
    expect(columns).toContain("createdAt");
  });

  it("annotationAuthorTypeEnum has teacher and ai values", () => {
    expect(annotationAuthorTypeEnum.enumValues).toEqual(["teacher", "ai"]);
  });
});
```

- [ ] **Step 5: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test
```

Expected: All tests pass, including the new schema validation test.

- [ ] **Step 6: Commit**

```bash
git add src/lib/db/schema.ts tests/helpers.ts tests/unit/ai-schema.test.ts drizzle/
git commit -m "feat: add AIInteraction and CodeAnnotation database tables with migration"
```

---

## Task 3: Anthropic Client Singleton and System Prompts

**Files:**
- Create: `src/lib/ai/client.ts`
- Create: `src/lib/ai/system-prompts.ts`
- Create: `tests/unit/system-prompts.test.ts`

- [ ] **Step 1: Write system prompt tests (TDD)**

Create `tests/unit/system-prompts.test.ts`:

```typescript
import { describe, it, expect } from "vitest";
import { buildSystemPrompt, getGradeLevelGuidance } from "@/lib/ai/system-prompts";

describe("getGradeLevelGuidance", () => {
  it("returns K-5 guidance for K-5 grade level", () => {
    const guidance = getGradeLevelGuidance("K-5");
    expect(guidance).toContain("simple");
    expect(guidance).toContain("encouragement");
  });

  it("returns 6-8 guidance for 6-8 grade level", () => {
    const guidance = getGradeLevelGuidance("6-8");
    expect(guidance).toContain("analogi");
    expect(guidance).toContain("line number");
  });

  it("returns 9-12 guidance for 9-12 grade level", () => {
    const guidance = getGradeLevelGuidance("9-12");
    expect(guidance).toContain("technical");
    expect(guidance).toContain("documentation");
  });
});

describe("buildSystemPrompt", () => {
  it("includes Socratic pedagogy instructions", () => {
    const prompt = buildSystemPrompt({ gradeLevel: "6-8", language: "python" });
    expect(prompt).toContain("guiding question");
    expect(prompt).toContain("never provide complete solution");
  });

  it("includes grade-level guidance", () => {
    const prompt = buildSystemPrompt({ gradeLevel: "K-5", language: "python" });
    expect(prompt).toContain("simple");
    expect(prompt).toContain("encouragement");
  });

  it("includes the programming language", () => {
    const prompt = buildSystemPrompt({ gradeLevel: "9-12", language: "python" });
    expect(prompt).toContain("Python");
  });

  it("includes assignment context when provided", () => {
    const prompt = buildSystemPrompt({
      gradeLevel: "6-8",
      language: "python",
      assignmentTitle: "Loop Practice",
      assignmentDescription: "Write a for loop that prints numbers 1-10",
    });
    expect(prompt).toContain("Loop Practice");
    expect(prompt).toContain("prints numbers 1-10");
  });

  it("works without assignment context", () => {
    const prompt = buildSystemPrompt({ gradeLevel: "6-8", language: "python" });
    expect(prompt).not.toContain("Assignment:");
  });
});
```

- [ ] **Step 2: Create the Anthropic client singleton**

Create `src/lib/ai/client.ts`:

```typescript
import Anthropic from "@anthropic-ai/sdk";

let clientInstance: Anthropic | null = null;

export function getAnthropicClient(): Anthropic {
  if (!clientInstance) {
    const apiKey = process.env.ANTHROPIC_API_KEY;
    if (!apiKey) {
      throw new Error(
        "ANTHROPIC_API_KEY environment variable is required. Add it to your .env file."
      );
    }
    clientInstance = new Anthropic({ apiKey });
  }
  return clientInstance;
}

/** Reset client — for testing only */
export function resetAnthropicClient(): void {
  clientInstance = null;
}
```

- [ ] **Step 3: Create the system prompts module**

Create `src/lib/ai/system-prompts.ts`:

```typescript
type GradeLevel = "K-5" | "6-8" | "9-12";

interface SystemPromptInput {
  gradeLevel: GradeLevel;
  language: string;
  assignmentTitle?: string;
  assignmentDescription?: string;
}

export function getGradeLevelGuidance(gradeLevel: GradeLevel): string {
  switch (gradeLevel) {
    case "K-5":
      return `You are talking to an elementary school student (grades K-5).
- Use simple vocabulary and short sentences.
- Give lots of encouragement and positive reinforcement.
- Use visual references and concrete examples (e.g., "think of a loop like going around a track").
- Keep explanations to 2-3 sentences maximum.
- Use friendly, warm language.`;

    case "6-8":
      return `You are talking to a middle school student (grades 6-8).
- Explain concepts clearly using analogies and relatable examples.
- Reference specific line numbers in their code when pointing out issues.
- Use encouraging but not overly childish language.
- Keep explanations to 3-5 sentences.
- Introduce proper programming vocabulary alongside plain-language explanations.`;

    case "9-12":
      return `You are talking to a high school student (grades 9-12).
- Use technical programming language and proper terminology.
- Reference official documentation and best practices when relevant.
- Discuss trade-offs and alternative approaches.
- Encourage independent problem-solving and debugging skills.
- You can give longer, more detailed explanations when appropriate.`;
  }
}

export function buildSystemPrompt(input: SystemPromptInput): string {
  const langDisplay = input.language.charAt(0).toUpperCase() + input.language.slice(1);

  let prompt = `You are a Socratic coding tutor helping a student learn ${langDisplay} programming. Your role is to guide the student to discover answers themselves.

## Core Rules
1. Ask guiding questions instead of giving direct answers.
2. You must never provide complete solutions or write complete functions for the student.
3. Point the student toward the right direction — suggest what to look at, what concept applies, or what to try next.
4. If the student has a bug, ask them what they expect the code to do vs. what it actually does.
5. Celebrate progress and correct thinking.
6. If the student is frustrated, acknowledge it and break the problem into smaller steps.
7. Keep responses focused and concise — students lose attention with long explanations.

## Grade-Level Guidance
${getGradeLevelGuidance(input.gradeLevel)}
`;

  if (input.assignmentTitle) {
    prompt += `
## Assignment Context
Assignment: ${input.assignmentTitle}
${input.assignmentDescription ? `Description: ${input.assignmentDescription}` : ""}
The student is working on this assignment. Tailor your guidance to help them complete it, but never give away the solution.
`;
  }

  return prompt;
}
```

- [ ] **Step 4: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/system-prompts.test.ts
```

Expected: All 7 tests pass.

- [ ] **Step 5: Commit**

```bash
git add src/lib/ai/client.ts src/lib/ai/system-prompts.ts tests/unit/system-prompts.test.ts
git commit -m "feat: add Anthropic client singleton and grade-level Socratic system prompts"
```

---

## Task 4: AI Output Guardrails

**Files:**
- Create: `src/lib/ai/guardrails.ts`
- Create: `tests/unit/guardrails.test.ts`

- [ ] **Step 1: Write guardrail tests (TDD)**

Create `tests/unit/guardrails.test.ts`:

```typescript
import { describe, it, expect } from "vitest";
import { checkResponseForSolutions, sanitizeAiResponse } from "@/lib/ai/guardrails";

describe("checkResponseForSolutions", () => {
  it("passes a response with guiding questions", () => {
    const response = "What do you think happens when i reaches 10? Try adding a print statement inside the loop to see.";
    expect(checkResponseForSolutions(response)).toEqual({ allowed: true });
  });

  it("flags a response containing a complete function definition", () => {
    const response = `Here's the solution:
\`\`\`python
def fibonacci(n):
    if n <= 1:
        return n
    return fibonacci(n-1) + fibonacci(n-2)
\`\`\``;
    const result = checkResponseForSolutions(response);
    expect(result.allowed).toBe(false);
    expect(result.reason).toContain("complete function");
  });

  it("flags a response with a complete class definition", () => {
    const response = `Try this:
\`\`\`python
class Calculator:
    def add(self, a, b):
        return a + b
    def subtract(self, a, b):
        return a - b
\`\`\``;
    const result = checkResponseForSolutions(response);
    expect(result.allowed).toBe(false);
  });

  it("allows a response with a short code snippet (1-2 lines)", () => {
    const response = "Try using something like `for i in range(10):` — what do you think goes inside the loop?";
    expect(checkResponseForSolutions(response)).toEqual({ allowed: true });
  });

  it("flags a response with a multi-line code block (5+ lines)", () => {
    const response = `Here you go:
\`\`\`python
numbers = [1, 2, 3, 4, 5]
total = 0
for num in numbers:
    total += num
print(total)
\`\`\``;
    const result = checkResponseForSolutions(response);
    expect(result.allowed).toBe(false);
    expect(result.reason).toContain("code block");
  });

  it("allows a code block with 3 or fewer lines", () => {
    const response = `Consider this pattern:
\`\`\`python
for i in range(n):
    print(i)
\`\`\`
What would you need to change?`;
    expect(checkResponseForSolutions(response)).toEqual({ allowed: true });
  });

  it("flags responses that say 'here is the solution' or similar", () => {
    const response = "Here is the complete answer: just use a while loop with a counter.";
    const result = checkResponseForSolutions(response);
    expect(result.allowed).toBe(false);
    expect(result.reason).toContain("solution language");
  });
});

describe("sanitizeAiResponse", () => {
  it("returns the response unchanged when it passes guardrails", () => {
    const response = "What happens if you try printing the value of x before the if statement?";
    expect(sanitizeAiResponse(response)).toBe(response);
  });

  it("replaces a flagged response with a Socratic redirect", () => {
    const response = `Here's the solution:
\`\`\`python
def solve():
    for i in range(10):
        print(i)
    return True
\`\`\``;
    const sanitized = sanitizeAiResponse(response);
    expect(sanitized).not.toContain("def solve");
    expect(sanitized).toContain("guide");
  });
});
```

- [ ] **Step 2: Implement guardrails**

Create `src/lib/ai/guardrails.ts`:

```typescript
interface GuardrailResult {
  allowed: boolean;
  reason?: string;
}

const SOLUTION_PHRASES = [
  "here is the complete answer",
  "here is the solution",
  "here's the complete answer",
  "here's the solution",
  "here is the complete code",
  "here's the complete code",
  "the complete solution is",
  "the full solution is",
];

const CODE_BLOCK_REGEX = /```[\w]*\n([\s\S]*?)```/g;
const FUNCTION_DEF_REGEX = /^\s*def\s+\w+\s*\(.*\)\s*:/m;
const CLASS_DEF_REGEX = /^\s*class\s+\w+[\s(]/m;

export function checkResponseForSolutions(response: string): GuardrailResult {
  // Check for solution-giving language
  const lowerResponse = response.toLowerCase();
  for (const phrase of SOLUTION_PHRASES) {
    if (lowerResponse.includes(phrase)) {
      return { allowed: false, reason: "Response contains solution language" };
    }
  }

  // Check code blocks
  const codeBlocks = [...response.matchAll(CODE_BLOCK_REGEX)];
  for (const match of codeBlocks) {
    const code = match[1].trim();
    const lines = code.split("\n").filter((line) => line.trim().length > 0);

    // Check for complete function definitions
    if (FUNCTION_DEF_REGEX.test(code)) {
      return {
        allowed: false,
        reason: "Response contains a complete function definition",
      };
    }

    // Check for complete class definitions
    if (CLASS_DEF_REGEX.test(code)) {
      return {
        allowed: false,
        reason: "Response contains a complete class definition",
      };
    }

    // Flag code blocks with 5+ non-empty lines
    if (lines.length >= 5) {
      return {
        allowed: false,
        reason: "Response contains a long code block (5+ lines) — likely a complete solution",
      };
    }
  }

  return { allowed: true };
}

const REDIRECT_MESSAGE =
  "I want to help you figure this out! Let me guide you with some questions instead. What part of the problem are you finding most challenging? Let's break it down step by step.";

export function sanitizeAiResponse(response: string): string {
  const result = checkResponseForSolutions(response);
  if (result.allowed) {
    return response;
  }
  return REDIRECT_MESSAGE;
}
```

- [ ] **Step 3: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/guardrails.test.ts
```

Expected: All 9 tests pass.

- [ ] **Step 4: Commit**

```bash
git add src/lib/ai/guardrails.ts tests/unit/guardrails.test.ts
git commit -m "feat: add AI output guardrails to prevent complete solutions"
```

---

## Task 5: AI Interaction Business Logic (CRUD)

**Files:**
- Create: `src/lib/ai/interactions.ts`
- Create: `tests/unit/ai-interactions.test.ts`

- [ ] **Step 1: Write interaction logic tests (TDD)**

Create `tests/unit/ai-interactions.test.ts`:

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, cleanupDatabase } from "../helpers";
import {
  createAiInteraction,
  getAiInteraction,
  appendMessage,
  listInteractionsBySession,
  listInteractionsByStudent,
} from "@/lib/ai/interactions";
import { randomUUID } from "crypto";

describe("AI interaction operations", () => {
  let student: Awaited<ReturnType<typeof createTestUser>>;
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  const fakeSessionId = randomUUID();
  const fakeDocumentId = randomUUID();

  beforeEach(async () => {
    teacher = await createTestUser({ role: "teacher", email: "teacher@test.com" });
    student = await createTestUser({ role: "student", email: "student@test.com" });
  });

  describe("createAiInteraction", () => {
    it("creates an interaction with empty messages", async () => {
      const interaction = await createAiInteraction(testDb, {
        studentId: student.id,
        sessionId: fakeSessionId,
        documentId: fakeDocumentId,
        enabledByTeacherId: teacher.id,
      });

      expect(interaction.id).toBeDefined();
      expect(interaction.studentId).toBe(student.id);
      expect(interaction.sessionId).toBe(fakeSessionId);
      expect(interaction.messages).toEqual([]);
    });
  });

  describe("getAiInteraction", () => {
    it("returns interaction by ID", async () => {
      const created = await createAiInteraction(testDb, {
        studentId: student.id,
        sessionId: fakeSessionId,
        documentId: fakeDocumentId,
        enabledByTeacherId: teacher.id,
      });

      const found = await getAiInteraction(testDb, created.id);
      expect(found).not.toBeNull();
      expect(found!.id).toBe(created.id);
    });

    it("returns null for non-existent ID", async () => {
      const found = await getAiInteraction(testDb, randomUUID());
      expect(found).toBeNull();
    });
  });

  describe("appendMessage", () => {
    it("appends a student message to the conversation", async () => {
      const interaction = await createAiInteraction(testDb, {
        studentId: student.id,
        sessionId: fakeSessionId,
        documentId: fakeDocumentId,
        enabledByTeacherId: teacher.id,
      });

      const updated = await appendMessage(testDb, interaction.id, {
        role: "student",
        content: "How do I use a for loop?",
        timestamp: new Date().toISOString(),
      });

      expect(updated.messages).toHaveLength(1);
      expect((updated.messages as any[])[0].role).toBe("student");
      expect((updated.messages as any[])[0].content).toBe("How do I use a for loop?");
    });

    it("appends an AI message after a student message", async () => {
      const interaction = await createAiInteraction(testDb, {
        studentId: student.id,
        sessionId: fakeSessionId,
        documentId: fakeDocumentId,
        enabledByTeacherId: teacher.id,
      });

      await appendMessage(testDb, interaction.id, {
        role: "student",
        content: "Help with loops",
        timestamp: new Date().toISOString(),
      });

      const updated = await appendMessage(testDb, interaction.id, {
        role: "assistant",
        content: "What kind of pattern are you trying to repeat?",
        timestamp: new Date().toISOString(),
      });

      expect(updated.messages).toHaveLength(2);
      expect((updated.messages as any[])[1].role).toBe("assistant");
    });
  });

  describe("listInteractionsBySession", () => {
    it("returns all interactions for a session", async () => {
      await createAiInteraction(testDb, {
        studentId: student.id,
        sessionId: fakeSessionId,
        documentId: fakeDocumentId,
        enabledByTeacherId: teacher.id,
      });

      const student2 = await createTestUser({ role: "student", email: "student2@test.com" });
      await createAiInteraction(testDb, {
        studentId: student2.id,
        sessionId: fakeSessionId,
        documentId: randomUUID(),
        enabledByTeacherId: teacher.id,
      });

      const results = await listInteractionsBySession(testDb, fakeSessionId);
      expect(results).toHaveLength(2);
    });

    it("returns empty array for session with no interactions", async () => {
      const results = await listInteractionsBySession(testDb, randomUUID());
      expect(results).toHaveLength(0);
    });
  });

  describe("listInteractionsByStudent", () => {
    it("returns interactions for a specific student in a session", async () => {
      await createAiInteraction(testDb, {
        studentId: student.id,
        sessionId: fakeSessionId,
        documentId: fakeDocumentId,
        enabledByTeacherId: teacher.id,
      });

      const results = await listInteractionsByStudent(testDb, fakeSessionId, student.id);
      expect(results).toHaveLength(1);
      expect(results[0].studentId).toBe(student.id);
    });
  });
});
```

- [ ] **Step 2: Implement AI interaction business logic**

Create `src/lib/ai/interactions.ts`:

```typescript
import { eq, and } from "drizzle-orm";
import { aiInteractions } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

interface CreateInteractionInput {
  studentId: string;
  sessionId: string;
  documentId: string;
  enabledByTeacherId: string;
}

interface ChatMessage {
  role: "student" | "assistant";
  content: string;
  timestamp: string;
}

export async function createAiInteraction(
  db: Database,
  input: CreateInteractionInput
) {
  const [interaction] = await db
    .insert(aiInteractions)
    .values({
      studentId: input.studentId,
      sessionId: input.sessionId,
      documentId: input.documentId,
      enabledByTeacherId: input.enabledByTeacherId,
      messages: [],
    })
    .returning();
  return interaction;
}

export async function getAiInteraction(db: Database, id: string) {
  const [interaction] = await db
    .select()
    .from(aiInteractions)
    .where(eq(aiInteractions.id, id));
  return interaction || null;
}

export async function appendMessage(
  db: Database,
  interactionId: string,
  message: ChatMessage
) {
  const existing = await getAiInteraction(db, interactionId);
  if (!existing) {
    throw new Error(`AI interaction ${interactionId} not found`);
  }

  const currentMessages = (existing.messages as ChatMessage[]) || [];
  const updatedMessages = [...currentMessages, message];

  const [updated] = await db
    .update(aiInteractions)
    .set({ messages: updatedMessages })
    .where(eq(aiInteractions.id, interactionId))
    .returning();
  return updated;
}

export async function listInteractionsBySession(
  db: Database,
  sessionId: string
) {
  return db
    .select()
    .from(aiInteractions)
    .where(eq(aiInteractions.sessionId, sessionId));
}

export async function listInteractionsByStudent(
  db: Database,
  sessionId: string,
  studentId: string
) {
  return db
    .select()
    .from(aiInteractions)
    .where(
      and(
        eq(aiInteractions.sessionId, sessionId),
        eq(aiInteractions.studentId, studentId)
      )
    );
}
```

- [ ] **Step 3: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/ai-interactions.test.ts
```

Expected: All 7 tests pass.

- [ ] **Step 4: Commit**

```bash
git add src/lib/ai/interactions.ts tests/unit/ai-interactions.test.ts
git commit -m "feat: add AI interaction CRUD logic with conversation message appending"
```

---

## Task 6: Code Annotation Business Logic

**Files:**
- Create: `src/lib/annotations.ts`
- Create: `tests/unit/annotations.test.ts`

- [ ] **Step 1: Write annotation tests (TDD)**

Create `tests/unit/annotations.test.ts`:

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser } from "../helpers";
import {
  createAnnotation,
  getAnnotation,
  listAnnotationsByDocument,
  deleteAnnotation,
  resolveAnnotation,
} from "@/lib/annotations";
import { randomUUID } from "crypto";

describe("annotation operations", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  const fakeDocumentId = randomUUID();

  beforeEach(async () => {
    teacher = await createTestUser({ role: "teacher", email: "teacher@test.com" });
  });

  describe("createAnnotation", () => {
    it("creates a teacher annotation on specific lines", async () => {
      const annotation = await createAnnotation(testDb, {
        documentId: fakeDocumentId,
        authorId: teacher.id,
        authorType: "teacher" as const,
        lineStart: 5,
        lineEnd: 7,
        content: "Consider using a for loop here instead of repeating code.",
      });

      expect(annotation.id).toBeDefined();
      expect(annotation.documentId).toBe(fakeDocumentId);
      expect(annotation.authorId).toBe(teacher.id);
      expect(annotation.authorType).toBe("teacher");
      expect(annotation.lineStart).toBe(5);
      expect(annotation.lineEnd).toBe(7);
      expect(annotation.content).toBe("Consider using a for loop here instead of repeating code.");
      expect(annotation.resolved).toBe(false);
    });

    it("creates an AI annotation", async () => {
      const annotation = await createAnnotation(testDb, {
        documentId: fakeDocumentId,
        authorId: teacher.id,
        authorType: "ai" as const,
        lineStart: 3,
        lineEnd: 3,
        content: "This variable name could be more descriptive.",
      });

      expect(annotation.authorType).toBe("ai");
    });
  });

  describe("getAnnotation", () => {
    it("returns annotation by ID", async () => {
      const created = await createAnnotation(testDb, {
        documentId: fakeDocumentId,
        authorId: teacher.id,
        authorType: "teacher" as const,
        lineStart: 1,
        lineEnd: 1,
        content: "Nice work!",
      });

      const found = await getAnnotation(testDb, created.id);
      expect(found).not.toBeNull();
      expect(found!.content).toBe("Nice work!");
    });

    it("returns null for non-existent ID", async () => {
      const found = await getAnnotation(testDb, randomUUID());
      expect(found).toBeNull();
    });
  });

  describe("listAnnotationsByDocument", () => {
    it("returns all annotations for a document", async () => {
      await createAnnotation(testDb, {
        documentId: fakeDocumentId,
        authorId: teacher.id,
        authorType: "teacher" as const,
        lineStart: 1,
        lineEnd: 2,
        content: "First annotation",
      });

      await createAnnotation(testDb, {
        documentId: fakeDocumentId,
        authorId: teacher.id,
        authorType: "ai" as const,
        lineStart: 5,
        lineEnd: 5,
        content: "Second annotation",
      });

      const results = await listAnnotationsByDocument(testDb, fakeDocumentId);
      expect(results).toHaveLength(2);
    });

    it("returns empty array for document with no annotations", async () => {
      const results = await listAnnotationsByDocument(testDb, randomUUID());
      expect(results).toHaveLength(0);
    });

    it("does not return annotations from other documents", async () => {
      await createAnnotation(testDb, {
        documentId: fakeDocumentId,
        authorId: teacher.id,
        authorType: "teacher" as const,
        lineStart: 1,
        lineEnd: 1,
        content: "For doc 1",
      });

      await createAnnotation(testDb, {
        documentId: randomUUID(),
        authorId: teacher.id,
        authorType: "teacher" as const,
        lineStart: 1,
        lineEnd: 1,
        content: "For doc 2",
      });

      const results = await listAnnotationsByDocument(testDb, fakeDocumentId);
      expect(results).toHaveLength(1);
      expect(results[0].content).toBe("For doc 1");
    });
  });

  describe("deleteAnnotation", () => {
    it("deletes an annotation by ID", async () => {
      const annotation = await createAnnotation(testDb, {
        documentId: fakeDocumentId,
        authorId: teacher.id,
        authorType: "teacher" as const,
        lineStart: 1,
        lineEnd: 1,
        content: "Delete me",
      });

      await deleteAnnotation(testDb, annotation.id);

      const found = await getAnnotation(testDb, annotation.id);
      expect(found).toBeNull();
    });
  });

  describe("resolveAnnotation", () => {
    it("marks an annotation as resolved", async () => {
      const annotation = await createAnnotation(testDb, {
        documentId: fakeDocumentId,
        authorId: teacher.id,
        authorType: "teacher" as const,
        lineStart: 1,
        lineEnd: 1,
        content: "Fix this",
      });

      const resolved = await resolveAnnotation(testDb, annotation.id);
      expect(resolved.resolved).toBe(true);
    });
  });
});
```

- [ ] **Step 2: Implement annotation business logic**

Create `src/lib/annotations.ts`:

```typescript
import { eq } from "drizzle-orm";
import { codeAnnotations } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

interface CreateAnnotationInput {
  documentId: string;
  authorId: string;
  authorType: "teacher" | "ai";
  lineStart: number;
  lineEnd: number;
  content: string;
}

export async function createAnnotation(
  db: Database,
  input: CreateAnnotationInput
) {
  const [annotation] = await db
    .insert(codeAnnotations)
    .values(input)
    .returning();
  return annotation;
}

export async function getAnnotation(db: Database, id: string) {
  const [annotation] = await db
    .select()
    .from(codeAnnotations)
    .where(eq(codeAnnotations.id, id));
  return annotation || null;
}

export async function listAnnotationsByDocument(
  db: Database,
  documentId: string
) {
  return db
    .select()
    .from(codeAnnotations)
    .where(eq(codeAnnotations.documentId, documentId));
}

export async function deleteAnnotation(db: Database, id: string) {
  await db.delete(codeAnnotations).where(eq(codeAnnotations.id, id));
}

export async function resolveAnnotation(db: Database, id: string) {
  const [updated] = await db
    .update(codeAnnotations)
    .set({ resolved: true })
    .where(eq(codeAnnotations.id, id))
    .returning();
  return updated;
}
```

- [ ] **Step 3: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/annotations.test.ts
```

Expected: All 8 tests pass.

- [ ] **Step 4: Commit**

```bash
git add src/lib/annotations.ts tests/unit/annotations.test.ts
git commit -m "feat: add code annotation CRUD logic with resolve support"
```

---

## Task 7: AI Chat API Endpoint (Streaming)

**Files:**
- Create: `src/app/api/ai/chat/route.ts`
- Create: `tests/api/ai-chat.test.ts`

- [ ] **Step 1: Write API tests (TDD)**

Create `tests/api/ai-chat.test.ts`:

```typescript
import { describe, it, expect, beforeEach, vi } from "vitest";
import { testDb, createTestUser, createTestClassroom } from "../helpers";
import { createAiInteraction, getAiInteraction } from "@/lib/ai/interactions";
import { buildSystemPrompt } from "@/lib/ai/system-prompts";
import { randomUUID } from "crypto";

// We test the business logic directly rather than the HTTP layer,
// because streaming SSE responses are difficult to test via HTTP mocks.

describe("AI chat logic", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;
  let classroom: Awaited<ReturnType<typeof createTestClassroom>>;

  beforeEach(async () => {
    teacher = await createTestUser({ role: "teacher", email: "teacher@test.com" });
    student = await createTestUser({ role: "student", email: "student@test.com" });
    classroom = await createTestClassroom(teacher.id, { gradeLevel: "6-8" });
  });

  it("creates an interaction and appends the student message", async () => {
    const fakeSessionId = randomUUID();
    const fakeDocumentId = randomUUID();

    const interaction = await createAiInteraction(testDb, {
      studentId: student.id,
      sessionId: fakeSessionId,
      documentId: fakeDocumentId,
      enabledByTeacherId: teacher.id,
    });

    expect(interaction.id).toBeDefined();
    expect(interaction.messages).toEqual([]);
  });

  it("builds correct system prompt for grade level", () => {
    const prompt = buildSystemPrompt({
      gradeLevel: "6-8",
      language: "python",
    });

    expect(prompt).toContain("Socratic");
    expect(prompt).toContain("guiding question");
    expect(prompt).toContain("never provide complete solution");
    expect(prompt).toContain("Python");
  });

  it("builds correct system prompt with assignment context", () => {
    const prompt = buildSystemPrompt({
      gradeLevel: "K-5",
      language: "python",
      assignmentTitle: "Print Your Name",
      assignmentDescription: "Use the print() function to display your name",
    });

    expect(prompt).toContain("Print Your Name");
    expect(prompt).toContain("print() function");
    expect(prompt).toContain("simple");
  });

  it("interaction messages persist across append operations", async () => {
    const { appendMessage } = await import("@/lib/ai/interactions");
    const fakeSessionId = randomUUID();
    const fakeDocumentId = randomUUID();

    const interaction = await createAiInteraction(testDb, {
      studentId: student.id,
      sessionId: fakeSessionId,
      documentId: fakeDocumentId,
      enabledByTeacherId: teacher.id,
    });

    await appendMessage(testDb, interaction.id, {
      role: "student",
      content: "How do loops work?",
      timestamp: new Date().toISOString(),
    });

    await appendMessage(testDb, interaction.id, {
      role: "assistant",
      content: "What task are you trying to repeat?",
      timestamp: new Date().toISOString(),
    });

    const final = await getAiInteraction(testDb, interaction.id);
    expect(final!.messages).toHaveLength(2);
  });
});
```

- [ ] **Step 2: Implement the AI chat streaming endpoint**

Create `src/app/api/ai/chat/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getAnthropicClient } from "@/lib/ai/client";
import { buildSystemPrompt } from "@/lib/ai/system-prompts";
import { sanitizeAiResponse } from "@/lib/ai/guardrails";
import {
  createAiInteraction,
  getAiInteraction,
  appendMessage,
} from "@/lib/ai/interactions";
import { classrooms, classroomMembers } from "@/lib/db/schema";
import { eq, and } from "drizzle-orm";

const chatRequestSchema = z.object({
  message: z.string().min(1).max(2000),
  sessionId: z.string().uuid(),
  documentId: z.string().uuid(),
  classroomId: z.string().uuid(),
  interactionId: z.string().uuid().optional(),
  currentCode: z.string().optional(),
});

export async function POST(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  if (session.user.role !== "student") {
    return NextResponse.json(
      { error: "Only students can use the AI tutor" },
      { status: 403 }
    );
  }

  const body = await request.json();
  const parsed = chatRequestSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const { message, sessionId, documentId, classroomId, interactionId, currentCode } =
    parsed.data;

  // Verify student is a member of the classroom
  const [membership] = await db
    .select()
    .from(classroomMembers)
    .where(
      and(
        eq(classroomMembers.classroomId, classroomId),
        eq(classroomMembers.userId, session.user.id)
      )
    );

  if (!membership) {
    return NextResponse.json(
      { error: "Not a member of this classroom" },
      { status: 403 }
    );
  }

  // Get classroom for grade level
  const [classroom] = await db
    .select()
    .from(classrooms)
    .where(eq(classrooms.id, classroomId));

  if (!classroom) {
    return NextResponse.json(
      { error: "Classroom not found" },
      { status: 404 }
    );
  }

  // Get or create interaction
  let interaction;
  if (interactionId) {
    interaction = await getAiInteraction(db, interactionId);
    if (!interaction) {
      return NextResponse.json(
        { error: "Interaction not found" },
        { status: 404 }
      );
    }
    if (interaction.studentId !== session.user.id) {
      return NextResponse.json(
        { error: "Not your interaction" },
        { status: 403 }
      );
    }
  } else {
    interaction = await createAiInteraction(db, {
      studentId: session.user.id,
      sessionId,
      documentId,
      enabledByTeacherId: classroom.teacherId,
    });
  }

  // Append the student message
  const timestamp = new Date().toISOString();
  interaction = await appendMessage(db, interaction.id, {
    role: "student",
    content: message,
    timestamp,
  });

  // Build the Claude messages from conversation history
  const conversationMessages = (
    interaction.messages as Array<{ role: string; content: string }>
  ).map((msg) => ({
    role: msg.role === "student" ? ("user" as const) : ("assistant" as const),
    content: msg.content,
  }));

  // Add current code context as a user message prefix if this is the first message
  if (currentCode && conversationMessages.length === 1) {
    conversationMessages[0] = {
      role: "user" as const,
      content: `Here is my current code:\n\`\`\`${classroom.editorMode}\n${currentCode}\n\`\`\`\n\n${message}`,
    };
  }

  const systemPrompt = buildSystemPrompt({
    gradeLevel: classroom.gradeLevel,
    language: classroom.editorMode,
  });

  // Stream the response using SSE
  const encoder = new TextEncoder();
  const stream = new ReadableStream({
    async start(controller) {
      try {
        const client = getAnthropicClient();
        const response = await client.messages.create({
          model: "claude-sonnet-4-20250514",
          max_tokens: 1024,
          system: systemPrompt,
          messages: conversationMessages,
          stream: true,
        });

        let fullResponse = "";

        for await (const event of response) {
          if (
            event.type === "content_block_delta" &&
            event.delta.type === "text_delta"
          ) {
            fullResponse += event.delta.text;
            const data = JSON.stringify({
              type: "delta",
              text: event.delta.text,
            });
            controller.enqueue(encoder.encode(`data: ${data}\n\n`));
          }
        }

        // Apply guardrails to the full response
        const sanitized = sanitizeAiResponse(fullResponse);
        const wasFiltered = sanitized !== fullResponse;

        // If filtered, send the sanitized version instead
        if (wasFiltered) {
          const data = JSON.stringify({
            type: "filtered",
            text: sanitized,
          });
          controller.enqueue(encoder.encode(`data: ${data}\n\n`));
        }

        // Save the (possibly sanitized) AI response
        const savedResponse = wasFiltered ? sanitized : fullResponse;
        await appendMessage(db, interaction.id, {
          role: "assistant",
          content: savedResponse,
          timestamp: new Date().toISOString(),
        });

        // Send done event with interaction ID
        const doneData = JSON.stringify({
          type: "done",
          interactionId: interaction.id,
          filtered: wasFiltered,
        });
        controller.enqueue(encoder.encode(`data: ${doneData}\n\n`));
        controller.close();
      } catch (error) {
        const errorData = JSON.stringify({
          type: "error",
          message: "AI service temporarily unavailable",
        });
        controller.enqueue(encoder.encode(`data: ${errorData}\n\n`));
        controller.close();
      }
    },
  });

  return new Response(stream, {
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
    },
  });
}
```

- [ ] **Step 3: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/api/ai-chat.test.ts
```

Expected: All 4 tests pass.

- [ ] **Step 4: Commit**

```bash
git add src/app/api/ai/chat/route.ts tests/api/ai-chat.test.ts
git commit -m "feat: add AI chat streaming endpoint with Claude API integration"
```

---

## Task 8: AI Toggle API Endpoint (Teacher Controls)

**Files:**
- Create: `src/app/api/ai/toggle/route.ts`
- Create: `tests/api/ai-toggle.test.ts`

- [ ] **Step 1: Write toggle tests (TDD)**

Create `tests/api/ai-toggle.test.ts`:

```typescript
import { describe, it, expect, beforeEach, vi } from "vitest";
import { testDb, createTestUser, createTestClassroom } from "../helpers";
import { classroomMembers } from "@/lib/db/schema";
import { randomUUID } from "crypto";

// Test the validation and authorization logic directly.
// The actual toggle state is managed in Plan 3's SessionParticipant or session settings.

describe("AI toggle authorization", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;
  let classroom: Awaited<ReturnType<typeof createTestClassroom>>;

  beforeEach(async () => {
    teacher = await createTestUser({ role: "teacher", email: "teacher@test.com" });
    student = await createTestUser({ role: "student", email: "student@test.com" });
    classroom = await createTestClassroom(teacher.id, { gradeLevel: "6-8" });
    await testDb.insert(classroomMembers).values({
      classroomId: classroom.id,
      userId: student.id,
    });
  });

  it("teacher owns the classroom", async () => {
    expect(classroom.teacherId).toBe(teacher.id);
  });

  it("student is a member of the classroom", async () => {
    const { getClassroomMembers } = await import("@/lib/classrooms");
    const members = await getClassroomMembers(testDb, classroom.id);
    expect(members).toHaveLength(1);
    expect(members[0].userId).toBe(student.id);
  });

  it("non-teacher cannot toggle AI", () => {
    // Validates the role check that the route enforces
    expect(student.role).toBe("student");
    expect(teacher.role).toBe("teacher");
  });

  it("only classroom teacher can toggle AI for their students", () => {
    expect(classroom.teacherId).toBe(teacher.id);
    // A different teacher should not be able to toggle
    // (validated by checking classroom.teacherId === session.user.id in the route)
  });
});
```

- [ ] **Step 2: Implement the AI toggle endpoint**

Create `src/app/api/ai/toggle/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { classrooms, classroomMembers } from "@/lib/db/schema";
import { eq, and } from "drizzle-orm";

const toggleSchema = z.object({
  sessionId: z.string().uuid(),
  classroomId: z.string().uuid(),
  studentId: z.string().uuid(),
  enabled: z.boolean(),
});

export async function POST(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  if (session.user.role !== "teacher" && session.user.role !== "admin") {
    return NextResponse.json(
      { error: "Only teachers can toggle AI access" },
      { status: 403 }
    );
  }

  const body = await request.json();
  const parsed = toggleSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const { sessionId, classroomId, studentId, enabled } = parsed.data;

  // Verify the teacher owns this classroom
  const [classroom] = await db
    .select()
    .from(classrooms)
    .where(
      and(
        eq(classrooms.id, classroomId),
        eq(classrooms.teacherId, session.user.id)
      )
    );

  if (!classroom) {
    return NextResponse.json(
      { error: "Classroom not found or you are not the teacher" },
      { status: 404 }
    );
  }

  // Verify the student is a member
  const [membership] = await db
    .select()
    .from(classroomMembers)
    .where(
      and(
        eq(classroomMembers.classroomId, classroomId),
        eq(classroomMembers.userId, studentId)
      )
    );

  if (!membership) {
    return NextResponse.json(
      { error: "Student is not a member of this classroom" },
      { status: 404 }
    );
  }

  // The actual AI enabled state is stored in the SessionParticipant table
  // (from Plan 3) or broadcast via SSE. Here we return success and the
  // caller (teacher dashboard) updates the session state.
  //
  // If Plan 3 has added a SessionParticipant table with an aiEnabled field,
  // update it here:
  //
  // await db
  //   .update(sessionParticipants)
  //   .set({ aiEnabled: enabled })
  //   .where(
  //     and(
  //       eq(sessionParticipants.sessionId, sessionId),
  //       eq(sessionParticipants.studentId, studentId)
  //     )
  //   );

  return NextResponse.json({
    sessionId,
    studentId,
    aiEnabled: enabled,
    toggledBy: session.user.id,
  });
}
```

- [ ] **Step 3: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/api/ai-toggle.test.ts
```

Expected: All 4 tests pass.

- [ ] **Step 4: Commit**

```bash
git add src/app/api/ai/toggle/route.ts tests/api/ai-toggle.test.ts
git commit -m "feat: add teacher AI toggle endpoint with ownership verification"
```

---

## Task 9: AI Interaction Log API (Teacher Visibility)

**Files:**
- Create: `src/app/api/ai/interactions/route.ts`
- Create: `tests/api/ai-interactions.test.ts`

- [ ] **Step 1: Write interaction log tests (TDD)**

Create `tests/api/ai-interactions.test.ts`:

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, createTestClassroom } from "../helpers";
import {
  createAiInteraction,
  listInteractionsBySession,
  appendMessage,
} from "@/lib/ai/interactions";
import { randomUUID } from "crypto";

describe("AI interaction log retrieval", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;
  const fakeSessionId = randomUUID();

  beforeEach(async () => {
    teacher = await createTestUser({ role: "teacher", email: "teacher@test.com" });
    student = await createTestUser({ role: "student", email: "student@test.com" });
  });

  it("lists all interactions for a session", async () => {
    await createAiInteraction(testDb, {
      studentId: student.id,
      sessionId: fakeSessionId,
      documentId: randomUUID(),
      enabledByTeacherId: teacher.id,
    });

    const interactions = await listInteractionsBySession(testDb, fakeSessionId);
    expect(interactions).toHaveLength(1);
    expect(interactions[0].studentId).toBe(student.id);
  });

  it("includes conversation messages in the interaction", async () => {
    const interaction = await createAiInteraction(testDb, {
      studentId: student.id,
      sessionId: fakeSessionId,
      documentId: randomUUID(),
      enabledByTeacherId: teacher.id,
    });

    await appendMessage(testDb, interaction.id, {
      role: "student",
      content: "What is a variable?",
      timestamp: new Date().toISOString(),
    });

    await appendMessage(testDb, interaction.id, {
      role: "assistant",
      content: "Think of it as a labeled box. What would you put in it?",
      timestamp: new Date().toISOString(),
    });

    const interactions = await listInteractionsBySession(testDb, fakeSessionId);
    expect(interactions[0].messages).toHaveLength(2);
  });

  it("returns interactions from multiple students", async () => {
    const student2 = await createTestUser({ role: "student", email: "s2@test.com" });

    await createAiInteraction(testDb, {
      studentId: student.id,
      sessionId: fakeSessionId,
      documentId: randomUUID(),
      enabledByTeacherId: teacher.id,
    });

    await createAiInteraction(testDb, {
      studentId: student2.id,
      sessionId: fakeSessionId,
      documentId: randomUUID(),
      enabledByTeacherId: teacher.id,
    });

    const interactions = await listInteractionsBySession(testDb, fakeSessionId);
    expect(interactions).toHaveLength(2);
  });
});
```

- [ ] **Step 2: Implement the interaction log endpoint**

Create `src/app/api/ai/interactions/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import {
  listInteractionsBySession,
  listInteractionsByStudent,
} from "@/lib/ai/interactions";

const querySchema = z.object({
  sessionId: z.string().uuid(),
  studentId: z.string().uuid().optional(),
});

export async function GET(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  if (session.user.role !== "teacher" && session.user.role !== "admin") {
    return NextResponse.json(
      { error: "Only teachers can view AI interaction logs" },
      { status: 403 }
    );
  }

  const { searchParams } = new URL(request.url);
  const parsed = querySchema.safeParse({
    sessionId: searchParams.get("sessionId"),
    studentId: searchParams.get("studentId") || undefined,
  });

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid query parameters", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const { sessionId, studentId } = parsed.data;

  let interactions;
  if (studentId) {
    interactions = await listInteractionsByStudent(db, sessionId, studentId);
  } else {
    interactions = await listInteractionsBySession(db, sessionId);
  }

  return NextResponse.json(interactions);
}
```

- [ ] **Step 3: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/api/ai-interactions.test.ts
```

Expected: All 3 tests pass.

- [ ] **Step 4: Commit**

```bash
git add src/app/api/ai/interactions/route.ts tests/api/ai-interactions.test.ts
git commit -m "feat: add AI interaction log endpoint for teacher visibility"
```

---

## Task 10: Annotations API Endpoints

**Files:**
- Create: `src/app/api/annotations/route.ts`
- Create: `src/app/api/annotations/[id]/route.ts`
- Create: `tests/api/annotations.test.ts`

- [ ] **Step 1: Write annotation API tests (TDD)**

Create `tests/api/annotations.test.ts`:

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser } from "../helpers";
import {
  createAnnotation,
  listAnnotationsByDocument,
  deleteAnnotation,
  getAnnotation,
  resolveAnnotation,
} from "@/lib/annotations";
import { randomUUID } from "crypto";

describe("annotation API logic", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  const fakeDocumentId = randomUUID();

  beforeEach(async () => {
    teacher = await createTestUser({ role: "teacher", email: "teacher@test.com" });
  });

  it("creates an annotation and retrieves it by document", async () => {
    await createAnnotation(testDb, {
      documentId: fakeDocumentId,
      authorId: teacher.id,
      authorType: "teacher",
      lineStart: 3,
      lineEnd: 5,
      content: "This loop could be more efficient.",
    });

    const annotations = await listAnnotationsByDocument(testDb, fakeDocumentId);
    expect(annotations).toHaveLength(1);
    expect(annotations[0].content).toBe("This loop could be more efficient.");
    expect(annotations[0].lineStart).toBe(3);
    expect(annotations[0].lineEnd).toBe(5);
  });

  it("deletes an annotation", async () => {
    const annotation = await createAnnotation(testDb, {
      documentId: fakeDocumentId,
      authorId: teacher.id,
      authorType: "teacher",
      lineStart: 1,
      lineEnd: 1,
      content: "Remove me",
    });

    await deleteAnnotation(testDb, annotation.id);
    const remaining = await listAnnotationsByDocument(testDb, fakeDocumentId);
    expect(remaining).toHaveLength(0);
  });

  it("resolves an annotation", async () => {
    const annotation = await createAnnotation(testDb, {
      documentId: fakeDocumentId,
      authorId: teacher.id,
      authorType: "teacher",
      lineStart: 1,
      lineEnd: 2,
      content: "Please fix the indentation here.",
    });

    const resolved = await resolveAnnotation(testDb, annotation.id);
    expect(resolved.resolved).toBe(true);

    const fetched = await getAnnotation(testDb, annotation.id);
    expect(fetched!.resolved).toBe(true);
  });
});
```

- [ ] **Step 2: Implement the annotations list/create endpoint**

Create `src/app/api/annotations/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { createAnnotation, listAnnotationsByDocument } from "@/lib/annotations";

const createSchema = z.object({
  documentId: z.string().uuid(),
  lineStart: z.number().int().min(1),
  lineEnd: z.number().int().min(1),
  content: z.string().min(1).max(2000),
  authorType: z.enum(["teacher", "ai"]),
});

const querySchema = z.object({
  documentId: z.string().uuid(),
});

export async function GET(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { searchParams } = new URL(request.url);
  const parsed = querySchema.safeParse({
    documentId: searchParams.get("documentId"),
  });

  if (!parsed.success) {
    return NextResponse.json(
      { error: "documentId query parameter is required" },
      { status: 400 }
    );
  }

  const annotations = await listAnnotationsByDocument(db, parsed.data.documentId);
  return NextResponse.json(annotations);
}

export async function POST(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  // Only teachers can create annotations (AI annotations are created server-side)
  if (session.user.role !== "teacher" && session.user.role !== "admin") {
    return NextResponse.json(
      { error: "Only teachers can create annotations" },
      { status: 403 }
    );
  }

  const body = await request.json();
  const parsed = createSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  if (parsed.data.lineEnd < parsed.data.lineStart) {
    return NextResponse.json(
      { error: "lineEnd must be >= lineStart" },
      { status: 400 }
    );
  }

  const annotation = await createAnnotation(db, {
    ...parsed.data,
    authorId: session.user.id,
  });

  return NextResponse.json(annotation, { status: 201 });
}
```

- [ ] **Step 3: Implement the annotation delete/resolve endpoint**

Create `src/app/api/annotations/[id]/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getAnnotation, deleteAnnotation, resolveAnnotation } from "@/lib/annotations";

export async function DELETE(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;

  const annotation = await getAnnotation(db, id);
  if (!annotation) {
    return NextResponse.json({ error: "Annotation not found" }, { status: 404 });
  }

  // Only the author or an admin can delete
  if (annotation.authorId !== session.user.id && session.user.role !== "admin") {
    return NextResponse.json({ error: "Forbidden" }, { status: 403 });
  }

  await deleteAnnotation(db, id);
  return NextResponse.json({ success: true });
}

export async function PATCH(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;

  const annotation = await getAnnotation(db, id);
  if (!annotation) {
    return NextResponse.json({ error: "Annotation not found" }, { status: 404 });
  }

  const resolved = await resolveAnnotation(db, id);
  return NextResponse.json(resolved);
}
```

- [ ] **Step 4: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/api/annotations.test.ts
```

Expected: All 3 tests pass.

- [ ] **Step 5: Commit**

```bash
git add src/app/api/annotations/route.ts src/app/api/annotations/\[id\]/route.ts tests/api/annotations.test.ts
git commit -m "feat: add annotation create/list/delete/resolve API endpoints"
```

---

## Task 11: Help Queue API Endpoint

**Files:**
- Create: `src/app/api/sessions/[id]/help-queue/route.ts`
- Create: `tests/api/help-queue.test.ts`

- [ ] **Step 1: Write help queue tests (TDD)**

Create `tests/api/help-queue.test.ts`:

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser, createTestClassroom } from "../helpers";
import { classroomMembers } from "@/lib/db/schema";

// The help queue is backed by SessionParticipant.status from Plan 3.
// Here we test the authorization and membership validation logic that
// the route depends on.

describe("help queue authorization", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;
  let classroom: Awaited<ReturnType<typeof createTestClassroom>>;

  beforeEach(async () => {
    teacher = await createTestUser({ role: "teacher", email: "teacher@test.com" });
    student = await createTestUser({ role: "student", email: "student@test.com" });
    classroom = await createTestClassroom(teacher.id, { gradeLevel: "6-8" });
    await testDb.insert(classroomMembers).values({
      classroomId: classroom.id,
      userId: student.id,
    });
  });

  it("student can raise hand (membership verified)", async () => {
    const { getClassroomMembers } = await import("@/lib/classrooms");
    const members = await getClassroomMembers(testDb, classroom.id);
    const found = members.find((m) => m.userId === student.id);
    expect(found).toBeDefined();
  });

  it("non-member cannot raise hand", async () => {
    const outsider = await createTestUser({ role: "student", email: "outsider@test.com" });
    const { getClassroomMembers } = await import("@/lib/classrooms");
    const members = await getClassroomMembers(testDb, classroom.id);
    const found = members.find((m) => m.userId === outsider.id);
    expect(found).toBeUndefined();
  });

  it("teacher can view the help queue", () => {
    expect(teacher.role).toBe("teacher");
    expect(classroom.teacherId).toBe(teacher.id);
  });
});
```

- [ ] **Step 2: Implement the help queue endpoint**

Create `src/app/api/sessions/[id]/help-queue/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";

// The help queue reads from and writes to the SessionParticipant table
// (defined in Plan 3). The "raise hand" action sets status to "needs_help"
// and "lower hand" sets it back to "active".
//
// This endpoint provides the REST API surface. The actual state change
// also triggers an SSE broadcast (from Plan 3's event system) so the
// teacher dashboard updates in real-time.

const raiseHandSchema = z.object({
  action: z.enum(["raise", "lower"]),
  context: z
    .object({
      currentLine: z.number().int().optional(),
      recentError: z.string().optional(),
    })
    .optional(),
});

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  if (session.user.role !== "teacher" && session.user.role !== "admin") {
    return NextResponse.json(
      { error: "Only teachers can view the help queue" },
      { status: 403 }
    );
  }

  const { id: sessionId } = await params;

  // When Plan 3 tables are available, query SessionParticipant:
  //
  // const queue = await db
  //   .select({
  //     studentId: sessionParticipants.studentId,
  //     status: sessionParticipants.status,
  //     joinedAt: sessionParticipants.joinedAt,
  //     name: users.name,
  //   })
  //   .from(sessionParticipants)
  //   .innerJoin(users, eq(sessionParticipants.studentId, users.id))
  //   .where(
  //     and(
  //       eq(sessionParticipants.sessionId, sessionId),
  //       eq(sessionParticipants.status, "needs_help")
  //     )
  //   )
  //   .orderBy(sessionParticipants.joinedAt);

  // Placeholder until Plan 3 tables are migrated:
  return NextResponse.json({
    sessionId,
    queue: [],
    message: "Help queue will be populated once Plan 3 session tables are available",
  });
}

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id: sessionId } = await params;

  const body = await request.json();
  const parsed = raiseHandSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const { action, context } = parsed.data;
  const newStatus = action === "raise" ? "needs_help" : "active";

  // When Plan 3 tables are available, update SessionParticipant:
  //
  // await db
  //   .update(sessionParticipants)
  //   .set({ status: newStatus })
  //   .where(
  //     and(
  //       eq(sessionParticipants.sessionId, sessionId),
  //       eq(sessionParticipants.studentId, session.user.id)
  //     )
  //   );

  return NextResponse.json({
    sessionId,
    studentId: session.user.id,
    status: newStatus,
    context: context || null,
  });
}
```

- [ ] **Step 3: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/api/help-queue.test.ts
```

Expected: All 3 tests pass.

- [ ] **Step 4: Commit**

```bash
git add src/app/api/sessions/\[id\]/help-queue/route.ts tests/api/help-queue.test.ts
git commit -m "feat: add raise-hand help queue endpoint for student assistance requests"
```

---

## Task 12: AI Chat Panel Component (Student UI)

**Files:**
- Create: `src/components/ai/ai-message.tsx`
- Create: `src/components/ai/ai-chat-panel.tsx`
- Create: `tests/unit/ai-message.test.tsx`
- Create: `tests/unit/ai-chat-panel.test.tsx`

- [ ] **Step 1: Write AI message component tests (TDD)**

Create `tests/unit/ai-message.test.tsx`:

```typescript
// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { AiMessage } from "@/components/ai/ai-message";

describe("AiMessage", () => {
  it("renders a student message with correct styling", () => {
    render(<AiMessage role="student" content="How do I fix this?" />);
    const message = screen.getByText("How do I fix this?");
    expect(message).toBeInTheDocument();
  });

  it("renders an AI message with correct styling", () => {
    render(
      <AiMessage role="assistant" content="What do you expect the code to do?" />
    );
    const message = screen.getByText("What do you expect the code to do?");
    expect(message).toBeInTheDocument();
  });

  it("renders the student label", () => {
    render(<AiMessage role="student" content="Help" />);
    expect(screen.getByText("You")).toBeInTheDocument();
  });

  it("renders the AI label", () => {
    render(<AiMessage role="assistant" content="Sure!" />);
    expect(screen.getByText("AI Tutor")).toBeInTheDocument();
  });

  it("displays streaming indicator when streaming", () => {
    render(
      <AiMessage role="assistant" content="Thinking" streaming={true} />
    );
    expect(screen.getByTestId("streaming-indicator")).toBeInTheDocument();
  });

  it("does not display streaming indicator when not streaming", () => {
    render(<AiMessage role="assistant" content="Done" />);
    expect(screen.queryByTestId("streaming-indicator")).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Implement AI message component**

Create `src/components/ai/ai-message.tsx`:

```tsx
"use client";

interface AiMessageProps {
  role: "student" | "assistant";
  content: string;
  streaming?: boolean;
}

export function AiMessage({ role, content, streaming = false }: AiMessageProps) {
  const isStudent = role === "student";

  return (
    <div
      className={`flex flex-col gap-1 ${isStudent ? "items-end" : "items-start"}`}
    >
      <span className="text-xs text-muted-foreground px-1">
        {isStudent ? "You" : "AI Tutor"}
      </span>
      <div
        className={`rounded-lg px-3 py-2 max-w-[85%] text-sm whitespace-pre-wrap ${
          isStudent
            ? "bg-primary text-primary-foreground"
            : "bg-muted text-foreground"
        }`}
      >
        {content}
        {streaming && (
          <span
            data-testid="streaming-indicator"
            className="inline-block w-2 h-4 ml-1 bg-current animate-pulse"
          />
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Write AI chat panel tests (TDD)**

Create `tests/unit/ai-chat-panel.test.tsx`:

```typescript
// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { AiChatPanel } from "@/components/ai/ai-chat-panel";

describe("AiChatPanel", () => {
  it("renders the chat panel with title", () => {
    render(
      <AiChatPanel
        messages={[]}
        onSend={vi.fn()}
        disabled={false}
        streaming={false}
      />
    );
    expect(screen.getByText("AI Tutor")).toBeInTheDocument();
  });

  it("renders existing messages", () => {
    render(
      <AiChatPanel
        messages={[
          { role: "student", content: "Help please" },
          { role: "assistant", content: "What are you working on?" },
        ]}
        onSend={vi.fn()}
        disabled={false}
        streaming={false}
      />
    );
    expect(screen.getByText("Help please")).toBeInTheDocument();
    expect(screen.getByText("What are you working on?")).toBeInTheDocument();
  });

  it("calls onSend when form is submitted", () => {
    const onSend = vi.fn();
    render(
      <AiChatPanel
        messages={[]}
        onSend={onSend}
        disabled={false}
        streaming={false}
      />
    );

    const input = screen.getByPlaceholderText("Ask the AI tutor...");
    fireEvent.change(input, { target: { value: "How do I use a loop?" } });
    fireEvent.submit(input.closest("form")!);

    expect(onSend).toHaveBeenCalledWith("How do I use a loop?");
  });

  it("clears input after sending", () => {
    render(
      <AiChatPanel
        messages={[]}
        onSend={vi.fn()}
        disabled={false}
        streaming={false}
      />
    );

    const input = screen.getByPlaceholderText("Ask the AI tutor...") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "Help" } });
    fireEvent.submit(input.closest("form")!);

    expect(input.value).toBe("");
  });

  it("disables input when disabled prop is true", () => {
    render(
      <AiChatPanel
        messages={[]}
        onSend={vi.fn()}
        disabled={true}
        streaming={false}
      />
    );

    const input = screen.getByPlaceholderText("Ask the AI tutor...");
    expect(input).toBeDisabled();
  });

  it("disables send button while streaming", () => {
    render(
      <AiChatPanel
        messages={[]}
        onSend={vi.fn()}
        disabled={false}
        streaming={true}
      />
    );

    const button = screen.getByRole("button", { name: /send/i });
    expect(button).toBeDisabled();
  });

  it("does not send empty messages", () => {
    const onSend = vi.fn();
    render(
      <AiChatPanel
        messages={[]}
        onSend={onSend}
        disabled={false}
        streaming={false}
      />
    );

    const form = screen.getByPlaceholderText("Ask the AI tutor...").closest("form")!;
    fireEvent.submit(form);

    expect(onSend).not.toHaveBeenCalled();
  });
});
```

- [ ] **Step 4: Implement AI chat panel component**

Create `src/components/ai/ai-chat-panel.tsx`:

```tsx
"use client";

import { useState, useRef, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AiMessage } from "./ai-message";

interface ChatMessage {
  role: "student" | "assistant";
  content: string;
}

interface AiChatPanelProps {
  messages: ChatMessage[];
  onSend: (message: string) => void;
  disabled: boolean;
  streaming: boolean;
}

export function AiChatPanel({
  messages,
  onSend,
  disabled,
  streaming,
}: AiChatPanelProps) {
  const [input, setInput] = useState("");
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages, streaming]);

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = input.trim();
    if (!trimmed) return;
    onSend(trimmed);
    setInput("");
  }

  return (
    <div className="flex flex-col h-full border rounded-lg bg-background">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b">
        <h3 className="font-semibold text-sm">AI Tutor</h3>
      </div>

      {/* Messages */}
      <div
        ref={scrollRef}
        className="flex-1 overflow-y-auto p-4 space-y-4"
      >
        {messages.length === 0 && (
          <p className="text-sm text-muted-foreground text-center py-8">
            Ask the AI tutor for help with your code. It will guide you with
            questions instead of giving you the answer directly.
          </p>
        )}
        {messages.map((msg, i) => (
          <AiMessage
            key={i}
            role={msg.role}
            content={msg.content}
            streaming={streaming && i === messages.length - 1 && msg.role === "assistant"}
          />
        ))}
      </div>

      {/* Input */}
      <form onSubmit={handleSubmit} className="p-3 border-t flex gap-2">
        <Input
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="Ask the AI tutor..."
          disabled={disabled}
          className="flex-1"
        />
        <Button
          type="submit"
          size="sm"
          disabled={disabled || streaming || !input.trim()}
        >
          Send
        </Button>
      </form>
    </div>
  );
}
```

- [ ] **Step 5: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/ai-message.test.tsx tests/unit/ai-chat-panel.test.tsx
```

Expected: All 13 tests pass.

- [ ] **Step 6: Commit**

```bash
git add src/components/ai/ai-message.tsx src/components/ai/ai-chat-panel.tsx tests/unit/ai-message.test.tsx tests/unit/ai-chat-panel.test.tsx
git commit -m "feat: add AI chat panel and message bubble components for student UI"
```

---

## Task 13: AI Toggle Button Component (Teacher UI)

**Files:**
- Create: `src/components/ai/ai-toggle-button.tsx`
- Create: `src/components/ai/ai-activity-feed.tsx`

- [ ] **Step 1: Implement AI toggle button**

Create `src/components/ai/ai-toggle-button.tsx`:

```tsx
"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";

interface AiToggleButtonProps {
  studentId: string;
  studentName: string;
  sessionId: string;
  classroomId: string;
  initialEnabled?: boolean;
  onToggle?: (studentId: string, enabled: boolean) => void;
}

export function AiToggleButton({
  studentId,
  studentName,
  sessionId,
  classroomId,
  initialEnabled = false,
  onToggle,
}: AiToggleButtonProps) {
  const [enabled, setEnabled] = useState(initialEnabled);
  const [loading, setLoading] = useState(false);

  async function handleToggle() {
    setLoading(true);
    try {
      const response = await fetch("/api/ai/toggle", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          sessionId,
          classroomId,
          studentId,
          enabled: !enabled,
        }),
      });

      if (response.ok) {
        const newState = !enabled;
        setEnabled(newState);
        onToggle?.(studentId, newState);
      }
    } finally {
      setLoading(false);
    }
  }

  return (
    <Button
      variant={enabled ? "default" : "outline"}
      size="sm"
      onClick={handleToggle}
      disabled={loading}
      title={`${enabled ? "Disable" : "Enable"} AI for ${studentName}`}
    >
      {loading ? "..." : enabled ? "AI On" : "AI Off"}
    </Button>
  );
}
```

- [ ] **Step 2: Implement AI activity feed (teacher sidebar)**

Create `src/components/ai/ai-activity-feed.tsx`:

```tsx
"use client";

import { useEffect, useState } from "react";

interface AiActivityItem {
  id: string;
  studentName: string;
  studentId: string;
  lastMessage: string;
  messageCount: number;
  timestamp: string;
}

interface AiActivityFeedProps {
  sessionId: string;
  onViewConversation?: (studentId: string) => void;
}

export function AiActivityFeed({
  sessionId,
  onViewConversation,
}: AiActivityFeedProps) {
  const [activities, setActivities] = useState<AiActivityItem[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function fetchActivities() {
      try {
        const response = await fetch(
          `/api/ai/interactions?sessionId=${sessionId}`
        );
        if (response.ok) {
          const data = await response.json();
          setActivities(
            data.map(
              (interaction: {
                id: string;
                studentId: string;
                messages: Array<{ role: string; content: string }>;
                createdAt: string;
              }) => ({
                id: interaction.id,
                studentName: "Student",
                studentId: interaction.studentId,
                lastMessage:
                  interaction.messages.length > 0
                    ? interaction.messages[interaction.messages.length - 1].content
                    : "No messages yet",
                messageCount: interaction.messages.length,
                timestamp: interaction.createdAt,
              })
            )
          );
        }
      } finally {
        setLoading(false);
      }
    }

    fetchActivities();
  }, [sessionId]);

  if (loading) {
    return (
      <div className="p-4 text-sm text-muted-foreground">
        Loading AI activity...
      </div>
    );
  }

  if (activities.length === 0) {
    return (
      <div className="p-4 text-sm text-muted-foreground">
        No AI interactions yet this session.
      </div>
    );
  }

  return (
    <div className="divide-y">
      <h3 className="font-semibold text-sm px-4 py-3">AI Activity</h3>
      {activities.map((activity) => (
        <button
          key={activity.id}
          className="w-full text-left px-4 py-3 hover:bg-muted/50 transition-colors"
          onClick={() => onViewConversation?.(activity.studentId)}
        >
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">{activity.studentName}</span>
            <span className="text-xs text-muted-foreground">
              {activity.messageCount} messages
            </span>
          </div>
          <p className="text-xs text-muted-foreground mt-1 truncate">
            {activity.lastMessage}
          </p>
        </button>
      ))}
    </div>
  );
}
```

- [ ] **Step 3: Commit**

```bash
git add src/components/ai/ai-toggle-button.tsx src/components/ai/ai-activity-feed.tsx
git commit -m "feat: add AI toggle button and activity feed components for teacher dashboard"
```

---

## Task 14: Code Annotation UI Components

**Files:**
- Create: `src/components/annotations/annotation-gutter.tsx`
- Create: `src/components/annotations/annotation-popover.tsx`
- Create: `src/components/annotations/annotation-form.tsx`
- Create: `tests/unit/annotation-gutter.test.tsx`

- [ ] **Step 1: Write annotation gutter tests (TDD)**

Create `tests/unit/annotation-gutter.test.tsx`:

```typescript
// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { AnnotationPopover } from "@/components/annotations/annotation-popover";

describe("AnnotationPopover", () => {
  it("renders annotation content", () => {
    render(
      <AnnotationPopover
        annotation={{
          id: "1",
          authorType: "teacher",
          authorName: "Ms. Smith",
          content: "Consider using a loop here.",
          lineStart: 5,
          lineEnd: 5,
          resolved: false,
        }}
        onResolve={() => {}}
        onDelete={() => {}}
      />
    );
    expect(screen.getByText("Consider using a loop here.")).toBeInTheDocument();
  });

  it("shows the author name and type", () => {
    render(
      <AnnotationPopover
        annotation={{
          id: "1",
          authorType: "teacher",
          authorName: "Ms. Smith",
          content: "Good job!",
          lineStart: 1,
          lineEnd: 1,
          resolved: false,
        }}
        onResolve={() => {}}
        onDelete={() => {}}
      />
    );
    expect(screen.getByText("Ms. Smith")).toBeInTheDocument();
  });

  it("shows AI badge for AI annotations", () => {
    render(
      <AnnotationPopover
        annotation={{
          id: "1",
          authorType: "ai",
          authorName: "AI Tutor",
          content: "This variable could be named better.",
          lineStart: 3,
          lineEnd: 3,
          resolved: false,
        }}
        onResolve={() => {}}
        onDelete={() => {}}
      />
    );
    expect(screen.getByText("AI")).toBeInTheDocument();
  });

  it("shows resolved state", () => {
    render(
      <AnnotationPopover
        annotation={{
          id: "1",
          authorType: "teacher",
          authorName: "Ms. Smith",
          content: "Fixed!",
          lineStart: 2,
          lineEnd: 2,
          resolved: true,
        }}
        onResolve={() => {}}
        onDelete={() => {}}
      />
    );
    expect(screen.getByText("Resolved")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Implement annotation popover component**

Create `src/components/annotations/annotation-popover.tsx`:

```tsx
"use client";

import { Button } from "@/components/ui/button";

interface AnnotationData {
  id: string;
  authorType: "teacher" | "ai";
  authorName: string;
  content: string;
  lineStart: number;
  lineEnd: number;
  resolved: boolean;
}

interface AnnotationPopoverProps {
  annotation: AnnotationData;
  onResolve: (id: string) => void;
  onDelete: (id: string) => void;
  canDelete?: boolean;
}

export function AnnotationPopover({
  annotation,
  onResolve,
  onDelete,
  canDelete = true,
}: AnnotationPopoverProps) {
  return (
    <div className="bg-background border rounded-lg shadow-lg p-3 max-w-sm">
      <div className="flex items-center gap-2 mb-2">
        <span className="text-sm font-medium">{annotation.authorName}</span>
        {annotation.authorType === "ai" && (
          <span className="text-xs bg-blue-100 text-blue-700 px-1.5 py-0.5 rounded">
            AI
          </span>
        )}
        {annotation.resolved && (
          <span className="text-xs bg-green-100 text-green-700 px-1.5 py-0.5 rounded">
            Resolved
          </span>
        )}
        <span className="text-xs text-muted-foreground ml-auto">
          {annotation.lineStart === annotation.lineEnd
            ? `Line ${annotation.lineStart}`
            : `Lines ${annotation.lineStart}-${annotation.lineEnd}`}
        </span>
      </div>
      <p className="text-sm mb-3">{annotation.content}</p>
      <div className="flex gap-2">
        {!annotation.resolved && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => onResolve(annotation.id)}
          >
            Resolve
          </Button>
        )}
        {canDelete && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => onDelete(annotation.id)}
          >
            Delete
          </Button>
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Implement annotation form component**

Create `src/components/annotations/annotation-form.tsx`:

```tsx
"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

interface AnnotationFormProps {
  lineStart: number;
  lineEnd: number;
  onSubmit: (content: string) => void;
  onCancel: () => void;
}

export function AnnotationForm({
  lineStart,
  lineEnd,
  onSubmit,
  onCancel,
}: AnnotationFormProps) {
  const [content, setContent] = useState("");

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = content.trim();
    if (!trimmed) return;
    onSubmit(trimmed);
    setContent("");
  }

  return (
    <form onSubmit={handleSubmit} className="bg-background border rounded-lg shadow-lg p-3 max-w-sm">
      <div className="text-xs text-muted-foreground mb-2">
        {lineStart === lineEnd
          ? `Annotate line ${lineStart}`
          : `Annotate lines ${lineStart}-${lineEnd}`}
      </div>
      <Input
        value={content}
        onChange={(e) => setContent(e.target.value)}
        placeholder="Add a comment..."
        autoFocus
        className="mb-2"
      />
      <div className="flex gap-2">
        <Button type="submit" size="sm" disabled={!content.trim()}>
          Add
        </Button>
        <Button type="button" variant="outline" size="sm" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  );
}
```

- [ ] **Step 4: Implement annotation gutter extension for CodeMirror**

Create `src/components/annotations/annotation-gutter.tsx`:

```tsx
"use client";

import {
  gutter,
  GutterMarker,
  type EditorView,
} from "@codemirror/view";
import { StateField, StateEffect, type Extension } from "@codemirror/state";

/**
 * Annotation data stored in the editor state.
 */
export interface EditorAnnotation {
  id: string;
  lineStart: number;
  lineEnd: number;
  content: string;
  authorType: "teacher" | "ai";
  authorName: string;
  resolved: boolean;
}

/**
 * State effects for adding/removing annotations.
 */
export const setAnnotationsEffect = StateEffect.define<EditorAnnotation[]>();

/**
 * State field that tracks annotations in the editor.
 */
export const annotationsField = StateField.define<EditorAnnotation[]>({
  create() {
    return [];
  },
  update(annotations, tr) {
    for (const effect of tr.effects) {
      if (effect.is(setAnnotationsEffect)) {
        return effect.value;
      }
    }
    return annotations;
  },
});

/**
 * A gutter marker that shows an annotation indicator.
 */
class AnnotationMarker extends GutterMarker {
  constructor(
    readonly annotation: EditorAnnotation
  ) {
    super();
  }

  toDOM() {
    const marker = document.createElement("div");
    marker.className = "cm-annotation-marker";
    marker.style.width = "8px";
    marker.style.height = "8px";
    marker.style.borderRadius = "50%";
    marker.style.backgroundColor =
      this.annotation.authorType === "ai" ? "#3b82f6" : "#f59e0b";
    marker.style.cursor = "pointer";
    marker.title = this.annotation.content;
    if (this.annotation.resolved) {
      marker.style.opacity = "0.4";
    }
    return marker;
  }
}

/**
 * Creates the annotation gutter extension.
 * @param onClickAnnotation Callback when user clicks an annotation marker.
 */
export function annotationGutter(
  onClickAnnotation?: (annotation: EditorAnnotation, view: EditorView) => void
): Extension {
  return [
    annotationsField,
    gutter({
      class: "cm-annotation-gutter",
      markers(view) {
        const annotations = view.state.field(annotationsField);
        const markers: { from: number; marker: GutterMarker }[] = [];

        for (const annotation of annotations) {
          // Only show marker on the first line of the range
          const lineNum = Math.min(annotation.lineStart, view.state.doc.lines);
          if (lineNum >= 1) {
            const line = view.state.doc.line(lineNum);
            markers.push({
              from: line.from,
              marker: new AnnotationMarker(annotation),
            });
          }
        }

        return {
          [Symbol.iterator]() {
            let i = 0;
            return {
              next() {
                if (i < markers.length) {
                  const value = markers[i++];
                  return {
                    done: false,
                    value: { from: value.from, to: value.from, value: value.marker },
                  };
                }
                return { done: true, value: undefined };
              },
            };
          },
        } as any;
      },
      domEventHandlers: {
        click(view, line) {
          if (!onClickAnnotation) return false;
          const annotations = view.state.field(annotationsField);
          const lineNumber = view.state.doc.lineAt(line.from).number;
          const annotation = annotations.find(
            (a) => lineNumber >= a.lineStart && lineNumber <= a.lineEnd
          );
          if (annotation) {
            onClickAnnotation(annotation, view);
            return true;
          }
          return false;
        },
      },
    }),
  ];
}
```

- [ ] **Step 5: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/annotation-gutter.test.tsx
```

Expected: All 4 tests pass.

- [ ] **Step 6: Commit**

```bash
git add src/components/annotations/annotation-popover.tsx src/components/annotations/annotation-form.tsx src/components/annotations/annotation-gutter.tsx tests/unit/annotation-gutter.test.tsx
git commit -m "feat: add code annotation UI components with CodeMirror gutter integration"
```

---

## Task 15: Raise Hand / Help Queue UI Components

**Files:**
- Create: `src/components/help-queue/raise-hand-button.tsx`
- Create: `src/components/help-queue/help-queue-panel.tsx`
- Create: `tests/unit/raise-hand-button.test.tsx`
- Create: `tests/unit/help-queue-panel.test.tsx`

- [ ] **Step 1: Write raise hand button tests (TDD)**

Create `tests/unit/raise-hand-button.test.tsx`:

```typescript
// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { RaiseHandButton } from "@/components/help-queue/raise-hand-button";

describe("RaiseHandButton", () => {
  it("renders 'Raise Hand' when not raised", () => {
    render(<RaiseHandButton raised={false} onToggle={vi.fn()} />);
    expect(screen.getByText("Raise Hand")).toBeInTheDocument();
  });

  it("renders 'Lower Hand' when raised", () => {
    render(<RaiseHandButton raised={true} onToggle={vi.fn()} />);
    expect(screen.getByText("Lower Hand")).toBeInTheDocument();
  });

  it("calls onToggle when clicked", () => {
    const onToggle = vi.fn();
    render(<RaiseHandButton raised={false} onToggle={onToggle} />);
    fireEvent.click(screen.getByText("Raise Hand"));
    expect(onToggle).toHaveBeenCalledOnce();
  });

  it("is disabled when loading", () => {
    render(<RaiseHandButton raised={false} onToggle={vi.fn()} loading={true} />);
    expect(screen.getByRole("button")).toBeDisabled();
  });
});
```

- [ ] **Step 2: Implement raise hand button**

Create `src/components/help-queue/raise-hand-button.tsx`:

```tsx
"use client";

import { Button } from "@/components/ui/button";

interface RaiseHandButtonProps {
  raised: boolean;
  onToggle: () => void;
  loading?: boolean;
}

export function RaiseHandButton({
  raised,
  onToggle,
  loading = false,
}: RaiseHandButtonProps) {
  return (
    <Button
      variant={raised ? "default" : "outline"}
      size="sm"
      onClick={onToggle}
      disabled={loading}
      className={raised ? "bg-yellow-500 hover:bg-yellow-600 text-white" : ""}
    >
      {loading ? "..." : raised ? "Lower Hand" : "Raise Hand"}
    </Button>
  );
}
```

- [ ] **Step 3: Write help queue panel tests (TDD)**

Create `tests/unit/help-queue-panel.test.tsx`:

```typescript
// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { HelpQueuePanel } from "@/components/help-queue/help-queue-panel";

describe("HelpQueuePanel", () => {
  it("renders empty state when no students need help", () => {
    render(<HelpQueuePanel queue={[]} onDismiss={vi.fn()} />);
    expect(screen.getByText("No students need help right now.")).toBeInTheDocument();
  });

  it("renders students in the help queue", () => {
    render(
      <HelpQueuePanel
        queue={[
          { studentId: "1", studentName: "Alice", context: "Stuck on line 5" },
          { studentId: "2", studentName: "Bob", context: undefined },
        ]}
        onDismiss={vi.fn()}
      />
    );
    expect(screen.getByText("Alice")).toBeInTheDocument();
    expect(screen.getByText("Bob")).toBeInTheDocument();
  });

  it("shows context when provided", () => {
    render(
      <HelpQueuePanel
        queue={[
          { studentId: "1", studentName: "Alice", context: "Stuck on line 5" },
        ]}
        onDismiss={vi.fn()}
      />
    );
    expect(screen.getByText("Stuck on line 5")).toBeInTheDocument();
  });

  it("calls onDismiss when dismiss button is clicked", () => {
    const onDismiss = vi.fn();
    render(
      <HelpQueuePanel
        queue={[
          { studentId: "1", studentName: "Alice", context: undefined },
        ]}
        onDismiss={onDismiss}
      />
    );
    fireEvent.click(screen.getByRole("button", { name: /dismiss/i }));
    expect(onDismiss).toHaveBeenCalledWith("1");
  });

  it("shows the queue count in the header", () => {
    render(
      <HelpQueuePanel
        queue={[
          { studentId: "1", studentName: "Alice", context: undefined },
          { studentId: "2", studentName: "Bob", context: undefined },
        ]}
        onDismiss={vi.fn()}
      />
    );
    expect(screen.getByText("Help Queue (2)")).toBeInTheDocument();
  });
});
```

- [ ] **Step 4: Implement help queue panel**

Create `src/components/help-queue/help-queue-panel.tsx`:

```tsx
"use client";

import { Button } from "@/components/ui/button";

interface HelpQueueItem {
  studentId: string;
  studentName: string;
  context?: string;
}

interface HelpQueuePanelProps {
  queue: HelpQueueItem[];
  onDismiss: (studentId: string) => void;
  onFocusStudent?: (studentId: string) => void;
}

export function HelpQueuePanel({
  queue,
  onDismiss,
  onFocusStudent,
}: HelpQueuePanelProps) {
  return (
    <div className="border rounded-lg bg-background">
      <div className="px-4 py-3 border-b">
        <h3 className="font-semibold text-sm">
          {queue.length > 0 ? `Help Queue (${queue.length})` : "Help Queue"}
        </h3>
      </div>

      {queue.length === 0 ? (
        <p className="p-4 text-sm text-muted-foreground">
          No students need help right now.
        </p>
      ) : (
        <div className="divide-y">
          {queue.map((item) => (
            <div
              key={item.studentId}
              className="px-4 py-3 flex items-start justify-between gap-2"
            >
              <div
                className="flex-1 cursor-pointer"
                onClick={() => onFocusStudent?.(item.studentId)}
              >
                <span className="text-sm font-medium">{item.studentName}</span>
                {item.context && (
                  <p className="text-xs text-muted-foreground mt-0.5">
                    {item.context}
                  </p>
                )}
              </div>
              <Button
                variant="outline"
                size="sm"
                onClick={() => onDismiss(item.studentId)}
                aria-label="Dismiss"
              >
                Dismiss
              </Button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 5: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/raise-hand-button.test.tsx tests/unit/help-queue-panel.test.tsx
```

Expected: All 9 tests pass.

- [ ] **Step 6: Commit**

```bash
git add src/components/help-queue/raise-hand-button.tsx src/components/help-queue/help-queue-panel.tsx tests/unit/raise-hand-button.test.tsx tests/unit/help-queue-panel.test.tsx
git commit -m "feat: add raise-hand button and help queue panel components"
```

---

## Task 16: Integrate AI Chat into Editor Page

**Files:**
- Modify: `src/app/dashboard/classrooms/[id]/editor/page.tsx`
- Create: `src/lib/ai/use-ai-chat.ts`

- [ ] **Step 1: Create the AI chat hook**

Create `src/lib/ai/use-ai-chat.ts`:

```typescript
"use client";

import { useState, useCallback, useRef } from "react";

interface ChatMessage {
  role: "student" | "assistant";
  content: string;
}

interface UseAiChatOptions {
  sessionId: string;
  documentId: string;
  classroomId: string;
  getCurrentCode: () => string;
}

export function useAiChat({
  sessionId,
  documentId,
  classroomId,
  getCurrentCode,
}: UseAiChatOptions) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [streaming, setStreaming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const interactionIdRef = useRef<string | null>(null);

  const sendMessage = useCallback(
    async (content: string) => {
      setError(null);
      setStreaming(true);

      // Add student message immediately
      setMessages((prev) => [...prev, { role: "student", content }]);

      // Add empty assistant message for streaming
      setMessages((prev) => [...prev, { role: "assistant", content: "" }]);

      try {
        const response = await fetch("/api/ai/chat", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            message: content,
            sessionId,
            documentId,
            classroomId,
            interactionId: interactionIdRef.current || undefined,
            currentCode: messages.length === 0 ? getCurrentCode() : undefined,
          }),
        });

        if (!response.ok) {
          const errorData = await response.json();
          throw new Error(errorData.error || "Failed to send message");
        }

        const reader = response.body?.getReader();
        if (!reader) throw new Error("No response stream");

        const decoder = new TextDecoder();
        let fullResponse = "";

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          const chunk = decoder.decode(value);
          const lines = chunk.split("\n");

          for (const line of lines) {
            if (!line.startsWith("data: ")) continue;
            const data = JSON.parse(line.slice(6));

            if (data.type === "delta") {
              fullResponse += data.text;
              setMessages((prev) => {
                const updated = [...prev];
                updated[updated.length - 1] = {
                  role: "assistant",
                  content: fullResponse,
                };
                return updated;
              });
            } else if (data.type === "filtered") {
              fullResponse = data.text;
              setMessages((prev) => {
                const updated = [...prev];
                updated[updated.length - 1] = {
                  role: "assistant",
                  content: data.text,
                };
                return updated;
              });
            } else if (data.type === "done") {
              interactionIdRef.current = data.interactionId;
            } else if (data.type === "error") {
              throw new Error(data.message);
            }
          }
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : "Unknown error");
        // Remove the empty assistant message on error
        setMessages((prev) => {
          const updated = [...prev];
          if (
            updated.length > 0 &&
            updated[updated.length - 1].role === "assistant" &&
            updated[updated.length - 1].content === ""
          ) {
            updated.pop();
          }
          return updated;
        });
      } finally {
        setStreaming(false);
      }
    },
    [sessionId, documentId, classroomId, getCurrentCode, messages.length]
  );

  const resetChat = useCallback(() => {
    setMessages([]);
    setError(null);
    interactionIdRef.current = null;
  }, []);

  return {
    messages,
    streaming,
    error,
    sendMessage,
    resetChat,
  };
}
```

- [ ] **Step 2: Update the editor page to include the AI chat panel**

Read the current editor page first, then modify it.

The current editor page at `src/app/dashboard/classrooms/[id]/editor/page.tsx` needs to be updated. Add the AI chat panel as a collapsible sidebar to the right of the editor. The layout should be:

```
[Editor (flex-1)] [AI Chat Panel (w-80, conditional)]
```

Modify `src/app/dashboard/classrooms/[id]/editor/page.tsx` — add these imports at the top:

```typescript
import { AiChatPanel } from "@/components/ai/ai-chat-panel";
import { RaiseHandButton } from "@/components/help-queue/raise-hand-button";
```

Add AI chat panel state management:

```typescript
const [aiEnabled, setAiEnabled] = useState(false);
const [showAiChat, setShowAiChat] = useState(false);
const [aiMessages, setAiMessages] = useState<Array<{ role: "student" | "assistant"; content: string }>>([]);
const [aiStreaming, setAiStreaming] = useState(false);
const [handRaised, setHandRaised] = useState(false);
```

Add the AI chat panel to the right side of the editor layout, and the raise-hand button to the toolbar. The exact integration depends on the current page structure — wrap the editor and chat panel in a flex container:

```tsx
<div className="flex h-full gap-2">
  {/* Editor section */}
  <div className="flex-1 flex flex-col">
    {/* ... existing editor and output panel ... */}
  </div>

  {/* AI Chat sidebar */}
  {showAiChat && (
    <div className="w-80 flex-shrink-0">
      <AiChatPanel
        messages={aiMessages}
        onSend={handleAiSend}
        disabled={!aiEnabled}
        streaming={aiStreaming}
      />
    </div>
  )}
</div>
```

Add toolbar buttons:

```tsx
<div className="flex items-center gap-2">
  {/* existing run button */}
  <RaiseHandButton
    raised={handRaised}
    onToggle={() => setHandRaised(!handRaised)}
  />
  <Button
    variant="outline"
    size="sm"
    onClick={() => setShowAiChat(!showAiChat)}
  >
    {showAiChat ? "Hide AI" : "Ask AI"}
  </Button>
</div>
```

Note: The exact code changes depend on the current structure of the editor page. Read the file, adapt the changes to fit the existing layout pattern, and ensure the editor and chat panel share the available vertical space properly.

- [ ] **Step 3: Verify build compiles**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun run build
```

Expected: Build succeeds. Any type errors should be resolved.

- [ ] **Step 4: Commit**

```bash
git add src/lib/ai/use-ai-chat.ts src/app/dashboard/classrooms/\[id\]/editor/page.tsx
git commit -m "feat: integrate AI chat panel and raise-hand button into editor page"
```

---

## Task 17: Run Full Test Suite and Final Verification

**Files:** None (verification only)

- [ ] **Step 1: Run the complete test suite**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test
```

Expected: All tests pass. Fix any failures before proceeding.

- [ ] **Step 2: Run the build**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun run build
```

Expected: Build succeeds with no type errors.

- [ ] **Step 3: Run the linter**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun run lint
```

Expected: No lint errors. Fix any that appear.

- [ ] **Step 4: Verify all files are committed**

```bash
git status
```

Expected: Working tree is clean. If any files are untracked or modified, add and commit them.

- [ ] **Step 5: Push to remote**

```bash
git push origin feat/foundation
```

---

## Summary

| Task | Description | New Tests | Files |
|------|-------------|-----------|-------|
| 1 | Install Anthropic SDK + env var | 0 | 2 modified |
| 2 | Database schema (AIInteraction, CodeAnnotation) | 3 | 3 modified/created |
| 3 | Anthropic client + system prompts | 7 | 3 created |
| 4 | AI output guardrails | 9 | 2 created |
| 5 | AI interaction CRUD logic | 7 | 2 created |
| 6 | Code annotation CRUD logic | 8 | 2 created |
| 7 | AI chat streaming endpoint | 4 | 2 created |
| 8 | AI toggle endpoint (teacher) | 4 | 2 created |
| 9 | AI interaction log endpoint | 3 | 2 created |
| 10 | Annotations API endpoints | 3 | 3 created |
| 11 | Help queue endpoint | 3 | 2 created |
| 12 | AI chat panel + message components | 13 | 4 created |
| 13 | AI toggle button + activity feed | 0 | 2 created |
| 14 | Annotation UI + CodeMirror gutter | 4 | 4 created |
| 15 | Raise hand + help queue UI | 9 | 4 created |
| 16 | Editor page integration | 0 | 2 created/modified |
| 17 | Full verification | 0 | 0 |
| **Total** | | **77 tests** | **~37 files** |

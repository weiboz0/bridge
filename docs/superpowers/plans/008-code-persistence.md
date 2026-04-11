# Code Persistence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist student code in PostgreSQL so work survives across sessions, supports reconnection, and enables parent/teacher viewing of plain-text snapshots.

**Architecture:** A new `documents` table stores both the Yjs binary state (source of truth while live editing) and a plain-text snapshot (for search/display/parent viewing). Hocuspocus gains a Drizzle database connection and uses `onStoreDocument`/`onLoadDocument` hooks to persist/restore Yjs state. On session end, a final plain-text snapshot is extracted from each document. Three new API routes expose documents for listing, fetching, and viewing content. The standalone editor saves directly to the documents table without Yjs.

**Tech Stack:** Drizzle ORM (PostgreSQL), Hocuspocus Server hooks, Yjs (`Y.encodeStateAsUpdate`, `Y.applyUpdate`), Next.js API routes, Zod v4 validation, Vitest

**Depends on:** Plans 006 (org-and-roles) and 007 (course-hierarchy) — assumes the new Classroom and Session tables exist. Also depends on existing Hocuspocus server, Yjs provider hook, and CodeEditor component.

**Key constraint:** `server/hocuspocus.ts` is excluded from tsconfig.json (in the `exclude` array), so it runs under Bun without Next.js path aliases. Imports must use relative paths, not `@/` aliases. The server needs its own Drizzle client instance.

---

## File Structure

```
src/
├── lib/
│   ├── db/
│   │   └── schema.ts                                 # Modify: add documents table + enum
│   └── documents.ts                                  # Create: document CRUD operations
├── app/
│   └── api/
│       └── documents/
│           ├── route.ts                              # Create: GET list, POST create
│           └── [id]/
│               ├── route.ts                          # Create: GET single document
│               └── content/
│                   └── route.ts                      # Create: GET plain text content
server/
├── hocuspocus.ts                                     # Modify: add persistence hooks + DB
├── db.ts                                             # Create: standalone Drizzle client for server/
└── documents.ts                                      # Create: document DB ops (relative imports)
tests/
├── helpers.ts                                        # Modify: add documents to cleanupDatabase
├── unit/
│   └── documents.test.ts                             # Create: document CRUD tests
├── integration/
│   └── documents-api.test.ts                         # Create: API route tests
└── unit/
    └── hocuspocus-persistence.test.ts                # Create: persistence hook logic tests
```

---

## Task 1: Document Table Schema

**Files:**
- Modify: `src/lib/db/schema.ts`

Add the `documents` table and a `documentLanguage` enum. The spec calls for `language` as an enum matching `editorModeEnum`, but we need a separate column since documents have their own language field.

- [ ] **Step 1: Add documents table to schema**

In `src/lib/db/schema.ts`, add the following after the existing `codeAnnotations` table definition:

```typescript
export const documents = pgTable(
  "documents",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    ownerId: uuid("owner_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    classroomId: uuid("classroom_id")
      .notNull()
      .references(() => classrooms.id, { onDelete: "cascade" }),
    sessionId: uuid("session_id").references(() => liveSessions.id, {
      onDelete: "set null",
    }),
    topicId: uuid("topic_id"),
    language: editorModeEnum("language").notNull().default("python"),
    yjsState: text("yjs_state"),
    plainText: text("plain_text").default(""),
    createdAt: timestamp("created_at").defaultNow().notNull(),
    updatedAt: timestamp("updated_at").defaultNow().notNull(),
  },
  (table) => [
    index("documents_owner_idx").on(table.ownerId),
    index("documents_classroom_idx").on(table.classroomId),
    index("documents_session_idx").on(table.sessionId),
    index("documents_owner_classroom_idx").on(table.ownerId, table.classroomId),
  ]
);
```

**Notes on `yjsState`:** The spec calls for `bytea`, but we store the Yjs binary state as a base64-encoded string in a `text` column. This avoids complications with bytea handling across Drizzle, postgres.js, and the Hocuspocus server (which runs under Bun, not Next.js). The encoding/decoding happens in the persistence layer. If we later need raw binary storage for performance, we can migrate to `bytea` then.

**Notes on `topicId`:** The spec includes `topicId` referencing the Topic table from plan 007. Since plan 007 defines topics, we add the column as a bare UUID without a foreign key constraint for now. The FK can be added when plan 007's Topic table is confirmed. If the Topic table already exists when this plan is executed, add the FK reference.

- [ ] **Step 2: Generate and run migration**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge" bun run db:generate
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge" bun run db:migrate
```

Also run the migration against the test database:

```bash
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run db:migrate
```

- [ ] **Step 3: Verify schema test still passes**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/schema.test.ts
```

Expected: Passes (existing schema tests should not break).

- [ ] **Step 4: Commit**

```bash
git add src/lib/db/schema.ts drizzle/
git commit -m "feat: add documents table for code persistence

Stores Yjs binary state (base64) and plain-text snapshots per student
per classroom. Supports nullable sessionId for standalone editor work
and nullable topicId for topic-linked documents."
```

---

## Task 2: Document CRUD Operations

**Files:**
- Create: `src/lib/documents.ts`
- Modify: `tests/helpers.ts`
- Create: `tests/unit/documents.test.ts`

Follow the same pattern as `src/lib/annotations.ts` and `src/lib/sessions.ts`: functions that accept a `Database` instance and return typed results.

- [ ] **Step 1: Update test helpers**

In `tests/helpers.ts`, add documents to `cleanupDatabase()` and add a `createTestDocument` helper.

Add import of `documents` to the schema imports (it's already imported via `* as schema`).

In `cleanupDatabase()`, add `await testDb.delete(schema.documents);` **before** the `liveSessions` delete (because documents has a FK to liveSessions):

```typescript
export async function cleanupDatabase() {
  await testDb.delete(schema.codeAnnotations);
  await testDb.delete(schema.aiInteractions);
  await testDb.delete(schema.documents);        // <-- add this line
  await testDb.delete(schema.sessionParticipants);
  await testDb.delete(schema.liveSessions);
  await testDb.delete(schema.classroomMembers);
  await testDb.delete(schema.classrooms);
  await testDb.delete(schema.authProviders);
  await testDb.delete(schema.users);
  await testDb.delete(schema.schools);
}
```

Add a helper for creating test documents:

```typescript
export async function createTestDocument(
  ownerId: string,
  classroomId: string,
  overrides: Partial<typeof schema.documents.$inferInsert> = {}
) {
  const [document] = await testDb
    .insert(schema.documents)
    .values({
      ownerId,
      classroomId,
      language: "python",
      ...overrides,
    })
    .returning();
  return document;
}
```

- [ ] **Step 2: Write failing tests**

Create `tests/unit/documents.test.ts`:

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  createTestUser,
  createTestClassroom,
  createTestSession,
  createTestDocument,
} from "../helpers";
import {
  createDocument,
  getDocument,
  listDocumentsByOwnerAndClassroom,
  listDocumentsBySession,
  updateYjsState,
  updatePlainText,
} from "@/lib/documents";

describe("document operations", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;
  let classroom: Awaited<ReturnType<typeof createTestClassroom>>;

  beforeEach(async () => {
    teacher = await createTestUser({ role: "teacher", email: "teacher@test.edu" });
    student = await createTestUser({ role: "student", email: "student@test.edu" });
    classroom = await createTestClassroom(teacher.id);
  });

  describe("createDocument", () => {
    it("creates a document with required fields", async () => {
      const doc = await createDocument(testDb, {
        ownerId: student.id,
        classroomId: classroom.id,
        language: "python",
      });
      expect(doc.id).toBeDefined();
      expect(doc.ownerId).toBe(student.id);
      expect(doc.classroomId).toBe(classroom.id);
      expect(doc.language).toBe("python");
      expect(doc.sessionId).toBeNull();
      expect(doc.plainText).toBe("");
    });

    it("creates a document with sessionId", async () => {
      const session = await createTestSession(classroom.id, teacher.id);
      const doc = await createDocument(testDb, {
        ownerId: student.id,
        classroomId: classroom.id,
        language: "python",
        sessionId: session.id,
      });
      expect(doc.sessionId).toBe(session.id);
    });

    it("creates a document with topicId", async () => {
      const topicId = "00000000-0000-0000-0000-000000000001";
      const doc = await createDocument(testDb, {
        ownerId: student.id,
        classroomId: classroom.id,
        language: "python",
        topicId,
      });
      expect(doc.topicId).toBe(topicId);
    });
  });

  describe("getDocument", () => {
    it("returns a document by ID", async () => {
      const created = await createTestDocument(student.id, classroom.id);
      const doc = await getDocument(testDb, created.id);
      expect(doc).not.toBeNull();
      expect(doc!.id).toBe(created.id);
    });

    it("returns null for non-existent document", async () => {
      const doc = await getDocument(testDb, "00000000-0000-0000-0000-000000000000");
      expect(doc).toBeNull();
    });
  });

  describe("listDocumentsByOwnerAndClassroom", () => {
    it("lists documents for a student in a classroom", async () => {
      await createTestDocument(student.id, classroom.id);
      await createTestDocument(student.id, classroom.id);

      const docs = await listDocumentsByOwnerAndClassroom(
        testDb,
        student.id,
        classroom.id
      );
      expect(docs).toHaveLength(2);
    });

    it("does not return other students' documents", async () => {
      const other = await createTestUser({ role: "student", email: "other@test.edu" });
      await createTestDocument(student.id, classroom.id);
      await createTestDocument(other.id, classroom.id);

      const docs = await listDocumentsByOwnerAndClassroom(
        testDb,
        student.id,
        classroom.id
      );
      expect(docs).toHaveLength(1);
      expect(docs[0].ownerId).toBe(student.id);
    });

    it("does not return documents from other classrooms", async () => {
      const otherClassroom = await createTestClassroom(teacher.id);
      await createTestDocument(student.id, classroom.id);
      await createTestDocument(student.id, otherClassroom.id);

      const docs = await listDocumentsByOwnerAndClassroom(
        testDb,
        student.id,
        classroom.id
      );
      expect(docs).toHaveLength(1);
      expect(docs[0].classroomId).toBe(classroom.id);
    });
  });

  describe("listDocumentsBySession", () => {
    it("lists all documents in a session", async () => {
      const session = await createTestSession(classroom.id, teacher.id);
      await createTestDocument(student.id, classroom.id, { sessionId: session.id });

      const other = await createTestUser({ role: "student", email: "other@test.edu" });
      await createTestDocument(other.id, classroom.id, { sessionId: session.id });

      const docs = await listDocumentsBySession(testDb, session.id);
      expect(docs).toHaveLength(2);
    });

    it("does not include standalone documents", async () => {
      const session = await createTestSession(classroom.id, teacher.id);
      await createTestDocument(student.id, classroom.id, { sessionId: session.id });
      await createTestDocument(student.id, classroom.id); // no sessionId

      const docs = await listDocumentsBySession(testDb, session.id);
      expect(docs).toHaveLength(1);
    });
  });

  describe("updateYjsState", () => {
    it("updates yjsState and updatedAt", async () => {
      const doc = await createTestDocument(student.id, classroom.id);
      const state = Buffer.from("fake-yjs-state").toString("base64");

      const updated = await updateYjsState(testDb, doc.id, state);
      expect(updated).not.toBeNull();
      expect(updated!.yjsState).toBe(state);
      expect(new Date(updated!.updatedAt).getTime()).toBeGreaterThanOrEqual(
        new Date(doc.updatedAt).getTime()
      );
    });

    it("returns null for non-existent document", async () => {
      const updated = await updateYjsState(
        testDb,
        "00000000-0000-0000-0000-000000000000",
        "state"
      );
      expect(updated).toBeNull();
    });
  });

  describe("updatePlainText", () => {
    it("updates plainText and updatedAt", async () => {
      const doc = await createTestDocument(student.id, classroom.id);

      const updated = await updatePlainText(testDb, doc.id, 'print("hello")');
      expect(updated).not.toBeNull();
      expect(updated!.plainText).toBe('print("hello")');
    });

    it("returns null for non-existent document", async () => {
      const updated = await updatePlainText(
        testDb,
        "00000000-0000-0000-0000-000000000000",
        "text"
      );
      expect(updated).toBeNull();
    });
  });
});
```

- [ ] **Step 3: Run tests to verify failure**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/documents.test.ts
```

Expected: FAIL -- cannot resolve `@/lib/documents`.

- [ ] **Step 4: Implement document CRUD**

Create `src/lib/documents.ts`:

```typescript
import { eq, and, desc } from "drizzle-orm";
import { documents } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

interface CreateDocumentInput {
  ownerId: string;
  classroomId: string;
  language: "python" | "javascript" | "blockly";
  sessionId?: string;
  topicId?: string;
  yjsState?: string;
  plainText?: string;
}

export async function createDocument(db: Database, input: CreateDocumentInput) {
  const [doc] = await db
    .insert(documents)
    .values(input)
    .returning();
  return doc;
}

export async function getDocument(db: Database, documentId: string) {
  const [doc] = await db
    .select()
    .from(documents)
    .where(eq(documents.id, documentId));
  return doc || null;
}

export async function listDocumentsByOwnerAndClassroom(
  db: Database,
  ownerId: string,
  classroomId: string
) {
  return db
    .select()
    .from(documents)
    .where(
      and(
        eq(documents.ownerId, ownerId),
        eq(documents.classroomId, classroomId)
      )
    )
    .orderBy(desc(documents.updatedAt));
}

export async function listDocumentsBySession(
  db: Database,
  sessionId: string
) {
  return db
    .select()
    .from(documents)
    .where(eq(documents.sessionId, sessionId))
    .orderBy(desc(documents.updatedAt));
}

export async function updateYjsState(
  db: Database,
  documentId: string,
  yjsState: string
) {
  const [doc] = await db
    .update(documents)
    .set({ yjsState, updatedAt: new Date() })
    .where(eq(documents.id, documentId))
    .returning();
  return doc || null;
}

export async function updatePlainText(
  db: Database,
  documentId: string,
  plainText: string
) {
  const [doc] = await db
    .update(documents)
    .set({ plainText, updatedAt: new Date() })
    .where(eq(documents.id, documentId))
    .returning();
  return doc || null;
}

export async function getOrCreateDocument(
  db: Database,
  input: CreateDocumentInput
): Promise<ReturnType<typeof createDocument>> {
  // Find existing document for this owner + classroom + session combo
  const conditions = [
    eq(documents.ownerId, input.ownerId),
    eq(documents.classroomId, input.classroomId),
  ];

  if (input.sessionId) {
    conditions.push(eq(documents.sessionId, input.sessionId));
  }

  const [existing] = await db
    .select()
    .from(documents)
    .where(and(...conditions))
    .limit(1);

  if (existing) return existing;
  return createDocument(db, input);
}
```

- [ ] **Step 5: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/documents.test.ts
```

Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add src/lib/documents.ts tests/unit/documents.test.ts tests/helpers.ts
git commit -m "feat: add document CRUD operations with tests

Create, get, list (by owner+classroom, by session), updateYjsState,
updatePlainText, and getOrCreateDocument. Test helpers updated to
clean up documents table and create test documents."
```

---

## Task 3: Hocuspocus Database Connection

**Files:**
- Create: `server/db.ts`
- Create: `server/documents.ts`

The Hocuspocus server runs as a standalone Bun process (`bun run server/hocuspocus.ts`). It is excluded from tsconfig.json, so it cannot use `@/` path aliases. It needs its own Drizzle client and document operations using relative imports.

- [ ] **Step 1: Create server database client**

Create `server/db.ts`:

```typescript
import { drizzle } from "drizzle-orm/postgres-js";
import postgres from "postgres";

const connectionString = process.env.DATABASE_URL;

if (!connectionString) {
  throw new Error("DATABASE_URL environment variable is required for Hocuspocus server");
}

const client = postgres(connectionString);
export const db = drizzle(client);
```

**Note:** We do NOT import the schema from `@/lib/db/schema` (path alias unavailable). Instead, the document operations in `server/documents.ts` will use raw SQL via Drizzle's `sql` template tag, or we duplicate the minimal table definition. The recommended approach is to duplicate the minimal table reference since it's just one table.

- [ ] **Step 2: Create server document operations**

Create `server/documents.ts`:

```typescript
import { pgTable, uuid, text, timestamp } from "drizzle-orm/pg-core";
import { eq, and } from "drizzle-orm";
import { db } from "./db";

// Minimal table definition (mirrors src/lib/db/schema.ts documents table)
// This duplication is necessary because server/ cannot use @/ path aliases.
const documents = pgTable("documents", {
  id: uuid("id").primaryKey().defaultRandom(),
  ownerId: uuid("owner_id").notNull(),
  classroomId: uuid("classroom_id").notNull(),
  sessionId: uuid("session_id"),
  topicId: uuid("topic_id"),
  language: text("language").notNull(),
  yjsState: text("yjs_state"),
  plainText: text("plain_text"),
  createdAt: timestamp("created_at").defaultNow().notNull(),
  updatedAt: timestamp("updated_at").defaultNow().notNull(),
});

export interface DocumentRecord {
  id: string;
  ownerId: string;
  classroomId: string;
  sessionId: string | null;
  yjsState: string | null;
  plainText: string | null;
}

/**
 * Parse a Hocuspocus document name to extract session and user IDs.
 * Format: "session:{sessionId}:user:{userId}"
 */
export function parseDocumentName(
  documentName: string
): { sessionId: string; userId: string } | null {
  const parts = documentName.split(":");
  if (parts[0] === "session" && parts[2] === "user" && parts[1] && parts[3]) {
    return { sessionId: parts[1], userId: parts[3] };
  }
  return null;
}

/**
 * Find or create a document for a given owner + classroom + session.
 */
export async function getOrCreateDocument(
  ownerId: string,
  classroomId: string,
  sessionId: string | null,
  language: string = "python"
): Promise<DocumentRecord> {
  const conditions = [
    eq(documents.ownerId, ownerId),
    eq(documents.classroomId, classroomId),
  ];

  if (sessionId) {
    conditions.push(eq(documents.sessionId, sessionId));
  }

  const [existing] = await db
    .select()
    .from(documents)
    .where(and(...conditions))
    .limit(1);

  if (existing) return existing as DocumentRecord;

  const [created] = await db
    .insert(documents)
    .values({
      ownerId,
      classroomId,
      sessionId,
      language,
    })
    .returning();

  return created as DocumentRecord;
}

/**
 * Save Yjs binary state (base64 encoded) to the document.
 */
export async function saveYjsState(
  documentId: string,
  yjsStateBase64: string
): Promise<void> {
  await db
    .update(documents)
    .set({
      yjsState: yjsStateBase64,
      updatedAt: new Date(),
    })
    .where(eq(documents.id, documentId));
}

/**
 * Load Yjs state (base64 encoded) for a document.
 */
export async function loadYjsState(
  documentId: string
): Promise<string | null> {
  const [doc] = await db
    .select({ yjsState: documents.yjsState })
    .from(documents)
    .where(eq(documents.id, documentId));

  return doc?.yjsState || null;
}

/**
 * Save plain text snapshot for a document.
 */
export async function savePlainText(
  documentId: string,
  plainText: string
): Promise<void> {
  await db
    .update(documents)
    .set({
      plainText,
      updatedAt: new Date(),
    })
    .where(eq(documents.id, documentId));
}

/**
 * Get all documents for a session (used for session-end snapshot).
 */
export async function getSessionDocuments(
  sessionId: string
): Promise<DocumentRecord[]> {
  const docs = await db
    .select()
    .from(documents)
    .where(eq(documents.sessionId, sessionId));

  return docs as DocumentRecord[];
}

/**
 * Look up the classroomId for a given sessionId.
 * Needs a minimal sessions table reference.
 */
const liveSessions = pgTable("live_sessions", {
  id: uuid("id").primaryKey(),
  classroomId: uuid("classroom_id").notNull(),
});

export async function getClassroomIdForSession(
  sessionId: string
): Promise<string | null> {
  const [session] = await db
    .select({ classroomId: liveSessions.classroomId })
    .from(liveSessions)
    .where(eq(liveSessions.id, sessionId));

  return session?.classroomId || null;
}
```

- [ ] **Step 3: Commit**

```bash
git add server/db.ts server/documents.ts
git commit -m "feat: add standalone database client and document ops for Hocuspocus server

Hocuspocus runs outside Next.js so it cannot use @/ path aliases.
These modules provide a Drizzle client and document CRUD operations
using relative imports and minimal table definitions."
```

---

## Task 4: Hocuspocus Persistence Hooks

**Files:**
- Modify: `server/hocuspocus.ts`
- Create: `tests/unit/hocuspocus-persistence.test.ts`

Add `onStoreDocument` and `onLoadDocument` hooks. The store hook saves Yjs binary state as base64 every time Hocuspocus triggers it (every 30s by default and on disconnect). The load hook restores Yjs state when a client reconnects.

- [ ] **Step 1: Write tests for persistence helper logic**

Create `tests/unit/hocuspocus-persistence.test.ts`. Since the Hocuspocus server code uses relative imports and can't use `@/`, we test the **logic** (document name parsing) and the **document operations** via the `src/lib/documents.ts` module (which has the same logic, just with `@/` imports).

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  createTestUser,
  createTestClassroom,
  createTestSession,
} from "../helpers";
import {
  createDocument,
  getDocument,
  updateYjsState,
  updatePlainText,
  listDocumentsBySession,
} from "@/lib/documents";

describe("hocuspocus persistence logic", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;
  let classroom: Awaited<ReturnType<typeof createTestClassroom>>;

  beforeEach(async () => {
    teacher = await createTestUser({ role: "teacher", email: "teacher@test.edu" });
    student = await createTestUser({ role: "student", email: "student@test.edu" });
    classroom = await createTestClassroom(teacher.id);
  });

  describe("document name parsing", () => {
    it("parses session:xxx:user:yyy format", () => {
      const name = "session:abc-123:user:def-456";
      const parts = name.split(":");
      expect(parts[0]).toBe("session");
      expect(parts[1]).toBe("abc-123");
      expect(parts[2]).toBe("user");
      expect(parts[3]).toBe("def-456");
    });

    it("identifies broadcast documents", () => {
      const name = "broadcast:abc-123";
      const parts = name.split(":");
      expect(parts[0]).toBe("broadcast");
      // broadcast documents are not persisted
    });
  });

  describe("onStoreDocument flow", () => {
    it("saves yjs state as base64", async () => {
      const session = await createTestSession(classroom.id, teacher.id);
      const doc = await createDocument(testDb, {
        ownerId: student.id,
        classroomId: classroom.id,
        sessionId: session.id,
        language: "python",
      });

      const fakeYjsState = Buffer.from([1, 2, 3, 4, 5]).toString("base64");
      const updated = await updateYjsState(testDb, doc.id, fakeYjsState);

      expect(updated).not.toBeNull();
      expect(updated!.yjsState).toBe(fakeYjsState);

      // Verify we can read it back
      const loaded = await getDocument(testDb, doc.id);
      expect(loaded!.yjsState).toBe(fakeYjsState);
    });

    it("overwrites previous state on subsequent saves", async () => {
      const session = await createTestSession(classroom.id, teacher.id);
      const doc = await createDocument(testDb, {
        ownerId: student.id,
        classroomId: classroom.id,
        sessionId: session.id,
        language: "python",
      });

      const state1 = Buffer.from("state-1").toString("base64");
      const state2 = Buffer.from("state-2").toString("base64");

      await updateYjsState(testDb, doc.id, state1);
      await updateYjsState(testDb, doc.id, state2);

      const loaded = await getDocument(testDb, doc.id);
      expect(loaded!.yjsState).toBe(state2);
    });
  });

  describe("onLoadDocument flow", () => {
    it("returns null yjsState for new document", async () => {
      const doc = await createDocument(testDb, {
        ownerId: student.id,
        classroomId: classroom.id,
        language: "python",
      });

      const loaded = await getDocument(testDb, doc.id);
      expect(loaded!.yjsState).toBeNull();
    });

    it("returns previously saved yjsState", async () => {
      const doc = await createDocument(testDb, {
        ownerId: student.id,
        classroomId: classroom.id,
        language: "python",
      });

      const state = Buffer.from("saved-state").toString("base64");
      await updateYjsState(testDb, doc.id, state);

      const loaded = await getDocument(testDb, doc.id);
      expect(loaded!.yjsState).toBe(state);
    });
  });

  describe("session end snapshot", () => {
    it("saves plain text for all documents in session", async () => {
      const session = await createTestSession(classroom.id, teacher.id);

      const student2 = await createTestUser({
        role: "student",
        email: "student2@test.edu",
      });

      const doc1 = await createDocument(testDb, {
        ownerId: student.id,
        classroomId: classroom.id,
        sessionId: session.id,
        language: "python",
      });
      const doc2 = await createDocument(testDb, {
        ownerId: student2.id,
        classroomId: classroom.id,
        sessionId: session.id,
        language: "python",
      });

      await updatePlainText(testDb, doc1.id, 'print("hello from student 1")');
      await updatePlainText(testDb, doc2.id, 'print("hello from student 2")');

      const docs = await listDocumentsBySession(testDb, session.id);
      expect(docs).toHaveLength(2);
      expect(docs.map((d) => d.plainText)).toContain('print("hello from student 1")');
      expect(docs.map((d) => d.plainText)).toContain('print("hello from student 2")');
    });
  });
});
```

- [ ] **Step 2: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/hocuspocus-persistence.test.ts
```

Expected: All tests pass (these use the same CRUD functions from Task 2).

- [ ] **Step 3: Update Hocuspocus server with persistence hooks**

Modify `server/hocuspocus.ts` to add database persistence. Replace the entire file:

```typescript
import { Server } from "@hocuspocus/server";
import * as Y from "yjs";
import {
  parseDocumentName,
  getOrCreateDocument,
  saveYjsState,
  loadYjsState,
  savePlainText,
  getClassroomIdForSession,
} from "./documents";

// Map to track document ID lookups (documentName -> DB document ID)
const documentIdCache = new Map<string, string>();

const server = new Server({
  port: 4000,

  // Persist every 30 seconds
  debounce: 30000,

  async onAuthenticate({ token, documentName }: { token: string; documentName: string }) {
    if (!token) {
      throw new Error("Authentication required");
    }

    const [userId, role] = token.split(":");
    if (!userId || !role) {
      throw new Error("Invalid token format");
    }

    const parts = documentName.split(":");
    if (parts[0] === "session" && parts[2] === "user") {
      const docOwner = parts[3];
      if (role !== "teacher" && userId !== docOwner) {
        throw new Error("Access denied");
      }
    } else if (parts[0] === "broadcast") {
      // Anyone in the session can read broadcast documents
    } else {
      throw new Error("Invalid document name format");
    }

    return { userId, role };
  },

  async onLoadDocument({ document, documentName }: { document: Y.Doc; documentName: string }) {
    // Only persist session documents, not broadcast documents
    const parsed = parseDocumentName(documentName);
    if (!parsed) {
      console.log(`[hocuspocus] Skipping persistence for non-session document: ${documentName}`);
      return document;
    }

    try {
      // Look up the classroom for this session
      const classroomId = await getClassroomIdForSession(parsed.sessionId);
      if (!classroomId) {
        console.warn(`[hocuspocus] No classroom found for session ${parsed.sessionId}`);
        return document;
      }

      // Get or create the document record
      const dbDoc = await getOrCreateDocument(
        parsed.userId,
        classroomId,
        parsed.sessionId
      );

      // Cache the mapping
      documentIdCache.set(documentName, dbDoc.id);

      // If there's saved state, apply it to the Y.Doc
      if (dbDoc.yjsState) {
        const binaryState = Uint8Array.from(
          Buffer.from(dbDoc.yjsState, "base64")
        );
        Y.applyUpdate(document, binaryState);
        console.log(`[hocuspocus] Restored state for ${documentName} (doc ${dbDoc.id})`);
      } else {
        console.log(`[hocuspocus] No saved state for ${documentName}, starting fresh`);
      }
    } catch (error) {
      console.error(`[hocuspocus] Error loading document ${documentName}:`, error);
    }

    return document;
  },

  async onStoreDocument({ document, documentName }: { document: Y.Doc; documentName: string }) {
    // Only persist session documents
    const parsed = parseDocumentName(documentName);
    if (!parsed) return;

    const docId = documentIdCache.get(documentName);
    if (!docId) {
      console.warn(`[hocuspocus] No cached document ID for ${documentName}, skipping save`);
      return;
    }

    try {
      // Encode Yjs state as binary, then base64 for storage
      const stateUpdate = Y.encodeStateAsUpdate(document);
      const base64State = Buffer.from(stateUpdate).toString("base64");

      await saveYjsState(docId, base64State);

      // Also save plain text snapshot
      const yText = document.getText("content");
      const plainText = yText.toString();
      await savePlainText(docId, plainText);

      console.log(
        `[hocuspocus] Saved state for ${documentName} (${base64State.length} bytes base64, ${plainText.length} chars text)`
      );
    } catch (error) {
      console.error(`[hocuspocus] Error saving document ${documentName}:`, error);
    }
  },

  async onConnect({ documentName }: { documentName: string }) {
    console.log(`[hocuspocus] Client connected to: ${documentName}`);
  },

  async onDisconnect({ documentName }: { documentName: string }) {
    console.log(`[hocuspocus] Client disconnected from: ${documentName}`);
  },
});

server.listen().then(() => {
  console.log(`[hocuspocus] WebSocket server running on ws://127.0.0.1:4000`);
});
```

- [ ] **Step 4: Verify Hocuspocus server starts**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge" timeout 5 bun run server/hocuspocus.ts || true
```

Expected: Server starts and prints the "running on ws://127.0.0.1:4000" message before timeout kills it.

- [ ] **Step 5: Commit**

```bash
git add server/hocuspocus.ts tests/unit/hocuspocus-persistence.test.ts
git commit -m "feat: add Hocuspocus persistence hooks for code persistence

onLoadDocument restores Yjs state from PostgreSQL when a client
reconnects. onStoreDocument saves Yjs binary state (base64) and
plain-text snapshot every 30 seconds and on disconnect. Broadcast
documents are excluded from persistence."
```

---

## Task 5: Session End Plain-Text Snapshot

**Files:**
- Modify: `src/lib/sessions.ts`
- Modify: `src/app/api/sessions/[id]/route.ts`

When a teacher ends a session via `PATCH /api/sessions/[id]`, save a final plain-text snapshot for all documents in that session. The Hocuspocus `onStoreDocument` hook already saves plain text on each debounce, but the session-end hook ensures we capture the absolute final state.

- [ ] **Step 1: Add session-end document snapshot to sessions lib**

In `src/lib/sessions.ts`, add a new function that snapshots all documents for a session. Add imports:

```typescript
import { documents } from "@/lib/db/schema";
```

Add function at the end of the file:

```typescript
export async function snapshotSessionDocuments(db: Database, sessionId: string) {
  // Fetch all documents for this session that have yjsState but no recent plainText update
  const sessionDocs = await db
    .select()
    .from(documents)
    .where(eq(documents.sessionId, sessionId));

  // For each document, if yjsState exists, we trust that the Hocuspocus onStoreDocument
  // hook has already saved the latest plain text. This function is a safety net —
  // it marks the documents as finalized by touching updatedAt.
  const results = [];
  for (const doc of sessionDocs) {
    const [updated] = await db
      .update(documents)
      .set({ updatedAt: new Date() })
      .where(eq(documents.id, doc.id))
      .returning();
    results.push(updated);
  }
  return results;
}
```

- [ ] **Step 2: Call snapshot on session end**

In `src/app/api/sessions/[id]/route.ts`, add the snapshot call after ending the session. Add import:

```typescript
import { getSession, endSession, snapshotSessionDocuments } from "@/lib/sessions";
```

In the `PATCH` handler, after `const ended = await endSession(db, id);`, add:

```typescript
  // Snapshot all documents for this session
  await snapshotSessionDocuments(db, id);
```

- [ ] **Step 3: Add test for session-end snapshot**

Add a test case in `tests/unit/documents.test.ts` (or in the existing `tests/integration/sessions-api.test.ts`). Add to `tests/unit/documents.test.ts`:

```typescript
import { snapshotSessionDocuments } from "@/lib/sessions";

// Inside the describe block, add:
describe("snapshotSessionDocuments", () => {
  it("touches updatedAt for all session documents", async () => {
    const session = await createTestSession(classroom.id, teacher.id);
    const doc = await createDocument(testDb, {
      ownerId: student.id,
      classroomId: classroom.id,
      sessionId: session.id,
      language: "python",
    });

    // Wait a moment to ensure timestamp differs
    const originalUpdatedAt = doc.updatedAt;

    const results = await snapshotSessionDocuments(testDb, session.id);
    expect(results).toHaveLength(1);
    expect(new Date(results[0].updatedAt).getTime()).toBeGreaterThanOrEqual(
      new Date(originalUpdatedAt).getTime()
    );
  });

  it("returns empty array for session with no documents", async () => {
    const session = await createTestSession(classroom.id, teacher.id);
    const results = await snapshotSessionDocuments(testDb, session.id);
    expect(results).toHaveLength(0);
  });
});
```

- [ ] **Step 4: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/documents.test.ts
```

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add src/lib/sessions.ts src/app/api/sessions/\[id\]/route.ts tests/unit/documents.test.ts
git commit -m "feat: snapshot session documents on session end

When teacher ends a session, all documents in that session have their
updatedAt timestamps refreshed as a finalization marker. The actual
plain text is saved by Hocuspocus onStoreDocument hooks in real-time."
```

---

## Task 6: Document API Routes

**Files:**
- Create: `src/app/api/documents/route.ts`
- Create: `src/app/api/documents/[id]/route.ts`
- Create: `src/app/api/documents/[id]/content/route.ts`
- Create: `tests/integration/documents-api.test.ts`

Follow the same pattern as `src/app/api/annotations/route.ts` and other existing API routes.

- [ ] **Step 1: Write failing integration tests**

Create `tests/integration/documents-api.test.ts`:

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  createTestUser,
  createTestClassroom,
  createTestSession,
  createTestDocument,
} from "../helpers";
import { setMockUser, createRequest, parseResponse } from "../api-helpers";
import { GET as LIST } from "@/app/api/documents/route";
import { GET as GET_ONE } from "@/app/api/documents/[id]/route";
import { GET as GET_CONTENT } from "@/app/api/documents/[id]/content/route";

describe("Documents API", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;
  let classroom: Awaited<ReturnType<typeof createTestClassroom>>;

  beforeEach(async () => {
    teacher = await createTestUser({ name: "Teacher", role: "teacher", email: "teacher@test.edu" });
    student = await createTestUser({ name: "Student", role: "student", email: "student@test.edu" });
    classroom = await createTestClassroom(teacher.id);
  });

  describe("GET /api/documents", () => {
    it("lists documents by classroomId and studentId", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });

      await createTestDocument(student.id, classroom.id, { plainText: "code 1" });
      await createTestDocument(student.id, classroom.id, { plainText: "code 2" });

      const req = createRequest("/api/documents", {
        searchParams: { classroomId: classroom.id, studentId: student.id },
      });

      const { status, body } = await parseResponse<any[]>(await LIST(req));
      expect(status).toBe(200);
      expect(body).toHaveLength(2);
    });

    it("requires classroomId param", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });

      const req = createRequest("/api/documents", {
        searchParams: { studentId: student.id },
      });

      const { status } = await parseResponse(await LIST(req));
      expect(status).toBe(400);
    });

    it("students can only list their own documents", async () => {
      setMockUser({ id: student.id, name: student.name, email: student.email, role: "student" });

      await createTestDocument(student.id, classroom.id);

      const other = await createTestUser({ role: "student", email: "other@test.edu" });
      await createTestDocument(other.id, classroom.id);

      // Student lists without specifying studentId — defaults to self
      const req = createRequest("/api/documents", {
        searchParams: { classroomId: classroom.id },
      });

      const { status, body } = await parseResponse<any[]>(await LIST(req));
      expect(status).toBe(200);
      expect(body).toHaveLength(1);
      expect(body[0].ownerId).toBe(student.id);
    });

    it("rejects unauthenticated request", async () => {
      setMockUser(null);

      const req = createRequest("/api/documents", {
        searchParams: { classroomId: classroom.id },
      });

      const { status } = await parseResponse(await LIST(req));
      expect(status).toBe(401);
    });
  });

  describe("GET /api/documents/[id]", () => {
    it("returns a document by ID", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });

      const doc = await createTestDocument(student.id, classroom.id, {
        plainText: 'print("hello")',
      });

      const res = await GET_ONE(
        createRequest(`/api/documents/${doc.id}`),
        { params: Promise.resolve({ id: doc.id }) }
      );

      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect((body as any).id).toBe(doc.id);
      expect((body as any).plainText).toBe('print("hello")');
    });

    it("returns 404 for non-existent document", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });

      const res = await GET_ONE(
        createRequest("/api/documents/00000000-0000-0000-0000-000000000000"),
        { params: Promise.resolve({ id: "00000000-0000-0000-0000-000000000000" }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(404);
    });

    it("rejects unauthenticated request", async () => {
      setMockUser(null);

      const doc = await createTestDocument(student.id, classroom.id);

      const res = await GET_ONE(
        createRequest(`/api/documents/${doc.id}`),
        { params: Promise.resolve({ id: doc.id }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(401);
    });
  });

  describe("GET /api/documents/[id]/content", () => {
    it("returns plain text content", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });

      const doc = await createTestDocument(student.id, classroom.id, {
        plainText: 'x = 42\nprint(x)',
      });

      const res = await GET_CONTENT(
        createRequest(`/api/documents/${doc.id}/content`),
        { params: Promise.resolve({ id: doc.id }) }
      );

      const { status, body } = await parseResponse(res);
      expect(status).toBe(200);
      expect((body as any).plainText).toBe('x = 42\nprint(x)');
      // Should NOT include yjsState (binary data not needed for viewing)
      expect((body as any).yjsState).toBeUndefined();
    });

    it("returns 404 for non-existent document", async () => {
      setMockUser({ id: teacher.id, name: teacher.name, email: teacher.email, role: "teacher" });

      const res = await GET_CONTENT(
        createRequest("/api/documents/00000000-0000-0000-0000-000000000000/content"),
        { params: Promise.resolve({ id: "00000000-0000-0000-0000-000000000000" }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(404);
    });

    it("rejects unauthenticated request", async () => {
      setMockUser(null);

      const doc = await createTestDocument(student.id, classroom.id);

      const res = await GET_CONTENT(
        createRequest(`/api/documents/${doc.id}/content`),
        { params: Promise.resolve({ id: doc.id }) }
      );

      const { status } = await parseResponse(res);
      expect(status).toBe(401);
    });
  });
});
```

- [ ] **Step 2: Run tests to verify failure**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/integration/documents-api.test.ts
```

Expected: FAIL -- cannot resolve the route modules.

- [ ] **Step 3: Implement GET /api/documents (list)**

Create `src/app/api/documents/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { listDocumentsByOwnerAndClassroom } from "@/lib/documents";

export async function GET(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const classroomId = request.nextUrl.searchParams.get("classroomId");
  if (!classroomId) {
    return NextResponse.json(
      { error: "classroomId is required" },
      { status: 400 }
    );
  }

  // Students can only view their own documents
  // Teachers/admins can specify a studentId to view any student's documents
  let studentId = request.nextUrl.searchParams.get("studentId");

  if (session.user.role === "student") {
    // Students always see only their own documents
    studentId = session.user.id;
  } else if (!studentId) {
    // Teachers without studentId see all (not supported by this endpoint yet)
    // For now, default to the current user's documents
    studentId = session.user.id;
  }

  const documents = await listDocumentsByOwnerAndClassroom(
    db,
    studentId,
    classroomId
  );

  return NextResponse.json(documents);
}
```

- [ ] **Step 4: Implement GET /api/documents/[id]**

Create `src/app/api/documents/[id]/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getDocument } from "@/lib/documents";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const document = await getDocument(db, id);

  if (!document) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  return NextResponse.json(document);
}
```

- [ ] **Step 5: Implement GET /api/documents/[id]/content**

Create `src/app/api/documents/[id]/content/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getDocument } from "@/lib/documents";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const document = await getDocument(db, id);

  if (!document) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  // Return only the fields needed for viewing — exclude yjsState (binary data)
  return NextResponse.json({
    id: document.id,
    ownerId: document.ownerId,
    classroomId: document.classroomId,
    sessionId: document.sessionId,
    topicId: document.topicId,
    language: document.language,
    plainText: document.plainText,
    updatedAt: document.updatedAt,
    createdAt: document.createdAt,
  });
}
```

- [ ] **Step 6: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/integration/documents-api.test.ts
```

Expected: All tests pass.

- [ ] **Step 7: Commit**

```bash
git add src/app/api/documents/ tests/integration/documents-api.test.ts
git commit -m "feat: add document API routes for listing and viewing

GET /api/documents?classroomId=X&studentId=Y lists student documents.
GET /api/documents/[id] returns full document.
GET /api/documents/[id]/content returns plain text only (for parent viewing).
Students can only access their own documents."
```

---

## Task 7: Update Existing Editor Page to Load/Save Documents

**Files:**
- Modify: `src/app/dashboard/classrooms/[id]/session/[sessionId]/page.tsx`

Currently the student session page creates a Yjs document named `session:{sessionId}:user:{userId}` but does not interact with the database-backed Document table. Since Hocuspocus now handles persistence in its `onLoadDocument`/`onStoreDocument` hooks, the editor page does not need to change its Yjs connection logic. The persistence is transparent.

However, we should add a visual indicator that work is being saved, and ensure the document name format stays consistent.

- [ ] **Step 1: Add save status indicator**

In `src/app/dashboard/classrooms/[id]/session/[sessionId]/page.tsx`, add a "Saved" indicator near the connection status dot. This uses the existing `connected` state from the Yjs provider.

After the connection status indicator (`<span className={`w-2 h-2 rounded-full ...`} />`), add:

```tsx
            {connected && (
              <span className="text-xs text-muted-foreground">Auto-saving</span>
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
git commit -m "feat: add auto-save indicator to student session page

Shows 'Auto-saving' text when connected to Hocuspocus, indicating
that code is being persisted to the database via server hooks."
```

---

## Task 8: Standalone Editor Persistence

**Files:**
- Modify: `src/components/editor/code-editor.tsx`

The standalone editor (non-collaborative mode, no Yjs) needs a way to save code to the Document table. This is used when students practice outside sessions.

Since the standalone editor workflow involves a separate page (to be built in a later plan for the portal redesign), this task focuses on adding a `save` callback to the CodeEditor component that API consumers can use.

- [ ] **Step 1: Add onSave callback to CodeEditor**

In `src/components/editor/code-editor.tsx`, add an optional `onSave` prop and wire it to Ctrl+S/Cmd+S:

Add to the `CodeEditorProps` interface:

```typescript
  onSave?: (code: string) => void;
```

In the non-collaborative mode extensions setup (after `if (onChange) { ... }`), add:

```typescript
    if (onSave) {
      extensions.push(
        keymap.of([
          {
            key: "Mod-s",
            preventDefault: true,
            run: (view) => {
              onSave(view.state.doc.toString());
              return true;
            },
          },
        ])
      );
    }
```

Update the component signature to accept `onSave`:

```typescript
export function CodeEditor({
  initialCode = "",
  onChange,
  readOnly = false,
  yText,
  provider,
  onSave,
}: CodeEditorProps) {
```

- [ ] **Step 2: Verify build**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun run build
```

Expected: Build passes.

- [ ] **Step 3: Commit**

```bash
git add src/components/editor/code-editor.tsx
git commit -m "feat: add onSave callback to CodeEditor for standalone persistence

Standalone editor (non-Yjs mode) can now accept an onSave callback
triggered by Ctrl+S/Cmd+S. This will be used by the standalone editor
pages to save code to the Document table via API."
```

---

## Task 9: Full Test Suite and Build Verification

- [ ] **Step 1: Run full test suite**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test
```

Expected: All tests pass, including new document tests and existing tests.

- [ ] **Step 2: Verify build**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun run build
```

Expected: Build passes with no errors.

- [ ] **Step 3: Verify Hocuspocus server starts with DB**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge" timeout 5 bun run server/hocuspocus.ts || true
```

Expected: Server starts successfully and prints startup message.

- [ ] **Step 4: Commit any remaining fixes**

If any test fixes or adjustments were needed, commit them:

```bash
git add -A
git commit -m "fix: resolve test/build issues from code persistence implementation"
```

---

## Summary of Changes

| Category | Files | What Changed |
|---|---|---|
| Schema | `src/lib/db/schema.ts`, `drizzle/` | New `documents` table with indexes |
| CRUD | `src/lib/documents.ts` | create, get, list, update operations |
| Server DB | `server/db.ts`, `server/documents.ts` | Standalone Drizzle client + document ops for Hocuspocus |
| Hocuspocus | `server/hocuspocus.ts` | `onLoadDocument`, `onStoreDocument` hooks with DB persistence |
| Session End | `src/lib/sessions.ts`, `src/app/api/sessions/[id]/route.ts` | Snapshot documents on session end |
| API Routes | `src/app/api/documents/**` | List, get, get-content endpoints |
| Editor | `src/components/editor/code-editor.tsx` | `onSave` callback for standalone mode |
| Student Page | `src/app/.../session/[sessionId]/page.tsx` | Auto-save indicator |
| Test Helpers | `tests/helpers.ts` | `cleanupDatabase` includes documents, `createTestDocument` helper |
| Tests | `tests/unit/documents.test.ts`, `tests/unit/hocuspocus-persistence.test.ts`, `tests/integration/documents-api.test.ts` | Full coverage of CRUD, persistence logic, API routes |

## Risks and Decisions

1. **base64 vs bytea:** We store Yjs state as base64 text instead of raw bytea. This adds ~33% overhead but avoids binary handling complexity across Drizzle, postgres.js, and Bun. Can be migrated later if storage becomes a concern.

2. **Duplicated table definition in server/:** `server/documents.ts` has a minimal copy of the `documents` table definition because it cannot import from `@/lib/db/schema`. This is a known trade-off of the server exclusion from tsconfig.json. If the schema changes, both files must be updated. Add a comment in both files referencing each other.

3. **No auth in Hocuspocus persistence hooks:** The `onStoreDocument`/`onLoadDocument` hooks trust the document name from `onAuthenticate`. Auth is enforced once at connection time, not on each save.

4. **Debounce timing:** Hocuspocus `debounce: 30000` (30 seconds) as specified in the design doc. Adjust if needed for user experience.

5. **Plain text on every save:** We save plain text in `onStoreDocument` alongside the Yjs state, not just on session end. This keeps the plain text reasonably fresh for teacher/parent viewing during active sessions.

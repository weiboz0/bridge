import { eq, and } from "drizzle-orm";
import { documents } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

interface CreateDocumentInput {
  ownerId: string;
  sessionId?: string;
  topicId?: string;
  language?: "python" | "javascript" | "blockly";
}

export async function createDocument(db: Database, input: CreateDocumentInput) {
  const [doc] = await db
    .insert(documents)
    .values({
      ownerId: input.ownerId,
      sessionId: input.sessionId,
      topicId: input.topicId,
      language: input.language,
    })
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

export async function listDocuments(
  db: Database,
  filters: { ownerId?: string; sessionId?: string }
) {
  const conditions = [];
  if (filters.ownerId) conditions.push(eq(documents.ownerId, filters.ownerId));
  if (filters.sessionId) conditions.push(eq(documents.sessionId, filters.sessionId));

  if (conditions.length === 0) return [];

  return db
    .select()
    .from(documents)
    .where(conditions.length === 1 ? conditions[0] : and(...conditions));
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
  ownerId: string,
  sessionId: string
) {
  // Check if document already exists for this owner+session
  const existing = await db
    .select()
    .from(documents)
    .where(
      and(
        eq(documents.ownerId, ownerId),
        eq(documents.sessionId, sessionId)
      )
    );

  if (existing.length > 0) return existing[0];

  const [doc] = await db
    .insert(documents)
    .values({ ownerId, sessionId })
    .returning();
  return doc;
}

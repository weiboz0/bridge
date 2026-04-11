import { eq, sql } from "drizzle-orm";
import { users } from "@/lib/db/schema";
import type { Database } from "@/lib/db";

export async function listUsers(db: Database) {
  return db.select().from(users);
}

export async function countUsers(db: Database) {
  const result = await db
    .select({ count: sql<number>`count(*)` })
    .from(users);
  return Number(result[0].count);
}

export async function getUserByEmail(db: Database, email: string) {
  const [user] = await db
    .select()
    .from(users)
    .where(eq(users.email, email));
  return user || null;
}

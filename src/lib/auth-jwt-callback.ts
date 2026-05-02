// Plan 050: extracted from `src/lib/auth.ts` so it's testable without
// pulling in NextAuth (which trips vitest's node resolution). The
// callback's behavior is exercised by
// `tests/unit/auth-jwt-refresh.test.ts` via this module.

import { eq } from "drizzle-orm";
import { db } from "@/lib/db";
import { users } from "@/lib/db/schema";

// Loose token shape — NextAuth's JWT type is `Record<string, unknown>`
// in practice, and we only read/write the three fields below.
export interface BridgeAuthToken {
  email?: string | null;
  name?: string | null;
  id?: string;
  isPlatformAdmin?: boolean;
  [k: string]: unknown;
}

// Reconcile `id` and `isPlatformAdmin` against the DB. Called from
// the NextAuth `callbacks.jwt` hook on EVERY request — that's the
// whole point of plan 050. Indexed lookup on `email`
// (`users_email_idx`) is sub-millisecond.
//
// Account-deletion case: if the row is gone (deleted/email race),
// null the elevated claims. NextAuth's `session()` callback reads
// `token.id`, so a missing id makes the next request behave as
// unauthenticated.
export async function refreshJwtFromDb(token: BridgeAuthToken): Promise<BridgeAuthToken> {
  const lookupEmail = token.email;
  if (!lookupEmail) return token;
  const [dbUser] = await db
    .select({
      id: users.id,
      isPlatformAdmin: users.isPlatformAdmin,
    })
    .from(users)
    .where(eq(users.email, lookupEmail));
  if (!dbUser) {
    token.id = undefined;
    token.isPlatformAdmin = false;
    return token;
  }
  token.id = dbUser.id;
  token.isPlatformAdmin = dbUser.isPlatformAdmin;
  return token;
}

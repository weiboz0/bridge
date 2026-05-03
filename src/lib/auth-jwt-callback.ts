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
// the NextAuth `callbacks.jwt` hook. Indexed lookup on `email`
// (`users_email_idx`) is sub-millisecond on the Node runtime.
//
// **Edge runtime guard**: NextAuth's `auth()` is invoked from
// middleware, which Next.js runs on the Edge runtime (no TCP
// sockets, fetch-only). drizzle/postgres-js opens TCP, so the
// query throws there. When we detect Edge, return the token
// unchanged — middleware uses whatever was on the JWT at last
// Node-side refresh. Plan 050's freshness guarantee still holds
// for Node-side route handlers and page renders, which is where
// admin UI actually gates on `isPlatformAdmin`. Middleware only
// uses the JWT to decide which page to render; the admin page
// itself runs on Node and re-checks via this helper at render
// time.
//
// Account-deletion case: if the row is gone (deleted/email race),
// null the elevated claims. NextAuth's `session()` callback reads
// `token.id`, so a missing id makes the next request behave as
// unauthenticated.
export async function refreshJwtFromDb(token: BridgeAuthToken): Promise<BridgeAuthToken> {
  const lookupEmail = token.email;
  if (!lookupEmail) return token;
  if (isEdgeRuntime()) return token;
  try {
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
  } catch (err) {
    // DB unreachable. Return the token unchanged rather than
    // crashing the auth flow with a JWTSessionError. Defense in
    // depth — the Edge guard above should already cover the
    // common case, but this catches any other connectivity
    // hiccup.
    console.error("[refreshJwtFromDb] DB lookup failed; returning cached token", err);
    return token;
  }
}

// isEdgeRuntime detects Vercel/Next Edge runtime. Vercel exposes
// `EdgeRuntime` as a global in Edge bundles; in Node, that global
// is undefined.
function isEdgeRuntime(): boolean {
  return typeof (globalThis as { EdgeRuntime?: unknown }).EdgeRuntime !== "undefined";
}

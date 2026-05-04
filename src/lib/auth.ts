import NextAuth, { type NextAuthConfig } from "next-auth";
import Google from "next-auth/providers/google";
import Credentials from "next-auth/providers/credentials";
import { cookies } from "next/headers";
import { db } from "@/lib/db";
import { users, authProviders } from "@/lib/db/schema";
import { eq, and } from "drizzle-orm";
import { z } from "zod";
import { getSessionCookieName, isSecureAuthScheme } from "@/lib/auth-cookie";
import { refreshJwtFromDb } from "@/lib/auth-jwt-callback";

const loginSchema = z.object({
  email: z.string().email(),
  password: z.string().min(8),
});

const SIGNUP_INTENT_COOKIE = "bridge-signup-intent";

/**
 * Plan 043 phase 5: reads + clears the signup-intent cookie set by the
 * register page before a Google OAuth redirect. Returns the role the
 * user picked, or null if no valid cookie is present.
 *
 * Called only from the OAuth signIn callback when creating a brand-new
 * user, so a stale cookie can't accidentally overwrite an existing
 * user's intendedRole.
 */
async function readSignupIntentRole(): Promise<"teacher" | "student" | null> {
  try {
    const cookieStore = await cookies();
    const raw = cookieStore.get(SIGNUP_INTENT_COOKIE)?.value;
    // Plan 043 Codex post-impl review: clear the cookie regardless of
    // outcome. Leaving a stale cookie around can corrupt the next signup's
    // role intent (a different user on the same browser, or the same user
    // re-attempting after a failed first round-trip).
    if (raw) {
      try {
        cookieStore.delete(SIGNUP_INTENT_COOKIE);
      } catch {
        // cookies().delete() can throw in contexts where setting cookies
        // isn't allowed (e.g., during a render phase that won't ship a
        // response). Falling back to a stale cookie is acceptable — the
        // 5-minute Max-Age caps the blast radius.
      }
    }
    if (!raw) return null;
    const parsed = JSON.parse(raw) as { role?: string };
    if (parsed?.role === "teacher" || parsed?.role === "student") {
      return parsed.role;
    }
    return null;
  } catch {
    return null;
  }
}

export const authConfig = {
  providers: [
    Google({
      clientId: process.env.GOOGLE_CLIENT_ID!,
      clientSecret: process.env.GOOGLE_CLIENT_SECRET!,
    }),
    Credentials({
      name: "Email",
      credentials: {
        email: { label: "Email", type: "email" },
        password: { label: "Password", type: "password" },
      },
      async authorize(credentials) {
        const parsed = loginSchema.safeParse(credentials);
        if (!parsed.success) return null;

        const [user] = await db
          .select()
          .from(users)
          .where(eq(users.email, parsed.data.email));

        if (!user || !user.passwordHash) return null;

        const bcrypt = await import("bcryptjs");
        const valid = await bcrypt.compare(
          parsed.data.password,
          user.passwordHash
        );
        if (!valid) return null;

        return {
          id: user.id,
          name: user.name,
          email: user.email,
        };
      },
    }),
  ],
  callbacks: {
    // Middleware authorization. Runs ONLY for paths matched by
    // src/middleware.ts. Two distinct branches:
    //
    //   - /api/orgs/*, /api/admin/*: pass through. The previous
    //     `auth as middleware` re-export had no callback, which
    //     defaults to `authorized = true` — the route handler does
    //     its own session check and returns 401 / handles the unauth
    //     case (e.g. /api/admin/impersonate/status returns
    //     `{ impersonating: null }` instead of 401 by design).
    //     Returning `isAuthed` here would break that contract by
    //     having Auth.js redirect to /login before the handler runs.
    //   - portal trees (/teacher/*, /student/*, /parent/*, /org/*,
    //     /admin/* — but NOT the API paths above): redirect to
    //     /login?callbackUrl=<original> when unauthenticated so deep
    //     links survive the sign-out → sign-in round-trip
    //     (review-002 P2 #7 fix).
    authorized({ request, auth: sessionAuth }) {
      const { pathname, search } = request.nextUrl;
      // Any /api/* path is pass-through. The route handler / Go
      // backend enforces auth and returns 401 on its own. This is
      // the legacy contract for /api/orgs and /api/admin (preserved
      // since plan 040), and plan 065 phase 2 extended it to every
      // proxied API path so unauth XHR/fetch calls don't get a
      // 307 redirect to /login when the matcher catches them.
      if (pathname.startsWith("/api/")) {
        return true;
      }

      const isAuthed = !!sessionAuth?.user;
      if (isAuthed) return true;

      // Portal tree, unauthenticated — redirect with callbackUrl baked in.
      const callback = encodeURIComponent(pathname + (search || ""));
      const loginUrl = new URL(
        `/login?callbackUrl=${callback}`,
        request.nextUrl.origin
      );
      return Response.redirect(loginUrl);
    },
    async signIn({ user, account }) {
      if (account?.provider === "google" && user.email) {
        const [existing] = await db
          .select()
          .from(users)
          .where(eq(users.email, user.email));

        let userId: string;

        if (existing) {
          userId = existing.id;
        } else {
          // Plan 043 phase 5: read the signup-intent cookie set by
          // /register before the Google round-trip, so a new OAuth user
          // gets the same intendedRole they picked on the form.
          // Existing users re-signing in are not touched.
          const intendedRole = await readSignupIntentRole();

          const [newUser] = await db
            .insert(users)
            .values({
              name: user.name || "Unknown",
              email: user.email,
              avatarUrl: user.image,
              intendedRole,
            })
            .returning();
          userId = newUser.id;
        }

        const [existingProvider] = await db
          .select()
          .from(authProviders)
          .where(
            and(
              eq(authProviders.provider, "google"),
              eq(authProviders.providerUserId, account.providerAccountId)
            )
          );

        if (!existingProvider) {
          await db.insert(authProviders).values({
            userId,
            provider: "google",
            providerUserId: account.providerAccountId,
          });
        }
      }
      return true;
    },
    async jwt({ token }) {
      // Plan 050 — refresh `id` and `isPlatformAdmin` from the DB on
      // EVERY request via the extracted helper (which is testable
      // without pulling NextAuth into vitest's node resolver).
      return refreshJwtFromDb(token);
    },
    async session({ session, token }) {
      if (token) {
        session.user.id = token.id as string;
        session.user.isPlatformAdmin = token.isPlatformAdmin as boolean;
      }
      return session;
    },
  },
  events: {
    // Plan 065 phase 2 — clear bridge.session on signout.
    //
    // The signout endpoint (`/api/auth/signout`) is OUTSIDE the
    // middleware matcher, so the wrapper's stale-cookie cleanup
    // never runs for the request that triggers signout. Without
    // this event handler the cookie persists for its full 7-day
    // TTL, which would let a signed-out user retain a valid
    // bridge.session — a real security issue once Phase 3 makes
    // Go trust this cookie. (Codex pass-1 of Phase-2 caught this.)
    //
    // events.signOut runs server-side as part of the signout HTTP
    // handler, so cookies().delete() reliably attaches a
    // Set-Cookie with Max-Age=0 to the response Auth.js sends
    // back to the browser.
    async signOut() {
      try {
        const cookieStore = await cookies();
        cookieStore.delete("bridge.session");
      } catch {
        // cookies().delete() can throw in edge cases (e.g., a
        // signOut called from a context where setting cookies
        // isn't allowed). The middleware wrapper's null-session
        // path is the safety net — eventually some authenticated
        // request will arrive and the cookie will be cleared
        // there. The 7-day TTL caps the worst-case window.
      }
    },
  },
  pages: {
    signIn: "/login",
  },
  session: {
    strategy: "jwt",
  },
  cookies: {
    sessionToken: {
      name: getSessionCookieName(),
      options: {
        httpOnly: true,
        sameSite: "lax",
        path: "/",
        secure: isSecureAuthScheme(),
      },
    },
  },
} satisfies NextAuthConfig;

export const { handlers, auth, signIn, signOut } = NextAuth(authConfig);

import NextAuth, { type NextAuthConfig } from "next-auth";
import Google from "next-auth/providers/google";
import Credentials from "next-auth/providers/credentials";
import { db } from "@/lib/db";
import { users, authProviders } from "@/lib/db/schema";
import { eq, and } from "drizzle-orm";
import { z } from "zod";
import { getSessionCookieName, isSecureAuthScheme } from "@/lib/auth-cookie";

const loginSchema = z.object({
  email: z.string().email(),
  password: z.string().min(8),
});

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
    // src/middleware.ts. Two distinct branches preserve the existing
    // contract while adding portal deep-link preservation:
    //
    //   - /api/orgs/*, /api/admin/*: require an authenticated session;
    //     Auth.js returns 401 when this returns false. Same effective
    //     behavior as the previous `auth as middleware` re-export.
    //   - portal trees (/teacher/*, /student/*, /parent/*, /org/*,
    //     /admin/* — but NOT the API paths above): require auth too,
    //     but on failure redirect to /login with callbackUrl set to
    //     the original path so the user lands back where they wanted
    //     after sign-in (review-002 P2 #7 fix).
    authorized({ request, auth: sessionAuth }) {
      const { pathname, search } = request.nextUrl;
      const isApiPath =
        pathname.startsWith("/api/orgs") || pathname.startsWith("/api/admin");
      const isAuthed = !!sessionAuth?.user;

      if (isApiPath) {
        // Preserve existing API auth-guard contract.
        return isAuthed;
      }

      // Portal tree — redirect with callbackUrl baked in.
      if (isAuthed) return true;
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
          // New user — no role assigned, just create account
          const [newUser] = await db
            .insert(users)
            .values({
              name: user.name || "Unknown",
              email: user.email,
              avatarUrl: user.image,
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
    async jwt({ token, user }) {
      if (user) {
        const [dbUser] = await db
          .select()
          .from(users)
          .where(eq(users.email, token.email!));
        if (dbUser) {
          token.id = dbUser.id;
          token.isPlatformAdmin = dbUser.isPlatformAdmin;
        }
      }
      return token;
    },
    async session({ session, token }) {
      if (token) {
        session.user.id = token.id as string;
        session.user.isPlatformAdmin = token.isPlatformAdmin as boolean;
      }
      return session;
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

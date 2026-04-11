import NextAuth from "next-auth";
import Google from "next-auth/providers/google";
import Credentials from "next-auth/providers/credentials";
import { db } from "@/lib/db";
import { users, authProviders } from "@/lib/db/schema";
import { eq, and } from "drizzle-orm";
import { z } from "zod";

const loginSchema = z.object({
  email: z.string().email(),
  password: z.string().min(8),
});

export const { handlers, auth, signIn, signOut } = NextAuth({
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
});

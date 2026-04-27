import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import bcrypt from "bcryptjs";
import { db } from "@/lib/db";
import { users, authProviders } from "@/lib/db/schema";
import { eq } from "drizzle-orm";

const registerSchema = z.object({
  name: z.string().min(1).max(255),
  email: z.string().email(),
  password: z.string().min(8).max(128),
  // Captures "what the user said when they signed up." Optional so other
  // callers (Google OAuth flow, future invite flows) aren't broken.
  role: z.enum(["teacher", "student"]).optional(),
});

export async function POST(request: NextRequest) {
  const body = await request.json();
  const parsed = registerSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const { name, email, password, role } = parsed.data;

  const [existing] = await db
    .select()
    .from(users)
    .where(eq(users.email, email));

  if (existing) {
    return NextResponse.json(
      { error: "Email already registered" },
      { status: 409 }
    );
  }

  const passwordHash = await bcrypt.hash(password, 10);

  const [user] = await db
    .insert(users)
    .values({
      name,
      email,
      passwordHash,
      intendedRole: role ?? null,
    })
    .returning({
      id: users.id,
      name: users.name,
      email: users.email,
      intendedRole: users.intendedRole,
    });

  await db.insert(authProviders).values({
    userId: user.id,
    provider: "email",
    providerUserId: user.id,
  });

  return NextResponse.json(user, { status: 201 });
}

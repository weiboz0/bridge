import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { createOrganization } from "@/lib/organizations";
import { addOrgMember, getUserMemberships } from "@/lib/org-memberships";

const createSchema = z.object({
  name: z.string().min(1).max(255),
  slug: z.string().min(1).max(255).regex(/^[a-z0-9-]+$/),
  type: z.enum(["school", "tutoring_center", "bootcamp", "other"]),
  contactEmail: z.string().email(),
  contactName: z.string().min(1).max(255),
  domain: z.string().max(255).optional(),
});

export async function POST(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const body = await request.json();
  const parsed = createSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  // Check for duplicate slug
  const { getOrganizationBySlug } = await import("@/lib/organizations");
  const existingSlug = await getOrganizationBySlug(db, parsed.data.slug);
  if (existingSlug) {
    return NextResponse.json(
      { error: "Organization with this slug already exists" },
      { status: 409 }
    );
  }

  const org = await createOrganization(db, parsed.data);

  // Auto-assign creator as org_admin
  await addOrgMember(db, {
    orgId: org.id,
    userId: session.user.id,
    role: "org_admin",
    status: "active",
  });

  return NextResponse.json(org, { status: 201 });
}

export async function GET() {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const memberships = await getUserMemberships(db, session.user.id);
  return NextResponse.json(memberships);
}

import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getOrganization, updateOrgStatus } from "@/lib/organizations";

const updateSchema = z.object({
  status: z.enum(["active", "suspended"]),
});

export async function PATCH(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id || !session.user.isPlatformAdmin) {
    return NextResponse.json({ error: "Platform admin required" }, { status: 403 });
  }

  const { id } = await params;
  const org = await getOrganization(db, id);

  if (!org) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  const body = await request.json();
  const parsed = updateSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const updated = await updateOrgStatus(db, id, parsed.data.status);
  return NextResponse.json(updated);
}

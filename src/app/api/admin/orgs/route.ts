import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { listOrganizations } from "@/lib/organizations";

export async function GET(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id || !session.user.isPlatformAdmin) {
    return NextResponse.json({ error: "Platform admin required" }, { status: 403 });
  }

  const status = request.nextUrl.searchParams.get("status") || undefined;
  const orgs = await listOrganizations(db, status);
  return NextResponse.json(orgs);
}

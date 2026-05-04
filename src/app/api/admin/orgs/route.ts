import { NextRequest, NextResponse } from "next/server";
import { db } from "@/lib/db";
import { requireAdmin } from "@/lib/identity";
import { listOrganizations } from "@/lib/organizations";

export async function GET(request: NextRequest) {
  // Plan 065 phase 4 — admin status comes from /api/me/identity
  // (live DB value via Phase 3 middleware), not from the Auth.js
  // JWT-carried claim. The helper returns null when the caller
  // isn't a live admin.
  const admin = await requireAdmin();
  if (!admin) {
    return NextResponse.json({ error: "Platform admin required" }, { status: 403 });
  }

  const status = request.nextUrl.searchParams.get("status") || undefined;
  const orgs = await listOrganizations(db, status);
  return NextResponse.json(orgs);
}

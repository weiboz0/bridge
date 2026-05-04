"use client";

import { signOut } from "next-auth/react";
import { Button } from "@/components/ui/button";
import { SIDEBAR_EXPANDED_STORAGE_KEY } from "@/lib/hooks/use-sidebar-sections";

async function handleSignOut() {
  // Clear both Auth.js cookie variants explicitly before Auth.js's own
  // signOut runs — see /api/auth/logout-cleanup. signOut() only clears
  // the variant it currently uses; the other can survive and leak.
  try {
    await fetch("/api/auth/logout-cleanup", { method: "POST" });
  } catch {
    // Even if cleanup fails, still proceed with signOut.
  }
  // Plan 067 — clear sidebar section state so the next user (or the
  // same user after re-sign-in) starts with defaults.
  try {
    localStorage.removeItem(SIDEBAR_EXPANDED_STORAGE_KEY);
  } catch {
    // Ignore quota errors.
  }
  await signOut({ callbackUrl: "/" });
}

export function SignOutButton() {
  return (
    <Button variant="ghost" size="sm" onClick={handleSignOut}>
      Sign Out
    </Button>
  );
}

import { cookies } from "next/headers";
import { auth } from "@/lib/auth";
import { getIdentity } from "@/lib/identity";

interface ImpersonationData {
  originalUserId: string;
  targetUserId: string;
  targetName: string;
  targetEmail: string;
}

/**
 * Get the effective user session, accounting for admin impersonation.
 *
 * If a platform admin is impersonating another user, returns a modified
 * session with the target user's identity. The original admin ID is
 * preserved in the `impersonating` field.
 *
 * Plan 065 phase 4 — `isPlatformAdmin` is now sourced from
 * `/api/me/identity` (the live DB value via Phase 3 middleware), not
 * from the Auth.js JWT-carried claim. A user who was demoted between
 * sign-in and this call no longer triggers impersonation.
 *
 * Usage: Replace `auth()` with `getEffectiveSession()` in any page/route
 * that should support impersonation.
 */
export async function getEffectiveSession() {
  const session = await auth();
  if (!session?.user?.id) return null;

  const cookieStore = await cookies();
  const impersonateCookie = cookieStore.get("bridge-impersonate");

  if (!impersonateCookie?.value) {
    return { ...session, impersonating: null };
  }

  // Only LIVE platform admins can impersonate. The check goes
  // through the identity helper so a stale JWT-carried admin claim
  // can't trigger impersonation after the user was demoted.
  const identity = await getIdentity();
  if (!identity?.isPlatformAdmin) {
    return { ...session, impersonating: null };
  }

  try {
    const data: ImpersonationData = JSON.parse(impersonateCookie.value);

    // Verify the cookie was set by the current admin
    if (data.originalUserId !== session.user.id) {
      return { ...session, impersonating: null };
    }

    return {
      ...session,
      user: {
        ...session.user,
        id: data.targetUserId,
        name: data.targetName,
        email: data.targetEmail,
        isPlatformAdmin: false, // Impersonated user is never admin
      },
      impersonating: {
        originalUserId: data.originalUserId,
        targetUserId: data.targetUserId,
        targetName: data.targetName,
      },
    };
  } catch {
    return { ...session, impersonating: null };
  }
}

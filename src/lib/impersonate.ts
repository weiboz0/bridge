import { cookies } from "next/headers";
import { auth } from "@/lib/auth";

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

  // Only platform admins can impersonate
  if (!session.user.isPlatformAdmin) {
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

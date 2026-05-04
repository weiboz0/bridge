import { auth } from "@/lib/auth";
import { getIdentity } from "@/lib/identity";

/**
 * Get the effective user session, accounting for admin impersonation.
 *
 * If a platform admin is impersonating another user, returns a modified
 * session with the target user's identity. The original admin ID is
 * preserved in the `impersonating` field.
 *
 * Plan 065 phase 4 — gate on `identity.impersonatedBy` from
 * `/api/me/identity` rather than re-validating the
 * `bridge-impersonate` cookie here. Go's middleware (Phase 3) is
 * the single authority for whether impersonation is active and
 * authorized. When it applies the overlay, it sets
 * `claims.ImpersonatedBy` to the admin's user id and rewrites
 * the rest of the claims to the target user. /api/me/identity
 * exposes those rewritten claims directly.
 *
 * Codex Phase-4 review caught the bug from the previous shape
 * (re-checking the cookie + admin status here): under
 * impersonation, Go returns the TARGET'S identity to the Next
 * helper, so `identity.isPlatformAdmin` is false even when the
 * admin originally authorized the impersonation. Reading
 * `identity.impersonatedBy` is the right primitive — non-empty
 * means "Go has authorized this impersonation."
 *
 * Usage: Replace `auth()` with `getEffectiveSession()` in any page/route
 * that should support impersonation.
 */
export async function getEffectiveSession() {
  const session = await auth();
  if (!session?.user?.id) return null;

  const identity = await getIdentity();
  if (!identity?.impersonatedBy) {
    return { ...session, impersonating: null };
  }

  return {
    ...session,
    user: {
      ...session.user,
      id: identity.userId,
      name: identity.name,
      email: identity.email,
      isPlatformAdmin: false, // Impersonated user is never admin
    },
    impersonating: {
      originalUserId: identity.impersonatedBy,
      targetUserId: identity.userId,
      targetName: identity.name,
    },
  };
}

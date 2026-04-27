/**
 * Dev-only identity drift logger.
 *
 * Server components that combine `auth()` (Next-side identity) with a Go
 * resource owned by some user ID often end up calling `notFound()` when the
 * IDs disagree. In production that's the right behavior — but in dev it
 * masks the auth canonicalization regression we shipped 039 to prevent.
 *
 * Call this right before notFound()/forbidden so the developer sees the
 * cause in the terminal instead of a generic 404.
 */
export function logIdentityMismatch(
  context: string,
  nextAuthUserId: string | null | undefined,
  goResourceUserId: string | null | undefined,
  extra?: Record<string, unknown>
) {
  if (process.env.NODE_ENV === "production") return;
  if (nextAuthUserId === goResourceUserId) return;
  console.error(
    `[identity-mismatch] ${context}: nextAuthUserId=${nextAuthUserId ?? "null"} goResourceUserId=${goResourceUserId ?? "null"}` +
      (extra ? ` extra=${JSON.stringify(extra)}` : "") +
      ` — visit /api/auth/debug for full diagnostic.`
  );
}

import { cache } from "react";
import { api, ApiError } from "@/lib/api-client";

// Plan 065 Phase 4 — Next-side identity helper.
//
// Replaces direct reads of `session.user.isPlatformAdmin` (sourced
// from the Auth.js JWT, refreshed only by `refreshJwtFromDb` per
// request — and only on the Node runtime, see PR #103). After
// Phase 3, Go middleware has the live DB value baked into every
// request's claims, so `/api/me/identity` is the authoritative
// source. Callsites switch to this helper.
//
// `React.cache` deduplicates calls within a single request so
// SSR + route handlers + nested server components that all need
// `isPlatformAdmin` make exactly one network call per render.

export interface Identity {
  userId: string;
  email: string;
  name: string;
  isPlatformAdmin: boolean;
  impersonatedBy: string;
}

// getIdentity returns the live identity for the current request,
// or null when the user is not authenticated. The returned object
// is from Go's perspective — `isPlatformAdmin` reflects the live
// DB value (Phase 3's middleware overwrote it), not the JWT-carried
// hint.
//
// Memoized via React.cache so SSR + route handlers + nested server
// components share one call per request render. Outside a React
// request scope (e.g., from a script), each call hits Go.
export const getIdentity = cache(async (): Promise<Identity | null> => {
  try {
    return await api<Identity>("/api/me/identity");
  } catch (err) {
    // 401 from Go = unauthenticated. Other errors mean the Go API
    // is unreachable or returned malformed data; surface as null
    // so callers can decide their own fallback (typically: treat
    // as unauthenticated and 401 the caller's response).
    if (err instanceof ApiError && err.status === 401) {
      return null;
    }
    if (err instanceof ApiError) {
      console.warn(
        `[identity] /api/me/identity returned ${err.status}; treating as unauthenticated`,
        err.body
      );
      return null;
    }
    console.warn(
      "[identity] failed to call /api/me/identity",
      err instanceof Error ? err.message : err
    );
    return null;
  }
});

// requireAdmin returns the live identity when the caller is a
// platform admin, or null otherwise. Callers use the null return
// to emit a 403. Replaces the legacy
// `session?.user?.id && session.user.isPlatformAdmin` checks
// scattered across route handlers; centralizing the read here
// means a single source of truth for "is this request an admin
// request" — the live DB value as Go sees it.
export async function requireAdmin(): Promise<Identity | null> {
  const id = await getIdentity();
  if (!id?.userId || !id.isPlatformAdmin) {
    return null;
  }
  return id;
}

"use client";

import { useEffect, useState } from "react";
import { getRealtimeToken, RealtimeMintError } from "./get-token";

// Plan 053 phase 3 — React hook wrapper around `getRealtimeToken` for
// the realtime-using callsites (problem editor, teacher watch, unit
// editor, student session, teacher dashboard, broadcast controls,
// student tile, parent live-session viewer). Returns "" while the
// mint is in flight; downstream `useYjsProvider` checks for non-empty
// token before connecting (matches the existing `shouldConnect`
// pattern).
//
// Pass `documentName === "noop"` (or empty) to suppress the fetch —
// matches how callsites already conditionalize the doc-name when the
// feature is inactive (e.g. broadcast-active flag).
//
// Plan 068 phase 4 — extended to surface the 503 ("realtime not
// configured") path explicitly via `unavailable: boolean`. The
// underlying `getRealtimeToken` already throws
// `RealtimeMintError{status: 503}` for 503 responses; the hook now
// propagates that to consumers so they can render the
// RealtimeConfigBanner instead of silently failing to connect.
//
// Backward compat: the return value is now an object. The previous
// "string" shape is replaced — all callsites are updated in the same
// PR (the realtime-using pages are enumerated in plan 068 §Phase 4).

export interface UseRealtimeToken {
  token: string;
  // True when the most recent mint attempt failed with HTTP 503
  // ("realtime not configured" — HOCUSPOCUS_TOKEN_SECRET unset on
  // the Go API). False during in-flight mints, when the token is
  // valid, or for any non-503 failure (those are real bugs and
  // surface via console errors instead).
  unavailable: boolean;
}

export function useRealtimeToken(documentName: string, sessionId?: string): UseRealtimeToken {
  const identity = sessionId === undefined ? documentName : `${documentName}\u0000${sessionId}`;
  const [result, setResult] = useState<{ identity: string; token: string; unavailable: boolean }>({
    identity: "",
    token: "",
    unavailable: false,
  });

  useEffect(() => {
    if (!documentName || documentName === "noop") {
      return;
    }
    let cancelled = false;
    getRealtimeToken(documentName, sessionId)
      .then((t) => {
        if (!cancelled) {
          setResult({ identity, token: t, unavailable: false });
        }
      })
      .catch((err) => {
        // Mint failures shouldn't crash the React tree. Log + leave
        // token empty; useYjsProvider's shouldConnect guard will
        // refuse to open the WS, and the user sees the
        // "Disconnected" UI instead of a stack trace.
        console.error(`[realtime] token mint failed for ${documentName}:`, err);
        if (cancelled) return;
        // Specifically mark "unavailable" only on 503 — other failure
        // modes (4xx auth errors, 5xx other, network blips) surface
        // via console error and the disconnect-state UI. The banner
        // copy points specifically at the env-config issue, so a
        // misleading false-positive on a transient network error
        // would be worse than no banner.
        setResult({
          identity,
          token: "",
          unavailable: err instanceof RealtimeMintError && err.status === 503,
        });
      });
    return () => {
      cancelled = true;
    };
  }, [documentName, identity, sessionId]);

  if (!documentName || documentName === "noop" || result.identity !== identity) {
    return { token: "", unavailable: false };
  }
  return { token: result.token, unavailable: result.unavailable };
}

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

export function useRealtimeToken(documentName: string): UseRealtimeToken {
  const [token, setToken] = useState<string>("");
  const [unavailable, setUnavailable] = useState<boolean>(false);

  useEffect(() => {
    if (!documentName || documentName === "noop") {
      setToken("");
      setUnavailable(false);
      return;
    }
    // CLEAR the previous doc's token synchronously before the new
    // mint resolves. Otherwise a docName change A→B leaves the
    // stale A-scoped token in state during the B-mint window, and
    // useYjsProvider would feed it into a Hocuspocus connection for
    // documentName=B → server-side `claims.scope === documentName`
    // check fails and the WS closes (best case) or scope confusion
    // happens (worst case). Resetting here forces useYjsProvider's
    // `shouldConnect` guard to skip the WS open until the B-mint
    // lands.
    setToken("");
    setUnavailable(false);
    let cancelled = false;
    getRealtimeToken(documentName)
      .then((t) => {
        if (!cancelled) {
          setToken(t);
          setUnavailable(false);
        }
      })
      .catch((err) => {
        // Mint failures shouldn't crash the React tree. Log + leave
        // token empty; useYjsProvider's shouldConnect guard will
        // refuse to open the WS, and the user sees the
        // "Disconnected" UI instead of a stack trace.
        console.error(`[realtime] token mint failed for ${documentName}:`, err);
        if (cancelled) return;
        setToken("");
        // Specifically mark "unavailable" only on 503 — other failure
        // modes (4xx auth errors, 5xx other, network blips) surface
        // via console error and the disconnect-state UI. The banner
        // copy points specifically at the env-config issue, so a
        // misleading false-positive on a transient network error
        // would be worse than no banner.
        if (err instanceof RealtimeMintError && err.status === 503) {
          setUnavailable(true);
        } else {
          setUnavailable(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [documentName]);

  return { token, unavailable };
}

"use client";

import { useEffect, useState } from "react";
import { getRealtimeToken } from "./get-token";

// Plan 053 phase 3 — React hook wrapper around `getRealtimeToken` for
// the 5 callsites (problem editor, teacher watch, unit editor,
// student session, teacher dashboard). Returns "" while the mint is
// in flight; downstream `useYjsProvider` checks for non-empty token
// before connecting (matches the existing `shouldConnect` pattern).
//
// Pass `documentName === "noop"` (or empty) to suppress the fetch —
// matches how callsites already conditionalize the doc-name when the
// feature is inactive (e.g. broadcast-active flag).
export function useRealtimeToken(documentName: string): string {
  const [token, setToken] = useState<string>("");

  useEffect(() => {
    if (!documentName || documentName === "noop") {
      setToken("");
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
    let cancelled = false;
    getRealtimeToken(documentName)
      .then((t) => {
        if (!cancelled) setToken(t);
      })
      .catch((err) => {
        // Mint failures shouldn't crash the React tree. Log + leave
        // token empty; useYjsProvider's shouldConnect guard will
        // refuse to open the WS, and the user sees the
        // "Disconnected" UI instead of a stack trace.
        console.error(`[realtime] token mint failed for ${documentName}:`, err);
        if (!cancelled) setToken("");
      });
    return () => {
      cancelled = true;
    };
  }, [documentName]);

  return token;
}

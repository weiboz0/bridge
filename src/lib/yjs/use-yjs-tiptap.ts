"use client";

import { useState, useRef, useEffect } from "react";
import * as Y from "yjs";
import { HocuspocusProvider } from "@hocuspocus/provider";
import Collaboration from "@tiptap/extension-collaboration";
import CollaborationCursor from "@tiptap/extension-collaboration-cursor";
import type { AnyExtension } from "@tiptap/react";
import { useRealtimeToken } from "@/lib/realtime/use-realtime-token";

// Fixed palette of 10 distinct, accessible colors for cursor labels.
const CURSOR_COLORS = [
  "#2563eb", // blue-600
  "#16a34a", // green-600
  "#dc2626", // red-600
  "#d97706", // amber-600
  "#7c3aed", // violet-600
  "#db2777", // pink-600
  "#0891b2", // cyan-600
  "#ea580c", // orange-600
  "#4f46e5", // indigo-600
  "#059669", // emerald-600
];

/**
 * Deterministically pick a cursor color from the palette based on the user ID.
 * The same user always gets the same color, regardless of session order.
 */
function colorFromUserId(userId: string): string {
  let hash = 0;
  for (let i = 0; i < userId.length; i++) {
    hash = (hash * 31 + userId.charCodeAt(i)) >>> 0;
  }
  return CURSOR_COLORS[hash % CURSOR_COLORS.length];
}

export interface UseYjsTiptapOptions {
  unitId: string;
  userId: string;
  userName: string;
  userColor?: string;
}

export interface UseYjsTiptapReturn {
  provider: HocuspocusProvider | null;
  ydoc: Y.Doc | null;
  /** Tiptap extensions to spread into the editor's `extensions` array. */
  extensions: AnyExtension[];
  connected: boolean;
  /**
   * Plan 068 phase 4 — true when the realtime token mint failed with HTTP 503
   * (HOCUSPOCUS_TOKEN_SECRET unset on the Go API). Consuming components
   * should render the RealtimeConfigBanner instead of letting the editor
   * silently fail to connect.
   */
  realtimeUnavailable: boolean;
  destroy: () => void;
}

/**
 * Sets up a Yjs document + HocuspocusProvider for collaborative editing of a
 * teaching unit and returns the Tiptap `Collaboration` and
 * `CollaborationCursor` extensions pre-configured with the shared doc and
 * the current user's awareness data.
 *
 * The caller is responsible for merging the returned `extensions` into the
 * Tiptap editor's extension list.  When the component unmounts, call
 * `destroy()` to clean up the provider and Y.Doc.
 *
 * Document namespace: `unit:{unitId}` — Hocuspocus provides realtime sync only.
 * Persistence happens via the teaching-unit API (save button), NOT Hocuspocus.
 */
export function useYjsTiptap({
  unitId,
  userId,
  userName,
  userColor,
}: UseYjsTiptapOptions): UseYjsTiptapReturn {
  const [connected, setConnected] = useState(false);
  const providerRef = useRef<HocuspocusProvider | null>(null);
  const ydocRef = useRef<Y.Doc | null>(null);
  // Use a counter to trigger re-renders when provider/ydoc refs change.
  const [, forceUpdate] = useState(0);

  // Plan 053 phase 3 — mint a JWT scoped to this unit doc instead of
  // constructing the legacy `${userId}:teacher` string. The hook
  // returns "" while the mint is in flight; useEffect re-runs once
  // the token resolves.
  // Plan 068 phase 4 — `unavailable` is forwarded to the caller so
  // the consuming component (e.g., teaching-unit-editor) can render
  // the realtime-config banner at page level.
  const documentName = unitId && unitId !== "noop" ? `unit:${unitId}` : "noop";
  const { token: realtimeToken, unavailable: realtimeUnavailable } = useRealtimeToken(documentName);

  const shouldConnect =
    Boolean(unitId) &&
    unitId !== "noop" &&
    Boolean(userId) &&
    !userId.startsWith(":") &&
    Boolean(realtimeToken);

  useEffect(() => {
    if (!shouldConnect) {
      ydocRef.current = null;
      providerRef.current = null;
      setConnected(false);
      forceUpdate((n) => n + 1);
      return;
    }

    const serverUrl =
      process.env.NEXT_PUBLIC_HOCUSPOCUS_URL ||
      (typeof window !== "undefined"
        ? `ws://${window.location.hostname}:4000`
        : "ws://127.0.0.1:4000");

    const ydoc = new Y.Doc();

    const provider = new HocuspocusProvider({
      url: serverUrl,
      name: documentName,
      document: ydoc,
      token: realtimeToken,
      onConnect: () => {
        console.log(`[yjs-tiptap] Connected to ${documentName}`);
        setConnected(true);
      },
      onDisconnect: () => {
        console.log(`[yjs-tiptap] Disconnected from ${documentName}`);
        setConnected(false);
      },
      onAuthenticationFailed: (data) => {
        console.error(`[yjs-tiptap] Auth failed for ${documentName}:`, data);
      },
    });

    // Set awareness data so collaboration cursors show the current user.
    const color = userColor ?? colorFromUserId(userId);
    provider.setAwarenessField("user", { name: userName || "Anonymous", color });

    ydocRef.current = ydoc;
    providerRef.current = provider;
    forceUpdate((n) => n + 1);

    return () => {
      provider.destroy();
      ydoc.destroy();
      ydocRef.current = null;
      providerRef.current = null;
    };
    // Re-run only when connection parameters change. realtimeToken
    // included so we rebuild the provider when the JWT refreshes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [shouldConnect, unitId, userId, userName, userColor, realtimeToken]);

  const ydoc = ydocRef.current;
  const provider = providerRef.current;

  const extensions: AnyExtension[] =
    ydoc && provider
      ? [
          Collaboration.configure({
            document: ydoc,
            field: "default",
          }),
          CollaborationCursor.configure({
            provider,
          }),
        ]
      : [];

  return {
    provider,
    ydoc,
    extensions,
    connected,
    realtimeUnavailable,
    destroy: () => {
      providerRef.current?.destroy();
      ydocRef.current?.destroy();
      providerRef.current = null;
      ydocRef.current = null;
    },
  };
}

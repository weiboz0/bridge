"use client";

import { useState, useRef, useEffect } from "react";
import * as Y from "yjs";
import { HocuspocusProvider } from "@hocuspocus/provider";

const CANVAS_FAST_RECOVERY_MS = 20_000;
const CANVAS_FAST_DELAY_MAX_MS = 2_000;
const CANVAS_TAIL_DELAY_MAX_MS = 30_000;

export interface CanvasReconnectState {
  retry: boolean;
  terminal: boolean;
  manualReload: boolean;
  delayMs: number;
  fastRecoveryUntil: number;
  attempt: number;
  compatibility?: "legacy";
}

/**
 * Canvas lifecycle closes are classified separately from legacy Yjs documents.
 * It is pure so every close path (provider callbacks and UI recovery) shares
 * the exact resettable freeze horizon without changing attempt/session retry
 * behavior.
 */
export function canvasReconnectPolicy({
  now,
  event,
  previous,
  believedLive,
  documentName = "canvas:",
}: {
  now: number;
  event: { code?: string | number; reason?: string };
  previous?: CanvasReconnectState;
  believedLive: boolean;
  documentName?: string;
}): CanvasReconnectState {
  if (!documentName.startsWith("canvas:")) {
    return {
      retry: true,
      terminal: false,
      manualReload: false,
      delayMs: 0,
      fastRecoveryUntil: now,
      attempt: 0,
      compatibility: "legacy",
    };
  }
  const code = String(event.code ?? event.reason ?? "transport_closed");
  const terminal = !believedLive || code === "jwt_refresh_failed" || code === "session_ended" || code === "canvas_jwt_expired";
  if (terminal) {
    return { retry: false, terminal: true, manualReload: false, delayMs: 0, fastRecoveryUntil: now, attempt: 0 };
  }
  const freeze = code === "session_freezing";
  const fastRecoveryUntil = freeze
    ? now + CANVAS_FAST_RECOVERY_MS
    : previous?.fastRecoveryUntil && previous.fastRecoveryUntil > now
      ? previous.fastRecoveryUntil
      : previous
        ? previous.fastRecoveryUntil
        : now + CANVAS_FAST_RECOVERY_MS;
  const attempt = (previous?.attempt ?? 0) + 1;
  if (now < fastRecoveryUntil) {
    return { retry: true, terminal: false, manualReload: false, delayMs: Math.min(CANVAS_FAST_DELAY_MAX_MS, 250 * 2 ** Math.min(attempt - 1, 3)), fastRecoveryUntil, attempt };
  }
  // The deterministic component avoids a test-only/random branch while still
  // spreading callers by their independently changing retry attempt and time.
  const cap = Math.min(CANVAS_TAIL_DELAY_MAX_MS, CANVAS_FAST_DELAY_MAX_MS * 2 ** Math.min(attempt - 1, 4));
  const jitter = 0.5 + ((now + attempt * 1_103) % 500) / 1_000;
  return { retry: true, terminal: false, manualReload: false, delayMs: Math.max(CANVAS_FAST_DELAY_MAX_MS + 1, Math.floor(cap * jitter)), fastRecoveryUntil, attempt };
}

interface UseYjsProviderOptions {
  documentName: string;
  token: string;
  serverUrl?: string;
}

interface UseYjsProviderReturn {
  yDoc: Y.Doc | null;
  yText: Y.Text | null;
  provider: HocuspocusProvider | null;
  connected: boolean;
}

export function useYjsProvider({
  documentName,
  token,
  serverUrl = process.env.NEXT_PUBLIC_HOCUSPOCUS_URL
    || (typeof window !== "undefined" ? `ws://${window.location.hostname}:4000` : "ws://127.0.0.1:4000"),
}: UseYjsProviderOptions): UseYjsProviderReturn {
  const [connected, setConnected] = useState(false);
  const providerRef = useRef<HocuspocusProvider | null>(null);
  const yDocRef = useRef<Y.Doc | null>(null);
  const yTextRef = useRef<Y.Text | null>(null);
  const [, forceUpdate] = useState(0);
  const reconnectRef = useRef<CanvasReconnectState | undefined>(undefined);

  // Don't connect for placeholder/empty document names
  const shouldConnect = documentName && documentName !== "noop" && token;

  useEffect(() => {
    if (!shouldConnect) {
      yDocRef.current = null;
      yTextRef.current = null;
      providerRef.current = null;
      setConnected(false);
      forceUpdate((n) => n + 1);
      return;
    }

    const yDoc = new Y.Doc();
    const yText = yDoc.getText("content");

    const canvas = documentName.startsWith("canvas:");
    const updateReconnect = (event: { code?: string | number; reason?: string }) => {
      if (!canvas) return;
      const next = canvasReconnectPolicy({
        now: Date.now(),
        event,
        previous: reconnectRef.current,
        believedLive: true,
        documentName,
      });
      reconnectRef.current = next;
      // Hocuspocus 3.4.4 retains one shared websocket provider. Updating its
      // retry config after each close preserves that provider's lifecycle and
      // applies the policy before the next attempt.
      const websocket = provider.configuration.websocketProvider;
      websocket.setConfiguration({
        delay: next.delayMs,
        maxDelay: next.delayMs,
        factor: 1,
        jitter: false,
        maxAttempts: next.retry ? 0 : 1,
      });
    };
    const provider = new HocuspocusProvider({
      url: serverUrl,
      name: documentName,
      document: yDoc,
      token,
      ...(canvas ? { delay: 250, maxDelay: CANVAS_FAST_DELAY_MAX_MS, factor: 2, jitter: true, maxAttempts: 0 } : {}),
      onConnect: () => {
        console.log(`[yjs] Connected to ${documentName}`);
        setConnected(true);
      },
      onDisconnect: () => {
        console.log(`[yjs] Disconnected from ${documentName}`);
        setConnected(false);
        updateReconnect({ code: "transport_closed" });
      },
      onClose: ({ event }) => updateReconnect({ code: (event as { code?: number }).code, reason: (event as { reason?: string }).reason }),
      onAuthenticationFailed: (data) => {
        console.error(`[yjs] Auth failed for ${documentName}:`, data);
        updateReconnect({ code: "jwt_refresh_failed" });
      },
    });

    yDocRef.current = yDoc;
    yTextRef.current = yText;
    providerRef.current = provider;
    forceUpdate((n) => n + 1);

    return () => {
      provider.destroy();
      reconnectRef.current = undefined;
      yDoc.destroy();
      yDocRef.current = null;
      yTextRef.current = null;
      providerRef.current = null;
    };
  }, [shouldConnect, documentName, token, serverUrl]);

  return {
    yDoc: yDocRef.current,
    yText: yTextRef.current,
    provider: providerRef.current,
    connected,
  };
}

"use client";

import { useState, useRef, useEffect } from "react";
import * as Y from "yjs";
import { HocuspocusProvider } from "@hocuspocus/provider";
import { IncomingMessage, MessageType } from "@hocuspocus/server";

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
  const code = String(event.reason ?? event.code ?? "transport_closed");
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

/** Single-close bridge shared by provider callbacks so CLOSE does not advance twice. */
export function createCanvasProviderEventBridge({ documentName, now = () => Date.now() }: { documentName: string; now?: () => number }) {
  let current: CanvasReconnectState | undefined;
  let consumedClose = false;
  const advance = (event: { code?: string | number; reason?: string }) => {
    if (consumedClose) return current;
    consumedClose = true;
    current = canvasReconnectPolicy({ now: now(), event, previous: current, believedLive: true, documentName });
    return current;
  };
  return {
    onClose: advance,
    onDisconnect: advance,
    onConnect() { consumedClose = false; current = undefined; },
    state: () => current,
  };
}

/** Physical CLOSE recovery refreshes the token before reconnect; no write queue is created. */
export function bindInstalledCanvasProvider({ provider, refreshToken, reconnect, onRecovered, now = () => Date.now(), schedule = (callback: () => void, delayMs: number) => setTimeout(callback, delayMs), cancelSchedule = (timer: ReturnType<typeof setTimeout>) => clearTimeout(timer) }: {
  provider: HocuspocusProvider;
  refreshToken: () => Promise<string>;
  reconnect?: () => void;
  onRecovered?: () => void;
  now?: () => number;
  schedule?: (callback: () => void, delayMs: number) => ReturnType<typeof setTimeout>;
  cancelSchedule?: (timer: ReturnType<typeof setTimeout>) => void;
}) {
  const original = provider.onMessage.bind(provider);
  const originalSend = provider.send.bind(provider);
  const originalAuthenticationFailed = provider.configuration.onAuthenticationFailed;
  let stopped = false;
  let refreshing = false;
  let sendingToken = false;
  let recovery: CanvasReconnectState | undefined;
  let retryTimer: ReturnType<typeof setTimeout> | undefined;
  const cancelRetry = () => {
    if (retryTimer !== undefined) cancelSchedule(retryTimer);
    retryTimer = undefined;
  };
  provider.send = ((...args: Parameters<typeof provider.send>) => {
    // sendToken's concrete provider message is the sole emission allowed
    // while fenced.  Sync, awareness, and local Yjs updates remain blocked.
    if (!refreshing || sendingToken) return originalSend(...args);
  }) as typeof provider.send;
  const remintCanvasToken = async () => {
    if (stopped) return;
    try {
      const token = await refreshToken();
      if (stopped) return;
      provider.setConfiguration({ token });
      // getToken/sendToken is async in pinned provider. Keep the local-write
      // fence until its Auth frame has been emitted, then start sync.
      sendingToken = true;
      try {
        await provider.sendToken();
      } finally {
        sendingToken = false;
      }
      if (stopped) return;
      refreshing = false;
      cancelRetry();
      provider.startSync();
      onRecovered?.();
      reconnect?.();
    } catch (error: unknown) {
      if (!stopped && isRetryableFreeze(error)) {
        recovery = canvasReconnectPolicy({ now: now(), event: { reason: "session_freezing" }, previous: recovery, believedLive: true });
        cancelRetry();
        retryTimer = schedule(() => {
          retryTimer = undefined;
          if (!stopped) void remintCanvasToken();
        }, recovery.delayMs);
        return;
      }
      cancelRetry();
      originalAuthenticationFailed({ reason: "canvas_jwt_refresh_failed" });
    }
  };
  const startRecovery = () => {
    if (stopped || refreshing) return;
    refreshing = true;
    void remintCanvasToken();
  };
  provider.onMessage = ((event: MessageEvent) => {
    if (stopped) return original(event);
    try {
      const bytes = event.data instanceof ArrayBuffer
        ? new Uint8Array(event.data)
        : event.data instanceof Uint8Array
          ? event.data
          : undefined;
      if (!bytes) return original(event);
      const message = new IncomingMessage(bytes);
      message.readVarString();
      if (message.readVarUint() !== MessageType.CLOSE) return original(event);
      const reason = message.readVarString();
      if (reason !== "canvas_jwt_expired" && reason !== "session_freezing") return original(event);
      original(event);
      startRecovery();
    } catch { original(event); }
  }) as typeof provider.onMessage;
  const recoverPermissionDenied = ({ reason }: { reason?: string }) => {
    if (reason !== "session_freezing" || refreshing || stopped) {
      originalAuthenticationFailed({ reason: reason ?? "permission-denied" });
      return;
    }
    startRecovery();
  };
  // Pinned provider emits its PermissionDenied reason through this event;
  // replace only the configured terminal listener so retryable freezing can
  // share the CLOSE recovery path without declaring permanent auth failure.
  provider.off("authenticationFailed", originalAuthenticationFailed);
  provider.on("authenticationFailed", recoverPermissionDenied);
  return () => {
    stopped = true;
    cancelRetry();
    provider.onMessage = original;
    provider.send = originalSend;
    provider.off("authenticationFailed", recoverPermissionDenied);
    provider.on("authenticationFailed", originalAuthenticationFailed);
  };
}

function isRetryableFreeze(error: unknown): boolean {
  if (!error || typeof error !== "object") return false;
  const value = error as { status?: unknown; code?: unknown; reason?: unknown };
  return value.status === 409 && (value.code === "session_freezing" || value.reason === "session_freezing");
}

interface UseYjsProviderOptions {
  documentName: string;
  token: string;
  serverUrl?: string;
  refreshToken?: () => Promise<string>;
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
  refreshToken,
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
    const eventBridge = canvas ? createCanvasProviderEventBridge({ documentName }) : undefined;
    const updateReconnect = (event: { code?: string | number; reason?: string }) => {
      if (!canvas) return;
      const next = eventBridge?.onClose(event);
      if (!next) return;
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
        eventBridge?.onConnect();
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
    const releaseInstalledRecovery = canvas && refreshToken
      ? bindInstalledCanvasProvider({ provider, refreshToken, onRecovered: () => eventBridge?.onConnect() })
      : undefined;

    yDocRef.current = yDoc;
    yTextRef.current = yText;
    providerRef.current = provider;
    forceUpdate((n) => n + 1);

    return () => {
      provider.destroy();
      releaseInstalledRecovery?.();
      reconnectRef.current = undefined;
      yDoc.destroy();
      yDocRef.current = null;
      yTextRef.current = null;
      providerRef.current = null;
    };
  }, [shouldConnect, documentName, token, serverUrl, refreshToken]);

  return {
    yDoc: yDocRef.current,
    yText: yTextRef.current,
    provider: providerRef.current,
    connected,
  };
}

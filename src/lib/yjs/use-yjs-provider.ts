"use client";

import { useState, useRef, useEffect } from "react";
import * as Y from "yjs";
import { HocuspocusProvider } from "@hocuspocus/provider";

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

    const provider = new HocuspocusProvider({
      url: serverUrl,
      name: documentName,
      document: yDoc,
      token,
      onConnect: () => {
        console.log(`[yjs] Connected to ${documentName}`);
        setConnected(true);
      },
      onDisconnect: () => {
        console.log(`[yjs] Disconnected from ${documentName}`);
        setConnected(false);
      },
      onAuthenticationFailed: (data) => {
        console.error(`[yjs] Auth failed for ${documentName}:`, data);
      },
    });

    yDocRef.current = yDoc;
    yTextRef.current = yText;
    providerRef.current = provider;
    forceUpdate((n) => n + 1);

    return () => {
      provider.destroy();
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

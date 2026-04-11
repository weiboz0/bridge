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
  serverUrl = typeof window !== "undefined"
    ? `ws://${window.location.hostname}:4000`
    : "ws://127.0.0.1:4000",
}: UseYjsProviderOptions): UseYjsProviderReturn {
  const [connected, setConnected] = useState(false);
  const providerRef = useRef<HocuspocusProvider | null>(null);
  const yDocRef = useRef<Y.Doc | null>(null);
  const yTextRef = useRef<Y.Text | null>(null);
  const [, forceUpdate] = useState(0);

  useEffect(() => {
    const yDoc = new Y.Doc();
    const yText = yDoc.getText("content");

    const provider = new HocuspocusProvider({
      url: serverUrl,
      name: documentName,
      document: yDoc,
      token,
      onConnect: () => setConnected(true),
      onDisconnect: () => setConnected(false),
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
  }, [documentName, token, serverUrl]);

  return {
    yDoc: yDocRef.current,
    yText: yTextRef.current,
    provider: providerRef.current,
    connected,
  };
}

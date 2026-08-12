"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import * as Y from "yjs";
import { getRealtimeToken } from "@/lib/realtime/get-token";
import { useRealtimeToken } from "@/lib/realtime/use-realtime-token";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import {
  observeExcalidrawScene,
  readExcalidrawScene,
  writeExcalidrawScene,
  type ExcalidrawScene,
} from "./excalidraw-yjs";

const SCENE_MAP_NAME = "excalidraw-scene";

export interface UseWhiteboardOptions {
  canvasId: string | null;
  sessionId: string;
  /** Archive surfaces and viewers must never publish an Excalidraw update. */
  readOnly: boolean;
}

export interface UseWhiteboardResult {
  scene: ExcalidrawScene | null;
  connected: boolean;
  realtimeUnavailable: boolean;
  onChange: (scene: ExcalidrawScene) => void;
}

/**
 * Connects one canvas-scoped Yjs document to the custom Excalidraw binding.
 * The server JWT remains authoritative; `readOnly` additionally prevents the
 * client from creating local Yjs writes on archive and viewer surfaces.
 */
export function useWhiteboard({ canvasId, sessionId, readOnly }: UseWhiteboardOptions): UseWhiteboardResult {
  const documentName = canvasId ? `canvas:${canvasId}` : "noop";
  const { token, unavailable: realtimeUnavailable } = useRealtimeToken(documentName, canvasId ? sessionId : undefined);
  const refreshToken = useCallback(() => getRealtimeToken(documentName, canvasId ? sessionId : undefined, { forceRefresh: true }), [documentName, canvasId, sessionId]);
  const { yDoc, connected } = useYjsProvider({ documentName, token, refreshToken: canvasId ? refreshToken : undefined });
  const [scene, setScene] = useState<ExcalidrawScene | null>(null);
  const sceneMapRef = useRef<Y.Map<unknown> | null>(null);
  const localOriginRef = useRef(Symbol("whiteboard-local-change"));

  useEffect(() => {
    if (!yDoc) {
      sceneMapRef.current = null;
      setScene(null);
      return;
    }

    const sceneMap = yDoc.getMap<unknown>(SCENE_MAP_NAME);
    sceneMapRef.current = sceneMap;
    setScene(readExcalidrawScene(sceneMap));
    return observeExcalidrawScene(sceneMap, localOriginRef.current, setScene);
  }, [yDoc]);

  const onChange = useCallback((nextScene: ExcalidrawScene) => {
    // This is intentionally a client-side guard as well as viewModeEnabled.
    // A live owner opening the archive route has a writable server token, so
    // the archive must not enqueue a local update in the first place.
    if (readOnly || !sceneMapRef.current) return;
    writeExcalidrawScene(sceneMapRef.current, nextScene, localOriginRef.current);
  }, [readOnly]);

  return { scene, connected, realtimeUnavailable, onChange };
}

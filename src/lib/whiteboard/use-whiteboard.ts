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
const WRITE_DEBOUNCE_MS = 100;

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
  const pendingSceneRef = useRef<ExcalidrawScene | null>(null);
  const writeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

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

  // A pending write targets whichever Yjs doc/map was live when the timer
  // fires, not the map captured at schedule time — otherwise a doc swap
  // mid-debounce would silently drop the trailing write.
  useEffect(() => () => {
    if (writeTimerRef.current) clearTimeout(writeTimerRef.current);
  }, []);

  const onChange = useCallback((nextScene: ExcalidrawScene) => {
    // This is intentionally a client-side guard as well as viewModeEnabled.
    // A live owner opening the archive route has a writable server token, so
    // the archive must not enqueue a local update in the first place.
    if (readOnly) return;
    pendingSceneRef.current = nextScene;
    if (writeTimerRef.current) clearTimeout(writeTimerRef.current);
    writeTimerRef.current = setTimeout(() => {
      writeTimerRef.current = null;
      const scheduled = pendingSceneRef.current;
      pendingSceneRef.current = null;
      if (scheduled && sceneMapRef.current) {
        writeExcalidrawScene(sceneMapRef.current, scheduled, localOriginRef.current);
      }
    }, WRITE_DEBOUNCE_MS);
  }, [readOnly]);

  return { scene, connected, realtimeUnavailable, onChange };
}

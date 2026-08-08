import * as Y from "yjs";

const SCENE_KEY = "scene";

export interface ExcalidrawScene {
  elements: readonly unknown[];
  appState: Record<string, unknown>;
  /** Accepted from Excalidraw's onChange API but intentionally not stored. */
  files?: unknown;
}

function isScene(value: unknown): value is ExcalidrawScene {
  if (!value || typeof value !== "object") return false;
  const scene = value as { elements?: unknown; appState?: unknown };
  return Array.isArray(scene.elements)
    && Boolean(scene.appState)
    && typeof scene.appState === "object"
    && !Array.isArray(scene.appState);
}

/**
 * Writes only the serializable Excalidraw scene. Binary files deliberately do
 * not belong in the collaborative document: the whiteboard MVP has no upload
 * path, and persisting blobs in a Yjs snapshot would make each update huge.
 */
export function writeExcalidrawScene(
  sceneMap: Y.Map<unknown>,
  scene: ExcalidrawScene,
  origin?: unknown,
): boolean {
  let serialized: string;
  try {
    serialized = JSON.stringify({
      elements: scene.elements,
      appState: scene.appState,
    });
  } catch {
    return false;
  }

  const write = () => sceneMap.set(SCENE_KEY, serialized);
  if (sceneMap.doc) {
    sceneMap.doc.transact(write, origin);
  } else {
    write();
  }
  return true;
}

/** Returns null for absent or invalid persisted state instead of throwing. */
export function readExcalidrawScene(sceneMap: Y.Map<unknown>): ExcalidrawScene | null {
  const serialized = sceneMap.get(SCENE_KEY);
  if (typeof serialized !== "string") return null;

  try {
    const parsed: unknown = JSON.parse(serialized);
    if (!isScene(parsed)) return null;
    return parsed;
  } catch {
    return null;
  }
}

/**
 * Calls the subscriber for remote changes only. The caller supplies its own
 * transaction origin so applying a remote scene to Excalidraw cannot loop
 * back through that editor's onChange handler.
 */
export function observeExcalidrawScene(
  sceneMap: Y.Map<unknown>,
  ownOrigin: unknown,
  callback: (scene: ExcalidrawScene | null) => void,
): () => void {
  const observer = (event: Y.YMapEvent<unknown>, transaction: Y.Transaction) => {
    if (transaction.origin === ownOrigin || !event.keysChanged.has(SCENE_KEY)) return;
    callback(readExcalidrawScene(sceneMap));
  };
  sceneMap.observe(observer);
  return () => sceneMap.unobserve(observer);
}

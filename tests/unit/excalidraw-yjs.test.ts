import { describe, expect, it } from "vitest";
import * as Y from "yjs";
import {
  observeExcalidrawScene,
  readExcalidrawScene,
  writeExcalidrawScene,
} from "../../src/lib/whiteboard/excalidraw-yjs";

const firstScene = {
  elements: [{ id: "rectangle-1", type: "rectangle", x: 10, y: 20 }],
  appState: { viewBackgroundColor: "#ffffff", scrollX: 0 },
};

const secondScene = {
  elements: [{ id: "ellipse-1", type: "ellipse", x: 30, y: 40 }],
  appState: { viewBackgroundColor: "#111111", scrollX: 12 },
};

function createSceneMap(): { doc: Y.Doc; sceneMap: Y.Map<unknown> } {
  const doc = new Y.Doc();
  return { doc, sceneMap: doc.getMap<unknown>("scene") };
}

describe("Excalidraw Yjs scene binding", () => {
  it("round-trips JSON-safe elements and appState through the Y.Map", () => {
    const { sceneMap } = createSceneMap();

    writeExcalidrawScene(sceneMap, firstScene);

    expect(readExcalidrawScene(sceneMap)).toEqual(firstScene);
  });

  it("does not persist Excalidraw files or blob payloads", () => {
    const { sceneMap } = createSceneMap();

    writeExcalidrawScene(sceneMap, {
      ...firstScene,
      files: {
        "image-file-1": {
          id: "image-file-1",
          dataURL: "data:image/png;base64,student-upload-bytes",
          mimeType: "image/png",
        },
      },
    });

    const stored = sceneMap.get("scene");
    expect(typeof stored).toBe("string");
    expect(JSON.parse(stored as string)).toEqual(firstScene);
    expect(stored).not.toContain("student-upload-bytes");
  });

  it.each([
    ["missing scene", undefined],
    ["invalid JSON", "{"],
    ["missing elements", JSON.stringify({ appState: {} })],
    ["non-array elements", JSON.stringify({ elements: {}, appState: {} })],
    ["missing appState", JSON.stringify({ elements: [] })],
    ["non-object appState", JSON.stringify({ elements: [], appState: [] })],
  ])("fails closed to null for %s", (_label, persistedScene) => {
    const { sceneMap } = createSceneMap();
    if (persistedScene !== undefined) sceneMap.set("scene", persistedScene);

    expect(() => readExcalidrawScene(sceneMap)).not.toThrow();
    expect(readExcalidrawScene(sceneMap)).toBeNull();
  });

  it("forwards remote scene changes while suppressing changes from its own origin", () => {
    const { doc, sceneMap } = createSceneMap();
    const ownOrigin = Symbol("local-excalidraw-on-change");
    const received: unknown[] = [];
    const stopObserving = observeExcalidrawScene(sceneMap, ownOrigin, (scene) => {
      received.push(scene);
    });

    writeExcalidrawScene(sceneMap, firstScene, ownOrigin);
    expect(received).toEqual([]);

    const remoteDoc = new Y.Doc();
    Y.applyUpdate(remoteDoc, Y.encodeStateAsUpdate(doc));
    const remoteSceneMap = remoteDoc.getMap<unknown>("scene");
    writeExcalidrawScene(remoteSceneMap, secondScene, Symbol("remote-editor"));
    Y.applyUpdate(doc, Y.encodeStateAsUpdate(remoteDoc, Y.encodeStateVector(doc)), "remote-sync");

    expect(received).toEqual([secondScene]);
    stopObserving();
  });
});

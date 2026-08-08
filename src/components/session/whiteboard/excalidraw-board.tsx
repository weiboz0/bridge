"use client";

import { useEffect, useRef, type ComponentProps } from "react";
import { Excalidraw } from "@excalidraw/excalidraw";
import "@excalidraw/excalidraw/index.css";
import type { ExcalidrawScene } from "@/lib/whiteboard/excalidraw-yjs";

interface ExcalidrawBoardProps {
  scene: ExcalidrawScene | null;
  readOnly: boolean;
  onChange: (scene: ExcalidrawScene) => void;
}

type ExcalidrawProps = ComponentProps<typeof Excalidraw>;
type ExcalidrawApi = Parameters<NonNullable<ExcalidrawProps["excalidrawAPI"]>>[0];

/** The dynamically loaded visual surface; synchronization stays in useWhiteboard. */
export function ExcalidrawBoard({ scene, readOnly, onChange }: ExcalidrawBoardProps) {
  const apiRef = useRef<ExcalidrawApi | null>(null);

  useEffect(() => {
    if (!scene || !apiRef.current) return;
    apiRef.current.updateScene({
      elements: scene.elements as never,
      appState: scene.appState as never,
    });
  }, [scene]);

  return (
    <div className="h-full min-h-[28rem]" data-testid="excalidraw-board">
      <Excalidraw
        excalidrawAPI={(api) => { apiRef.current = api; }}
        viewModeEnabled={readOnly}
        onChange={(elements, appState) => {
          onChange({
            elements,
            appState: appState as unknown as Record<string, unknown>,
          });
        }}
      />
    </div>
  );
}

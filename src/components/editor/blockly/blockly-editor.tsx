"use client";

import { useRef, useEffect, useCallback, useState } from "react";
import { k5Toolbox } from "./toolbox";

interface BlocklyEditorProps {
  initialState?: string; // JSON serialized workspace
  onChange?: (state: string, generatedCode: string) => void;
}

export function BlocklyEditor({ initialState, onChange }: BlocklyEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const workspaceRef = useRef<any>(null);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    if (!containerRef.current) return;

    // Dynamic import to avoid SSR issues
    import("blockly").then((Blockly) => {
      import("blockly/javascript").then((jsGenerator) => {
        if (!containerRef.current) return;

        const workspace = Blockly.inject(containerRef.current, {
          toolbox: k5Toolbox,
          grid: { spacing: 20, length: 3, colour: "#ccc", snap: true },
          zoom: { controls: true, wheel: true, startScale: 1.0, maxScale: 3, minScale: 0.3 },
          trashcan: true,
        });

        // Restore state if provided
        if (initialState) {
          try {
            const state = JSON.parse(initialState);
            Blockly.serialization.workspaces.load(state, workspace);
          } catch {
            // Invalid state — start fresh
          }
        }

        // Listen for changes (skip UI events like scroll/drag)
        workspace.addChangeListener((event: any) => {
          if (!onChange) return;
          if (event.isUiEvent) return;
          const state = Blockly.serialization.workspaces.save(workspace);
          const code = jsGenerator.javascriptGenerator.workspaceToCode(workspace);
          onChange(JSON.stringify(state), code);
        });

        workspaceRef.current = workspace;
        setLoaded(true);
      });
    });

    return () => {
      workspaceRef.current?.dispose();
      workspaceRef.current = null;
    };
  }, []);

  return (
    <div className="relative h-full">
      {!loaded && (
        <div className="absolute inset-0 flex items-center justify-center bg-background z-10">
          <p className="text-sm text-muted-foreground">Loading Blockly...</p>
        </div>
      )}
      <div ref={containerRef} className="h-full w-full" />
    </div>
  );
}

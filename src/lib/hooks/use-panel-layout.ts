"use client";

import { useState, useCallback, useEffect } from "react";

interface PanelLayout {
  leftVisible: boolean;
  rightVisible: boolean;
  leftWidth: number; // percentage
  rightWidth: number; // percentage
}

const DEFAULT_LAYOUT: PanelLayout = {
  leftVisible: true,
  rightVisible: true,
  leftWidth: 20,
  rightWidth: 25,
};

const STORAGE_KEY = "bridge-teacher-panel-layout";

export function usePanelLayout() {
  const [layout, setLayoutState] = useState<PanelLayout>(DEFAULT_LAYOUT);

  useEffect(() => {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) {
      try {
        setLayoutState(JSON.parse(stored));
      } catch {}
    }
  }, []);

  const setLayout = useCallback((updates: Partial<PanelLayout>) => {
    setLayoutState((prev) => {
      const next = { ...prev, ...updates };
      localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
      return next;
    });
  }, []);

  const toggleLeft = useCallback(() => {
    setLayout({ leftVisible: !layout.leftVisible });
  }, [layout.leftVisible, setLayout]);

  const toggleRight = useCallback(() => {
    setLayout({ rightVisible: !layout.rightVisible });
  }, [layout.rightVisible, setLayout]);

  return { layout, setLayout, toggleLeft, toggleRight };
}

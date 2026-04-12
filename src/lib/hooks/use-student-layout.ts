"use client";

import { useState, useCallback, useEffect } from "react";

export type StudentLayoutMode = "side-by-side" | "stacked";

const STORAGE_KEY = "bridge-student-layout";

export function useStudentLayout() {
  const [mode, setModeState] = useState<StudentLayoutMode>("side-by-side");

  useEffect(() => {
    const stored = localStorage.getItem(STORAGE_KEY) as StudentLayoutMode | null;
    if (stored === "side-by-side" || stored === "stacked") {
      setModeState(stored);
    } else {
      // Default based on screen width
      if (window.innerWidth < 768) {
        setModeState("stacked");
      }
    }
  }, []);

  const setMode = useCallback((newMode: StudentLayoutMode) => {
    setModeState(newMode);
    localStorage.setItem(STORAGE_KEY, newMode);
  }, []);

  const toggle = useCallback(() => {
    setMode(mode === "side-by-side" ? "stacked" : "side-by-side");
  }, [mode, setMode]);

  return { mode, setMode, toggle };
}

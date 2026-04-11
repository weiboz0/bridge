"use client";

import { useState, useEffect, useCallback } from "react";

export type Theme = "light" | "dark";

export function useTheme() {
  const [theme, setThemeState] = useState<Theme>("dark");

  useEffect(() => {
    const stored = localStorage.getItem("bridge-theme") as Theme | null;
    const current = stored || (document.documentElement.classList.contains("dark") ? "dark" : "light");
    setThemeState(current);
  }, []);

  const setTheme = useCallback((newTheme: Theme) => {
    setThemeState(newTheme);
    localStorage.setItem("bridge-theme", newTheme);
    if (newTheme === "dark") {
      document.documentElement.classList.add("dark");
    } else {
      document.documentElement.classList.remove("dark");
    }
  }, []);

  const toggleTheme = useCallback(() => {
    // Read current state from DOM to avoid stale closure
    const current = document.documentElement.classList.contains("dark") ? "dark" : "light";
    setTheme(current === "dark" ? "light" : "dark");
  }, [setTheme]);

  return { theme, setTheme, toggleTheme };
}

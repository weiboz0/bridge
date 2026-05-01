"use client";

import { useState, useEffect, useCallback } from "react";

export type Theme = "light" | "dark";

const COOKIE_NAME = "bridge-theme";
const COOKIE_MAX_AGE = 60 * 60 * 24 * 365; // 1 year

function writeThemeCookie(theme: Theme) {
  document.cookie = `${COOKIE_NAME}=${theme}; path=/; max-age=${COOKIE_MAX_AGE}; samesite=lax`;
}

export function useTheme() {
  const [theme, setThemeState] = useState<Theme>("light");

  useEffect(() => {
    // The server already set the `dark` class via cookie; sync local
    // state from the DOM rather than a fresh localStorage read so
    // we agree with what the user is actually seeing.
    const current = document.documentElement.classList.contains("dark") ? "dark" : "light";
    setThemeState(current);
    // Backwards compat: if a legacy localStorage entry exists but no
    // cookie, mirror it so subsequent SSR renders pick the right
    // theme on first paint.
    const stored = localStorage.getItem(COOKIE_NAME) as Theme | null;
    if (stored && !document.cookie.includes(`${COOKIE_NAME}=`)) {
      writeThemeCookie(stored);
    }
  }, []);

  const setTheme = useCallback((newTheme: Theme) => {
    setThemeState(newTheme);
    localStorage.setItem(COOKIE_NAME, newTheme);
    writeThemeCookie(newTheme);
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

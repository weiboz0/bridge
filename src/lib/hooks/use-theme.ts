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
    const cookieAlreadySet = document.cookie.includes(`${COOKIE_NAME}=`);
    const stored = localStorage.getItem(COOKIE_NAME) as Theme | null;
    // Backwards compat for users with a legacy localStorage entry
    // and no cookie yet: mirror localStorage to the cookie AND apply
    // it to the current page (DOM + state) so they don't see a
    // one-time light flash on first migration. Without this branch,
    // the SSR pass — which had no cookie — already rendered light;
    // we'd only fix the *next* visit.
    if (!cookieAlreadySet && stored === "dark") {
      writeThemeCookie(stored);
      document.documentElement.classList.add("dark");
      setThemeState("dark");
      return;
    }
    if (!cookieAlreadySet && stored === "light") {
      writeThemeCookie(stored);
    }
    // Otherwise sync local state from the DOM (the server-rendered
    // class is the source of truth for what the user is actually
    // seeing).
    const current = document.documentElement.classList.contains("dark") ? "dark" : "light";
    setThemeState(current);
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

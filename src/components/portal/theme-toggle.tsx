"use client";

import { useTheme } from "@/lib/hooks/use-theme";
import { Button } from "@/components/ui/button";

interface ThemeToggleProps {
  collapsed?: boolean;
}

export function ThemeToggle({ collapsed }: ThemeToggleProps) {
  const { theme, toggleTheme } = useTheme();

  return (
    <Button variant="ghost" size="sm" onClick={toggleTheme} title={`Switch to ${theme === "dark" ? "light" : "dark"} mode`}>
      {theme === "dark" ? "☀️" : "🌙"}
      {!collapsed && <span className="ml-2">{theme === "dark" ? "Light" : "Dark"}</span>}
    </Button>
  );
}

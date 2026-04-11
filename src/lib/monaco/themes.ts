import type * as monacoTypes from "monaco-editor";

// Light theme — derived from the platform's :root CSS variables
// Background: oklch(1 0 0) → #ffffff
// Foreground: oklch(0.145 0 0) → #1a1a1a (approx)
// Muted foreground: oklch(0.556 0 0) → #7a7a7a (approx)
// Border: oklch(0.922 0 0) → #e5e5e5 (approx)
// Secondary: oklch(0.97 0 0) → #f5f5f5 (approx)
export const bridgeLightTheme: monacoTypes.editor.IStandaloneThemeData = {
  base: "vs",
  inherit: true,
  rules: [
    { token: "comment", foreground: "6a737d", fontStyle: "italic" },
    { token: "keyword", foreground: "d73a49" },
    { token: "string", foreground: "032f62" },
    { token: "number", foreground: "005cc5" },
    { token: "type", foreground: "6f42c1" },
    { token: "function", foreground: "6f42c1" },
    { token: "variable", foreground: "e36209" },
    { token: "operator", foreground: "d73a49" },
    { token: "delimiter", foreground: "1a1a1a" },
  ],
  colors: {
    "editor.background": "#ffffff",
    "editor.foreground": "#1a1a1a",
    "editor.lineHighlightBackground": "#f5f5f5",
    "editorLineNumber.foreground": "#7a7a7a",
    "editorLineNumber.activeForeground": "#1a1a1a",
    "editor.selectionBackground": "#add6ff80",
    "editor.inactiveSelectionBackground": "#add6ff40",
    "editorCursor.foreground": "#1a1a1a",
    "editorWhitespace.foreground": "#d4d4d4",
    "editorIndentGuide.background": "#e5e5e5",
    "editorBracketMatch.background": "#add6ff40",
    "editorBracketMatch.border": "#add6ff",
    "minimap.background": "#f5f5f5",
    "editorGutter.background": "#ffffff",
  },
};

// Dark theme — derived from the platform's .dark CSS variables
// Background: oklch(0.145 0 0) → #1a1a1a (approx)
// Foreground: oklch(0.985 0 0) → #fafafa (approx)
// Card/popover: oklch(0.205 0 0) → #2d2d2d (approx)
// Muted foreground: oklch(0.708 0 0) → #a3a3a3 (approx)
// Border: oklch(1 0 0 / 10%) → rgba(255,255,255,0.1)
export const bridgeDarkTheme: monacoTypes.editor.IStandaloneThemeData = {
  base: "vs-dark",
  inherit: true,
  rules: [
    { token: "comment", foreground: "6a737d", fontStyle: "italic" },
    { token: "keyword", foreground: "f97583" },
    { token: "string", foreground: "9ecbff" },
    { token: "number", foreground: "79b8ff" },
    { token: "type", foreground: "b392f0" },
    { token: "function", foreground: "b392f0" },
    { token: "variable", foreground: "ffab70" },
    { token: "operator", foreground: "f97583" },
    { token: "delimiter", foreground: "fafafa" },
  ],
  colors: {
    "editor.background": "#1a1a1a",
    "editor.foreground": "#fafafa",
    "editor.lineHighlightBackground": "#2d2d2d",
    "editorLineNumber.foreground": "#a3a3a3",
    "editorLineNumber.activeForeground": "#fafafa",
    "editor.selectionBackground": "#3a3d41",
    "editor.inactiveSelectionBackground": "#3a3d4180",
    "editorCursor.foreground": "#fafafa",
    "editorWhitespace.foreground": "#404040",
    "editorIndentGuide.background": "#404040",
    "editorBracketMatch.background": "#3a3d4180",
    "editorBracketMatch.border": "#888888",
    "minimap.background": "#1a1a1a",
    "editorGutter.background": "#1a1a1a",
  },
};

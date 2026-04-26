"use client"

import { useCallback, useEffect, useState } from "react"

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const STORAGE_KEY = "bridge:editor-help-seen"

// ---------------------------------------------------------------------------
// Tooltip callout data
// ---------------------------------------------------------------------------

interface HelpCallout {
  title: string
  description: string
  /** CSS position relative to the overlay (percentage-based for responsiveness). */
  top: string
  left: string
  /** Arrow pointing direction. */
  arrow: "up" | "down" | "left" | "right" | "none"
}

const CALLOUTS: HelpCallout[] = [
  {
    title: "Format text with the toolbar",
    description: "Bold, italic, headings, lists, colors, and more.",
    top: "6%",
    left: "30%",
    arrow: "up",
  },
  {
    title: "Type / to open the command menu",
    description: "Insert blocks, media, code, tables, and AI-generated content.",
    top: "35%",
    left: "25%",
    arrow: "left",
  },
  {
    title: "Drag blocks to reorder",
    description: "Hover the left margin to see drag handles for each block.",
    top: "50%",
    left: "5%",
    arrow: "right",
  },
  {
    title: "Select text and use AI to rewrite",
    description: "Highlight text, then use the bubble toolbar AI actions.",
    top: "45%",
    left: "55%",
    arrow: "none",
  },
  {
    title: "Save with Cmd+Enter",
    description: "Or click the Save button in the toolbar.",
    top: "6%",
    left: "75%",
    arrow: "up",
  },
]

// ---------------------------------------------------------------------------
// Arrow SVGs
// ---------------------------------------------------------------------------

function Arrow({ direction }: { direction: HelpCallout["arrow"] }) {
  if (direction === "none") return null

  const rotations: Record<string, string> = {
    up: "rotate(0)",
    down: "rotate(180deg)",
    left: "rotate(-90deg)",
    right: "rotate(90deg)",
  }

  return (
    <div
      className="absolute"
      style={{
        ...(direction === "up" ? { top: "-8px", left: "50%", transform: "translateX(-50%)" } : {}),
        ...(direction === "down" ? { bottom: "-8px", left: "50%", transform: "translateX(-50%)" } : {}),
        ...(direction === "left" ? { left: "-8px", top: "50%", transform: "translateY(-50%)" } : {}),
        ...(direction === "right" ? { right: "-8px", top: "50%", transform: "translateY(-50%)" } : {}),
      }}
    >
      <svg
        width="16"
        height="8"
        viewBox="0 0 16 8"
        fill="none"
        style={{ transform: rotations[direction] }}
      >
        <path d="M8 0L16 8H0L8 0Z" fill="white" />
      </svg>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Help overlay component
// ---------------------------------------------------------------------------

interface HelpOverlayProps {
  /** If true, force show even if the user already dismissed it. */
  forceShow?: boolean
  /** Callback when the overlay is dismissed. */
  onDismiss?: () => void
}

export function HelpOverlay({ forceShow, onDismiss }: HelpOverlayProps) {
  const [visible, setVisible] = useState(false)

  useEffect(() => {
    if (forceShow) {
      setVisible(true)
      return
    }
    // Show on first visit only.
    try {
      const seen = localStorage.getItem(STORAGE_KEY)
      if (!seen) {
        setVisible(true)
      }
    } catch {
      // localStorage unavailable — skip.
    }
  }, [forceShow])

  const dismiss = useCallback(() => {
    setVisible(false)
    try {
      localStorage.setItem(STORAGE_KEY, "1")
    } catch {
      // Ignore.
    }
    onDismiss?.()
  }, [onDismiss])

  if (!visible) return null

  return (
    <div
      className="fixed inset-0 z-[200] flex items-center justify-center bg-black/40 backdrop-blur-[2px]"
      onClick={(e) => {
        // Dismiss on background click.
        if (e.target === e.currentTarget) dismiss()
      }}
    >
      {/* Callout cards */}
      {CALLOUTS.map((callout, i) => (
        <div
          key={i}
          className="absolute max-w-[220px] rounded-lg bg-white p-3 shadow-xl"
          style={{ top: callout.top, left: callout.left }}
        >
          <Arrow direction={callout.arrow} />
          <h4 className="text-sm font-semibold text-zinc-900">{callout.title}</h4>
          <p className="mt-0.5 text-xs text-zinc-500">{callout.description}</p>
        </div>
      ))}

      {/* "Got it" dismiss button */}
      <div className="absolute bottom-[12%] left-1/2 -translate-x-1/2">
        <button
          type="button"
          onClick={dismiss}
          className={
            "rounded-lg bg-zinc-900 px-6 py-2.5 text-sm font-semibold text-white shadow-lg " +
            "transition-colors hover:bg-zinc-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/50"
          }
        >
          Got it
        </button>
      </div>
    </div>
  )
}

/**
 * Check whether the help overlay should auto-show (first visit).
 * Returns true if the user has NOT yet dismissed it.
 */
export function shouldShowHelp(): boolean {
  try {
    return !localStorage.getItem(STORAGE_KEY)
  } catch {
    return false
  }
}

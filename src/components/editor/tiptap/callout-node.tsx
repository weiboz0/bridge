"use client"

import { useState } from "react"
import { nanoid } from "nanoid"
import { Node, mergeAttributes } from "@tiptap/core"
import {
  NodeViewWrapper,
  NodeViewContent,
  ReactNodeViewRenderer,
  type NodeViewProps,
} from "@tiptap/react"

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type CalloutVariant = "info" | "warning" | "tip" | "danger"

type CalloutNodeAttrs = {
  id: string
  variant: CalloutVariant
}

// ---------------------------------------------------------------------------
// Variant config
// ---------------------------------------------------------------------------

const VARIANTS: Record<
  CalloutVariant,
  { label: string; icon: string; borderColor: string; bgColor: string; textColor: string }
> = {
  info: {
    label: "Info",
    icon: "i",
    borderColor: "border-blue-400",
    bgColor: "bg-blue-50",
    textColor: "text-blue-700",
  },
  warning: {
    label: "Warning",
    icon: "!",
    borderColor: "border-yellow-400",
    bgColor: "bg-yellow-50",
    textColor: "text-yellow-700",
  },
  tip: {
    label: "Tip",
    icon: "★",
    borderColor: "border-green-400",
    bgColor: "bg-green-50",
    textColor: "text-green-700",
  },
  danger: {
    label: "Danger",
    icon: "✕",
    borderColor: "border-red-400",
    bgColor: "bg-red-50",
    textColor: "text-red-700",
  },
}

const VARIANT_ORDER: CalloutVariant[] = ["info", "warning", "tip", "danger"]

function nextVariant(current: CalloutVariant): CalloutVariant {
  const idx = VARIANT_ORDER.indexOf(current)
  return VARIANT_ORDER[(idx + 1) % VARIANT_ORDER.length]
}

// ---------------------------------------------------------------------------
// Node view
// ---------------------------------------------------------------------------

function CalloutNodeView({ node, updateAttributes }: NodeViewProps) {
  const { variant } = node.attrs as CalloutNodeAttrs
  const config = VARIANTS[variant] ?? VARIANTS.info
  const [hovered, setHovered] = useState(false)

  return (
    <NodeViewWrapper className="callout-node my-3">
      <div
        className={`rounded-r-md border-l-4 px-4 py-3 ${config.borderColor} ${config.bgColor}`}
      >
        <div className="flex items-start gap-3">
          {/* Clickable icon cycles through variants */}
          <button
            type="button"
            contentEditable={false}
            title={`Callout type: ${config.label} (click to change)`}
            aria-label={`Callout type: ${config.label}. Click to cycle variant.`}
            onMouseEnter={() => setHovered(true)}
            onMouseLeave={() => setHovered(false)}
            onClick={() => updateAttributes({ variant: nextVariant(variant) })}
            className={`
              mt-0.5 flex h-5 w-5 shrink-0 select-none items-center justify-center
              rounded-full border text-[10px] font-bold
              transition-opacity cursor-pointer
              ${config.borderColor} ${config.textColor}
              ${hovered ? "opacity-70" : "opacity-100"}
            `}
          >
            {config.icon}
          </button>
          <NodeViewContent
            className={`flex-1 text-sm [&>*:first-child]:mt-0 [&>*:last-child]:mb-0 ${config.textColor}`}
          />
        </div>
      </div>
    </NodeViewWrapper>
  )
}

// ---------------------------------------------------------------------------
// Tiptap node definition
// ---------------------------------------------------------------------------

export const CalloutNode = Node.create({
  name: "callout",
  group: "block",
  content: "block+",

  addAttributes() {
    return {
      id: {
        default: () => nanoid(),
        parseHTML: (element: Element) => element.getAttribute("data-callout-id") ?? nanoid(),
      },
      variant: {
        default: "info" as CalloutVariant,
        parseHTML: (element: Element) => {
          const v = element.getAttribute("data-callout-variant")
          if (v === "info" || v === "warning" || v === "tip" || v === "danger") return v
          return "info"
        },
      },
    }
  },

  parseHTML() {
    return [{ tag: 'div[data-type="callout"]' }]
  },

  renderHTML({ node, HTMLAttributes }: any) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-type": "callout",
        "data-callout-id": node.attrs.id,
        "data-callout-variant": node.attrs.variant,
      }),
      0, // content hole
    ]
  },

  addNodeView() {
    return ReactNodeViewRenderer(CalloutNodeView)
  },
})

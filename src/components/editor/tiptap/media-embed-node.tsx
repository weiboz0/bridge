"use client"

import { useCallback, useState } from "react"
import { nanoid } from "nanoid"
import { Node, mergeAttributes } from "@tiptap/core"
import {
  NodeViewWrapper,
  ReactNodeViewRenderer,
  type NodeViewProps,
} from "@tiptap/react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"

type MediaType = "image" | "video" | "pdf" | "link"

type MediaEmbedNodeAttrs = {
  id: string
  url: string
  alt: string
  mediaType: MediaType
}

const MEDIA_TYPES: { value: MediaType; label: string }[] = [
  { value: "image", label: "Image" },
  { value: "video", label: "Video" },
  { value: "pdf", label: "PDF" },
  { value: "link", label: "Link" },
]

function MediaPreview({ url, alt, mediaType }: { url: string; alt: string; mediaType: MediaType }) {
  if (!url) {
    return (
      <div className="flex h-32 items-center justify-center rounded-md border border-dashed border-zinc-300 bg-zinc-50 text-sm text-zinc-400">
        No URL set — click Edit to add a media URL.
      </div>
    )
  }

  switch (mediaType) {
    case "image":
      return (
        <img
          src={url}
          alt={alt || "Embedded image"}
          className="max-h-96 w-full rounded-md object-contain border border-zinc-200 bg-zinc-50"
        />
      )
    case "video":
      return (
        <video
          src={url}
          controls
          className="w-full rounded-md border border-zinc-200 bg-zinc-900"
          aria-label={alt || "Embedded video"}
        >
          Your browser does not support the video element.
        </video>
      )
    case "pdf":
      return (
        <iframe
          src={url}
          title={alt || "Embedded PDF"}
          className="h-96 w-full rounded-md border border-zinc-200"
        />
      )
    case "link":
    default:
      return (
        <a
          href={url}
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center gap-3 rounded-md border border-zinc-200 bg-zinc-50 p-3 hover:bg-zinc-100"
        >
          <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded bg-zinc-200">
            <svg
              xmlns="http://www.w3.org/2000/svg"
              className="h-4 w-4 text-zinc-500"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1"
              />
            </svg>
          </div>
          <div className="min-w-0">
            <p className="text-sm font-medium text-zinc-900 truncate">{alt || url}</p>
            <p className="text-xs text-zinc-500 truncate">{url}</p>
          </div>
        </a>
      )
  }
}

function MediaEmbedNodeView({ node, updateAttributes }: NodeViewProps) {
  const { url, alt, mediaType } = node.attrs as MediaEmbedNodeAttrs
  const [editing, setEditing] = useState(false)
  const [draftUrl, setDraftUrl] = useState(url)
  const [draftAlt, setDraftAlt] = useState(alt)
  const [draftType, setDraftType] = useState<MediaType>(mediaType)

  const openEditor = useCallback(() => {
    setDraftUrl(url)
    setDraftAlt(alt)
    setDraftType(mediaType)
    setEditing(true)
  }, [url, alt, mediaType])

  const handleSave = useCallback(() => {
    updateAttributes({ url: draftUrl.trim(), alt: draftAlt.trim(), mediaType: draftType })
    setEditing(false)
  }, [draftUrl, draftAlt, draftType, updateAttributes])

  const handleCancel = useCallback(() => {
    setEditing(false)
  }, [])

  return (
    <NodeViewWrapper className="media-embed-node my-3" contentEditable={false}>
      <div className="space-y-2">
        {/* Label + edit control */}
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium uppercase tracking-wide text-zinc-400">
            {mediaType}
          </span>
          <Button
            variant="outline"
            size="sm"
            className="h-6 px-2 text-xs"
            onClick={openEditor}
          >
            Edit
          </Button>
        </div>

        {/* Media preview */}
        <MediaPreview url={url} alt={alt} mediaType={mediaType} />

        {/* Inline editor */}
        {editing && (
          <div className="rounded-md border border-zinc-200 bg-zinc-50 p-3 space-y-3">
            <div className="space-y-1">
              <label className="text-xs font-medium text-zinc-600">Type</label>
              <select
                value={draftType}
                onChange={(e) => setDraftType(e.target.value as MediaType)}
                className="w-full rounded border border-zinc-300 bg-white px-2 py-1.5 text-sm text-zinc-900 focus:outline-none focus:ring-1 focus:ring-zinc-400"
              >
                {MEDIA_TYPES.map(({ value, label }) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </select>
            </div>
            <div className="space-y-1">
              <label className="text-xs font-medium text-zinc-600">URL</label>
              <Input
                type="url"
                value={draftUrl}
                onChange={(e) => setDraftUrl(e.target.value)}
                placeholder="https://..."
                className="text-sm"
              />
            </div>
            <div className="space-y-1">
              <label className="text-xs font-medium text-zinc-600">Alt text / label</label>
              <Input
                type="text"
                value={draftAlt}
                onChange={(e) => setDraftAlt(e.target.value)}
                placeholder="Describe this media..."
                className="text-sm"
              />
            </div>
            <div className="flex gap-2">
              <Button size="sm" className="h-7 text-xs" onClick={handleSave}>
                Save
              </Button>
              <Button
                variant="outline"
                size="sm"
                className="h-7 text-xs"
                onClick={handleCancel}
              >
                Cancel
              </Button>
            </div>
          </div>
        )}
      </div>
    </NodeViewWrapper>
  )
}

export const MediaEmbedNode = Node.create({
  name: "media-embed",
  group: "block",
  atom: true,
  addAttributes() {
    return {
      id: {
        default: () => nanoid(),
      },
      url: {
        default: "",
      },
      alt: {
        default: "",
      },
      mediaType: {
        default: "image" as MediaType,
        parseHTML: (element: Element) => {
          const t = element.getAttribute("data-media-embed-type")
          if (t === "image" || t === "video" || t === "pdf" || t === "link") return t
          return "image"
        },
      },
    }
  },
  parseHTML() {
    return [
      {
        tag: 'div[data-type="media-embed"]',
      },
    ]
  },
  renderHTML({ node, HTMLAttributes }: any) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-type": "media-embed",
        "data-media-embed-id": node.attrs.id,
        "data-media-embed-url": node.attrs.url,
        "data-media-embed-alt": node.attrs.alt,
        "data-media-embed-type": node.attrs.mediaType,
      }),
    ]
  },
  addNodeView() {
    return ReactNodeViewRenderer(MediaEmbedNodeView)
  },
})

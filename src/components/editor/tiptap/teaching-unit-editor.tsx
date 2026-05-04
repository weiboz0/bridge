"use client"

import { useCallback, useEffect, useImperativeHandle, useRef, useState, forwardRef } from "react"
import { nanoid } from "nanoid"
import { EditorContent, type Editor, type JSONContent, useEditor } from "@tiptap/react"
import { Button } from "@/components/ui/button"
import { teachingUnitExtensions } from "./extensions"
import { SlashCommandExtension } from "./slash-menu"
import { BlockKeyboardShortcuts } from "./keyboard-shortcuts"
import { AIDraftPanel } from "./ai-draft-panel"
import { BubbleToolbar } from "./bubble-toolbar"
import { EditorToolbar } from "./editor-toolbar"
import { BlockHandle } from "./block-handle"
import { ContextMenu } from "./context-menu"
import { useYjsTiptap } from "@/lib/yjs/use-yjs-tiptap"
import { RealtimeConfigBanner } from "@/components/realtime/realtime-config-banner"
import { uploadFile } from "./media-embed-node"
import { HelpOverlay, shouldShowHelp } from "./help-overlay"
import { TableToolbar } from "./table-toolbar"

/** Options for enabling real-time collaborative editing via Yjs/Hocuspocus. */
export interface CollaborativeOptions {
  /** The unit ID — used as the Hocuspocus document name (`unit:{unitId}`). */
  unitId: string
  /** Authenticated user ID — used for the Hocuspocus auth token and cursor color. */
  userId: string
  /** Display name shown on the awareness cursor label. */
  userName: string
  /** Optional explicit cursor color. Derived from userId hash when omitted. */
  userColor?: string
}

export interface TeachingUnitEditorProps {
  initialDoc?: JSONContent
  onSave: (doc: JSONContent) => Promise<void>
  onDirty?: (dirty: boolean) => void
  /**
   * The teaching unit ID. Used for AI drafting and derived from collaborative
   * options when omitted.
   */
  unitId?: string
  /**
   * When provided, the editor runs in collaborative mode via Yjs + Hocuspocus.
   * Yjs becomes the source of truth; `initialDoc` is ignored once any remote
   * state has synced.  The `onSave` callback still works — it serializes the
   * current editor JSON and calls the save API.
   */
  collaborative?: CollaborativeOptions
}

export interface TeachingUnitEditorHandle {
  /** Programmatically trigger a save of the current document. */
  save: () => Promise<void>
}

// ---------------------------------------------------------------------------
// Sticky TOC sidebar (Gap 4)
// ---------------------------------------------------------------------------

type HeadingEntry = {
  level: number
  text: string
  pos: number
}

function TocSidebar({ editor }: { editor: Editor }) {
  const [headings, setHeadings] = useState<HeadingEntry[]>([])
  const [activeIndex, setActiveIndex] = useState(0)

  useEffect(() => {
    function collectHeadings() {
      const entries: HeadingEntry[] = []
      editor.state.doc.forEach((node, offset) => {
        if (node.type.name === "heading") {
          entries.push({
            level: node.attrs.level as number,
            text: node.textContent,
            pos: offset,
          })
        }
      })
      setHeadings(entries)
    }

    collectHeadings()
    editor.on("update", collectHeadings)
    return () => {
      editor.off("update", collectHeadings)
    }
  }, [editor])

  // Track which heading is currently in view
  useEffect(() => {
    if (headings.length === 0) return

    const handleScroll = () => {
      let closest = 0
      let closestDist = Infinity

      for (let i = 0; i < headings.length; i++) {
        try {
          const domPos = editor.view.domAtPos(headings[i].pos + 1)
          const el = domPos.node as HTMLElement
          const target = el.nodeType === 3 ? el.parentElement : el
          if (!target) continue
          const rect = target.getBoundingClientRect()
          const dist = Math.abs(rect.top - 80) // offset for toolbar
          if (dist < closestDist) {
            closestDist = dist
            closest = i
          }
        } catch {
          // Ignore
        }
      }
      setActiveIndex(closest)
    }

    const scrollEl = editor.view.dom.closest(".overflow-y-auto") ?? window
    scrollEl.addEventListener("scroll", handleScroll, { passive: true })
    return () => scrollEl.removeEventListener("scroll", handleScroll)
  }, [editor, headings])

  // Only show when document has 3+ headings
  if (headings.length < 3) return null

  function scrollToHeading(pos: number) {
    editor
      .chain()
      .focus()
      .setTextSelection(pos + 1)
      .run()

    try {
      const domPos = editor.view.domAtPos(pos + 1)
      const el = domPos.node as HTMLElement
      const target = el.nodeType === 3 ? el.parentElement : el
      target?.scrollIntoView({ behavior: "smooth", block: "start" })
    } catch {
      // Ignore
    }
  }

  return (
    <div className="sticky top-20 hidden w-48 shrink-0 xl:block">
      <p className="mb-2 text-[10px] font-semibold uppercase tracking-wider text-zinc-400">
        On this page
      </p>
      <nav aria-label="Table of contents sidebar">
        <ul className="space-y-0.5 border-l border-zinc-200">
          {headings.map((h, i) => (
            <li
              key={i}
              style={{ paddingLeft: `${(h.level - 1) * 8 + 12}px` }}
            >
              <button
                type="button"
                onClick={() => scrollToHeading(h.pos)}
                className={
                  "block w-full truncate text-left text-xs transition-colors " +
                  (i === activeIndex
                    ? "font-medium text-zinc-900 -ml-px border-l-2 border-zinc-900 pl-[10px]"
                    : "text-zinc-500 hover:text-zinc-700")
                }
              >
                {h.text || "Untitled"}
              </button>
            </li>
          ))}
        </ul>
      </nav>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Phase 0 — Block validation on load
// ---------------------------------------------------------------------------
// Known block types registered in our Tiptap schema (StarterKit + custom nodes).
// Used to sanitize loaded documents before passing to the editor.
const KNOWN_NODE_TYPES = new Set([
  // StarterKit built-in types
  "doc",
  "text",
  "paragraph",
  "heading",
  "bulletList",
  "orderedList",
  "listItem",
  "blockquote",
  "codeBlock",
  "horizontalRule",
  "hardBreak",
  // New extensions (Phase 1)
  "taskList",
  "taskItem",
  "table",
  "tableRow",
  "tableCell",
  "tableHeader",
  // Custom teaching nodes
  "problem-ref",
  "teacher-note",
  "code-snippet",
  "media-embed",
  "solution-ref",
  "test-case-ref",
  "live-cue",
  "assignment-variant",
  // Math / KaTeX nodes
  "math-block",
  "math-inline",
  // Inline mention pill
  "mention",
])

/**
 * Sanitizes a loaded document JSON before passing to the editor.
 * - Replaces blocks with unknown types with a paragraph containing a warning.
 * - Assigns missing `attrs.id` for custom blocks that declare an id attr.
 * This prevents corrupted or forward-incompatible documents from crashing
 * the editor.
 */
function sanitizeDoc(doc: JSONContent): JSONContent {
  if (!doc || doc.type !== "doc" || !Array.isArray(doc.content)) {
    return doc
  }

  const sanitizedContent = doc.content.map((node) => sanitizeNode(node))
  return { ...doc, content: sanitizedContent }
}

function sanitizeNode(node: JSONContent): JSONContent {
  const nodeType = node.type ?? ""

  // Check for unknown node types at this level
  if (nodeType && !KNOWN_NODE_TYPES.has(nodeType)) {
    console.warn(`[TeachingUnitEditor] Unknown block type "${nodeType}" replaced with paragraph fallback.`)
    return {
      type: "paragraph",
      content: [
        { type: "text", text: `[Unknown block type: ${nodeType}]` },
      ],
    }
  }

  // Assign missing id attrs for custom blocks that should have them
  let attrs = node.attrs
  if (attrs && typeof attrs === "object" && "id" in attrs) {
    if (!attrs.id || (typeof attrs.id === "string" && attrs.id.length === 0)) {
      console.warn(`[TeachingUnitEditor] Missing attrs.id on "${nodeType}" node, assigning one.`)
      attrs = { ...attrs, id: nanoid() }
    }
  }

  // Recursively sanitize child content
  let content = node.content
  if (Array.isArray(content)) {
    content = content.map((child) => sanitizeNode(child))
  }

  if (attrs !== node.attrs || content !== node.content) {
    return { ...node, attrs, content }
  }

  return node
}

// ---------------------------------------------------------------------------
// Node ID assignment (existing)
// ---------------------------------------------------------------------------

function assignMissingTopLevelNodeIds(editor: Editor) {
  let transaction = editor.state.tr
  let needsUpdate = false

  editor.state.doc.forEach((node: { type: { spec: { attrs?: Record<string, unknown> } }; attrs?: Record<string, unknown> }, offset: number) => {
    // Only process node types that declare an `id` attribute in their schema.
    // Calling setNodeMarkup on nodes without a declared `id` (e.g. paragraph,
    // heading) would pass unknown attrs into ProseMirror's schema validation
    // and cause a "NodeType.create can't construct text nodes" error.
    if (!node.type.spec.attrs || !("id" in node.type.spec.attrs)) {
      return
    }

    const attrs = node.attrs as Record<string, unknown> | null
    if (typeof attrs?.id === "string" && attrs.id.length > 0) {
      return
    }

    const nextAttrs = {
      ...attrs,
      id: nanoid(),
    }

    transaction = transaction.setNodeMarkup(offset, undefined, nextAttrs)
    needsUpdate = true
  })

  if (needsUpdate) {
    editor.view.dispatch(transaction)
  }
}

// ---------------------------------------------------------------------------
// AI block type mapping
// ---------------------------------------------------------------------------

/**
 * Maps AI-generated block types (e.g. "prose", "teacher-note", "code-snippet")
 * to valid Tiptap JSONContent that the editor can insert. The AI endpoint uses
 * its own vocabulary for block types — this function bridges the gap.
 */
function mapAIBlockToTiptap(block: Record<string, unknown>): JSONContent {
  const blockType = typeof block.type === "string" ? block.type : ""

  // The Go endpoint returns blocks with structured `content` arrays (valid
  // Tiptap JSON). Text-only fallback for plain-string responses.
  const hasStructuredContent = Array.isArray(block.content)
  const text =
    typeof block.text === "string"
      ? block.text
      : typeof block.content === "string"
        ? block.content
        : ""

  switch (blockType) {
    case "prose":
      // Go returns: { type:"prose", content:[{type:"paragraph",...}] }
      // "prose" isn't a Tiptap node — unwrap to its children (paragraphs).
      if (hasStructuredContent) {
        return {
          type: "doc",
          content: block.content as JSONContent[],
        }
      }
      if (!text) return { type: "paragraph" }
      return {
        type: "doc",
        content: text.split(/\n{2,}/).map((chunk: string) => ({
          type: "paragraph",
          content: chunk.trim()
            ? [{ type: "text", text: chunk.trim() }]
            : [],
        })),
      }

    case "heading": {
      const level =
        typeof block.level === "number" && block.level >= 1 && block.level <= 3
          ? block.level
          : 2
      return {
        type: "heading",
        attrs: { level },
        content: text ? [{ type: "text", text }] : [],
      }
    }

    case "teacher-note":
      // Go returns valid Tiptap JSON with content array — pass through
      if (hasStructuredContent) {
        return {
          type: "teacher-note",
          attrs: { id: nanoid(), ...(block.attrs as Record<string, unknown> ?? {}) },
          content: block.content as JSONContent[],
        }
      }
      return {
        type: "teacher-note",
        attrs: { id: nanoid() },
        content: [
          {
            type: "paragraph",
            content: text ? [{ type: "text", text }] : [],
          },
        ],
      }

    case "code-snippet": {
      // Go returns: { type:"code-snippet", attrs:{id, language, code} }
      // Attrs come from Go — merge with a fresh ID
      const attrs = (block.attrs as Record<string, unknown>) ?? {}
      const language =
        typeof attrs.language === "string" ? attrs.language
        : typeof block.language === "string" ? block.language
        : "python"
      const code =
        typeof attrs.code === "string" ? attrs.code
        : typeof block.code === "string" ? block.code
        : text
      return {
        type: "code-snippet",
        attrs: { id: nanoid(), language, code },
      }
    }

    case "problem-ref":
    case "problem": {
      const attrs = (block.attrs as Record<string, unknown>) ?? {}
      const problemId =
        typeof attrs.problemId === "string" ? attrs.problemId
        : typeof block.problemId === "string" ? block.problemId
        : ""
      const visibility =
        typeof attrs.visibility === "string" ? attrs.visibility : "always"
      return {
        type: "problem-ref",
        attrs: {
          id: nanoid(),
          problemId,
          pinnedRevision: null,
          visibility,
          overrideStarter: null,
        },
      }
    }

    case "solution-ref":
    case "solution": {
      const solutionId =
        typeof block.solutionId === "string" ? block.solutionId : ""
      return {
        type: "solution-ref",
        attrs: { id: nanoid(), solutionId, reveal: "always" },
      }
    }

    case "media-embed":
    case "media": {
      const url = typeof block.url === "string" ? block.url : ""
      const alt = typeof block.alt === "string" ? block.alt : ""
      const mediaType =
        typeof block.mediaType === "string" ? block.mediaType : "image"
      return {
        type: "media-embed",
        attrs: { id: nanoid(), url, alt, mediaType },
      }
    }

    case "live-cue":
      return {
        type: "live-cue",
        attrs: { id: nanoid() },
        content: [
          {
            type: "paragraph",
            content: text ? [{ type: "text", text }] : [],
          },
        ],
      }

    case "assignment-variant":
    case "assignment": {
      const title = typeof block.title === "string" ? block.title : ""
      const timeLimitMinutes =
        typeof block.timeLimitMinutes === "number"
          ? block.timeLimitMinutes
          : null
      return {
        type: "assignment-variant",
        attrs: { id: nanoid(), title, timeLimitMinutes },
        content: [
          {
            type: "paragraph",
            content: text ? [{ type: "text", text }] : [],
          },
        ],
      }
    }

    default:
      // Fallback: wrap any text content in a paragraph.
      if (text) {
        return {
          type: "paragraph",
          content: [{ type: "text", text }],
        }
      }
      // Last resort: try to use the block as-is (it might already be valid
      // Tiptap JSON).
      return block as JSONContent
  }
}

// ---------------------------------------------------------------------------
// Save format note (Phase 0, Task 0.2)
// ---------------------------------------------------------------------------
// Future: wrap the doc JSON in an envelope with a schema version marker, e.g.:
//   { version: 1, doc: { type: "doc", content: [...] } }
// The Go store currently handles raw doc JSON. Changing the save format would
// require a migration. Deferred until a concrete need arises (new block type
// requiring schema-level migration).

// ---------------------------------------------------------------------------
// Editor footer — word count / character count / reading time
// ---------------------------------------------------------------------------

function EditorFooter({ editor }: { editor: Editor }) {
  // Force re-render on editor updates so counts stay current.
  const [, setTick] = useState(0)
  useEffect(() => {
    const handler = () => setTick((t) => t + 1)
    editor.on("update", handler)
    return () => { editor.off("update", handler) }
  }, [editor])

  const charCount = editor.storage.characterCount?.characters() ?? 0
  const wordCount = editor.storage.characterCount?.words() ?? 0
  const readingTime = Math.max(1, Math.ceil(wordCount / 200))

  return (
    <div className="flex items-center gap-3 rounded-b-lg border border-t-0 border-zinc-200 bg-white px-3 py-1 text-xs text-zinc-400">
      <span>{wordCount} {wordCount === 1 ? "word" : "words"}</span>
      <span className="text-zinc-300">/</span>
      <span>{charCount} {charCount === 1 ? "character" : "characters"}</span>
      <span className="text-zinc-300">/</span>
      <span>~{readingTime} min read</span>
    </div>
  )
}

export const TeachingUnitEditor = forwardRef<TeachingUnitEditorHandle, TeachingUnitEditorProps>(
function TeachingUnitEditor({ initialDoc, onSave, onDirty, unitId, collaborative }, ref) {
  const [showAIPanel, setShowAIPanel] = useState(false)
  const [showHelp, setShowHelp] = useState(() => shouldShowHelp())
  const [presentationMode, setPresentationMode] = useState(false)
  const [aiPromptMode, setAiPromptMode] = useState(false)
  const [aiGenerating, setAiGenerating] = useState(false)
  type PageWidth = "standard" | "wide" | "full"
  const [pageWidth, setPageWidth] = useState<PageWidth>(() => {
    if (typeof window === "undefined") return "standard"
    return (localStorage.getItem("bridge:editor-width") as PageWidth) || "standard"
  })

  // Resolve the unit ID from the explicit prop or the collaborative options.
  const resolvedUnitId = unitId ?? collaborative?.unitId ?? ""
  // Collaborative mode: set up Yjs binding when `collaborative` is provided.
  // The hook is always called (Rules of Hooks) but does nothing when
  // `unitId` or `userId` are empty strings.
  const { extensions: yjsExtensions, connected, realtimeUnavailable } = useYjsTiptap({
    unitId: collaborative?.unitId ?? "",
    userId: collaborative?.userId ?? "",
    userName: collaborative?.userName ?? "",
    userColor: collaborative?.userColor,
  })

  const isCollaborative = Boolean(collaborative) && yjsExtensions.length > 0

  // Phase 0: Sanitize loaded documents before passing to the editor.
  // This strips unknown block types and assigns missing IDs.
  const sanitizedInitialDoc = initialDoc ? sanitizeDoc(initialDoc) : undefined

  const editor = useEditor({
    immediatelyRender: false,
    extensions: [
      ...teachingUnitExtensions(),
      SlashCommandExtension,
      BlockKeyboardShortcuts,
      ...(isCollaborative ? yjsExtensions : []),
    ],
    content: isCollaborative && connected
      ? undefined
      : (sanitizedInitialDoc ?? { type: "doc", content: [] }),
    editorProps: {
      handleDrop: (view, event) => {
        const files = event.dataTransfer?.files
        if (!files || files.length === 0) return false
        const imageFiles = Array.from(files).filter((f) =>
          f.type.startsWith("image/") || f.type === "application/pdf",
        )
        if (imageFiles.length === 0) return false
        event.preventDefault()
        for (const file of imageFiles) {
          uploadFile(file).then((url) => {
            view.dispatch(
              view.state.tr.replaceSelectionWith(
                view.state.schema.nodes["media-embed"].create({
                  id: nanoid(),
                  url,
                  alt: file.name.replace(/\.[^.]+$/, ""),
                  mediaType: file.type === "application/pdf" ? "pdf" : "image",
                }),
              ),
            )
          }).catch((err) => {
            console.error("[TeachingUnitEditor] Drop upload failed:", err)
          })
        }
        return true
      },
      handlePaste: (view, event) => {
        const items = event.clipboardData?.items
        if (!items) return false
        const imageItems = Array.from(items).filter((item) =>
          item.type.startsWith("image/"),
        )
        if (imageItems.length === 0) return false
        event.preventDefault()
        for (const item of imageItems) {
          const file = item.getAsFile()
          if (!file) continue
          uploadFile(file).then((url) => {
            view.dispatch(
              view.state.tr.replaceSelectionWith(
                view.state.schema.nodes["media-embed"].create({
                  id: nanoid(),
                  url,
                  alt: "Pasted image",
                  mediaType: "image",
                }),
              ),
            )
          }).catch((err) => {
            console.error("[TeachingUnitEditor] Paste upload failed:", err)
          })
        }
        return true
      },
    },
    onCreate({ editor: e }: { editor: Editor }) {
      assignMissingTopLevelNodeIds(e)
    },
    onUpdate({ editor: e }: { editor: Editor }) {
      assignMissingTopLevelNodeIds(e)
      onDirty?.(true)
    },
  })

  const handleInsertAIBlocks = useCallback((blocks: unknown[]) => {
    if (!editor) return
    // Map AI output types to valid Tiptap node types, then insert.
    for (const block of blocks) {
      if (typeof block !== "object" || block === null) continue
      const mapped = mapAIBlockToTiptap(block as Record<string, unknown>)
      // A "doc" wrapper means the mapper produced multiple nodes (e.g. prose
      // split into paragraphs). Insert each child individually.
      if (mapped.type === "doc" && Array.isArray(mapped.content)) {
        for (const child of mapped.content) {
          editor.chain().focus().insertContent(child).run()
        }
      } else {
        editor.chain().focus().insertContent(mapped).run()
      }
    }
    onDirty?.(true)
  }, [editor, onDirty])

  const handleSave = useCallback(async () => {
    if (!editor) {
      return
    }

    const doc = editor.getJSON()
    await onSave(doc)
    onDirty?.(false)
  }, [editor, onDirty, onSave])

  useImperativeHandle(ref, () => ({
    save: handleSave,
  }), [handleSave])

  // Debounced autosave — saves 2 seconds after the last edit
  const autosaveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(() => {
    if (!editor) return

    const handleUpdate = () => {
      if (autosaveTimerRef.current) clearTimeout(autosaveTimerRef.current)
      autosaveTimerRef.current = setTimeout(() => {
        const doc = editor.getJSON()
        onSave(doc).then(() => onDirty?.(false)).catch(() => {})
      }, 2000)
    }

    editor.on("update", handleUpdate)
    return () => {
      editor.off("update", handleUpdate)
      if (autosaveTimerRef.current) clearTimeout(autosaveTimerRef.current)
    }
  }, [editor, onSave, onDirty])

  // Listen for keyboard-shortcut–triggered save (Mod-Enter)
  useEffect(() => {
    const handler = () => { handleSave() }
    window.addEventListener("tiptap:save", handler)
    return () => window.removeEventListener("tiptap:save", handler)
  }, [handleSave])

  // Listen for AI prompt mode from the slash menu — user types prompt in
  // the current paragraph, then Cmd+J generates content
  useEffect(() => {
    const handler = () => setAiPromptMode(true)
    window.addEventListener("tiptap:ai-prompt-mode", handler)
    return () => window.removeEventListener("tiptap:ai-prompt-mode", handler)
  }, [])

  // Cmd+J triggers AI generation when in prompt mode
  useEffect(() => {
    if (!aiPromptMode || !editor || !resolvedUnitId) return

    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "j") {
        e.preventDefault()
        // Get the text of the current paragraph as the prompt
        const { from } = editor.state.selection
        const resolved = editor.state.doc.resolve(from)
        const blockStart = resolved.before(1)
        const blockEnd = resolved.after(1)
        const promptText = editor.state.doc.textBetween(blockStart, blockEnd, " ").trim()

        if (!promptText) return

        setAiGenerating(true)
        // Delete the prompt paragraph
        editor.chain().focus().deleteRange({ from: blockStart, to: blockEnd }).run()

        // Call AI
        import("@/lib/teaching-units").then(({ draftWithAI }) => {
          draftWithAI(resolvedUnitId, promptText)
            .then((result) => {
              if (result?.blocks) handleInsertAIBlocks(result.blocks)
              setAiPromptMode(false)
            })
            .catch(() => setAiPromptMode(false))
            .finally(() => setAiGenerating(false))
        })
      }
      if (e.key === "Escape") {
        setAiPromptMode(false)
      }
    }

    document.addEventListener("keydown", handleKeyDown)
    return () => document.removeEventListener("keydown", handleKeyDown)
  }, [aiPromptMode, editor, resolvedUnitId, handleInsertAIBlocks])

  // Listen for AI "Continue Writing" from the slash menu
  useEffect(() => {
    const handler = async (e: Event) => {
      const detail = (e as CustomEvent).detail as { intent?: string } | undefined
      if (!detail?.intent || !resolvedUnitId) return
      setAiGenerating(true)
      try {
        const { draftWithAI } = await import("@/lib/teaching-units")
        const result = await draftWithAI(resolvedUnitId, detail.intent)
        if (result?.blocks) {
          handleInsertAIBlocks(result.blocks)
        }
      } catch {
        // Silent — the content just doesn't appear
      } finally {
        setAiGenerating(false)
      }
    }
    window.addEventListener("tiptap:ai-continue", handler)
    return () => window.removeEventListener("tiptap:ai-continue", handler)
  }, [resolvedUnitId, handleInsertAIBlocks])


  // Presentation mode: make the editor read-only and hide editing chrome.
  const togglePresentationMode = useCallback(() => {
    if (!editor) return
    const entering = !presentationMode
    setPresentationMode(entering)
    editor.setEditable(!entering)
    if (entering) {
      // Hide AI panel when entering presentation mode.
      setShowAIPanel(false)
      setAiPromptMode(false)
    }
  }, [editor, presentationMode])

  // Presentation mode: clean read-only view.
  if (presentationMode) {
    return (
      <div className="relative min-h-screen bg-white">
        {/* Floating exit button */}
        <button
          type="button"
          onClick={togglePresentationMode}
          className={
            "fixed right-4 top-4 z-50 rounded-lg bg-zinc-900/80 px-4 py-2 text-sm font-medium text-white shadow-lg " +
            "transition-opacity hover:bg-zinc-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/50"
          }
        >
          Exit Presentation
        </button>
        {/* Document content in presentation style */}
        <div className="mx-auto max-w-3xl px-8 py-12">
          <EditorContent
            editor={editor}
            className="prose prose-lg max-w-none [&_.ProseMirror]:min-h-0 [&_.ProseMirror]:p-0"
          />
        </div>
      </div>
    )
  }

  return (
    <>
      <RealtimeConfigBanner unavailable={realtimeUnavailable} />
    <div className="flex gap-4">
    <div className="min-w-0 flex-1 space-y-0">
      {/* Help overlay (first visit + re-openable) */}
      {showHelp && (
        <HelpOverlay
          forceShow={true}
          onDismiss={() => setShowHelp(false)}
        />
      )}

      {/* Fixed top toolbar (Phase 1) */}
      {editor && (
        <EditorToolbar
          editor={editor}
          collaborative={collaborative ? { connected } : undefined}
          onSave={handleSave}
          onToggleAI={() => setShowAIPanel((prev) => !prev)}
          showAI={showAIPanel}
          unitId={resolvedUnitId || undefined}
          onShowHelp={() => setShowHelp(true)}
          presentationMode={presentationMode}
          onTogglePresentation={togglePresentationMode}
          pageWidth={pageWidth}
          onSetPageWidth={(w: PageWidth) => { setPageWidth(w); localStorage.setItem("bridge:editor-width", w) }}
        />
      )}

      {/* AI draft panel */}
      {showAIPanel && resolvedUnitId && (
        <AIDraftPanel
          unitId={resolvedUnitId}
          onInsertBlocks={handleInsertAIBlocks}
          onClose={() => setShowAIPanel(false)}
        />
      )}


      {/* Editor content with bubble toolbar, block handle, and context menu.
          data-block-handle-wrapper lets BlockHandle listen on this div so the
          handle doesn't disappear when the mouse moves from editor to handle. */}
      <div className={`relative border border-t-0 border-zinc-200 bg-white ${pageWidth === "standard" ? "mx-auto max-w-3xl" : pageWidth === "wide" ? "mx-auto max-w-5xl" : ""}`} data-block-handle-wrapper>
        {editor && <BubbleToolbar editor={editor} unitId={resolvedUnitId || undefined} />}
        {editor && <BlockHandle editor={editor} />}
        {editor && <ContextMenu editor={editor} />}
        {editor && <TableToolbar editor={editor} />}
        <EditorContent editor={editor} className="min-h-60 px-3 py-2 pl-10" />
        {/* AI prompt mode indicator */}
        {aiPromptMode && (
          <div className="mx-3 mb-2 flex items-center gap-2 rounded-lg border border-blue-300 bg-blue-50 px-3 py-1.5 text-xs text-blue-600">
            <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded bg-blue-100 text-[9px] font-bold">AI</span>
            {aiGenerating
              ? <span>Generating...</span>
              : <span>Type your prompt above, then press <kbd className="rounded bg-blue-100 px-1 font-mono">Cmd+J</kbd> to generate. <kbd className="rounded bg-blue-100 px-1 font-mono">Esc</kbd> to cancel.</span>
            }
          </div>
        )}
      </div>

      {/* Word count / character count footer */}
      {editor && <EditorFooter editor={editor} />}
    </div>
    {/* Sticky TOC sidebar (Gap 4) — only visible on xl+ screens */}
    {editor && !presentationMode && <TocSidebar editor={editor} />}
    </div>
    </>
  )
})
TeachingUnitEditor.displayName = "TeachingUnitEditor"

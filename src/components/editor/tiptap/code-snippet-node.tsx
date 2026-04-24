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

type Language = "python" | "javascript" | "blockly"

type CodeSnippetNodeAttrs = {
  id: string
  language: Language
  code: string
}

const LANGUAGES: Language[] = ["python", "javascript", "blockly"]

function CodeSnippetNodeView({ node, updateAttributes }: NodeViewProps) {
  const { language, code } = node.attrs as CodeSnippetNodeAttrs
  const [editing, setEditing] = useState(false)
  const [draftCode, setDraftCode] = useState(code)
  const [draftLang, setDraftLang] = useState<Language>(language)

  const openEditor = useCallback(() => {
    setDraftCode(code)
    setDraftLang(language)
    setEditing(true)
  }, [code, language])

  const handleSave = useCallback(() => {
    updateAttributes({ code: draftCode, language: draftLang })
    setEditing(false)
  }, [draftCode, draftLang, updateAttributes])

  const handleCancel = useCallback(() => {
    setEditing(false)
  }, [])

  return (
    <NodeViewWrapper className="code-snippet-node my-3" contentEditable={false}>
      <div className="overflow-hidden rounded-md border border-zinc-200 bg-zinc-900">
        {/* Header bar */}
        <div className="flex items-center justify-between border-b border-zinc-700 bg-zinc-800 px-3 py-1.5">
          <span className="text-xs font-medium text-zinc-400">{language}</span>
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-2 text-xs text-zinc-400 hover:text-zinc-100"
            onClick={openEditor}
          >
            Edit
          </Button>
        </div>

        {/* Code display */}
        <pre className="overflow-x-auto p-4 text-sm leading-relaxed">
          <code className="font-mono text-zinc-100 whitespace-pre">
            {code || <span className="text-zinc-500 italic">No code yet — click Edit to add.</span>}
          </code>
        </pre>

        {/* Inline editor overlay */}
        {editing && (
          <div className="border-t border-zinc-700 bg-zinc-800 p-3 space-y-2">
            <div className="flex items-center gap-2">
              <label className="text-xs text-zinc-400 w-20 shrink-0">Language</label>
              <select
                value={draftLang}
                onChange={(e) => setDraftLang(e.target.value as Language)}
                className="rounded border border-zinc-600 bg-zinc-700 px-2 py-1 text-xs text-zinc-100 focus:outline-none focus:ring-1 focus:ring-zinc-400"
              >
                {LANGUAGES.map((l) => (
                  <option key={l} value={l}>
                    {l}
                  </option>
                ))}
              </select>
            </div>
            <textarea
              value={draftCode}
              onChange={(e) => setDraftCode(e.target.value)}
              rows={8}
              spellCheck={false}
              className="w-full rounded border border-zinc-600 bg-zinc-700 p-2 font-mono text-xs text-zinc-100 leading-relaxed resize-y focus:outline-none focus:ring-1 focus:ring-zinc-400"
              placeholder="Paste or type code here..."
            />
            <div className="flex gap-2">
              <Button
                size="sm"
                className="h-7 text-xs bg-zinc-600 hover:bg-zinc-500 text-zinc-100"
                onClick={handleSave}
              >
                Save
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="h-7 text-xs text-zinc-400 hover:text-zinc-100"
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

export const CodeSnippetNode = Node.create({
  name: "code-snippet",
  group: "block",
  atom: true,
  addAttributes() {
    return {
      id: {
        default: () => nanoid(),
      },
      language: {
        default: "python" as Language,
      },
      code: {
        default: "",
      },
    }
  },
  parseHTML() {
    return [
      {
        tag: 'div[data-type="code-snippet"]',
      },
    ]
  },
  renderHTML({ node, HTMLAttributes }: any) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-type": "code-snippet",
        "data-code-snippet-id": node.attrs.id,
        "data-code-snippet-language": node.attrs.language,
        "data-code-snippet-code": node.attrs.code,
      }),
    ]
  },
  addNodeView() {
    return ReactNodeViewRenderer(CodeSnippetNodeView)
  },
})

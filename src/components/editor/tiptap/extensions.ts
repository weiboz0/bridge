import type { AnyExtension } from "@tiptap/react"
import { Extension } from "@tiptap/core"
import StarterKit from "@tiptap/starter-kit"
import Underline from "@tiptap/extension-underline"
import Link from "@tiptap/extension-link"
import Highlight from "@tiptap/extension-highlight"
import { TextStyle } from "@tiptap/extension-text-style"
import Color from "@tiptap/extension-color"
import Subscript from "@tiptap/extension-subscript"
import Superscript from "@tiptap/extension-superscript"
import TextAlign from "@tiptap/extension-text-align"
import Placeholder from "@tiptap/extension-placeholder"
import CharacterCount from "@tiptap/extension-character-count"
import Typography from "@tiptap/extension-typography"
import TaskList from "@tiptap/extension-task-list"
import TaskItem from "@tiptap/extension-task-item"
import { Table } from "@tiptap/extension-table"
import TableRow from "@tiptap/extension-table-row"
import TableCell from "@tiptap/extension-table-cell"
import TableHeader from "@tiptap/extension-table-header"

import { ProblemRefNode } from "./problem-ref-node"
import { TeacherNoteNode } from "./teacher-note-node"
import { CodeSnippetNode } from "./code-snippet-node"
import { MediaEmbedNode } from "./media-embed-node"
import { SolutionRefNode } from "./solution-ref-node"
import { TestCaseRefNode } from "./test-case-ref-node"
import { LiveCueNode } from "./live-cue-node"
import { AssignmentVariantNode } from "./assignment-variant-node"
import { CalloutNode } from "./callout-node"
import { ToggleNode } from "./toggle-node"
import { BookmarkNode } from "./bookmark-node"
import { TocNode } from "./toc-node"
import { ColumnsNode, ColumnNode } from "./columns-node"
import { MathBlockNode, MathInlineNode } from "./math-node"
import { EmojiPickerExtension } from "./emoji-picker"
import Focus from "@tiptap/extension-focus"
import { MentionNode, MentionSuggestionExtension } from "./mention-node"

// ---------------------------------------------------------------------------
// Block-level color attribute (Gap 3)
// ---------------------------------------------------------------------------

export const BLOCK_COLORS = [
  { label: "Default", value: "" },
  { label: "Light Gray", value: "block-color-gray" },
  { label: "Light Yellow", value: "block-color-yellow" },
  { label: "Light Green", value: "block-color-green" },
  { label: "Light Blue", value: "block-color-blue" },
  { label: "Light Purple", value: "block-color-purple" },
  { label: "Light Pink", value: "block-color-pink" },
  { label: "Light Red", value: "block-color-red" },
] as const

const BlockColorExtension = Extension.create({
  name: "blockColor",

  addGlobalAttributes() {
    return [
      {
        types: ["paragraph", "heading", "bulletList", "orderedList", "blockquote", "codeBlock", "taskList"],
        attributes: {
          blockColor: {
            default: null,
            parseHTML: (element: HTMLElement) => element.getAttribute("data-block-color") || null,
            renderHTML: (attributes: Record<string, unknown>) => {
              if (!attributes.blockColor) return {}
              return {
                "data-block-color": attributes.blockColor as string,
                class: attributes.blockColor as string,
              }
            },
          },
        },
      },
    ]
  },
})

export function teachingUnitExtensions(): AnyExtension[] {
  return [
    StarterKit.configure({
      heading: { levels: [1, 2, 3] },
      // Disable extensions we register separately with custom config below
      // to avoid "duplicate extension" warnings
      link: false,
      underline: false,
    }),
    // Inline formatting (registered explicitly for custom config)
    Underline,
    Link.configure({
      openOnClick: false,
      autolink: true,
    }),
    Highlight.configure({ multicolor: false }),
    TextStyle,
    Color,
    Subscript,
    Superscript,
    // Block-level formatting
    TextAlign.configure({
      types: ["heading", "paragraph"],
      alignments: ["left", "center", "right"],
    }),
    // UX polish — random tips on every empty paragraph
    Placeholder.configure({
      placeholder: ({ node, pos }) => {
        if (node.type.name === "heading") {
          return `Heading ${node.attrs.level ?? ""}`
        }
        if (node.type.name !== "paragraph") return ""
        if (pos === 0) return "Type / for commands, or start writing..."
        const tips = [
          "Type / for commands...",
          "Press Cmd+B to bold, Cmd+I to italic",
          "Type /ai to generate with AI",
          "Drag blocks with the handle on the left",
          "Press Cmd+Enter to save",
          "Type /table to insert a table",
          "Type :emoji: to insert emoji",
          "Press Cmd+K to add a link",
          "Type /callout for an info box",
          "Press Cmd+Shift+D to duplicate a block",
        ]
        return tips[pos % tips.length]
      },
      showOnlyCurrent: true,
    }),
    CharacterCount,
    Typography,
    Focus.configure({ className: "has-focus", mode: "deepest" }),
    // Task lists
    TaskList,
    TaskItem.configure({ nested: true }),
    // Tables
    Table.configure({ resizable: true }),
    TableRow,
    TableCell,
    TableHeader,
    // Custom teaching nodes
    ProblemRefNode,
    TeacherNoteNode,
    CodeSnippetNode,
    MediaEmbedNode,
    SolutionRefNode,
    TestCaseRefNode,
    LiveCueNode,
    AssignmentVariantNode,
    // Phase 3 block types
    CalloutNode,
    ToggleNode,
    BookmarkNode,
    TocNode,
    ColumnsNode,
    ColumnNode,
    // Math / KaTeX nodes
    MathBlockNode,
    MathInlineNode,
    // Emoji picker (:shortcode: suggestion)
    EmojiPickerExtension,
    // @-mention (inline user pills)
    MentionNode,
    MentionSuggestionExtension,
    // Block-level color (data-block-color attribute)
    BlockColorExtension,
  ] as AnyExtension[]
}

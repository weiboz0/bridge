// @vitest-environment jsdom
import { describe, it, expect, beforeEach, afterEach } from "vitest"
import { Editor } from "@tiptap/core"
import StarterKit from "@tiptap/starter-kit"

import {
  moveBlockUp,
  moveBlockDown,
  duplicateBlock,
  deleteBlock,
} from "@/components/editor/tiptap/keyboard-shortcuts"

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Create a fresh editor with three paragraphs. */
function makeEditor(content = "<p>First</p><p>Second</p><p>Third</p>") {
  return new Editor({
    extensions: [StarterKit],
    content,
  })
}

/** Place the cursor inside the Nth top-level block (1-based). */
function selectBlock(editor: Editor, n: number) {
  // Each paragraph is a top-level node.  Position of block n is:
  //   sum of sizes of all preceding blocks + 1 (start of block content)
  let pos = 0
  for (let i = 0; i < n - 1; i++) {
    pos += editor.state.doc.child(i).nodeSize
  }
  // pos now points to the position before block n; +1 enters the node
  const resolved = editor.state.doc.resolve(pos + 1)
  const { tr } = editor.state
  const { TextSelection } = require("@tiptap/pm/state")
  tr.setSelection(TextSelection.near(resolved))
  editor.view.dispatch(tr)
}

/** Returns the text content of each top-level node as an array of strings. */
function blockTexts(editor: Editor): string[] {
  const texts: string[] = []
  editor.state.doc.forEach((node) => {
    texts.push(node.textContent)
  })
  return texts
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("moveBlockUp", () => {
  let editor: Editor

  beforeEach(() => {
    editor = makeEditor()
  })

  afterEach(() => {
    editor.destroy()
  })

  it("returns false and does nothing when cursor is on the first block", () => {
    selectBlock(editor, 1)
    const result = moveBlockUp(editor)
    expect(result).toBe(false)
    expect(blockTexts(editor)).toEqual(["First", "Second", "Third"])
  })

  it("moves second block above first", () => {
    selectBlock(editor, 2)
    const result = moveBlockUp(editor)
    expect(result).toBe(true)
    expect(blockTexts(editor)).toEqual(["Second", "First", "Third"])
  })

  it("moves third block above second", () => {
    selectBlock(editor, 3)
    const result = moveBlockUp(editor)
    expect(result).toBe(true)
    expect(blockTexts(editor)).toEqual(["First", "Third", "Second"])
  })

  it("does not change block count", () => {
    selectBlock(editor, 2)
    moveBlockUp(editor)
    expect(editor.state.doc.childCount).toBe(3)
  })
})

describe("moveBlockDown", () => {
  let editor: Editor

  beforeEach(() => {
    editor = makeEditor()
  })

  afterEach(() => {
    editor.destroy()
  })

  it("returns false and does nothing when cursor is on the last block", () => {
    selectBlock(editor, 3)
    const result = moveBlockDown(editor)
    expect(result).toBe(false)
    expect(blockTexts(editor)).toEqual(["First", "Second", "Third"])
  })

  it("moves first block below second", () => {
    selectBlock(editor, 1)
    const result = moveBlockDown(editor)
    expect(result).toBe(true)
    expect(blockTexts(editor)).toEqual(["Second", "First", "Third"])
  })

  it("moves second block below third", () => {
    selectBlock(editor, 2)
    const result = moveBlockDown(editor)
    expect(result).toBe(true)
    expect(blockTexts(editor)).toEqual(["First", "Third", "Second"])
  })

  it("does not change block count", () => {
    selectBlock(editor, 1)
    moveBlockDown(editor)
    expect(editor.state.doc.childCount).toBe(3)
  })
})

describe("duplicateBlock", () => {
  let editor: Editor

  beforeEach(() => {
    editor = makeEditor()
  })

  afterEach(() => {
    editor.destroy()
  })

  it("returns true and increases block count by one", () => {
    selectBlock(editor, 1)
    const before = editor.state.doc.childCount
    const result = duplicateBlock(editor)
    expect(result).toBe(true)
    expect(editor.state.doc.childCount).toBe(before + 1)
  })

  it("inserts a copy immediately after the current block", () => {
    selectBlock(editor, 1)
    duplicateBlock(editor)
    const texts = blockTexts(editor)
    expect(texts[0]).toBe("First")
    expect(texts[1]).toBe("First") // duplicate
    expect(texts[2]).toBe("Second")
    expect(texts[3]).toBe("Third")
  })

  it("duplicating the last block appends to the end", () => {
    selectBlock(editor, 3)
    duplicateBlock(editor)
    const texts = blockTexts(editor)
    expect(texts[3]).toBe("Third")
  })

  it("the duplicate is a separate node (deep copy)", () => {
    selectBlock(editor, 1)
    duplicateBlock(editor)
    // Verify the two "First" nodes are different node instances
    const n1 = editor.state.doc.child(0)
    const n2 = editor.state.doc.child(1)
    expect(n1).not.toBe(n2)
  })
})

describe("deleteBlock", () => {
  let editor: Editor

  beforeEach(() => {
    editor = makeEditor()
  })

  afterEach(() => {
    editor.destroy()
  })

  it("returns true and decreases block count by one", () => {
    selectBlock(editor, 1)
    const before = editor.state.doc.childCount
    const result = deleteBlock(editor)
    expect(result).toBe(true)
    expect(editor.state.doc.childCount).toBe(before - 1)
  })

  it("deletes the correct block (first)", () => {
    selectBlock(editor, 1)
    deleteBlock(editor)
    expect(blockTexts(editor)).toEqual(["Second", "Third"])
  })

  it("deletes the correct block (second)", () => {
    selectBlock(editor, 2)
    deleteBlock(editor)
    expect(blockTexts(editor)).toEqual(["First", "Third"])
  })

  it("deletes the correct block (last)", () => {
    selectBlock(editor, 3)
    deleteBlock(editor)
    expect(blockTexts(editor)).toEqual(["First", "Second"])
  })

  it("replaces with empty paragraph when only one block remains", () => {
    const singleEditor = makeEditor("<p>Only</p>")
    selectBlock(singleEditor, 1)
    const result = deleteBlock(singleEditor)
    expect(result).toBe(true)
    // Doc must still contain at least one paragraph (ProseMirror invariant)
    expect(singleEditor.state.doc.childCount).toBe(1)
    expect(singleEditor.state.doc.child(0).textContent).toBe("")
    singleEditor.destroy()
  })
})

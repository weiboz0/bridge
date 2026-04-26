import { Extension } from "@tiptap/core"
import type { Editor } from "@tiptap/core"
import { TextSelection } from "@tiptap/pm/state"

// ---------------------------------------------------------------------------
// Block manipulation helpers
// ---------------------------------------------------------------------------

/**
 * Resolves the top-level block position for the current selection.
 * Returns the position before the top-level node and the node itself,
 * or null if the selection is not inside a top-level block.
 */
function resolveTopBlock(editor: Editor) {
  const { $from } = editor.state.selection
  // depth 0 = doc, depth 1 = top-level block
  if ($from.depth < 1) return null
  const pos = $from.before(1)
  const node = editor.state.doc.nodeAt(pos)
  if (!node) return null
  return { pos, node }
}

/**
 * Move the current top-level block up (swap with previous sibling).
 */
export function moveBlockUp(editor: Editor): boolean {
  const block = resolveTopBlock(editor)
  if (!block) return false

  const { pos, node } = block
  // Already the first block — can't move up
  if (pos <= 0) return false

  // Find the previous sibling
  const $pos = editor.state.doc.resolve(pos)
  const prevIndex = $pos.index(0) - 1
  if (prevIndex < 0) return false

  const prevNode = editor.state.doc.child(prevIndex)
  const prevPos = pos - prevNode.nodeSize

  // Build a transaction: delete current node, insert before previous
  const { tr } = editor.state
  // Delete the current node
  tr.delete(pos, pos + node.nodeSize)
  // Insert it before the previous node (which is now at prevPos)
  tr.insert(prevPos, node)
  // Set selection inside the moved node
  tr.setSelection(
    TextSelection.near(tr.doc.resolve(prevPos + 1))
  )

  editor.view.dispatch(tr)
  return true
}

/**
 * Move the current top-level block down (swap with next sibling).
 */
export function moveBlockDown(editor: Editor): boolean {
  const block = resolveTopBlock(editor)
  if (!block) return false

  const { pos, node } = block
  const $pos = editor.state.doc.resolve(pos)
  const index = $pos.index(0)
  const docChildCount = editor.state.doc.childCount

  // Already the last block — can't move down
  if (index >= docChildCount - 1) return false

  const nextNode = editor.state.doc.child(index + 1)
  const nextPos = pos + node.nodeSize

  // Build a transaction: delete the next node, insert it before current
  const { tr } = editor.state
  tr.delete(nextPos, nextPos + nextNode.nodeSize)
  tr.insert(pos, nextNode)

  // Set selection inside the moved node (which is now after the inserted node)
  const newPos = pos + nextNode.nodeSize
  tr.setSelection(
    TextSelection.near(tr.doc.resolve(newPos + 1))
  )

  editor.view.dispatch(tr)
  return true
}

/**
 * Duplicate the current top-level block (insert a copy below).
 */
export function duplicateBlock(editor: Editor): boolean {
  const block = resolveTopBlock(editor)
  if (!block) return false

  const { pos, node } = block
  const insertPos = pos + node.nodeSize

  // Deep-copy the node via JSON round-trip to avoid shared references
  const copy = node.type.schema.nodeFromJSON(node.toJSON())

  const { tr } = editor.state
  tr.insert(insertPos, copy)
  // Move selection into the new copy
  tr.setSelection(
    TextSelection.near(tr.doc.resolve(insertPos + 1))
  )

  editor.view.dispatch(tr)
  return true
}

/**
 * Delete the current top-level block.
 */
export function deleteBlock(editor: Editor): boolean {
  const block = resolveTopBlock(editor)
  if (!block) return false

  const { pos, node } = block
  const { tr } = editor.state

  // If this is the only block, replace with an empty paragraph instead of
  // leaving a completely empty document (which ProseMirror doesn't allow).
  if (editor.state.doc.childCount <= 1) {
    tr.replaceWith(
      pos,
      pos + node.nodeSize,
      editor.state.schema.nodes.paragraph.create()
    )
  } else {
    tr.delete(pos, pos + node.nodeSize)
  }

  // Place selection at a reasonable position
  const resolvedPos = tr.doc.resolve(Math.min(pos, tr.doc.content.size - 1))
  tr.setSelection(
    TextSelection.near(resolvedPos)
  )

  editor.view.dispatch(tr)
  return true
}

// ---------------------------------------------------------------------------
// Tiptap Extension
// ---------------------------------------------------------------------------

/**
 * BlockKeyboardShortcuts — adds keyboard shortcuts for block-level operations:
 *   Mod-Shift-ArrowUp    Move block up
 *   Mod-Shift-ArrowDown  Move block down
 *   Mod-Shift-d          Duplicate block
 *   Mod-Shift-Delete     Delete block
 *   Mod-Enter            Save document (dispatches custom event)
 */
export const BlockKeyboardShortcuts = Extension.create({
  name: "blockKeyboardShortcuts",

  addKeyboardShortcuts() {
    return {
      "Mod-Shift-ArrowUp": () => moveBlockUp(this.editor),
      "Mod-Shift-ArrowDown": () => moveBlockDown(this.editor),
      "Mod-Shift-d": () => duplicateBlock(this.editor),
      "Mod-Shift-Delete": () => deleteBlock(this.editor),
      "Mod-Enter": () => {
        window.dispatchEvent(new CustomEvent("tiptap:save"))
        return true
      },
    }
  },
})

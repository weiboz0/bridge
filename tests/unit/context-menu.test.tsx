// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest"
import { render, screen, fireEvent, act } from "@testing-library/react"
import React from "react"

// ---------------------------------------------------------------------------
// Mock lucide-react icons (they just need to render something)
// ---------------------------------------------------------------------------
vi.mock("lucide-react", () => ({
  Scissors: () => React.createElement("span", { "data-icon": "scissors" }),
  Copy: () => React.createElement("span", { "data-icon": "copy" }),
  ClipboardPaste: () => React.createElement("span", { "data-icon": "paste" }),
  Trash2: () => React.createElement("span", { "data-icon": "trash" }),
  CopyPlus: () => React.createElement("span", { "data-icon": "copy-plus" }),
  MoveUp: () => React.createElement("span", { "data-icon": "move-up" }),
  MoveDown: () => React.createElement("span", { "data-icon": "move-down" }),
  ChevronRight: () => React.createElement("span", { "data-icon": "chevron-right" }),
}))

// ---------------------------------------------------------------------------
// Mock keyboard-shortcuts to avoid needing a real ProseMirror editor
// ---------------------------------------------------------------------------
const mockMoveBlockUp = vi.fn()
const mockMoveBlockDown = vi.fn()
const mockDuplicateBlock = vi.fn()
const mockDeleteBlock = vi.fn()

vi.mock("@/components/editor/tiptap/keyboard-shortcuts", () => ({
  moveBlockUp: (...args: any[]) => mockMoveBlockUp(...args),
  moveBlockDown: (...args: any[]) => mockMoveBlockDown(...args),
  duplicateBlock: (...args: any[]) => mockDuplicateBlock(...args),
  deleteBlock: (...args: any[]) => mockDeleteBlock(...args),
  BlockKeyboardShortcuts: { create: vi.fn() },
}))

// ---------------------------------------------------------------------------
// Mock @tiptap/pm/state to prevent TextSelection.near from crashing
// ---------------------------------------------------------------------------
vi.mock("@tiptap/pm/state", () => ({
  TextSelection: {
    near: vi.fn().mockReturnValue({}),
  },
}))

// ---------------------------------------------------------------------------
// Import after mocks
// ---------------------------------------------------------------------------
import { ContextMenu } from "@/components/editor/tiptap/context-menu"

// ---------------------------------------------------------------------------
// Minimal Editor mock — must satisfy what ContextMenu reads from editor.
// The contextmenu handler calls editor.view.posAtCoords, doc.resolve, and
// view.dispatch. We mock all of these.
// ---------------------------------------------------------------------------
function makeMockEditor() {
  const dom = document.createElement("div")
  dom.setAttribute("data-testid", "editor-dom")
  document.body.appendChild(dom)

  const mockSetSelection = vi.fn().mockReturnThis()
  const mockDispatch = vi.fn()

  const editor = {
    view: {
      dom,
      posAtCoords: vi.fn().mockReturnValue({ pos: 1 }),
      dispatch: mockDispatch,
    },
    state: {
      doc: {
        resolve: vi.fn().mockReturnValue({ depth: 1 }),
      },
      tr: {
        setSelection: mockSetSelection,
      },
      selection: { from: 0, to: 0 },
    },
    chain: vi.fn().mockReturnValue({
      focus: vi.fn().mockReturnThis(),
      insertContent: vi.fn().mockReturnThis(),
      setParagraph: vi.fn().mockReturnThis(),
      setHeading: vi.fn().mockReturnThis(),
      toggleBulletList: vi.fn().mockReturnThis(),
      toggleBlockquote: vi.fn().mockReturnThis(),
      toggleCodeBlock: vi.fn().mockReturnThis(),
      run: vi.fn(),
    }),
  } as unknown as import("@tiptap/react").Editor

  return { editor, dom, cleanup: () => dom.remove() }
}

// ---------------------------------------------------------------------------
// Helper to fire a contextmenu event on the editor DOM
// ---------------------------------------------------------------------------
function triggerContextMenu(dom: HTMLElement, x = 100, y = 200) {
  act(() => {
    fireEvent.contextMenu(dom, { clientX: x, clientY: y })
  })
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------
describe("ContextMenu — not rendered when no position", () => {
  it("renders nothing before a right-click", () => {
    const { editor, cleanup } = makeMockEditor()
    render(<ContextMenu editor={editor} />)
    // None of the menu items should be visible
    expect(screen.queryByText("Cut")).toBeNull()
    expect(screen.queryByText("Copy")).toBeNull()
    expect(screen.queryByText("Paste")).toBeNull()
    cleanup()
  })
})

describe("ContextMenu — renders after right-click", () => {
  let editor: ReturnType<typeof makeMockEditor>["editor"]
  let dom: HTMLElement
  let cleanup: () => void

  beforeEach(() => {
    const result = makeMockEditor()
    editor = result.editor
    dom = result.dom
    cleanup = result.cleanup
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it("appears after contextmenu event on editor DOM", () => {
    render(<ContextMenu editor={editor} />)
    triggerContextMenu(dom)
    expect(screen.getByText("Cut")).toBeInTheDocument()
  })

  it("renders Cut, Copy, Paste items", () => {
    render(<ContextMenu editor={editor} />)
    triggerContextMenu(dom)
    expect(screen.getByText("Cut")).toBeInTheDocument()
    expect(screen.getByText("Copy")).toBeInTheDocument()
    expect(screen.getByText("Paste")).toBeInTheDocument()
  })

  it("renders Delete Block and Duplicate Block items", () => {
    render(<ContextMenu editor={editor} />)
    triggerContextMenu(dom)
    expect(screen.getByText("Delete Block")).toBeInTheDocument()
    expect(screen.getByText("Duplicate Block")).toBeInTheDocument()
  })

  it("renders Move Up and Move Down items", () => {
    render(<ContextMenu editor={editor} />)
    triggerContextMenu(dom)
    expect(screen.getByText("Move Up")).toBeInTheDocument()
    expect(screen.getByText("Move Down")).toBeInTheDocument()
  })

  it("renders Turn Into submenu trigger", () => {
    render(<ContextMenu editor={editor} />)
    triggerContextMenu(dom)
    expect(screen.getByText("Turn Into...")).toBeInTheDocument()
  })

  it("is positioned at the right-click coordinates", () => {
    const { container } = render(<ContextMenu editor={editor} />)
    triggerContextMenu(dom, 150, 300)
    const menu = container.querySelector(".fixed") as HTMLElement
    expect(menu).toBeTruthy()
    expect(menu.style.left).toBe("150px")
    expect(menu.style.top).toBe("300px")
  })
})

describe("ContextMenu — closes on Escape", () => {
  it("closes when Escape key is pressed", () => {
    const { editor, dom, cleanup } = makeMockEditor()
    render(<ContextMenu editor={editor} />)
    triggerContextMenu(dom)
    expect(screen.getByText("Cut")).toBeInTheDocument()

    act(() => {
      fireEvent.keyDown(document, { key: "Escape" })
    })
    expect(screen.queryByText("Cut")).toBeNull()
    cleanup()
  })

  it("does not close on other key presses", () => {
    const { editor, dom, cleanup } = makeMockEditor()
    render(<ContextMenu editor={editor} />)
    triggerContextMenu(dom)
    act(() => {
      fireEvent.keyDown(document, { key: "Enter" })
    })
    expect(screen.getByText("Cut")).toBeInTheDocument()
    cleanup()
  })
})

describe("ContextMenu — closes on click outside", () => {
  it("closes when clicking outside the menu", () => {
    const { editor, dom, cleanup } = makeMockEditor()
    render(<ContextMenu editor={editor} />)
    triggerContextMenu(dom)
    expect(screen.getByText("Cut")).toBeInTheDocument()

    // Click outside
    const outside = document.createElement("div")
    document.body.appendChild(outside)
    act(() => {
      fireEvent.mouseDown(outside)
    })
    expect(screen.queryByText("Cut")).toBeNull()
    outside.remove()
    cleanup()
  })

  it("does NOT close when clicking inside the menu", () => {
    const { editor, dom, cleanup } = makeMockEditor()
    render(<ContextMenu editor={editor} />)
    triggerContextMenu(dom)
    const cutButton = screen.getByText("Cut")
    // Mousedown on the menu itself should not close it
    act(() => {
      fireEvent.mouseDown(cutButton)
    })
    // Menu stays open (Cut is still there until the click handler closes it)
    expect(screen.getByText("Copy")).toBeInTheDocument()
    cleanup()
  })
})

describe("ContextMenu — block operations", () => {
  let editor: ReturnType<typeof makeMockEditor>["editor"]
  let dom: HTMLElement
  let cleanup: () => void

  beforeEach(() => {
    const result = makeMockEditor()
    editor = result.editor
    dom = result.dom
    cleanup = result.cleanup
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it("clicking Delete Block calls deleteBlock and closes menu", () => {
    render(<ContextMenu editor={editor} />)
    triggerContextMenu(dom)
    act(() => {
      fireEvent.click(screen.getByText("Delete Block"))
    })
    expect(mockDeleteBlock).toHaveBeenCalledWith(editor)
    expect(screen.queryByText("Delete Block")).toBeNull()
  })

  it("clicking Duplicate Block calls duplicateBlock and closes menu", () => {
    render(<ContextMenu editor={editor} />)
    triggerContextMenu(dom)
    act(() => {
      fireEvent.click(screen.getByText("Duplicate Block"))
    })
    expect(mockDuplicateBlock).toHaveBeenCalledWith(editor)
    expect(screen.queryByText("Duplicate Block")).toBeNull()
  })

  it("clicking Move Up calls moveBlockUp and closes menu", () => {
    render(<ContextMenu editor={editor} />)
    triggerContextMenu(dom)
    act(() => {
      fireEvent.click(screen.getByText("Move Up"))
    })
    expect(mockMoveBlockUp).toHaveBeenCalledWith(editor)
    expect(screen.queryByText("Move Up")).toBeNull()
  })

  it("clicking Move Down calls moveBlockDown and closes menu", () => {
    render(<ContextMenu editor={editor} />)
    triggerContextMenu(dom)
    act(() => {
      fireEvent.click(screen.getByText("Move Down"))
    })
    expect(mockMoveBlockDown).toHaveBeenCalledWith(editor)
    expect(screen.queryByText("Move Down")).toBeNull()
  })
})

describe("ContextMenu — Turn Into submenu", () => {
  let editor: ReturnType<typeof makeMockEditor>["editor"]
  let dom: HTMLElement
  let cleanup: () => void

  beforeEach(() => {
    const result = makeMockEditor()
    editor = result.editor
    dom = result.dom
    cleanup = result.cleanup
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it("hovering Turn Into opens the submenu", () => {
    render(<ContextMenu editor={editor} />)
    triggerContextMenu(dom)
    act(() => {
      fireEvent.mouseEnter(screen.getByText("Turn Into..."))
    })
    expect(screen.getByText("Paragraph")).toBeInTheDocument()
    expect(screen.getByText("Heading 1")).toBeInTheDocument()
  })

  it("submenu is not visible before hovering", () => {
    render(<ContextMenu editor={editor} />)
    triggerContextMenu(dom)
    expect(screen.queryByText("Paragraph")).toBeNull()
  })

  it("clicking a Turn Into item closes the menu", () => {
    render(<ContextMenu editor={editor} />)
    triggerContextMenu(dom)
    act(() => {
      fireEvent.mouseEnter(screen.getByText("Turn Into..."))
    })
    act(() => {
      fireEvent.click(screen.getByText("Paragraph"))
    })
    expect(screen.queryByText("Turn Into...")).toBeNull()
  })
})

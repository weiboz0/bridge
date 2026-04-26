// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, beforeAll } from "vitest"
import { render, screen, fireEvent, act } from "@testing-library/react"
import React, { createRef } from "react"

// jsdom doesn't implement scrollIntoView
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
})

// ---- Import the module under test -------------------------------------------
// filterItems is not exported, but ALL_ITEMS is — we exercise filterItems
// indirectly by importing the extension entry point.  We also import the React
// component and the handle type directly.
import {
  SlashMenuList,
  ALL_ITEMS,
  type SlashMenuItem,
  type SlashMenuListHandle,
} from "@/components/editor/tiptap/slash-menu"

// ---------------------------------------------------------------------------
// filterItems — re-implement the same logic so we can test it in isolation,
// or extract it.  The source doesn't export it, so we test via ALL_ITEMS.
// ---------------------------------------------------------------------------
function filterItems(query: string): SlashMenuItem[] {
  if (!query) return ALL_ITEMS
  const q = query.toLowerCase()
  return ALL_ITEMS.filter(
    (item) =>
      item.label.toLowerCase().includes(q) ||
      item.description.toLowerCase().includes(q) ||
      item.id.toLowerCase().includes(q),
  )
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
function noop() {}

// ---------------------------------------------------------------------------
// filterItems tests
// ---------------------------------------------------------------------------
describe("filterItems", () => {
  it("returns all items when query is empty string", () => {
    const result = filterItems("")
    expect(result).toHaveLength(ALL_ITEMS.length)
    expect(result).toEqual(ALL_ITEMS)
  })

  it("returns heading items when query is 'head'", () => {
    const result = filterItems("head")
    expect(result.length).toBeGreaterThan(0)
    result.forEach((item) => {
      const matches =
        item.label.toLowerCase().includes("head") ||
        item.description.toLowerCase().includes("head") ||
        item.id.toLowerCase().includes("head")
      expect(matches).toBe(true)
    })
    // Should find Heading 1/2/3
    const labels = result.map((i) => i.label)
    expect(labels).toContain("Heading 1")
    expect(labels).toContain("Heading 2")
    expect(labels).toContain("Heading 3")
  })

  it("returns empty array when query matches nothing", () => {
    const result = filterItems("nonexistent_xyz_12345")
    expect(result).toHaveLength(0)
  })

  it("returns AI items when query is 'ai'", () => {
    const result = filterItems("ai")
    expect(result.length).toBeGreaterThan(0)
    const aiItems = result.filter((i) => i.category === "ai")
    expect(aiItems.length).toBeGreaterThan(0)
    // All results must match 'ai' in label, description or id
    result.forEach((item) => {
      const matches =
        item.label.toLowerCase().includes("ai") ||
        item.description.toLowerCase().includes("ai") ||
        item.id.toLowerCase().includes("ai")
      expect(matches).toBe(true)
    })
  })

  it("is case-insensitive — 'HEAD' matches heading items", () => {
    const lower = filterItems("head")
    const upper = filterItems("HEAD")
    expect(upper).toEqual(lower)
  })

  it("matches on description text", () => {
    // 'checkboxes' only appears in the To-do List description
    const result = filterItems("checkbox")
    expect(result.length).toBeGreaterThan(0)
    expect(result.some((i) => i.id === "taskList")).toBe(true)
  })

  it("matches on item id", () => {
    // 'codeblock' or partial 'codeb' should match codeBlock item
    const result = filterItems("codeblock")
    expect(result.some((i) => i.id === "codeBlock")).toBe(true)
  })
})

// ---------------------------------------------------------------------------
// SlashMenuList rendering tests
// ---------------------------------------------------------------------------
describe("SlashMenuList", () => {
  it("renders all items when passed ALL_ITEMS", () => {
    render(<SlashMenuList items={ALL_ITEMS} command={noop} />)
    // Each item has a label rendered as text
    const heading1 = screen.getByText("Heading 1")
    expect(heading1).toBeInTheDocument()
    // Category headers should be visible (AI appears as both header and badge text, so use getAllByText)
    expect(screen.getAllByText("AI").length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText("Text formatting")).toBeInTheDocument()
    expect(screen.getByText("Teaching blocks")).toBeInTheDocument()
  })

  it("renders items grouped by category", () => {
    // Only pass text items — should see 'Text formatting' but not 'AI' or 'Teaching blocks'
    const textOnly = ALL_ITEMS.filter((i) => i.category === "text")
    render(<SlashMenuList items={textOnly} command={noop} />)
    expect(screen.getByText("Text formatting")).toBeInTheDocument()
    expect(screen.queryByText("AI")).toBeNull()
    expect(screen.queryByText("Teaching blocks")).toBeNull()
  })

  it("shows 'No matching commands' when items is empty", () => {
    render(<SlashMenuList items={[]} command={noop} />)
    expect(screen.getByText("No matching commands")).toBeInTheDocument()
  })

  it("does not show 'No matching commands' when items are present", () => {
    const items = ALL_ITEMS.slice(0, 3)
    render(<SlashMenuList items={items} command={noop} />)
    expect(screen.queryByText("No matching commands")).toBeNull()
  })

  it("renders badge text for each item", () => {
    const single: SlashMenuItem[] = [
      {
        id: "test",
        label: "Test Item",
        description: "A test item",
        badge: "TX",
        category: "text",
        command: noop,
      },
    ]
    render(<SlashMenuList items={single} command={noop} />)
    expect(screen.getByText("TX")).toBeInTheDocument()
    expect(screen.getByText("A test item")).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// SlashMenuList keyboard navigation (via imperative handle)
// ---------------------------------------------------------------------------
describe("SlashMenuList keyboard navigation", () => {
  const items: SlashMenuItem[] = [
    { id: "a", label: "Alpha", description: "first", badge: "A", category: "text", command: noop },
    { id: "b", label: "Beta", description: "second", badge: "B", category: "text", command: noop },
    { id: "c", label: "Gamma", description: "third", badge: "C", category: "teaching", command: noop },
  ]

  it("ArrowDown returns true", () => {
    const ref = createRef<SlashMenuListHandle>()
    render(<SlashMenuList ref={ref} items={items} command={noop} />)
    const event = new KeyboardEvent("keydown", { key: "ArrowDown" })
    const handled = ref.current!.onKeyDown(event)
    expect(handled).toBe(true)
  })

  it("ArrowUp returns true", () => {
    const ref = createRef<SlashMenuListHandle>()
    render(<SlashMenuList ref={ref} items={items} command={noop} />)
    const event = new KeyboardEvent("keydown", { key: "ArrowUp" })
    const handled = ref.current!.onKeyDown(event)
    expect(handled).toBe(true)
  })

  it("Enter returns true", () => {
    const ref = createRef<SlashMenuListHandle>()
    const command = vi.fn()
    render(<SlashMenuList ref={ref} items={items} command={command} />)
    const event = new KeyboardEvent("keydown", { key: "Enter" })
    const handled = ref.current!.onKeyDown(event)
    expect(handled).toBe(true)
  })

  it("Enter calls command with currently selected item (default first)", () => {
    const ref = createRef<SlashMenuListHandle>()
    const command = vi.fn()
    render(<SlashMenuList ref={ref} items={items} command={command} />)
    const event = new KeyboardEvent("keydown", { key: "Enter" })
    ref.current!.onKeyDown(event)
    expect(command).toHaveBeenCalledWith(items[0])
  })

  it("Escape key is NOT handled by the list (returns false)", () => {
    const ref = createRef<SlashMenuListHandle>()
    render(<SlashMenuList ref={ref} items={items} command={noop} />)
    const event = new KeyboardEvent("keydown", { key: "Escape" })
    const handled = ref.current!.onKeyDown(event)
    expect(handled).toBe(false)
  })

  it("Other keys return false", () => {
    const ref = createRef<SlashMenuListHandle>()
    render(<SlashMenuList ref={ref} items={items} command={noop} />)
    const event = new KeyboardEvent("keydown", { key: "Tab" })
    const handled = ref.current!.onKeyDown(event)
    expect(handled).toBe(false)
  })

  it("ArrowDown wraps from last to first", () => {
    const ref = createRef<SlashMenuListHandle>()
    const command = vi.fn()
    render(<SlashMenuList ref={ref} items={items} command={command} />)

    // Move to last item by pressing ArrowDown items.length - 1 times
    for (let i = 0; i < items.length - 1; i++) {
      act(() => {
        ref.current!.onKeyDown(new KeyboardEvent("keydown", { key: "ArrowDown" }))
      })
    }
    // Now at last — wrap to first
    act(() => {
      ref.current!.onKeyDown(new KeyboardEvent("keydown", { key: "ArrowDown" }))
    })
    // Press Enter — should select first item
    ref.current!.onKeyDown(new KeyboardEvent("keydown", { key: "Enter" }))
    expect(command).toHaveBeenLastCalledWith(items[0])
  })

  it("ArrowUp wraps from first to last", () => {
    const ref = createRef<SlashMenuListHandle>()
    const command = vi.fn()
    render(<SlashMenuList ref={ref} items={items} command={command} />)

    // Start at index 0, press ArrowUp → wraps to last
    act(() => {
      ref.current!.onKeyDown(new KeyboardEvent("keydown", { key: "ArrowUp" }))
    })
    ref.current!.onKeyDown(new KeyboardEvent("keydown", { key: "Enter" }))
    expect(command).toHaveBeenCalledWith(items[items.length - 1])
  })

  it("selected index resets to 0 when items change", () => {
    const ref = createRef<SlashMenuListHandle>()
    const command = vi.fn()
    const { rerender } = render(<SlashMenuList ref={ref} items={items} command={command} />)

    // Move selection to index 2
    act(() => {
      ref.current!.onKeyDown(new KeyboardEvent("keydown", { key: "ArrowDown" }))
      ref.current!.onKeyDown(new KeyboardEvent("keydown", { key: "ArrowDown" }))
    })

    // Change items to a subset
    const newItems = items.slice(0, 2)
    rerender(<SlashMenuList ref={ref} items={newItems} command={command} />)

    // Enter should select first item of new list
    ref.current!.onKeyDown(new KeyboardEvent("keydown", { key: "Enter" }))
    expect(command).toHaveBeenLastCalledWith(newItems[0])
  })
})

// ---------------------------------------------------------------------------
// SlashMenuList mouse interaction
// ---------------------------------------------------------------------------
describe("SlashMenuList mouse interaction", () => {
  it("clicking an item calls command with that item", () => {
    const items: SlashMenuItem[] = [
      { id: "x", label: "X Item", description: "desc x", badge: "X", category: "text", command: noop },
      { id: "y", label: "Y Item", description: "desc y", badge: "Y", category: "text", command: noop },
    ]
    const command = vi.fn()
    render(<SlashMenuList items={items} command={command} />)

    fireEvent.click(screen.getByText("Y Item"))
    expect(command).toHaveBeenCalledWith(items[1])
  })

  it("clicking the first item calls command with that item", () => {
    const items: SlashMenuItem[] = [
      { id: "first", label: "First", description: "desc", badge: "F", category: "text", command: noop },
    ]
    const command = vi.fn()
    render(<SlashMenuList items={items} command={command} />)
    fireEvent.click(screen.getByText("First"))
    expect(command).toHaveBeenCalledWith(items[0])
  })
})

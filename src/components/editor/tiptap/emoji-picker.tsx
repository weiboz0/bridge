"use client"

import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
  useState,
} from "react"
import { createPortal } from "react-dom"
import { createRoot } from "react-dom/client"
import { Extension } from "@tiptap/core"
import Suggestion, {
  type SuggestionOptions,
  type SuggestionProps,
  type SuggestionKeyDownProps,
} from "@tiptap/suggestion"
import type { Editor, Range } from "@tiptap/core"
import { PluginKey } from "@tiptap/pm/state"

// ---------------------------------------------------------------------------
// Bundled emoji dataset (~200 common emoji with shortcodes)
// ---------------------------------------------------------------------------

interface EmojiEntry {
  shortcode: string
  emoji: string
}

const EMOJI_DATA: EmojiEntry[] = [
  // Smileys & people
  { shortcode: "smile", emoji: "\u{1F604}" },
  { shortcode: "smiley", emoji: "\u{1F603}" },
  { shortcode: "grinning", emoji: "\u{1F600}" },
  { shortcode: "grin", emoji: "\u{1F601}" },
  { shortcode: "laughing", emoji: "\u{1F606}" },
  { shortcode: "sweat_smile", emoji: "\u{1F605}" },
  { shortcode: "rofl", emoji: "\u{1F923}" },
  { shortcode: "joy", emoji: "\u{1F602}" },
  { shortcode: "wink", emoji: "\u{1F609}" },
  { shortcode: "blush", emoji: "\u{1F60A}" },
  { shortcode: "innocent", emoji: "\u{1F607}" },
  { shortcode: "heart_eyes", emoji: "\u{1F60D}" },
  { shortcode: "kissing_heart", emoji: "\u{1F618}" },
  { shortcode: "kissing", emoji: "\u{1F617}" },
  { shortcode: "relaxed", emoji: "☺️" },
  { shortcode: "stuck_out_tongue", emoji: "\u{1F61B}" },
  { shortcode: "stuck_out_tongue_winking_eye", emoji: "\u{1F61C}" },
  { shortcode: "yum", emoji: "\u{1F60B}" },
  { shortcode: "sunglasses", emoji: "\u{1F60E}" },
  { shortcode: "thinking", emoji: "\u{1F914}" },
  { shortcode: "neutral_face", emoji: "\u{1F610}" },
  { shortcode: "expressionless", emoji: "\u{1F611}" },
  { shortcode: "unamused", emoji: "\u{1F612}" },
  { shortcode: "rolling_eyes", emoji: "\u{1F644}" },
  { shortcode: "grimacing", emoji: "\u{1F62C}" },
  { shortcode: "relieved", emoji: "\u{1F60C}" },
  { shortcode: "pensive", emoji: "\u{1F614}" },
  { shortcode: "sleepy", emoji: "\u{1F62A}" },
  { shortcode: "sleeping", emoji: "\u{1F634}" },
  { shortcode: "mask", emoji: "\u{1F637}" },
  { shortcode: "nerd", emoji: "\u{1F913}" },
  { shortcode: "confused", emoji: "\u{1F615}" },
  { shortcode: "worried", emoji: "\u{1F61F}" },
  { shortcode: "frown", emoji: "☹️" },
  { shortcode: "open_mouth", emoji: "\u{1F62E}" },
  { shortcode: "hushed", emoji: "\u{1F62F}" },
  { shortcode: "astonished", emoji: "\u{1F632}" },
  { shortcode: "flushed", emoji: "\u{1F633}" },
  { shortcode: "scream", emoji: "\u{1F631}" },
  { shortcode: "fearful", emoji: "\u{1F628}" },
  { shortcode: "cold_sweat", emoji: "\u{1F630}" },
  { shortcode: "cry", emoji: "\u{1F622}" },
  { shortcode: "sob", emoji: "\u{1F62D}" },
  { shortcode: "angry", emoji: "\u{1F620}" },
  { shortcode: "rage", emoji: "\u{1F621}" },
  { shortcode: "triumph", emoji: "\u{1F624}" },
  { shortcode: "skull", emoji: "\u{1F480}" },
  { shortcode: "poop", emoji: "\u{1F4A9}" },
  { shortcode: "clown", emoji: "\u{1F921}" },
  { shortcode: "ghost", emoji: "\u{1F47B}" },
  { shortcode: "alien", emoji: "\u{1F47D}" },
  { shortcode: "robot", emoji: "\u{1F916}" },
  // Gestures & body
  { shortcode: "wave", emoji: "\u{1F44B}" },
  { shortcode: "ok_hand", emoji: "\u{1F44C}" },
  { shortcode: "thumbsup", emoji: "\u{1F44D}" },
  { shortcode: "+1", emoji: "\u{1F44D}" },
  { shortcode: "thumbsdown", emoji: "\u{1F44E}" },
  { shortcode: "-1", emoji: "\u{1F44E}" },
  { shortcode: "clap", emoji: "\u{1F44F}" },
  { shortcode: "pray", emoji: "\u{1F64F}" },
  { shortcode: "handshake", emoji: "\u{1F91D}" },
  { shortcode: "muscle", emoji: "\u{1F4AA}" },
  { shortcode: "point_up", emoji: "☝️" },
  { shortcode: "point_down", emoji: "\u{1F447}" },
  { shortcode: "point_left", emoji: "\u{1F448}" },
  { shortcode: "point_right", emoji: "\u{1F449}" },
  { shortcode: "raised_hand", emoji: "✋" },
  { shortcode: "v", emoji: "✌️" },
  { shortcode: "crossed_fingers", emoji: "\u{1F91E}" },
  { shortcode: "eyes", emoji: "\u{1F440}" },
  { shortcode: "brain", emoji: "\u{1F9E0}" },
  // Hearts & symbols
  { shortcode: "heart", emoji: "❤️" },
  { shortcode: "red_heart", emoji: "❤️" },
  { shortcode: "orange_heart", emoji: "\u{1F9E1}" },
  { shortcode: "yellow_heart", emoji: "\u{1F49B}" },
  { shortcode: "green_heart", emoji: "\u{1F49A}" },
  { shortcode: "blue_heart", emoji: "\u{1F499}" },
  { shortcode: "purple_heart", emoji: "\u{1F49C}" },
  { shortcode: "broken_heart", emoji: "\u{1F494}" },
  { shortcode: "sparkling_heart", emoji: "\u{1F496}" },
  { shortcode: "star", emoji: "⭐" },
  { shortcode: "sparkles", emoji: "✨" },
  { shortcode: "fire", emoji: "\u{1F525}" },
  { shortcode: "100", emoji: "\u{1F4AF}" },
  { shortcode: "tada", emoji: "\u{1F389}" },
  { shortcode: "confetti_ball", emoji: "\u{1F38A}" },
  { shortcode: "party_popper", emoji: "\u{1F389}" },
  { shortcode: "balloon", emoji: "\u{1F388}" },
  { shortcode: "trophy", emoji: "\u{1F3C6}" },
  { shortcode: "medal", emoji: "\u{1F3C5}" },
  { shortcode: "rocket", emoji: "\u{1F680}" },
  { shortcode: "airplane", emoji: "✈️" },
  // Objects & education
  { shortcode: "book", emoji: "\u{1F4D6}" },
  { shortcode: "books", emoji: "\u{1F4DA}" },
  { shortcode: "pencil", emoji: "✏️" },
  { shortcode: "memo", emoji: "\u{1F4DD}" },
  { shortcode: "bulb", emoji: "\u{1F4A1}" },
  { shortcode: "magnifying_glass", emoji: "\u{1F50D}" },
  { shortcode: "computer", emoji: "\u{1F4BB}" },
  { shortcode: "keyboard", emoji: "⌨️" },
  { shortcode: "gear", emoji: "⚙️" },
  { shortcode: "wrench", emoji: "\u{1F527}" },
  { shortcode: "hammer", emoji: "\u{1F528}" },
  { shortcode: "link", emoji: "\u{1F517}" },
  { shortcode: "paperclip", emoji: "\u{1F4CE}" },
  { shortcode: "scissors", emoji: "✂️" },
  { shortcode: "lock", emoji: "\u{1F512}" },
  { shortcode: "key", emoji: "\u{1F511}" },
  { shortcode: "bell", emoji: "\u{1F514}" },
  { shortcode: "calendar", emoji: "\u{1F4C5}" },
  { shortcode: "clock", emoji: "\u{1F552}" },
  { shortcode: "hourglass", emoji: "⌛" },
  { shortcode: "chart_increasing", emoji: "\u{1F4C8}" },
  { shortcode: "chart_decreasing", emoji: "\u{1F4C9}" },
  // Nature & weather
  { shortcode: "sun", emoji: "☀️" },
  { shortcode: "moon", emoji: "\u{1F319}" },
  { shortcode: "cloud", emoji: "☁️" },
  { shortcode: "rainbow", emoji: "\u{1F308}" },
  { shortcode: "umbrella", emoji: "☔" },
  { shortcode: "snowflake", emoji: "❄️" },
  { shortcode: "zap", emoji: "⚡" },
  { shortcode: "droplet", emoji: "\u{1F4A7}" },
  { shortcode: "tree", emoji: "\u{1F333}" },
  { shortcode: "flower", emoji: "\u{1F33B}" },
  { shortcode: "seedling", emoji: "\u{1F331}" },
  { shortcode: "leaf", emoji: "\u{1F343}" },
  // Animals
  { shortcode: "dog", emoji: "\u{1F436}" },
  { shortcode: "cat", emoji: "\u{1F431}" },
  { shortcode: "bear", emoji: "\u{1F43B}" },
  { shortcode: "panda", emoji: "\u{1F43C}" },
  { shortcode: "penguin", emoji: "\u{1F427}" },
  { shortcode: "bird", emoji: "\u{1F426}" },
  { shortcode: "butterfly", emoji: "\u{1F98B}" },
  { shortcode: "bug", emoji: "\u{1F41B}" },
  { shortcode: "turtle", emoji: "\u{1F422}" },
  { shortcode: "snake", emoji: "\u{1F40D}" },
  { shortcode: "fish", emoji: "\u{1F41F}" },
  { shortcode: "octopus", emoji: "\u{1F419}" },
  // Food & drink
  { shortcode: "apple", emoji: "\u{1F34E}" },
  { shortcode: "pizza", emoji: "\u{1F355}" },
  { shortcode: "hamburger", emoji: "\u{1F354}" },
  { shortcode: "coffee", emoji: "☕" },
  { shortcode: "tea", emoji: "\u{1F375}" },
  { shortcode: "cake", emoji: "\u{1F382}" },
  { shortcode: "cookie", emoji: "\u{1F36A}" },
  { shortcode: "ice_cream", emoji: "\u{1F368}" },
  // Status / check marks
  { shortcode: "white_check_mark", emoji: "✅" },
  { shortcode: "check", emoji: "✔️" },
  { shortcode: "x", emoji: "❌" },
  { shortcode: "warning", emoji: "⚠️" },
  { shortcode: "question", emoji: "❓" },
  { shortcode: "exclamation", emoji: "❗" },
  { shortcode: "info", emoji: "ℹ️" },
  { shortcode: "no_entry", emoji: "⛔" },
  // Arrows & navigation
  { shortcode: "arrow_up", emoji: "⬆️" },
  { shortcode: "arrow_down", emoji: "⬇️" },
  { shortcode: "arrow_left", emoji: "⬅️" },
  { shortcode: "arrow_right", emoji: "➡️" },
  { shortcode: "repeat", emoji: "\u{1F501}" },
  { shortcode: "back", emoji: "\u{1F519}" },
  // Misc
  { shortcode: "flag", emoji: "\u{1F3F4}" },
  { shortcode: "earth", emoji: "\u{1F30D}" },
  { shortcode: "wave_emoji", emoji: "\u{1F30A}" },
  { shortcode: "mountain", emoji: "⛰️" },
  { shortcode: "house", emoji: "\u{1F3E0}" },
  { shortcode: "school", emoji: "\u{1F3EB}" },
  { shortcode: "hospital", emoji: "\u{1F3E5}" },
  { shortcode: "car", emoji: "\u{1F697}" },
  { shortcode: "bike", emoji: "\u{1F6B2}" },
  { shortcode: "crown", emoji: "\u{1F451}" },
  { shortcode: "gem", emoji: "\u{1F48E}" },
  { shortcode: "gift", emoji: "\u{1F381}" },
  { shortcode: "music", emoji: "\u{1F3B5}" },
  { shortcode: "art", emoji: "\u{1F3A8}" },
  { shortcode: "movie", emoji: "\u{1F3AC}" },
  { shortcode: "mic", emoji: "\u{1F3A4}" },
  { shortcode: "game", emoji: "\u{1F3AE}" },
  { shortcode: "dice", emoji: "\u{1F3B2}" },
  { shortcode: "puzzle", emoji: "\u{1F9E9}" },
  { shortcode: "target", emoji: "\u{1F3AF}" },
]

// ---------------------------------------------------------------------------
// Filter
// ---------------------------------------------------------------------------

function filterEmoji(query: string): EmojiEntry[] {
  if (!query) return EMOJI_DATA.slice(0, 10) // show first 10 when no query
  const q = query.toLowerCase()
  return EMOJI_DATA.filter((e) => e.shortcode.includes(q)).slice(0, 10)
}

// ---------------------------------------------------------------------------
// Floating emoji list component
// ---------------------------------------------------------------------------

interface EmojiListProps {
  items: EmojiEntry[]
  command: (item: EmojiEntry) => void
}

export interface EmojiListHandle {
  onKeyDown: (event: KeyboardEvent) => boolean
}

export const EmojiList = forwardRef<EmojiListHandle, EmojiListProps>(
  function EmojiList({ items, command }, ref) {
    const [selectedIndex, setSelectedIndex] = useState(0)
    const containerRef = useRef<HTMLDivElement>(null)

    useEffect(() => {
      setSelectedIndex(0)
    }, [items])

    useEffect(() => {
      const container = containerRef.current
      if (!container) return
      const selected = container.querySelector("[data-selected=true]")
      if (selected) {
        selected.scrollIntoView({ block: "nearest" })
      }
    }, [selectedIndex])

    const selectItem = useCallback(
      (index: number) => {
        const item = items[index]
        if (item) command(item)
      },
      [items, command],
    )

    useImperativeHandle(ref, () => ({
      onKeyDown: (event: KeyboardEvent) => {
        if (event.key === "ArrowUp") {
          setSelectedIndex((prev) => (prev <= 0 ? items.length - 1 : prev - 1))
          return true
        }
        if (event.key === "ArrowDown") {
          setSelectedIndex((prev) => (prev >= items.length - 1 ? 0 : prev + 1))
          return true
        }
        if (event.key === "Enter") {
          selectItem(selectedIndex)
          return true
        }
        return false
      },
    }))

    if (items.length === 0) {
      return (
        <div className="rounded-lg border border-zinc-200 bg-white px-3 py-3 text-center text-sm text-zinc-400 shadow-lg">
          No matching emoji
        </div>
      )
    }

    return (
      <div
        ref={containerRef}
        className="max-h-60 w-56 overflow-y-auto rounded-lg border border-zinc-200 bg-white py-1 shadow-lg"
      >
        {items.map((item, idx) => (
          <button
            key={item.shortcode}
            type="button"
            data-selected={idx === selectedIndex}
            className={
              "flex w-full items-center gap-2.5 px-3 py-1.5 text-left transition-colors " +
              (idx === selectedIndex ? "bg-zinc-100" : "hover:bg-zinc-50")
            }
            onClick={() => selectItem(idx)}
            onMouseEnter={() => setSelectedIndex(idx)}
          >
            <span className="text-lg leading-none">{item.emoji}</span>
            <span className="text-sm text-zinc-600">:{item.shortcode}:</span>
          </button>
        ))}
      </div>
    )
  },
)
EmojiList.displayName = "EmojiList"

// ---------------------------------------------------------------------------
// Floating wrapper (portal-based positioning)
// ---------------------------------------------------------------------------

interface FloatingWrapperProps {
  clientRect: (() => DOMRect | null) | null
  children: React.ReactNode
}

function FloatingWrapper({ clientRect, children }: FloatingWrapperProps) {
  const wrapperRef = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    const el = wrapperRef.current
    if (!el || !clientRect) return

    const rect = clientRect()
    if (!rect) return

    el.style.position = "fixed"
    el.style.left = `${rect.left}px`
    el.style.top = `${rect.bottom + 4}px`
    el.style.zIndex = "50"
  })

  return createPortal(
    <div ref={wrapperRef} style={{ position: "fixed" }}>
      {children}
    </div>,
    document.body,
  )
}

// ---------------------------------------------------------------------------
// Suggestion render adapter
// ---------------------------------------------------------------------------

function createEmojiRenderer() {
  let component: {
    ref: EmojiListHandle | null
    updateProps: (props: SuggestionProps<EmojiEntry, EmojiEntry>) => void
    destroy: () => void
  } | null = null

  return {
    onStart(props: SuggestionProps<EmojiEntry, EmojiEntry>) {
      const container = document.createElement("div")
      document.body.appendChild(container)
      const root = createRoot(container)

      let currentRef: EmojiListHandle | null = null

      const renderWith = (p: SuggestionProps<EmojiEntry, EmojiEntry>) => {
        root.render(
          <FloatingWrapper clientRect={p.clientRect ?? null}>
            <EmojiList
              ref={(r) => { currentRef = r }}
              items={p.items}
              command={p.command}
            />
          </FloatingWrapper>,
        )
      }

      renderWith(props)

      component = {
        get ref() { return currentRef },
        updateProps: renderWith,
        destroy: () => { root.unmount(); container.remove() },
      }
    },

    onUpdate(props: SuggestionProps<EmojiEntry, EmojiEntry>) {
      component?.updateProps(props)
    },

    onKeyDown(props: SuggestionKeyDownProps) {
      if (props.event.key === "Escape") return true
      return component?.ref?.onKeyDown(props.event) ?? false
    },

    onExit() {
      component?.destroy()
      component = null
    },
  }
}

// ---------------------------------------------------------------------------
// Tiptap Extension
// ---------------------------------------------------------------------------

const emojiPluginKey = new PluginKey("emojiSuggestion")

export const EmojiPickerExtension = Extension.create({
  name: "emojiPicker",

  addOptions() {
    return {
      suggestion: {
        char: ":",
        startOfLine: false,
        allowSpaces: false,
        allowedPrefixes: null,
        pluginKey: emojiPluginKey,
        command: ({
          editor,
          range,
          props: item,
        }: {
          editor: Editor
          range: Range
          props: EmojiEntry
        }) => {
          editor
            .chain()
            .focus()
            .deleteRange(range)
            .insertContent(item.emoji)
            .run()
        },
        items: ({ query }: { query: string }) => filterEmoji(query),
        render: () => createEmojiRenderer(),
      } satisfies Omit<SuggestionOptions<EmojiEntry, EmojiEntry>, "editor">,
    }
  },

  addProseMirrorPlugins() {
    return [
      Suggestion<EmojiEntry, EmojiEntry>({
        editor: this.editor,
        ...this.options.suggestion,
      }),
    ]
  },
})

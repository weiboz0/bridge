import type { AnyExtension } from "@tiptap/react"
import StarterKit from "@tiptap/starter-kit"

import { ProblemRefNode } from "./problem-ref-node"

export function teachingUnitExtensions(): AnyExtension[] {
  return [
    StarterKit.configure({
      heading: { levels: [1, 2, 3] },
    }),
    ProblemRefNode,
  ] as AnyExtension[]
}

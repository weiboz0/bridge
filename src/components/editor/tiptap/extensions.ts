import StarterKit from "@tiptap/starter-kit"

import { ProblemRefNode } from "./problem-ref-node"

export function teachingUnitExtensions() {
  const starterKit = StarterKit as {
    configure: (options: {
      heading: {
        levels: number[]
      }
    }) => unknown
  }

  return [
    starterKit.configure({
      heading: {
        levels: [1, 2, 3],
      },
    }),
    ProblemRefNode,
  ]
}

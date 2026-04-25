import type { AnyExtension } from "@tiptap/react"
import StarterKit from "@tiptap/starter-kit"

import { ProblemRefNode } from "./problem-ref-node"
import { TeacherNoteNode } from "./teacher-note-node"
import { CodeSnippetNode } from "./code-snippet-node"
import { MediaEmbedNode } from "./media-embed-node"
import { SolutionRefNode } from "./solution-ref-node"
import { TestCaseRefNode } from "./test-case-ref-node"
import { LiveCueNode } from "./live-cue-node"
import { AssignmentVariantNode } from "./assignment-variant-node"

export function teachingUnitExtensions(): AnyExtension[] {
  return [
    StarterKit.configure({
      heading: { levels: [1, 2, 3] },
    }),
    ProblemRefNode,
    TeacherNoteNode,
    CodeSnippetNode,
    MediaEmbedNode,
    SolutionRefNode,
    TestCaseRefNode,
    LiveCueNode,
    AssignmentVariantNode,
  ] as AnyExtension[]
}

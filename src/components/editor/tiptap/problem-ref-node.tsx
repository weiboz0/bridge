"use client"

import { useEffect, useState } from "react"
import { nanoid } from "nanoid"
import { Node, mergeAttributes } from "@tiptap/core"
import {
  NodeViewWrapper,
  ReactNodeViewRenderer,
  type NodeViewProps,
} from "@tiptap/react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"

type ProblemVisibility = "always" | "when-unit-active"

type ProblemRefNodeAttrs = {
  id: string
  problemId: string
  pinnedRevision: string | null
  visibility: ProblemVisibility
  overrideStarter: string | null
}

type ProblemSummary = {
  title?: string
  difficulty?: string
  tags?: string[]
}

function ProblemRefNodeView({ node }: NodeViewProps) {
  const { problemId } = node.attrs as ProblemRefNodeAttrs
  const [problem, setProblem] = useState<ProblemSummary | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  useEffect(() => {
    let canceled = false

    const fetchProblem = async () => {
      if (!problemId) {
        if (!canceled) {
          setProblem(null)
          setError(true)
          setLoading(false)
        }
        return
      }

      setLoading(true)
      setError(false)
      try {
        const response = await fetch(`/api/problems/${problemId}`)
        if (!response.ok) {
          throw new Error("Problem unavailable")
        }

        const payload = (await response.json()) as ProblemSummary
        if (canceled) {
          return
        }

        setProblem({
          title: typeof payload.title === "string" ? payload.title : "",
          difficulty: typeof payload.difficulty === "string" ? payload.difficulty : "",
          tags: Array.isArray(payload.tags) ? payload.tags.filter((tag) => typeof tag === "string") : [],
        })
      } catch (fetchError) {
        if (!canceled) {
          setProblem(null)
          setError(true)
        }
      } finally {
        if (!canceled) {
          setLoading(false)
        }
      }
    }

    fetchProblem()

    return () => {
      canceled = true
    }
  }, [problemId])

  if (loading) {
    return (
      <NodeViewWrapper className="problem-ref-node">
        <Card className="border-zinc-200 bg-zinc-50">
          <CardHeader className="pb-2">
            <CardTitle className="text-zinc-900">Loading problem</CardTitle>
            <CardDescription>Fetching problem data</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            <div className="h-4 w-2/3 animate-pulse rounded bg-zinc-200" />
            <div className="h-3 w-1/3 animate-pulse rounded bg-zinc-200" />
            <div className="mt-2 h-6 w-20 animate-pulse rounded bg-zinc-200" />
          </CardContent>
        </Card>
      </NodeViewWrapper>
    )
  }

  if (error) {
    return (
      <NodeViewWrapper className="problem-ref-node">
        <Card className="border-zinc-200 bg-zinc-50">
          <CardHeader>
            <CardTitle className="text-zinc-900">Problem unavailable</CardTitle>
            <CardDescription>Problem {problemId} could not be loaded.</CardDescription>
          </CardHeader>
        </Card>
      </NodeViewWrapper>
    )
  }

  return (
    <NodeViewWrapper className="problem-ref-node">
      <Card className="border-zinc-200 bg-zinc-50">
        <CardHeader>
          <CardTitle className="text-zinc-900">{problem?.title || "Untitled problem"}</CardTitle>
          <CardDescription className="text-zinc-600">Problem ID: {problemId}</CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-zinc-700">Difficulty: {problem?.difficulty || "Unknown"}</p>
          <div className="mt-2 flex flex-wrap gap-2">
            {(problem?.tags ?? []).map((tag) => (
              <span
                key={tag}
                className="rounded-full border border-zinc-200 bg-zinc-100 px-2 py-1 text-xs text-zinc-700"
              >
                {tag}
              </span>
            ))}
          </div>
        </CardContent>
      </Card>
    </NodeViewWrapper>
  )
}

export const ProblemRefNode = Node.create({
  name: "problem-ref",
  group: "block",
  atom: true,
  addAttributes() {
    return {
      id: {
        default: () => nanoid(),
      },
      problemId: {
        default: "",
      },
      pinnedRevision: {
        default: null,
      },
      visibility: {
        default: "always",
        parseHTML: (element: Element) => {
          const visibility = element.getAttribute("data-problem-ref-visibility")
          if (visibility === "always" || visibility === "when-unit-active") {
            return visibility
          }
          return "always"
        },
      },
      overrideStarter: {
        default: null,
      },
    }
  },
  parseHTML() {
    return [
      {
        tag: 'div[data-type="problem-ref"]',
      },
    ]
  },
  renderHTML({
    node,
    HTMLAttributes,
  }: {
    node: { attrs: ProblemRefNodeAttrs }
    HTMLAttributes: Record<string, unknown>
  }) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-type": "problem-ref",
        "data-problem-ref-id": node.attrs.id,
        "data-problem-ref-problem-id": node.attrs.problemId,
        "data-problem-ref-pinned-revision": node.attrs.pinnedRevision,
        "data-problem-ref-visibility": node.attrs.visibility,
        "data-problem-ref-override-starter": node.attrs.overrideStarter,
      }),
    ]
  },
  addNodeView() {
    return ReactNodeViewRenderer(ProblemRefNodeView)
  },
})

"use client";

import type { LessonContent, ContentBlock } from "@/lib/lesson-content";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { useState } from "react";

interface LessonRendererProps {
  content: LessonContent;
}

export function LessonRenderer({ content }: LessonRendererProps) {
  if (content.blocks.length === 0) {
    return <p className="text-sm text-muted-foreground italic">No lesson content yet.</p>;
  }

  return (
    <div className="space-y-4">
      {content.blocks.map((block, i) => (
        <BlockRenderer key={i} block={block} />
      ))}
    </div>
  );
}

function BlockRenderer({ block }: { block: ContentBlock }) {
  switch (block.type) {
    case "markdown":
      return <MarkdownBlock content={block.content} />;
    case "code":
      return <CodeBlock language={block.language} content={block.content} />;
    case "image":
      return <ImageBlock url={block.url} alt={block.alt} />;
    default:
      return null;
  }
}

function MarkdownBlock({ content }: { content: string }) {
  return (
    <div className="prose prose-sm dark:prose-invert max-w-none">
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
    </div>
  );
}

function CodeBlock({ language, content }: { language: string; content: string }) {
  const [copied, setCopied] = useState(false);

  function handleCopy() {
    navigator.clipboard.writeText(content);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <div className="relative group">
      <div className="flex items-center justify-between px-3 py-1 bg-zinc-800 rounded-t-lg text-xs text-zinc-400">
        <span>{language}</span>
        <button
          onClick={handleCopy}
          className="opacity-0 group-hover:opacity-100 transition-opacity text-zinc-400 hover:text-white"
        >
          {copied ? "Copied!" : "Copy"}
        </button>
      </div>
      <pre className="bg-zinc-900 text-zinc-100 p-3 rounded-b-lg overflow-x-auto text-sm">
        <code>{content}</code>
      </pre>
    </div>
  );
}

function ImageBlock({ url, alt }: { url: string; alt: string }) {
  return (
    <figure>
      <img src={url} alt={alt} className="rounded-lg max-w-full h-auto" />
      {alt && <figcaption className="text-xs text-muted-foreground mt-1 text-center">{alt}</figcaption>}
    </figure>
  );
}

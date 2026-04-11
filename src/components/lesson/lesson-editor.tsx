"use client";

import { useState, useCallback } from "react";
import type { LessonContent, ContentBlock } from "@/lib/lesson-content";
import { addBlock, removeBlock, moveBlock, updateBlock } from "@/lib/lesson-content";
import { LessonRenderer } from "./lesson-renderer";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

interface LessonEditorProps {
  initialContent: LessonContent;
  onSave: (content: LessonContent) => void;
  saving?: boolean;
}

export function LessonEditor({ initialContent, onSave, saving }: LessonEditorProps) {
  const [content, setContent] = useState<LessonContent>(initialContent);
  const [preview, setPreview] = useState(false);

  const handleAdd = useCallback((type: ContentBlock["type"]) => {
    let block: ContentBlock;
    switch (type) {
      case "markdown":
        block = { type: "markdown", content: "" };
        break;
      case "code":
        block = { type: "code", language: "python", content: "" };
        break;
      case "image":
        block = { type: "image", url: "", alt: "" };
        break;
    }
    setContent((c) => addBlock(c, block));
  }, []);

  const handleUpdate = useCallback((index: number, block: ContentBlock) => {
    setContent((c) => updateBlock(c, index, block));
  }, []);

  const handleRemove = useCallback((index: number) => {
    setContent((c) => removeBlock(c, index));
  }, []);

  const handleMove = useCallback((from: number, direction: "up" | "down") => {
    const to = direction === "up" ? from - 1 : from + 1;
    setContent((c) => {
      if (to < 0 || to >= c.blocks.length) return c;
      return moveBlock(c, from, to);
    });
  }, []);

  if (preview) {
    return (
      <div className="space-y-4">
        <div className="flex justify-between">
          <h3 className="text-sm font-medium">Preview</h3>
          <Button variant="outline" size="sm" onClick={() => setPreview(false)}>
            Edit
          </Button>
        </div>
        <LessonRenderer content={content} />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium">Lesson Content ({content.blocks.length} blocks)</h3>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={() => setPreview(true)}>
            Preview
          </Button>
          <Button size="sm" onClick={() => onSave(content)} disabled={saving}>
            {saving ? "Saving..." : "Save"}
          </Button>
        </div>
      </div>

      {content.blocks.map((block, i) => (
        <div key={i} className="border rounded-lg p-3 space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-xs text-muted-foreground font-mono">{block.type}</span>
            <div className="flex gap-1">
              <Button variant="ghost" size="sm" onClick={() => handleMove(i, "up")} disabled={i === 0}>↑</Button>
              <Button variant="ghost" size="sm" onClick={() => handleMove(i, "down")} disabled={i === content.blocks.length - 1}>↓</Button>
              <Button variant="ghost" size="sm" onClick={() => handleRemove(i)} className="text-destructive">×</Button>
            </div>
          </div>

          {block.type === "markdown" && (
            <textarea
              className="w-full min-h-[100px] p-2 border rounded text-sm font-mono bg-background"
              value={block.content}
              onChange={(e) => handleUpdate(i, { ...block, content: e.target.value })}
              placeholder="Write markdown..."
            />
          )}

          {block.type === "code" && (
            <div className="space-y-2">
              <select
                className="text-sm border rounded px-2 py-1 bg-background"
                value={block.language}
                onChange={(e) => handleUpdate(i, { ...block, language: e.target.value })}
              >
                <option value="python">Python</option>
                <option value="javascript">JavaScript</option>
                <option value="html">HTML</option>
                <option value="css">CSS</option>
              </select>
              <textarea
                className="w-full min-h-[80px] p-2 border rounded text-sm font-mono bg-zinc-950 text-zinc-100"
                value={block.content}
                onChange={(e) => handleUpdate(i, { ...block, content: e.target.value })}
                placeholder="Write code..."
              />
            </div>
          )}

          {block.type === "image" && (
            <div className="space-y-2">
              <div>
                <Label className="text-xs">Image URL</Label>
                <Input
                  value={block.url}
                  onChange={(e) => handleUpdate(i, { ...block, url: e.target.value })}
                  placeholder="https://..."
                  className="text-sm"
                />
              </div>
              <div>
                <Label className="text-xs">Alt text</Label>
                <Input
                  value={block.alt}
                  onChange={(e) => handleUpdate(i, { ...block, alt: e.target.value })}
                  placeholder="Description..."
                  className="text-sm"
                />
              </div>
              {block.url && (
                <img src={block.url} alt={block.alt} className="max-h-32 rounded" />
              )}
            </div>
          )}
        </div>
      ))}

      <div className="flex gap-2 border-t pt-3">
        <Button variant="outline" size="sm" onClick={() => handleAdd("markdown")}>
          + Markdown
        </Button>
        <Button variant="outline" size="sm" onClick={() => handleAdd("code")}>
          + Code
        </Button>
        <Button variant="outline" size="sm" onClick={() => handleAdd("image")}>
          + Image
        </Button>
      </div>
    </div>
  );
}

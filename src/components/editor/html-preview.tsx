"use client";

import { useState, useEffect, useRef } from "react";

interface HtmlPreviewProps {
  html: string;
  className?: string;
}

export function HtmlPreview({ html, className = "" }: HtmlPreviewProps) {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!iframeRef.current) return;

    const wrappedHtml = `<!DOCTYPE html>
<html><head>
<style>body{margin:8px;font-family:system-ui,sans-serif;}</style>
</head><body>
${html}
<script>
  window.addEventListener("error", function(e) {
    parent.postMessage({ type: "preview-error", text: e.message }, "*");
  });
</script>
</body></html>`;

    iframeRef.current.srcdoc = wrappedHtml;
    setError(null);
  }, [html]);

  useEffect(() => {
    function handleMessage(e: MessageEvent) {
      if (e.source !== iframeRef.current?.contentWindow) return;
      if (e.data?.type === "preview-error") {
        setError(e.data.text);
      }
    }
    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, []);

  return (
    <div className={`border rounded-lg overflow-hidden ${className}`}>
      <div className="flex items-center justify-between px-3 py-1 bg-muted/50 border-b">
        <span className="text-xs text-muted-foreground">Preview</span>
        {error && <span className="text-xs text-red-500">{error}</span>}
      </div>
      <iframe
        ref={iframeRef}
        sandbox="allow-scripts"
        className="w-full h-full min-h-[200px] bg-white"
        title="HTML Preview"
      />
    </div>
  );
}

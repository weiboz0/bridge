"use client";

import { useState, useRef, useCallback, useEffect } from "react";
import type { OutputLine } from "@/components/editor/output-panel";

const SANDBOX_HTML = `<!DOCTYPE html>
<html><head><style>body{margin:0;font-family:monospace;font-size:14px;}</style></head>
<body><script>
  const _origLog = console.log;
  const _origErr = console.error;
  const _origWarn = console.warn;

  function send(type, text) {
    parent.postMessage({ type, text: String(text) }, "*");
  }

  console.log = function() { send("stdout", Array.from(arguments).join(" ")); };
  console.error = function() { send("stderr", Array.from(arguments).join(" ")); };
  console.warn = function() { send("stdout", "[warn] " + Array.from(arguments).join(" ")); };
  console.info = function() { send("stdout", Array.from(arguments).join(" ")); };

  window.onerror = function(msg) { send("stderr", String(msg)); };
  window.onunhandledrejection = function(e) { send("stderr", "Unhandled rejection: " + e.reason); };

  window.addEventListener("message", function(e) {
    if (e.data && e.data.type === "run") {
      var id = e.data.id;
      try {
        var result = new Function(e.data.code)();
        if (result && typeof result.then === "function") {
          result.then(function() {
            parent.postMessage({ type: "done", id: id, success: true }, "*");
          }).catch(function(err) {
            send("stderr", err.message || String(err));
            parent.postMessage({ type: "done", id: id, success: false }, "*");
          });
        } else {
          parent.postMessage({ type: "done", id: id, success: true }, "*");
        }
      } catch(err) {
        send("stderr", err.message);
        parent.postMessage({ type: "done", id: id, success: false }, "*");
      }
    }
  });

  parent.postMessage({ type: "ready" }, "*");
</script></body></html>`;

interface UseJsRunnerReturn {
  ready: boolean;
  running: boolean;
  output: OutputLine[];
  runCode: (code: string) => void;
  clearOutput: () => void;
}

export function useJsRunner(): UseJsRunnerReturn {
  const [ready, setReady] = useState(false);
  const [running, setRunning] = useState(false);
  const [output, setOutput] = useState<OutputLine[]>([]);
  const iframeRef = useRef<HTMLIFrameElement | null>(null);

  useEffect(() => {
    // Create hidden iframe
    const iframe = document.createElement("iframe");
    iframe.sandbox.add("allow-scripts");
    iframe.style.display = "none";
    iframe.srcdoc = SANDBOX_HTML;
    document.body.appendChild(iframe);
    iframeRef.current = iframe;

    function handleMessage(e: MessageEvent) {
      if (e.source !== iframe.contentWindow) return;
      const { data } = e;

      switch (data.type) {
        case "ready":
          setReady(true);
          break;
        case "stdout":
          setOutput((prev) => [...prev, { type: "stdout", text: data.text }]);
          break;
        case "stderr":
          setOutput((prev) => [...prev, { type: "stderr", text: data.text }]);
          break;
        case "done":
          setRunning(false);
          break;
      }
    }

    window.addEventListener("message", handleMessage);

    return () => {
      window.removeEventListener("message", handleMessage);
      iframe.remove();
      iframeRef.current = null;
    };
  }, []);

  const runCode = useCallback((code: string) => {
    if (!iframeRef.current?.contentWindow || !ready) return;
    setRunning(true);
    setOutput([]);
    const id = crypto.randomUUID();
    iframeRef.current.contentWindow.postMessage({ type: "run", code, id }, "*");
  }, [ready]);

  const clearOutput = useCallback(() => {
    setOutput([]);
  }, []);

  return { ready, running, output, runCode, clearOutput };
}

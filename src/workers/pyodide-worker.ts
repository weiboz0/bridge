/// <reference lib="webworker" />

declare const self: DedicatedWorkerGlobalScope;

interface RunMessage {
  type: "run";
  code: string;
  id: string;
}

interface InitMessage {
  type: "init";
}

type WorkerMessage = RunMessage | InitMessage;

let pyodide: any = null;

async function initPyodide() {
  if (pyodide) return;

  importScripts("https://cdn.jsdelivr.net/pyodide/v0.27.5/full/pyodide.js");

  pyodide = await (self as any).loadPyodide({
    stdout: (text: string) => {
      self.postMessage({ type: "stdout", text });
    },
    stderr: (text: string) => {
      self.postMessage({ type: "stderr", text });
    },
  });

  self.postMessage({ type: "ready" });
}

async function runCode(code: string, id: string) {
  if (!pyodide) {
    self.postMessage({ type: "stderr", text: "Pyodide not initialized" });
    self.postMessage({ type: "done", id, success: false });
    return;
  }

  try {
    await pyodide.runPythonAsync(code);
    self.postMessage({ type: "done", id, success: true });
  } catch (err: any) {
    self.postMessage({ type: "stderr", text: err.message });
    self.postMessage({ type: "done", id, success: false });
  }
}

self.onmessage = async (event: MessageEvent<WorkerMessage>) => {
  const { data } = event;

  switch (data.type) {
    case "init":
      await initPyodide();
      break;
    case "run":
      await runCode(data.code, data.id);
      break;
  }
};

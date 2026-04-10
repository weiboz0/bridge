/* eslint-disable no-restricted-globals */
const workerSelf = self as any;

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

  workerSelf.importScripts("https://cdn.jsdelivr.net/pyodide/v0.27.5/full/pyodide.js");

  pyodide = await (workerSelf as any).loadPyodide({
    stdout: (text: string) => {
      workerSelf.postMessage({ type: "stdout", text });
    },
    stderr: (text: string) => {
      workerSelf.postMessage({ type: "stderr", text });
    },
  });

  workerSelf.postMessage({ type: "ready" });
}

async function runCode(code: string, id: string) {
  if (!pyodide) {
    workerSelf.postMessage({ type: "stderr", text: "Pyodide not initialized" });
    workerSelf.postMessage({ type: "done", id, success: false });
    return;
  }

  try {
    await pyodide.runPythonAsync(code);
    workerSelf.postMessage({ type: "done", id, success: true });
  } catch (err: any) {
    workerSelf.postMessage({ type: "stderr", text: err.message });
    workerSelf.postMessage({ type: "done", id, success: false });
  }
}

workerSelf.onmessage = async (event: MessageEvent<WorkerMessage>) => {
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

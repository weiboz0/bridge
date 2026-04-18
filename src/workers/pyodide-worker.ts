/* eslint-disable no-restricted-globals */
const workerSelf = self as any;

interface RunMessage {
  type: "run";
  code: string;
  id: string;
  /** Newline-separated lines. Empty string = no stdin. Full interactive
   *  stdin (SharedArrayBuffer-backed) lands in a later plan. */
  stdin?: string;
}

interface InitMessage {
  type: "init";
}

type WorkerMessage = RunMessage | InitMessage;

let pyodide: any = null;

// Mutable buffer of remaining stdin lines for the currently-executing program.
// Each `input()` call in Python pops one line. If the buffer is empty, input()
// returns EOF which raises EOFError — the UI should warn when a program runs
// out of pre-supplied input.
let stdinLines: string[] = [];

function popStdinLine(): string | null {
  if (stdinLines.length === 0) return null;
  return stdinLines.shift() ?? null;
}

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
    stdin: () => popStdinLine(),
  });

  workerSelf.postMessage({ type: "ready" });
}

function primeStdin(raw: string | undefined) {
  if (!raw) {
    stdinLines = [];
    return;
  }
  // Preserve trailing empty line if the user intentionally added one.
  stdinLines = raw.split("\n");
  // `split` on a trailing "\n" gives a trailing "" — drop it so input() doesn't
  // return "" spuriously at the end.
  if (stdinLines.length > 0 && stdinLines[stdinLines.length - 1] === "") {
    stdinLines.pop();
  }
}

async function runCode(code: string, id: string, stdin?: string) {
  if (!pyodide) {
    workerSelf.postMessage({ type: "stderr", text: "Pyodide not initialized" });
    workerSelf.postMessage({ type: "done", id, success: false });
    return;
  }

  primeStdin(stdin);

  try {
    await pyodide.runPythonAsync(code);
    workerSelf.postMessage({ type: "done", id, success: true });
  } catch (err: any) {
    workerSelf.postMessage({ type: "stderr", text: err.message });
    workerSelf.postMessage({ type: "done", id, success: false });
  } finally {
    stdinLines = [];
  }
}

workerSelf.onmessage = async (event: MessageEvent<WorkerMessage>) => {
  const { data } = event;

  switch (data.type) {
    case "init":
      await initPyodide();
      break;
    case "run":
      await runCode(data.code, data.id, data.stdin);
      break;
  }
};

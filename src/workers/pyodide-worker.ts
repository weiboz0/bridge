/* eslint-disable no-restricted-globals */
const workerSelf = self as any;

interface InitStdinMessage {
  type: "init-stdin";
  /** SharedArrayBuffer with: Int32Array[0]=state (0=idle, 1=ready),
   *  Int32Array[1]=byteLength, then UTF-8 bytes from offset 8. */
  sab: SharedArrayBuffer;
}

interface RunMessage {
  type: "run";
  code: string;
  id: string;
  /** Newline-separated pre-supplied stdin. The main thread feeds lines into
   *  the SAB on demand; this string is retained for the legacy buffered path
   *  when the page is NOT cross-origin isolated. */
  stdin?: string;
}

interface InitMessage {
  type: "init";
}

type WorkerMessage = InitMessage | InitStdinMessage | RunMessage;

let pyodide: any = null;

// SAB-backed stdin (cross-origin isolated path).
let sabControl: Int32Array | null = null;
let sabBuf: Uint8Array | null = null;
const decoder = new TextDecoder();

// Legacy buffered stdin (fallback for non-isolated dev contexts).
let legacyBuffer: string[] = [];

function readBlockingStdin(): string | null {
  // Prefer SAB when wired up. Otherwise fall back to the legacy pre-fed buffer.
  if (sabControl && sabBuf) {
    workerSelf.postMessage({ type: "input_request" });
    // Block worker thread until the main thread sets state to 1.
    Atomics.wait(sabControl, 0, 0);
    const length = Atomics.load(sabControl, 1);
    const bytes = sabBuf.slice(0, length);
    // Reset for the next read.
    Atomics.store(sabControl, 0, 0);
    return decoder.decode(bytes);
  }
  if (legacyBuffer.length === 0) return null;
  return legacyBuffer.shift() ?? null;
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
    stdin: () => readBlockingStdin(),
  });
  workerSelf.postMessage({ type: "ready" });
}

function primeLegacyStdin(raw: string | undefined) {
  if (!raw) {
    legacyBuffer = [];
    return;
  }
  legacyBuffer = raw.split("\n");
  if (legacyBuffer.length > 0 && legacyBuffer[legacyBuffer.length - 1] === "") {
    legacyBuffer.pop();
  }
}

async function runCode(code: string, id: string, stdin?: string) {
  if (!pyodide) {
    workerSelf.postMessage({ type: "stderr", text: "Pyodide not initialized" });
    workerSelf.postMessage({ type: "done", id, success: false });
    return;
  }

  primeLegacyStdin(stdin);

  try {
    await pyodide.runPythonAsync(code);
    workerSelf.postMessage({ type: "done", id, success: true });
  } catch (err: any) {
    workerSelf.postMessage({ type: "stderr", text: err.message });
    workerSelf.postMessage({ type: "done", id, success: false });
  } finally {
    legacyBuffer = [];
  }
}

workerSelf.onmessage = async (event: MessageEvent<WorkerMessage>) => {
  const { data } = event;
  switch (data.type) {
    case "init":
      await initPyodide();
      break;
    case "init-stdin":
      sabControl = new Int32Array(data.sab, 0, 2);
      sabBuf = new Uint8Array(data.sab, 8);
      break;
    case "run":
      await runCode(data.code, data.id, data.stdin);
      break;
  }
};

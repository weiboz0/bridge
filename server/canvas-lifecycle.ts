import { createHash, timingSafeEqual } from "node:crypto";
import * as Y from "yjs";
import { IncomingMessage, MessageType } from "@hocuspocus/server";
import { messageYjsSyncStep2, messageYjsUpdate } from "y-protocols/sync";

/**
 * The public websocket still serves attempt/session documents, so this is a
 * compatibility ceiling rather than a canvas-specific admission limit.
 */
export const HOCUSPOCUS_SHARED_MAX_PAYLOAD = 100 * 1024 * 1024;
export const CANVAS_UPDATE_LIMIT = 1_048_576;
const MAX_PERSISTED_UPDATE = 4 * 1024 * 1024;
const MAX_CURRENT_STATE = 4 * 1024 * 1024;
const MAX_SNAPSHOT = 8 * 1024 * 1024;
const MAX_SNAPSHOTS = 50;
const MAX_SNAPSHOT_TOTAL = 32 * 1024 * 1024;
const MAX_RESPONSE_BYTES = 48 * 1024 * 1024;
const RESIDENT_LEDGER_LIMIT = 128 * 1024 * 1024;
const CAPTURE_LEDGER_LIMIT = 256 * 1024 * 1024;
const CAPTURE_RESERVATION = 64 * 1024 * 1024;
const MAX_MUTATION_ADMISSIONS = 8;
const DEFAULT_AUTHORIZATION_DEADLINE_MS = 500;
const DEFAULT_FREEZE_BUDGET_MS = 2_000;

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

export class CanvasLifecycleError extends Error {
  readonly status?: number;
  readonly code: string | number;
  readonly retryable: boolean;
  readonly closeCode?: number;
  readonly reason: string;

  constructor(code: string | number, message = String(code), options: {
    status?: number;
    retryable?: boolean;
    closeCode?: number;
    reason?: string;
  } = {}) {
    super(message);
    this.name = "CanvasLifecycleError";
    this.code = code;
    this.status = options.status;
    this.retryable = options.retryable ?? false;
    this.closeCode = options.closeCode;
    this.reason = options.reason ?? String(code);
  }
}

/** Decode the installed Hocuspocus document/sync envelope before enforcing the canvas-only limit. */
export function decodeCanvasMutationUpdate({ documentName, frame }: { documentName: string; frame: Uint8Array }): Uint8Array {
  if (!canvasDocumentName(documentName)) throw lifecycleError("not_canvas_mutation", "Frame is not a canvas mutation");
  try {
    const message = new IncomingMessage(frame);
    const framedName = message.readVarString();
    if (framedName !== documentName) throw lifecycleError("canvas_frame_document_mismatch", "Canvas frame document does not match hook document");
    const type = message.readVarUint();
    if (type !== MessageType.Sync && type !== MessageType.SyncReply) throw lifecycleError("not_canvas_mutation", "Frame is not a Yjs sync mutation");
    const sync = message.readVarUint();
    if (sync !== messageYjsSyncStep2 && sync !== messageYjsUpdate) throw lifecycleError("not_canvas_mutation", "Frame is not a Yjs update");
    const update = message.readVarUint8Array();
    if (update.byteLength > CANVAS_UPDATE_LIMIT) throw lifecycleError("canvas_update_too_large", "Canvas update exceeds 1 MiB");
    return update;
  } catch (error) {
    if (error instanceof CanvasLifecycleError) throw error;
    throw lifecycleError("invalid_canvas_frame", "Canvas mutation frame is malformed");
  }
}

export function decodeCanvasAuthorizationResponse({ status, json }: { status: number; json: unknown }): AuthorizationDecision {
  if (status === 409 && json !== null && typeof json === "object" && (json as { code?: unknown }).code === "session_freezing") {
    return { allowed: false, readOnly: false, code: "session_freezing", reason: "Session whiteboards are temporarily freezing", retryable: true } as AuthorizationDecision & { retryable: boolean };
  }
  if (status === 409 && json !== null && typeof json === "object" && typeof (json as { code?: unknown }).code === "string") {
    return { allowed: false, readOnly: true, code: (json as { code: string }).code };
  }
  if (json === null || typeof json !== "object" || typeof (json as { allowed?: unknown }).allowed !== "boolean" || typeof (json as { readOnly?: unknown }).readOnly !== "boolean") {
    throw lifecycleError("invalid_authorization_response", "Canvas authorization response is invalid");
  }
  const body = json as { allowed: boolean; readOnly: boolean; code?: unknown; reason?: unknown };
  return { allowed: body.allowed, readOnly: body.readOnly, code: typeof body.code === "string" ? body.code : undefined, reason: typeof body.reason === "string" ? body.reason : undefined };
}

function lifecycleError(code: string, message?: string, options?: ConstructorParameters<typeof CanvasLifecycleError>[2]) {
  return new CanvasLifecycleError(code, message, options);
}

function canonicalUuid(value: unknown): value is string {
  return typeof value === "string" && UUID.test(value);
}

function canvasDocumentName(documentName: string): boolean {
  return documentName.startsWith("canvas:");
}

function canvasIdFromDocument(documentName: string): string {
  const id = documentName.slice("canvas:".length);
  if (!canonicalUuid(id)) throw lifecycleError("invalid_canvas_document", "Canvas document name is invalid");
  return id;
}

/** Matches pg's signed int4 conversion of the first UUID word. */
export function sessionLifecycleKey(sessionId: string): number {
  if (!canonicalUuid(sessionId)) throw lifecycleError("invalid_session_id", "Session ID must be a canonical UUID");
  const unsigned = Number.parseInt(sessionId.replaceAll("-", "").slice(0, 8), 16);
  return unsigned > 0x7fff_ffff ? unsigned - 0x1_0000_0000 : unsigned;
}

export type CanvasControlOperation = "freeze" | "unfreeze" | "complete";

export interface FreezeControlRequest {
  sessionId: string;
  freezeToken: string;
  canvasIds: string[];
}

export interface TerminalControlRequest {
  sessionId: string;
  freezeToken: string;
}

/** Strictly decode the tiny private control protocol before state is touched. */
export function parseCanvasControlRequest(operation: CanvasControlOperation, value: unknown): FreezeControlRequest | TerminalControlRequest {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    throw lifecycleError("invalid_control_request", "Control request must be an object");
  }
  const body = value as Record<string, unknown>;
  const required = operation === "freeze"
    ? ["sessionId", "freezeToken", "canvasIds"]
    : ["sessionId", "freezeToken"];
  if (Object.keys(body).length !== required.length || required.some((key) => !(key in body))) {
    throw lifecycleError("invalid_control_request", "Control request has missing or unknown fields");
  }
  if (!canonicalUuid(body.sessionId) || !canonicalUuid(body.freezeToken)) {
    throw lifecycleError("invalid_control_request", "Control request IDs must be canonical UUIDs");
  }
  if (operation !== "freeze") return { sessionId: body.sessionId, freezeToken: body.freezeToken };
  if (!Array.isArray(body.canvasIds) || body.canvasIds.length > MAX_SNAPSHOTS) {
    throw lifecycleError("invalid_control_request", "Canvas IDs must contain at most 50 entries");
  }
  const canvasIds = body.canvasIds.map((id) => {
    if (!canonicalUuid(id)) throw lifecycleError("invalid_control_request", "Canvas ID must be a canonical UUID");
    return id;
  });
  if (canvasIds.some((id, index) => index > 0 && canvasIds[index - 1] >= id)) {
    throw lifecycleError("invalid_control_request", "Canvas IDs must be sorted and unique");
  }
  return { sessionId: body.sessionId, freezeToken: body.freezeToken, canvasIds };
}

function constantTimeBearerMatch(header: string | undefined, secret: string): boolean {
  const expected = Buffer.from(`Bearer ${secret}`);
  const received = Buffer.from(header ?? "");
  return expected.length === received.length && timingSafeEqual(expected, received);
}

export interface CanvasControlDispatch {
  method: string;
  path: string;
  authorization?: string;
  json?: unknown;
}

export interface CanvasControlResponse {
  status: number;
  json: Record<string, unknown>;
}

type ControlLifecycle = Partial<Pick<CanvasLifecycle, "freeze" | "unfreeze" | "complete">>;

/** A testable handler used by the separate loopback HTTP listener. */
export function createCanvasControlServer({ secret, lifecycle }: { secret: string; lifecycle: ControlLifecycle }) {
  return {
    async dispatch(request: CanvasControlDispatch): Promise<CanvasControlResponse> {
      const operationByPath: Record<string, CanvasControlOperation> = {
        "/internal/canvas-sessions/freeze": "freeze",
        "/internal/canvas-sessions/unfreeze": "unfreeze",
        "/internal/canvas-sessions/complete": "complete",
      };
      const operation = operationByPath[request.path];
      if (!operation) return { status: 404, json: { code: "not_found" } };
      if (request.method !== "POST") return { status: 405, json: { code: "method_not_allowed" } };
      if (!constantTimeBearerMatch(request.authorization, secret)) return { status: 401, json: { code: "unauthorized" } };
      try {
        const parsed = parseCanvasControlRequest(operation, request.json);
        if (operation === "freeze") {
          if (!lifecycle.freeze) throw lifecycleError("lifecycle_unavailable", "Canvas lifecycle is unavailable", { status: 503 });
          return { status: 200, json: await lifecycle.freeze(parsed as FreezeControlRequest) };
        }
        if (operation === "unfreeze") {
          if (!lifecycle.unfreeze) throw lifecycleError("lifecycle_unavailable", "Canvas lifecycle is unavailable", { status: 503 });
          return { status: 200, json: await lifecycle.unfreeze(parsed as TerminalControlRequest) };
        }
        if (!lifecycle.complete) throw lifecycleError("lifecycle_unavailable", "Canvas lifecycle is unavailable", { status: 503 });
        return { status: 200, json: await lifecycle.complete(parsed as TerminalControlRequest) };
      } catch (error) {
        const known = error instanceof CanvasLifecycleError;
        return {
          status: known ? error.status ?? 400 : 500,
          json: { code: known ? error.code : "control_failure" },
        };
      }
    },
  };
}

class Turnstile {
  private locked = false;
  private queue: Array<{
    resolve: (release: () => void) => void;
    reject: (error: unknown) => void;
    signal?: AbortSignal;
    abort?: () => void;
  }> = [];

  get isLocked() { return this.locked; }

  acquire(signal?: AbortSignal): Promise<() => void> {
    if (signal?.aborted) return Promise.reject(lifecycleError("admission_cancelled", "Admission was cancelled", { retryable: true }));
    if (!this.locked) {
      this.locked = true;
      return Promise.resolve(this.release);
    }
    return new Promise<() => void>((resolve, reject) => {
      const waiter = { resolve, reject, signal };
      const abort = () => {
        const index = this.queue.indexOf(waiter);
        if (index >= 0) this.queue.splice(index, 1);
        reject(lifecycleError("admission_cancelled", "Admission was cancelled", { retryable: true }));
      };
      waiter.abort = abort;
      signal?.addEventListener("abort", abort, { once: true });
      this.queue.push(waiter);
    });
  }

  private release = () => {
    for (;;) {
      const next = this.queue.shift();
      if (!next) {
        this.locked = false;
        return;
      }
      next.signal?.removeEventListener("abort", next.abort!);
      if (next.signal?.aborted) continue;
      next.resolve(this.release);
      return;
    }
  };
}

type Snapshot = { canvasId: string; stateBase64: string; sha256: string };
type FreezeResult = { snapshots: Snapshot[]; closed: number };

interface Writer {
  controller: AbortController;
  promise: Promise<void>;
}

interface Operation {
  sessionId: string;
  token: string;
  identity: symbol;
  active: boolean;
  terminal?: "unfreeze" | "complete";
  deadline: number;
  controller: AbortController;
  freezePromise: Promise<FreezeResult>;
  result?: FreezeResult;
  captureBytes: number;
  readers: number;
  writers: Set<Writer>;
  captureConnections: Array<{ close?: (event?: { code?: number; reason?: string }) => void }>;
  // One serialized cleanup can satisfy either terminal endpoint.  The
  // endpoint acknowledgement is deliberately layered over this shared work.
  terminalPromise?: Promise<void>;
  unfreezePromise?: Promise<{ unfrozen: true }>;
  completePromise?: Promise<{ released: true }>;
  expiryTimer?: ReturnType<typeof setTimeout>;
}

interface PendingAdmission {
  identity: symbol;
  connection: unknown;
  release?: () => void;
  update: Uint8Array;
  controller: AbortController;
  settled: Promise<void>;
  resolveSettled: () => void;
  authorization?: Promise<AuthorizationDecision>;
}

interface ManagedDocument {
  document: Y.Doc;
  shadow: Y.Doc;
  turnstile: Turnstile;
  admissions: number;
  residentBytes: number;
  generation: number;
  destroyed: boolean;
  unloading: boolean;
  pending: Map<symbol, PendingAdmission>;
  admissionControllers: Set<AbortController>;
  cancellationReason?: string;
  registry?: Map<string, Y.Doc>;
}

interface PendingLoad {
  document: Y.Doc;
  generation: number;
  released: boolean;
}

export interface LeaseDecision {
  allowed: boolean;
  remainingMs?: number;
}

export interface AuthorizationDecision {
  allowed: boolean;
  readOnly: boolean;
  retryable?: boolean;
  code?: string;
  reason?: string;
}

export interface MutationConnection {
  close?: (event?: { code?: number; reason?: string }) => void;
  readOnly?: boolean;
}

export interface CanvasLifecycleOptions {
  now?: () => number;
  validateLease?: (input: { sessionId: string; freezeToken: string; signal: AbortSignal }) => Promise<LeaseDecision>;
  authorizeMutation?: (input: { documentName: string; sessionId: string; userId: string; signal: AbortSignal }) => Promise<AuthorizationDecision>;
  capture?: (input: { sessionId: string; freezeToken: string; canvasIds: string[]; signal: AbortSignal }) => Promise<FreezeResult>;
  documents?: Map<string, Y.Doc>;
  authorizationDeadlineMs?: number;
  writerNoProgressMs?: number;
  writerDeadlineMs?: number;
  limits?: { loadScratchBytes?: number; residentBytes?: number; sessionCaptureBytes?: number; captureReservationBytes?: number };
  scheduleTombstoneEviction?: (callback: () => void, delayMs: number) => () => void;
}

export interface AdmissionInput {
  documentName: string;
  sessionId: string;
  userId: string;
  connection: MutationConnection;
  update: Uint8Array;
}

export class CanvasLifecycle {
  private readonly now: () => number;
  private readonly validateLease;
  private readonly authorizeMutation;
  private readonly captureOverride?: CanvasLifecycleOptions["capture"];
  private readonly documents: Map<string, Y.Doc>;
  private readonly authorizationDeadlineMs: number;
  private readonly writerNoProgressMs: number;
  private readonly writerDeadlineMs: number;
  private readonly loadScratchBytes: number;
  private readonly residentLimit: number;
  private readonly captureLimit: number;
  private readonly captureReservation: number;
  private readonly completedTokens = new Map<string, { token: string; expiresAt: number; cancel: () => void }>();
  private readonly scheduleTombstoneEviction: (callback: () => void, delayMs: number) => () => void;
  private readonly operations = new Map<string, Operation>();
  private readonly serializers = new Map<string, Promise<void>>();
  private readonly managed = new Map<string, ManagedDocument>();
  private readonly pendingLoads = new Map<string, PendingLoad>();
  private residentBytes = 0;
  private captureBytes = 0;
  private generation = 0;
  // Dependency-injected capture is a deterministic test seam. It has no
  // production caller; preserving its already-produced bytes does not change
  // the real-document capture invariant.
  private readonly testCaptureCache = new Map<string, FreezeResult>();

  constructor(options: CanvasLifecycleOptions = {}) {
    this.now = options.now ?? (() => performance.now());
    this.validateLease = options.validateLease ?? (async () => ({ allowed: true, remainingMs: DEFAULT_FREEZE_BUDGET_MS }));
    this.authorizeMutation = options.authorizeMutation ?? (async () => ({ allowed: true, readOnly: false }));
    this.captureOverride = options.capture;
    this.documents = options.documents ?? new Map<string, Y.Doc>();
    this.authorizationDeadlineMs = options.authorizationDeadlineMs ?? DEFAULT_AUTHORIZATION_DEADLINE_MS;
    this.writerNoProgressMs = options.writerNoProgressMs ?? 250;
    this.writerDeadlineMs = options.writerDeadlineMs ?? DEFAULT_FREEZE_BUDGET_MS;
    this.loadScratchBytes = options.limits?.loadScratchBytes ?? 16 * 1024 * 1024;
    this.residentLimit = options.limits?.residentBytes ?? RESIDENT_LEDGER_LIMIT;
    this.captureLimit = options.limits?.sessionCaptureBytes ?? CAPTURE_LEDGER_LIMIT;
    this.captureReservation = options.limits?.captureReservationBytes ?? CAPTURE_RESERVATION;
    this.scheduleTombstoneEviction = options.scheduleTombstoneEviction ?? ((callback, delayMs) => {
      const timer = setTimeout(callback, delayMs);
      timer.unref?.();
      return () => clearTimeout(timer);
    });
  }

  accounting() { return { residentBytes: this.residentBytes, captureBytes: this.captureBytes }; }

  /** Diagnostic-only bounded-tombstone count used by lifecycle regression tests. */
  inspectTombstones(): number {
    this.pruneCompletedTokens();
    return this.completedTokens.size;
  }

  /** Raw diagnostic count, intentionally does not opportunistically prune. */
  inspectRawTombstonesForTesting(): number { return this.completedTokens.size; }

  inspect(sessionId: string) {
    const entry = this.operations.get(sessionId);
    return entry && {
      freezeToken: entry.token,
      deadline: entry.deadline,
      active: entry.active,
      readers: entry.readers,
      terminal: entry.terminal,
      terminalQueueDepth: entry.terminalPromise ? 1 : 0,
    };
  }

  inspectDocument(documentName: string) {
    const state = this.managed.get(documentName);
    if (!state) return undefined;
    return {
      document: state.document,
      admissions: state.admissions,
      turnstileLocked: state.turnstile.isLocked,
      shadowMatchesAuthoritative: equalUpdates(Y.encodeStateAsUpdate(state.shadow), Y.encodeStateAsUpdate(state.document)),
      destroyed: state.destroyed,
      generation: state.generation,
    };
  }

  freeze(request: FreezeControlRequest): Promise<FreezeResult> {
    const parsed = parseCanvasControlRequest("freeze", request) as FreezeControlRequest;
    const current = this.operations.get(parsed.sessionId);
    this.pruneCompletedTokens();
    if (!current && this.completedTokens.get(parsed.sessionId)?.token === parsed.freezeToken) {
      return Promise.reject(lifecycleError("operation_completed", "Canvas lifecycle operation was completed", { retryable: true }));
    }
    if (current) {
      if (current.token !== parsed.freezeToken) throw lifecycleError("freeze_token_mismatch", "A different token owns this session", { status: 409, retryable: true });
      return current.freezePromise;
    }
    const operation = this.newOperation(parsed.sessionId, parsed.freezeToken);
    this.operations.set(parsed.sessionId, operation);
    operation.freezePromise = this.serialize(parsed.sessionId, async () => this.runFreeze(operation, parsed));
    // Terminal control can race the caller before it attaches its response
    // handler.  Observe the rejection internally while preserving the exact
    // original promise and error for that caller.
    void operation.freezePromise.catch(() => undefined);
    return operation.freezePromise;
  }

  unfreeze(request: TerminalControlRequest): Promise<{ unfrozen: true }> {
    const parsed = parseCanvasControlRequest("unfreeze", request) as TerminalControlRequest;
    const operation = this.operations.get(parsed.sessionId);
    if (!operation) return Promise.resolve({ unfrozen: true });
    if (operation.token !== parsed.freezeToken) return Promise.reject(lifecycleError("freeze_token_mismatch", "A different token owns this session", { status: 409, retryable: true }));
    if (operation.unfreezePromise) return operation.unfreezePromise;
    operation.unfreezePromise = this.enqueueTerminal(operation, "unfreeze").then(() => ({ unfrozen: true } as const));
    return operation.unfreezePromise;
  }

  complete(request: TerminalControlRequest): Promise<{ released: true }> {
    const parsed = parseCanvasControlRequest("complete", request) as TerminalControlRequest;
    const operation = this.operations.get(parsed.sessionId);
    if (!operation) return Promise.resolve({ released: true });
    if (operation.token !== parsed.freezeToken) return Promise.reject(lifecycleError("freeze_token_mismatch", "A different token owns this session", { status: 409, retryable: true }));
    if (operation.completePromise) return operation.completePromise;
    operation.completePromise = this.enqueueTerminal(operation, "complete").then(() => ({ released: true } as const));
    return operation.completePromise;
  }

  async sweepExpired(): Promise<void> {
    const expired = [...this.operations.values()].filter((entry) => entry.deadline <= this.now());
    await Promise.all(expired.map(async (entry) => {
      if (this.operations.get(entry.sessionId) !== entry || entry.deadline > this.now()) return;
      // A validation can be paused inside the serializer and may settle only
      // after this abort. Never queue cancellation behind that validation.
      entry.active = false;
      entry.controller.abort();
      await Promise.allSettled([entry.freezePromise]);
      await this.serialize(entry.sessionId, async () => this.cleanup(entry));
    }));
  }

  stream(input: {
    sessionId: string;
    freezeToken: string;
    write: (chunk: string) => boolean | void;
    onceDrain?: () => Promise<void>;
    signal?: AbortSignal;
  }): Promise<void> {
    const entry = this.operations.get(input.sessionId);
    if (!entry || entry.token !== input.freezeToken || !entry.result) {
      throw lifecycleError("freeze_not_found", "No cached freeze result is available", { status: 409, retryable: true });
    }
    if (input.signal?.aborted) {
      return Promise.reject(lifecycleError("writer_aborted", "Response writer was aborted", { retryable: true }));
    }
    const controller = new AbortController();
    const abort = () => controller.abort();
    input.signal?.addEventListener("abort", abort, { once: true });
    entry.readers += 1;
    const writer = {} as Writer;
    const deadline = setTimeout(() => controller.abort(), Math.min(this.writerDeadlineMs, Math.max(0, entry.deadline - this.now())));
    const promise = Promise.resolve().then(async () => {
      try {
        const write = async (chunk: string) => {
          if (controller.signal.aborted) throw lifecycleError("writer_aborted", "Response writer was aborted", { retryable: true });
          if (input.write(chunk) === false) await waitForDrain(input.onceDrain, controller.signal, this.writerNoProgressMs, this.writerDeadlineMs);
        };
        await write('{"snapshots":[');
        for (let index = 0; index < entry.result.snapshots.length; index += 1) {
          if (index > 0) await write(",");
          await write(JSON.stringify(entry.result.snapshots[index]));
        }
        await write(`],"closed":${entry.result.closed}}`);
      } finally {
        clearTimeout(deadline);
        input.signal?.removeEventListener("abort", abort);
        entry.readers -= 1;
        entry.writers.delete(writer);
      }
    });
    Object.assign(writer, { controller, promise });
    entry.writers.add(writer);
    void promise.catch(() => undefined);
    return promise;
  }

  async validateParsedFrame({ documentName, update }: { documentName: string; update: Uint8Array }): Promise<void> {
    this.checkParsedFrame(documentName, update);
  }

  async admitMutation(input: AdmissionInput): Promise<{ handoff: true }> {
    const { state, pending } = await this.prepareAdmission(input);
    try {
      Y.applyUpdate(state.document, input.update, input.connection);
      this.commitAdmission({ documentName: input.documentName, admission: pending.identity });
      return { handoff: true };
    } catch (error) {
      this.rollbackAdmission({ documentName: input.documentName, admission: pending.identity });
      throw error;
    }
  }

  async beginAdmission(input: AdmissionInput): Promise<symbol> {
    return (await this.prepareAdmission(input)).pending.identity;
  }

  commitAdmission({ documentName, admission, connection }: { documentName: string; admission?: symbol; connection?: unknown }): void {
    const state = this.managed.get(documentName);
    const pending = state && (admission !== undefined
      ? state.pending.get(admission)
      : [...state.pending.values()].find((entry) => entry.connection === connection));
    if (!state || !pending) return;
    state.pending.delete(pending.identity);
    pending.resolveSettled();
    state.admissionControllers.delete(pending.controller);
    this.replaceShadowFromAuthoritative(state);
    pending.release?.();
    this.finishAdmission(state);
  }

  rollbackAdmission({ documentName, admission, connection }: { documentName: string; admission?: symbol; connection?: unknown }): void {
    const state = this.managed.get(documentName);
    const pending = state && (admission !== undefined
      ? state.pending.get(admission)
      : [...state.pending.values()].find((entry) => entry.connection === connection));
    if (!state || !pending) return;
    state.pending.delete(pending.identity);
    pending.resolveSettled();
    state.admissionControllers.delete(pending.controller);
    this.replaceShadowFromAuthoritative(state);
    pending.release?.();
    this.finishAdmission(state);
  }

  async cancelAdmissions({ documentName, connection, reason }: { documentName: string; connection?: unknown; reason: string }): Promise<void> {
    const state = this.managed.get(documentName);
    if (!state) return;
    state.cancellationReason = reason;
    const pending = [...state.pending.values()].filter((entry) => connection === undefined || entry.connection === connection);
    for (const entry of pending) entry.controller.abort();
    await Promise.allSettled(pending.map((entry) => entry.settled));
  }

  async prepareLoad({ documentName, persistedUpdate = new Uint8Array() }: { documentName: string; persistedUpdate?: Uint8Array }): Promise<Y.Doc> {
    if (canvasDocumentName(documentName) && persistedUpdate.byteLength > MAX_PERSISTED_UPDATE) throw lifecycleError("persisted_canvas_too_large", "Persisted canvas update exceeds 4 MiB");
    if (persistedUpdate.byteLength > this.loadScratchBytes) throw lifecycleError("load_scratch_exhausted", "Canvas load scratch ledger is full", { retryable: true });
    if (this.residentBytes + this.loadScratchBytes > this.residentLimit) throw lifecycleError("resident_ledger_exhausted", "Canvas resident ledger is full", { retryable: true });
    this.residentBytes += this.loadScratchBytes;
    const document = new Y.Doc();
    try {
      if (persistedUpdate.byteLength) Y.applyUpdate(document, persistedUpdate);
    } catch (error) {
      this.residentBytes -= this.loadScratchBytes;
      document.destroy();
      throw lifecycleError("invalid_persisted_canvas", error instanceof Error ? error.message : "Persisted canvas could not be applied");
    }
    const old = this.pendingLoads.get(documentName);
    if (old) {
      old.released = true;
      this.residentBytes -= this.loadScratchBytes;
      old.document.destroy();
    }
    const pending: PendingLoad = { document, generation: ++this.generation, released: false };
    this.pendingLoads.set(documentName, pending);
    setImmediate(() => {
      if (this.pendingLoads.get(documentName) !== pending || pending.released) return;
      this.pendingLoads.delete(documentName);
      pending.released = true;
      this.residentBytes -= this.loadScratchBytes;
      pending.document.destroy();
    });
    return document;
  }

  claimPreparedLoad({ documentName, document, registry }: { documentName: string; document: Y.Doc; registry: Map<string, Y.Doc> }): (() => void) | undefined {
    const pending = this.pendingLoads.get(documentName);
    if (!pending || pending.released) return;
    this.pendingLoads.delete(documentName);
    pending.released = true;
    const old = this.managed.get(documentName);
    if (old) this.releaseDocument(documentName, old);
    this.documents.set(documentName, document);
    let state: ManagedDocument | undefined;
    try {
      if (pending.document !== document) Y.applyUpdate(document, Y.encodeStateAsUpdate(pending.document));
      state = this.makeManagedDocument(document, pending.generation);
      state.registry = registry;
      this.managed.set(documentName, state);
      setImmediate(() => {
        if (this.managed.get(documentName) === state && registry.get(documentName) !== document) this.releaseDocument(documentName, state);
      });
      return () => {
        if (this.managed.get(documentName) === state) this.releaseDocument(documentName, state);
      };
    } catch (error) {
      // afterLoad is before Hocuspocus publishes the document.  Roll every
      // claim-side effect back here, because its normal destroy path cannot
      // reach an unregistered authoritative instance.
      if (state) this.releaseDocument(documentName, state);
      else if (this.documents.get(documentName) === document) this.documents.delete(documentName);
      throw error;
    } finally {
      this.residentBytes -= this.loadScratchBytes;
      pending.document.destroy();
    }
  }

  async beforeUnload({ documentName, document, generation, registry }: { documentName: string; document: Y.Doc; generation: number; registry?: Map<string, Y.Doc> }): Promise<void> {
    const state = this.managed.get(documentName);
    if (!state || state.document !== document || state.generation !== generation) return;
    state.unloading = true;
    try {
      await this.cancelAdmissions({ documentName, reason: "before_unload" });
      const release = await state.turnstile.acquire();
      try {
        if (registry && registry.get(documentName) !== document) return;
      } finally {
        release();
      }
    } finally {
      // Pinned Hocuspocus may cancel unload after this hook yields. Destroy is
      // therefore the only point that releases listeners or ledger bytes.
      state.unloading = false;
    }
  }

  private newOperation(sessionId: string, token: string): Operation {
    const operation: Partial<Operation> = {
      sessionId,
      token,
      identity: Symbol(token),
      active: true,
      deadline: Number.POSITIVE_INFINITY,
      controller: new AbortController(),
      captureBytes: 0,
      readers: 0,
      writers: new Set<Writer>(),
      captureConnections: [],
    };
    return operation as Operation;
  }

  private async runFreeze(operation: Operation, request: FreezeControlRequest): Promise<FreezeResult> {
    const validationStart = this.now();
    operation.deadline = validationStart + DEFAULT_FREEZE_BUDGET_MS;
    this.scheduleExpiry(operation);
    try {
      const decision = await this.validateLease({ sessionId: request.sessionId, freezeToken: request.freezeToken, signal: operation.controller.signal });
      this.assertOperationActive(operation);
      if (!decision.allowed || !Number.isInteger(decision.remainingMs) || decision.remainingMs <= 0) {
        throw lifecycleError("lease_not_valid", "The freeze lease is not valid", { status: 409, retryable: true });
      }
      operation.deadline = Math.min(validationStart + decision.remainingMs, validationStart + DEFAULT_FREEZE_BUDGET_MS);
      if (operation.deadline <= this.now()) throw lifecycleError("lease_expired", "The freeze lease expired", { status: 409, retryable: true });
      this.scheduleExpiry(operation);
      this.assertOperationActive(operation);
      const result = await this.capture(operation, request);
      this.assertOperationActive(operation);
      operation.result = deepFreezeResult(result);
      const exactBytes = responseBytes(operation.result);
      if (operation.captureBytes === 0) {
        if (exactBytes > MAX_RESPONSE_BYTES || this.captureBytes + exactBytes > this.captureLimit) {
          throw lifecycleError("capture_ledger_exhausted", "Canvas capture ledger is full", { status: 503, retryable: true });
        }
        operation.captureBytes = exactBytes;
        this.captureBytes += exactBytes;
      }
      if (exactBytes > MAX_RESPONSE_BYTES || operation.captureBytes !== exactBytes) {
        throw lifecycleError("capture_ledger_exhausted", "Canvas capture ledger is full", { status: 503, retryable: true });
      }
      for (const connection of operation.captureConnections) connection.close?.({ code: 1013, reason: "session_freezing" });
      operation.captureConnections = [];
      return operation.result;
    } catch (error) {
      // A rejected Go validation owns no durable barrier. Capture failures
      // after an accepted validation retain the bounded token fence until its
      // terminal callback or the conservative deadline cleans it up.
      if (!operation.result && error instanceof CanvasLifecycleError && ["lease_not_valid", "lease_expired"].includes(String(error.code))) {
        operation.active = false;
        await this.cleanup(operation);
      }
      throw error;
    }
  }

  private async capture(operation: Operation, request: FreezeControlRequest): Promise<FreezeResult> {
    if (request.canvasIds.length === 0) return { snapshots: [], closed: 0 };
    if (this.captureBytes + this.captureReservation > this.captureLimit) {
      throw lifecycleError("capture_ledger_exhausted", "Canvas capture ledger is full", { status: 503, retryable: true });
    }
    // Reserve before any encoder runs; the reservation is converted to the
    // actual immutable response accounting below.
    this.captureBytes += this.captureReservation;
    let reserved = true;
    try {
      this.assertOperationActive(operation);
      let result: FreezeResult;
      if (this.captureOverride) {
        const cached = this.testCaptureCache.get(request.sessionId);
        result = cached ?? await this.captureOverride({ ...request, signal: operation.controller.signal });
        this.testCaptureCache.set(request.sessionId, result);
      } else {
        const snapshots: Snapshot[] = [];
        const connections: Array<{ close?: () => void }> = [];
        let total = 0;
        for (const canvasId of request.canvasIds) {
          this.assertOperationActive(operation);
          const documentName = `canvas:${canvasId}`;
          const document = this.documents.get(documentName);
          if (!document) continue;
          const managed = this.documentState(documentName);
          const releaseTurnstile = await managed.turnstile.acquire(operation.controller.signal);
          const installed = document as Y.Doc & { saveMutex?: { acquire: () => Promise<() => void> }; connections?: Map<unknown, { connection?: { close?: () => void } }> };
          // Hocuspocus owns this mutex; acquiring the same turnstile first
          // prevents an admitted mutation from passing the fence while a
          // snapshot is being encoded.
          let releaseSave: (() => void) | undefined;
          let update: Uint8Array;
          try {
            releaseSave = installed.saveMutex
              ? await acquireOwnedMutex(installed.saveMutex, operation.controller.signal, Math.max(0, operation.deadline - this.now()))
              : undefined;
            this.assertOperationActive(operation);
            update = Y.encodeStateAsUpdate(document);
            for (const value of installed.connections?.values() ?? []) {
              if (value.connection) connections.push(value.connection);
            }
          } finally {
            releaseSave?.();
            releaseTurnstile();
          }
          if (update.byteLength > MAX_SNAPSHOT) throw lifecycleError("snapshot_too_large", "Canvas snapshot exceeds 8 MiB");
          total += update.byteLength;
          if (total > MAX_SNAPSHOT_TOTAL) throw lifecycleError("snapshot_aggregate_too_large", "Canvas snapshots exceed 32 MiB");
          snapshots.push({
            canvasId,
            stateBase64: Buffer.from(update).toString("base64"),
            sha256: createHash("sha256").update(update).digest("hex"),
          });
          // Give the event loop a chance to observe an expiry/fence change
          // between documents; never close a socket until all captures pass.
          await new Promise<void>((resolve) => setImmediate(resolve));
        }
        this.assertOperationActive(operation);
        operation.captureConnections = connections;
        result = { snapshots, closed: connections.length };
      }
      validateFreezeResult(result, request.canvasIds);
      const exactBytes = responseBytes(result);
      if (exactBytes > MAX_RESPONSE_BYTES || this.captureBytes - this.captureReservation + exactBytes > this.captureLimit) {
        throw lifecycleError("capture_ledger_exhausted", "Canvas capture ledger is full", { status: 503, retryable: true });
      }
      this.captureBytes += exactBytes - this.captureReservation;
      operation.captureBytes = exactBytes;
      reserved = false;
      return result;
    } finally {
      if (reserved) this.captureBytes -= this.captureReservation;
    }
  }

  private async prepareAdmission(input: AdmissionInput): Promise<{ state: ManagedDocument; pending: PendingAdmission }> {
    // The local fence is deliberately ahead of document lookup/allocation:
    // freezing must not leave even an empty managed generation behind.
    this.assertNotFrozen(input.sessionId);
    if (!canvasDocumentName(input.documentName)) {
      const state = this.documentState(input.documentName);
      const settlement = Promise.withResolvers<void>();
      const pending: PendingAdmission = { identity: Symbol("admission"), connection: input.connection, update: input.update, controller: new AbortController(), settled: settlement.promise, resolveSettled: settlement.resolve };
      return { state, pending };
    }
    canvasIdFromDocument(input.documentName);
    this.checkParsedFrame(input.documentName, input.update);
    const state = this.documentState(input.documentName);
    if (state.admissions >= MAX_MUTATION_ADMISSIONS) {
      throw new CanvasLifecycleError(1013, "Canvas mutation admission is saturated", { retryable: true, closeCode: 1013, reason: "canvas_admission_saturated" });
    }
    state.admissions += 1;
    const controller = new AbortController();
    state.admissionControllers.add(controller);
    const settlement = Promise.withResolvers<void>();
    const pending: PendingAdmission = { identity: Symbol("admission"), connection: input.connection, update: input.update, controller, settled: settlement.promise, resolveSettled: settlement.resolve };
    state.pending.set(pending.identity, pending);
    state.cancellationReason = undefined;
    let release: (() => void) | undefined;
    try {
      this.assertNotFrozen(input.sessionId);
      release = await state.turnstile.acquire(controller.signal);
      if (controller.signal.aborted) {
        throw lifecycleError(state.cancellationReason ?? "admission_cancelled", "Canvas admission was cancelled", { retryable: true });
      }
      this.assertNotFrozen(input.sessionId);
      pending.authorization = this.authorizeMutation({ documentName: input.documentName, sessionId: input.sessionId, userId: input.userId, signal: controller.signal });
      const decision = await withTimeout(
        pending.authorization,
        controller,
        this.authorizationDeadlineMs,
        () => state.cancellationReason,
      );
      this.assertNotFrozen(input.sessionId);
      if (decision.readOnly) {
        input.connection.readOnly = true;
        state.pending.delete(pending.identity);
        pending.resolveSettled();
        state.admissionControllers.delete(controller);
        release?.();
        this.replaceShadowFromAuthoritative(state);
        this.finishAdmission(state);
        return { state, pending };
      }
      if (!decision.allowed) {
        throw lifecycleError(decision.code ?? "canvas_mutation_denied", decision.reason ?? "Canvas mutation is not allowed");
      }
      const scratchBytes = 16 * 1024 * 1024;
      if (this.residentBytes + scratchBytes > this.residentLimit) {
        throw new CanvasLifecycleError(1013, "Canvas resident ledger is full", { retryable: true, closeCode: 1013, reason: "resident_ledger_exhausted" });
      }
      this.residentBytes += scratchBytes;
      const candidate = new Y.Doc();
      let scratchReserved = true;
      try {
      Y.applyUpdate(candidate, Y.encodeStateAsUpdate(state.shadow));
      Y.applyUpdate(candidate, input.update);
      const store = candidate.store as unknown as { pendingStructs?: unknown; pendingDs?: unknown };
      if (store.pendingStructs || store.pendingDs) throw lifecycleError("canvas_update_has_pending_structs", "Canvas update has unresolved dependencies");
      const encoded = Y.encodeStateAsUpdate(candidate);
      if (encoded.byteLength > MAX_CURRENT_STATE) throw lifecycleError("canvas_state_too_large", "Canvas state exceeds 4 MiB");
      const candidateBytes = encoded.byteLength * 2;
      const projected = this.residentBytes - scratchBytes - state.residentBytes + candidateBytes;
      if (projected > this.residentLimit) {
        throw new CanvasLifecycleError(1013, "Canvas resident ledger is full", { retryable: true, closeCode: 1013, reason: "resident_ledger_exhausted" });
      }
      state.shadow.destroy();
      state.shadow = candidate;
      this.residentBytes = projected;
      scratchReserved = false;
      state.residentBytes = candidateBytes;
      pending.release = release;
      return { state, pending };
      } finally {
        if (scratchReserved) this.residentBytes -= scratchBytes;
      }
    } catch (error) {
      controller.abort();
      state.admissionControllers.delete(controller);
      if (pending.authorization) await Promise.allSettled([pending.authorization]);
      if (state.pending.get(pending.identity) === pending) {
        state.pending.delete(pending.identity);
        pending.resolveSettled();
      }
      if (release) release();
      this.replaceShadowFromAuthoritative(state);
      this.finishAdmission(state);
      throw error;
    }
  }

  private documentState(documentName: string): ManagedDocument {
    const existing = this.managed.get(documentName);
    if (existing) return existing;
    const document = this.documents.get(documentName) ?? new Y.Doc();
    this.documents.set(documentName, document);
    const state = this.makeManagedDocument(document, ++this.generation);
    this.managed.set(documentName, state);
    return state;
  }

  private checkParsedFrame(documentName: string, update: Uint8Array): void {
    if (canvasDocumentName(documentName) && update.byteLength > CANVAS_UPDATE_LIMIT) {
      throw lifecycleError("canvas_update_too_large", "Canvas update exceeds 1 MiB");
    }
  }

  private makeManagedDocument(document: Y.Doc, generation: number): ManagedDocument {
    const shadow = new Y.Doc();
    Y.applyUpdate(shadow, Y.encodeStateAsUpdate(document));
    const bytes = Y.encodeStateAsUpdate(document).byteLength * 2;
    if (this.residentBytes + bytes > this.residentLimit) {
      throw new CanvasLifecycleError(1013, "Canvas resident ledger is full", { retryable: true, closeCode: 1013, reason: "resident_ledger_exhausted" });
    }
    const state: ManagedDocument = { document, shadow, turnstile: new Turnstile(), admissions: 0, residentBytes: bytes, generation, destroyed: false, unloading: false, pending: new Map(), admissionControllers: new Set() };
    this.residentBytes += bytes;
    document.on("destroy", () => {
      const entry = [...this.managed.entries()].find(([, candidate]) => candidate === state);
      if (entry) this.releaseDocument(entry[0], state);
    });
    return state;
  }

  private replaceShadowFromAuthoritative(state: ManagedDocument): void {
    const next = new Y.Doc();
    Y.applyUpdate(next, Y.encodeStateAsUpdate(state.document));
    state.shadow.destroy();
    state.shadow = next;
    const bytes = Y.encodeStateAsUpdate(state.document).byteLength * 2;
    this.residentBytes += bytes - state.residentBytes;
    state.residentBytes = bytes;
  }

  private releaseDocument(documentName: string, state: ManagedDocument): void {
    if (state.destroyed) return;
    state.destroyed = true;
    for (const pending of state.pending.values()) {
      pending.controller.abort();
      pending.release?.();
      pending.resolveSettled();
    }
    state.pending.clear();
    for (const controller of state.admissionControllers) controller.abort();
    state.admissionControllers.clear();
    state.shadow.destroy();
    this.residentBytes -= state.residentBytes;
    this.documents.delete(documentName);
    if (this.managed.get(documentName) === state) this.managed.delete(documentName);
  }

  private finishAdmission(state: ManagedDocument): void {
    state.admissions = Math.max(0, state.admissions - 1);
  }

  private assertNotFrozen(sessionId: string): void {
    const operation = this.operations.get(sessionId);
    if (operation?.active && operation.deadline > this.now()) {
      throw lifecycleError("session_freezing", "Session whiteboards are temporarily freezing", { retryable: true });
    }
  }

  private assertOperationActive(operation: Operation): void {
    if (!operation.active || operation.controller.signal.aborted || this.operations.get(operation.sessionId) !== operation) {
      throw lifecycleError("operation_completed", "Canvas lifecycle operation was completed", { retryable: true });
    }
    if (operation.deadline !== Number.POSITIVE_INFINITY && operation.deadline <= this.now()) {
      operation.active = false;
      throw lifecycleError("lease_expired", "Canvas freeze lease expired", { retryable: true });
    }
  }

  private scheduleExpiry(operation: Operation): void {
    if (operation.expiryTimer) clearTimeout(operation.expiryTimer);
    const schedule = () => {
      const remaining = operation.deadline - this.now();
      if (remaining > 0) {
        operation.expiryTimer = setTimeout(schedule, Math.min(remaining, 2_147_483_647));
        return;
      }
      void this.sweepExpired();
    };
    operation.expiryTimer = setTimeout(schedule, Math.max(0, Math.min(operation.deadline - this.now(), 2_147_483_647)));
  }

  private async cleanup(operation: Operation): Promise<void> {
    if (this.operations.get(operation.sessionId) !== operation) return;
    operation.active = false;
    operation.controller.abort();
    if (operation.expiryTimer) clearTimeout(operation.expiryTimer);
    for (const writer of operation.writers) writer.controller.abort();
    await Promise.allSettled([...operation.writers].map((writer) => writer.promise));
    if (operation.readers !== 0) return;
    this.captureBytes -= operation.captureBytes;
    operation.captureBytes = 0;
    if (this.operations.get(operation.sessionId) === operation) {
      this.operations.delete(operation.sessionId);
      if (operation.terminal === "complete") {
        const prior = this.completedTokens.get(operation.sessionId);
        prior?.cancel();
        const entry = { token: operation.token, expiresAt: this.now() + DEFAULT_FREEZE_BUDGET_MS, cancel: () => undefined };
        entry.cancel = this.scheduleTombstoneEviction(() => {
          if (this.completedTokens.get(operation.sessionId) === entry) this.completedTokens.delete(operation.sessionId);
        }, DEFAULT_FREEZE_BUDGET_MS);
        this.completedTokens.set(operation.sessionId, entry);
      }
    }
  }

  private enqueueTerminal(operation: Operation, requested: "unfreeze" | "complete"): Promise<void> {
    // Complete is irreversible and therefore wins a queued unfreeze before
    // cleanup begins.  Both HTTP callers still receive their endpoint's ACK.
    operation.terminal = requested === "complete" ? "complete" : operation.terminal ?? "unfreeze";
    operation.active = false;
    operation.controller.abort();
    if (!operation.terminalPromise) {
      operation.terminalPromise = this.serialize(operation.sessionId, async () => {
        await this.cleanup(operation);
      });
    }
    return operation.terminalPromise;
  }

  private serialize<T>(sessionId: string, task: () => Promise<T>): Promise<T> {
    const prior = this.serializers.get(sessionId) ?? Promise.resolve();
    const result = prior.then(task, task);
    const tail = result.then(() => undefined, () => undefined);
    this.serializers.set(sessionId, tail);
    void tail.finally(() => {
      if (this.serializers.get(sessionId) === tail && !this.operations.has(sessionId)) this.serializers.delete(sessionId);
    });
    return result;
  }

  private pruneCompletedTokens(): void {
    for (const [sessionId, entry] of this.completedTokens) {
      if (entry.expiresAt <= this.now()) {
        entry.cancel();
        this.completedTokens.delete(sessionId);
      }
    }
  }
}

export function createCanvasLifecycle(options: CanvasLifecycleOptions = {}): CanvasLifecycle {
  return new CanvasLifecycle(options);
}

function validateFreezeResult(result: FreezeResult, requested: string[]): void {
  if (!result || !Array.isArray(result.snapshots) || !Number.isInteger(result.closed) || result.closed < 0 || result.snapshots.length > MAX_SNAPSHOTS) {
    throw lifecycleError("invalid_capture", "Canvas capture returned an invalid bundle");
  }
  let total = 0;
  for (let index = 0; index < result.snapshots.length; index += 1) {
    const snapshot = result.snapshots[index];
    if (!canonicalUuid(snapshot.canvasId) || !requested.includes(snapshot.canvasId) || (index > 0 && result.snapshots[index - 1].canvasId >= snapshot.canvasId)) {
      throw lifecycleError("invalid_capture", "Canvas capture returned an unexpected canvas");
    }
    const decoded = Buffer.from(snapshot.stateBase64, "base64");
    if (decoded.byteLength > MAX_SNAPSHOT || !/^[0-9a-f]{64}$/.test(snapshot.sha256) || createHash("sha256").update(decoded).digest("hex") !== snapshot.sha256) {
      throw lifecycleError("invalid_capture", "Canvas capture returned invalid state");
    }
    total += decoded.byteLength;
  }
  if (total > MAX_SNAPSHOT_TOTAL) throw lifecycleError("invalid_capture", "Canvas capture exceeds aggregate limit");
}

function deepFreezeResult(result: FreezeResult): FreezeResult {
  const snapshots = result.snapshots.map((snapshot) => Object.freeze({ ...snapshot }));
  return Object.freeze({ snapshots: Object.freeze(snapshots), closed: result.closed });
}

function responseBytes(result: FreezeResult): number {
  const snapshotBytes = result.snapshots.reduce((total, snapshot) => total + Buffer.byteLength(JSON.stringify(snapshot)), 0);
  return Buffer.byteLength('{"snapshots":[') + snapshotBytes + Math.max(0, result.snapshots.length - 1) + Buffer.byteLength(`],"closed":${result.closed}}`);
}

function equalUpdates(a: Uint8Array, b: Uint8Array): boolean {
  return a.byteLength === b.byteLength && a.every((value, index) => value === b[index]);
}

async function withTimeout<T>(promise: Promise<T>, controller: AbortController, ms: number, cancellationReason?: () => string | undefined): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  let abort: (() => void) | undefined;
  try {
    return await Promise.race([
      promise,
      new Promise<T>((_, reject) => {
        timer = setTimeout(() => {
          reject(lifecycleError("authorization_timeout", "Canvas authorization timed out", { retryable: true }));
          controller.abort();
        }, ms);
      }),
      new Promise<T>((_, reject) => {
        abort = () => reject(lifecycleError(cancellationReason?.() ?? "admission_cancelled", "Canvas admission was cancelled", { retryable: true }));
        controller.signal.addEventListener("abort", abort, { once: true });
      }),
    ]);
  } finally {
    if (timer) clearTimeout(timer);
    if (abort) controller.signal.removeEventListener("abort", abort);
  }
}

/**
 * Hocuspocus' mutex does not accept AbortSignal.  If its acquisition loses our
 * lifecycle deadline, release a late grant immediately so it cannot strand a
 * future capture behind an orphaned lock.
 */
async function acquireOwnedMutex(mutex: { acquire: () => Promise<() => void> }, signal: AbortSignal, ms: number): Promise<() => void> {
  let relinquishLateGrant = false;
  const acquired = mutex.acquire().then((release) => {
    if (relinquishLateGrant) release();
    return release;
  });
  let timer: ReturnType<typeof setTimeout> | undefined;
  let abort: (() => void) | undefined;
  try {
    const release = await Promise.race([
      acquired,
      new Promise<never>((_, reject) => {
        timer = setTimeout(() => reject(lifecycleError("freeze_deadline_expired", "Canvas capture deadline expired", { retryable: true })), ms);
      }),
      new Promise<never>((_, reject) => {
        abort = () => reject(lifecycleError("operation_completed", "Canvas lifecycle operation was completed", { retryable: true }));
        signal.addEventListener("abort", abort, { once: true });
      }),
    ]);
    return release;
  } finally {
    // If the race leaves before mutex acquisition, the continuation above
    // returns its eventual grant to the Hocuspocus mutex immediately.
    if (timer || abort) relinquishLateGrant = true;
    if (timer) clearTimeout(timer);
    if (abort) signal.removeEventListener("abort", abort);
  }
}

async function waitForDrain(onceDrain: (() => Promise<void>) | undefined, signal: AbortSignal, noProgressMs: number, absoluteMs: number): Promise<void> {
  let abort: (() => void) | undefined;
  let noProgressTimer: ReturnType<typeof setTimeout> | undefined;
  let absoluteTimer: ReturnType<typeof setTimeout> | undefined;
  try {
    const aborted = new Promise<never>((_, reject) => {
      abort = () => reject(lifecycleError("writer_aborted", "Response writer was aborted", { retryable: true }));
      signal.addEventListener("abort", abort, { once: true });
    });
    const noProgress = new Promise<never>((_, reject) => {
      noProgressTimer = setTimeout(() => reject(lifecycleError("writer_aborted", "Response writer made no progress", { retryable: true })), noProgressMs);
    });
    const absolute = new Promise<never>((_, reject) => {
      absoluteTimer = setTimeout(() => reject(lifecycleError("writer_aborted", "Response writer deadline expired", { retryable: true })), absoluteMs);
    });
    await Promise.race([onceDrain?.() ?? Promise.resolve(), aborted, noProgress, absolute]);
  } finally {
    if (abort) signal.removeEventListener("abort", abort);
    if (noProgressTimer) clearTimeout(noProgressTimer);
    if (absoluteTimer) clearTimeout(absoluteTimer);
  }
}

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
  readonly code: string;
  readonly retryable: boolean;
  readonly closeCode?: number;

  constructor(code: string, message = code, options: {
    status?: number;
    retryable?: boolean;
    closeCode?: number;
  } = {}) {
    super(message);
    this.name = "CanvasLifecycleError";
    this.code = code;
    this.status = options.status;
    this.retryable = options.retryable ?? false;
    this.closeCode = options.closeCode;
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
  expiryTimer?: ReturnType<typeof setTimeout>;
}

interface PendingAdmission {
  connection: unknown;
  release: () => void;
  update: Uint8Array;
  controller: AbortController;
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
  pending: Map<unknown, PendingAdmission>;
  admissionControllers: Set<AbortController>;
  cancellationReason?: string;
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
}

export interface CanvasLifecycleOptions {
  now?: () => number;
  validateLease?: (input: { sessionId: string; freezeToken: string; signal: AbortSignal }) => Promise<LeaseDecision>;
  authorizeMutation?: (input: { documentName: string; sessionId: string; signal: AbortSignal }) => Promise<AuthorizationDecision>;
  capture?: (input: { sessionId: string; freezeToken: string; canvasIds: string[]; signal: AbortSignal }) => Promise<FreezeResult>;
  documents?: Map<string, Y.Doc>;
  authorizationDeadlineMs?: number;
  writerNoProgressMs?: number;
  writerDeadlineMs?: number;
  limits?: { loadScratchBytes?: number; residentBytes?: number; sessionCaptureBytes?: number; captureReservationBytes?: number };
}

export interface AdmissionInput {
  documentName: string;
  sessionId: string;
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
  private readonly completedTokens = new Map<string, string>();
  private readonly operations = new Map<string, Operation>();
  private readonly serializers = new Map<string, Promise<void>>();
  private readonly managed = new Map<string, ManagedDocument>();
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
  }

  accounting() { return { residentBytes: this.residentBytes, captureBytes: this.captureBytes }; }

  inspect(sessionId: string) {
    const entry = this.operations.get(sessionId);
    return entry && { freezeToken: entry.token, deadline: entry.deadline, active: entry.active, readers: entry.readers };
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
    if (!current && this.completedTokens.get(parsed.sessionId) === parsed.freezeToken) {
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

  async unfreeze(request: TerminalControlRequest): Promise<{ unfrozen: true }> {
    const parsed = parseCanvasControlRequest("unfreeze", request) as TerminalControlRequest;
    const operation = this.operations.get(parsed.sessionId);
    if (!operation) return { unfrozen: true };
    if (operation.token !== parsed.freezeToken) throw lifecycleError("freeze_token_mismatch", "A different token owns this session", { status: 409, retryable: true });
    operation.terminal = operation.terminal === "complete" ? "complete" : "unfreeze";
    operation.active = false;
    operation.controller.abort();
    await this.serialize(parsed.sessionId, async () => this.cleanup(operation));
    return { unfrozen: true };
  }

  async complete(request: TerminalControlRequest): Promise<{ released: true }> {
    const parsed = parseCanvasControlRequest("complete", request) as TerminalControlRequest;
    const operation = this.operations.get(parsed.sessionId);
    if (!operation) return { released: true };
    if (operation.token !== parsed.freezeToken) throw lifecycleError("freeze_token_mismatch", "A different token owns this session", { status: 409, retryable: true });
    operation.terminal = "complete";
    operation.active = false;
    operation.controller.abort();
    await this.serialize(parsed.sessionId, async () => this.cleanup(operation));
    return { released: true };
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
  }): Promise<void> {
    const entry = this.operations.get(input.sessionId);
    if (!entry || entry.token !== input.freezeToken || !entry.result) {
      throw lifecycleError("freeze_not_found", "No cached freeze result is available", { status: 409, retryable: true });
    }
    const controller = new AbortController();
    entry.readers += 1;
    const writer = {} as Writer;
    const promise = (async () => {
      try {
        const result = JSON.stringify(entry.result);
        if (input.write(result) === false) await waitForDrain(input.onceDrain, controller.signal, this.writerNoProgressMs, this.writerDeadlineMs);
        if (controller.signal.aborted) throw lifecycleError("writer_aborted", "Response writer was aborted", { retryable: true });
      } finally {
        entry.readers -= 1;
        entry.writers.delete(writer);
      }
    })();
    Object.assign(writer, { controller, promise });
    entry.writers.add(writer);
    void promise.catch(() => undefined);
    return promise;
  }

  async validateParsedFrame({ documentName, update }: { documentName: string; update: Uint8Array }): Promise<void> {
    this.checkParsedFrame(documentName, update);
  }

  async admitMutation(input: AdmissionInput): Promise<{ handoff: true }> {
    const state = await this.prepareAdmission(input);
    try {
      Y.applyUpdate(state.document, input.update, input.connection);
      this.commitAdmission({ documentName: input.documentName, connection: input.connection });
      return { handoff: true };
    } catch (error) {
      this.rollbackAdmission({ documentName: input.documentName, connection: input.connection });
      throw error;
    }
  }

  async beginAdmission(input: AdmissionInput): Promise<void> {
    await this.prepareAdmission(input);
  }

  commitAdmission({ documentName, connection }: { documentName: string; connection?: unknown }): void {
    const state = this.managed.get(documentName);
    const pending = state && connection !== undefined ? state.pending.get(connection) : undefined;
    if (!state || !pending) return;
    state.pending.delete(pending.connection);
    state.admissionControllers.delete(pending.controller);
    this.replaceShadowFromAuthoritative(state);
    pending.release();
    this.finishAdmission(state);
  }

  rollbackAdmission({ documentName, connection }: { documentName: string; connection?: unknown }): void {
    const state = this.managed.get(documentName);
    const pending = state && connection !== undefined ? state.pending.get(connection) : undefined;
    if (!state || !pending) return;
    state.pending.delete(pending.connection);
    state.admissionControllers.delete(pending.controller);
    this.replaceShadowFromAuthoritative(state);
    pending.release();
    this.finishAdmission(state);
  }

  async cancelAdmissions({ documentName, reason }: { documentName: string; reason: string }): Promise<void> {
    const state = this.managed.get(documentName);
    if (!state) return;
    state.cancellationReason = reason;
    for (const controller of state.admissionControllers) controller.abort();
    for (const pending of [...state.pending.values()]) this.rollbackAdmission({ documentName, connection: pending.connection });
  }

  async beginLoad({ documentName, document, persistedUpdate = new Uint8Array() }: { documentName: string; document: Y.Doc; persistedUpdate?: Uint8Array }): Promise<number> {
    if (canvasDocumentName(documentName) && persistedUpdate.byteLength > MAX_PERSISTED_UPDATE) {
      throw lifecycleError("persisted_canvas_too_large", "Persisted canvas update exceeds 4 MiB");
    }
    if (persistedUpdate.byteLength > this.loadScratchBytes) {
      throw lifecycleError("load_scratch_exhausted", "Canvas load scratch ledger is full", { retryable: true });
    }
    if (this.residentBytes + this.loadScratchBytes > this.residentLimit) {
      throw lifecycleError("resident_ledger_exhausted", "Canvas resident ledger is full", { retryable: true });
    }
    this.residentBytes += this.loadScratchBytes;
    try {
      if (persistedUpdate.byteLength > 0) Y.applyUpdate(document, persistedUpdate);
    } catch (error) {
      this.residentBytes -= this.loadScratchBytes;
      throw lifecycleError("invalid_persisted_canvas", error instanceof Error ? error.message : "Persisted canvas could not be applied");
    }
    const generation = ++this.generation;
    const old = this.managed.get(documentName);
    if (old) this.releaseDocument(documentName, old);
    this.documents.set(documentName, document);
    let state: ManagedDocument;
    try {
      state = this.makeManagedDocument(document, generation);
    } finally {
      this.residentBytes -= this.loadScratchBytes;
    }
    this.managed.set(documentName, state);
    // This watchdog is generation- and instance-owned. A later successful
    // load replaces the map entry, so an old next-turn cleanup cannot free it.
    setImmediate(() => {
      const current = this.managed.get(documentName);
      if (current !== state || state.destroyed) return;
      // Hocuspocus has synchronously registered successful loads by this turn.
      // Standalone callers have no registry, so the exact active instance is
      // retained until its destroy listener runs.
    });
    return generation;
  }

  async beforeUnload({ documentName, document, generation }: { documentName: string; document: Y.Doc; generation: number }): Promise<void> {
    const state = this.managed.get(documentName);
    if (!state || state.document !== document || state.generation !== generation) return;
    state.unloading = true;
    // Pinned Hocuspocus may cancel unload after this hook yields. Destroy is
    // therefore the only point that releases listeners or ledger bytes.
    state.unloading = false;
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
      operation.captureBytes = responseBytes(operation.result);
      if (operation.captureBytes > MAX_RESPONSE_BYTES || this.captureBytes + operation.captureBytes > this.captureLimit) {
        throw lifecycleError("capture_ledger_exhausted", "Canvas capture ledger is full", { status: 503, retryable: true });
      }
      this.captureBytes += operation.captureBytes;
      return operation.result;
    } catch (error) {
      // A rejected Go validation owns no durable barrier. Capture failures
      // after an accepted validation retain the bounded token fence until its
      // terminal callback or the conservative deadline cleans it up.
      if (!operation.result && error instanceof CanvasLifecycleError && ["lease_not_valid", "lease_expired"].includes(error.code)) {
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
        let closed = 0;
        for (const connection of connections) {
          connection.close?.();
          closed += 1;
        }
        result = { snapshots, closed };
      }
      validateFreezeResult(result, request.canvasIds);
      return result;
    } finally {
      this.captureBytes -= this.captureReservation;
    }
  }

  private async prepareAdmission(input: AdmissionInput): Promise<ManagedDocument> {
    if (!canvasDocumentName(input.documentName)) return this.documentState(input.documentName);
    canvasIdFromDocument(input.documentName);
    this.checkParsedFrame(input.documentName, input.update);
    const state = this.documentState(input.documentName);
    if (state.admissions >= MAX_MUTATION_ADMISSIONS) {
      input.connection.close?.({ code: 1013, reason: "canvas_admission_saturated" });
      throw lifecycleError("canvas_admission_saturated", "Canvas mutation admission is saturated", { retryable: true, closeCode: 1013 });
    }
    state.admissions += 1;
    const controller = new AbortController();
    state.admissionControllers.add(controller);
    state.cancellationReason = undefined;
    let release: (() => void) | undefined;
    try {
      this.assertNotFrozen(input.sessionId);
      release = await state.turnstile.acquire(controller.signal);
      if (controller.signal.aborted) {
        throw lifecycleError(state.cancellationReason ?? "admission_cancelled", "Canvas admission was cancelled", { retryable: true });
      }
      this.assertNotFrozen(input.sessionId);
      const decision = await withTimeout(
        this.authorizeMutation({ documentName: input.documentName, sessionId: input.sessionId, signal: controller.signal }),
        controller,
        this.authorizationDeadlineMs,
        () => state.cancellationReason,
      );
      this.assertNotFrozen(input.sessionId);
      if (!decision.allowed || decision.readOnly) {
        throw lifecycleError(decision.code ?? "canvas_mutation_denied", decision.reason ?? "Canvas mutation is not allowed");
      }
      const candidate = new Y.Doc();
      Y.applyUpdate(candidate, Y.encodeStateAsUpdate(state.shadow));
      Y.applyUpdate(candidate, input.update);
      const store = candidate.store as unknown as { pendingStructs?: unknown; pendingDs?: unknown };
      if (store.pendingStructs || store.pendingDs) throw lifecycleError("canvas_update_has_pending_structs", "Canvas update has unresolved dependencies");
      const encoded = Y.encodeStateAsUpdate(candidate);
      if (encoded.byteLength > MAX_CURRENT_STATE) throw lifecycleError("canvas_state_too_large", "Canvas state exceeds 4 MiB");
      const candidateBytes = encoded.byteLength * 2;
      const projected = this.residentBytes - state.residentBytes + candidateBytes;
      if (projected > this.residentLimit) throw lifecycleError("resident_ledger_exhausted", "Canvas resident ledger is full", { retryable: true, closeCode: 1013 });
      state.shadow.destroy();
      state.shadow = candidate;
      this.residentBytes = projected;
      state.residentBytes = candidateBytes;
      state.pending.set(input.connection, { connection: input.connection, release, update: input.update, controller });
      return state;
    } catch (error) {
      controller.abort();
      state.admissionControllers.delete(controller);
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
    if (this.residentBytes + bytes > this.residentLimit) throw lifecycleError("resident_ledger_exhausted", "Canvas resident ledger is full", { retryable: true });
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
    for (const pending of state.pending.values()) pending.release();
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
      if (operation.terminal === "complete") this.completedTokens.set(operation.sessionId, operation.token);
    }
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
  return Buffer.byteLength(JSON.stringify(result));
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

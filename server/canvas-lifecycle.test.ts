import { describe, expect, test } from "bun:test";
import { createHash, randomUUID } from "node:crypto";
import * as Y from "yjs";

// This is deliberately the Phase 10 public contract.  There is no production
// lifecycle module yet: every case below must be RED until the implementation
// supplies this interface without weakening the asserted boundary.
const sessionId = "12345678-1234-4234-8234-123456789abc";
const token = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
const canvasId = "22222222-2222-4222-8222-222222222222";

async function lifecycle() {
  return import("./canvas-lifecycle");
}

function updateWith(key: string, value = "value") {
  const source = new Y.Doc();
  source.getMap("elements").set(key, value);
  return Y.encodeStateAsUpdate(source);
}

describe("Phase 10 canvas lifecycle RED contract", () => {
  test("matches Go and PostgreSQL signed advisory-key vectors", async () => {
    const { sessionLifecycleKey } = await lifecycle();
    expect(sessionLifecycleKey("00000000-0000-4000-8000-000000000000")).toBe(0);
    expect(sessionLifecycleKey("12345678-0000-4000-8000-000000000000")).toBe(305419896);
    expect(sessionLifecycleKey("7fffffff-0000-4000-8000-000000000000")).toBe(2147483647);
    expect(sessionLifecycleKey("80000000-0000-4000-8000-000000000000")).toBe(-2147483648);
    expect(sessionLifecycleKey("ffffffff-0000-4000-8000-000000000000")).toBe(-1);
  });

  test("requires strict authenticated freeze, unfreeze, and complete control requests", async () => {
    const { parseCanvasControlRequest } = await lifecycle();
    expect(() => parseCanvasControlRequest("freeze", { sessionId, freezeToken: token, canvasIds: [canvasId], extra: true })).toThrow();
    expect(() => parseCanvasControlRequest("freeze", { sessionId, freezeToken: token, canvasIds: [canvasId, canvasId] })).toThrow();
    expect(() => parseCanvasControlRequest("freeze", { sessionId, freezeToken: token, canvasIds: Array.from({ length: 51 }, randomUUID) })).toThrow();
    expect(parseCanvasControlRequest("complete", { sessionId, freezeToken: token })).toEqual({ sessionId, freezeToken: token });
  });

  test("rejects malformed bearer, endpoint, and response schemas before lifecycle work", async () => {
    const { createCanvasControlServer } = await lifecycle();
    const control = createCanvasControlServer({ secret: "a".repeat(32), lifecycle: {} });
    await expect(control.dispatch({ method: "GET", path: "/internal/canvas-sessions/freeze", authorization: "Bearer " + "a".repeat(32) })).resolves.toMatchObject({ status: 405 });
    await expect(control.dispatch({ method: "POST", path: "/internal/canvas-sessions/freeze", authorization: "Bearer bad", json: {} })).resolves.toMatchObject({ status: 401 });
    await expect(control.dispatch({ method: "POST", path: "/internal/canvas-sessions/unknown", authorization: "Bearer " + "a".repeat(32), json: {} })).resolves.toMatchObject({ status: 404 });
  });

  test("serializes freeze, unfreeze, and complete, with complete winning a matching terminal race", async () => {
    const { createCanvasLifecycle } = await lifecycle();
    const gate = Promise.withResolvers<void>();
    const sut = createCanvasLifecycle({ validateLease: async () => { await gate.promise; return { allowed: true, remainingMs: 2_000 }; } });
    const freezing = sut.freeze({ sessionId, freezeToken: token, canvasIds: [] });
    const unfreeze = sut.unfreeze({ sessionId, freezeToken: token });
    const complete = sut.complete({ sessionId, freezeToken: token });
    gate.resolve();
    await expect(complete).resolves.toEqual({ released: true });
    await expect(unfreeze).resolves.toEqual({ unfrozen: true });
    await expect(freezing).rejects.toMatchObject({ code: "operation_completed" });
  });

  test("shares same-token immutable cached capture, rejects foreign tokens, and permits validated replacement after expiry", async () => {
    const { createCanvasLifecycle } = await lifecycle();
    let captures = 0;
    let now = 0;
    const sut = createCanvasLifecycle({ now: () => now, validateLease: async () => ({ allowed: true, remainingMs: 500 }), capture: async () => ({ snapshots: [{ canvasId, stateBase64: "AQ==", sha256: createHash("sha256").update(Buffer.from([1])).digest("hex") }], closed: ++captures }) });
    const first = await sut.freeze({ sessionId, freezeToken: token, canvasIds: [canvasId] });
    await expect(sut.freeze({ sessionId, freezeToken: token, canvasIds: [canvasId] })).resolves.toEqual(first);
    await expect(sut.unfreeze({ sessionId, freezeToken: randomUUID() })).rejects.toMatchObject({ status: 409, code: "freeze_token_mismatch" });
    now = 501;
    await sut.sweepExpired();
    await expect(sut.freeze({ sessionId, freezeToken: randomUUID(), canvasIds: [canvasId] })).resolves.toMatchObject({ snapshots: [{ canvasId }] });
    expect(captures).toBe(1);
  });

  test("uses monotonic remaining lease time, expires by token and entry identity, and releases cached accounting", async () => {
    const { createCanvasLifecycle } = await lifecycle();
    let now = 10;
    const sut = createCanvasLifecycle({ now: () => now, validateLease: async () => ({ allowed: true, remainingMs: 100 }) });
    await sut.freeze({ sessionId, freezeToken: token, canvasIds: [] });
    now = 111;
    await sut.sweepExpired();
    expect(sut.inspect(sessionId)).toBeUndefined();
    expect(sut.accounting().captureBytes).toBe(0);
  });

  test("bounds half-open response writers and reference-counts cached entries through complete", async () => {
    const { createCanvasLifecycle } = await lifecycle();
    const sut = createCanvasLifecycle({ validateLease: async () => ({ allowed: true, remainingMs: 2_000 }), writerNoProgressMs: 1, writerDeadlineMs: 2 });
    await sut.freeze({ sessionId, freezeToken: token, canvasIds: [] });
    const stalled = sut.stream({ sessionId, freezeToken: token, write: () => false, onceDrain: () => new Promise(() => {}) });
    await expect(sut.complete({ sessionId, freezeToken: token })).resolves.toEqual({ released: true });
    await expect(stalled).rejects.toMatchObject({ code: "writer_aborted" });
    expect(sut.accounting().captureBytes).toBe(0);
  });

  test("captures sorted loaded subsets, enforces output limits, and validates empty lists without allocation", async () => {
    const { createCanvasLifecycle } = await lifecycle();
    const sut = createCanvasLifecycle({ validateLease: async () => ({ allowed: true, remainingMs: 2_000 }), documents: new Map([[`canvas:${canvasId}`, new Y.Doc()]]) });
    await expect(sut.freeze({ sessionId, freezeToken: token, canvasIds: [] })).resolves.toEqual({ snapshots: [], closed: 0 });
    await expect(sut.freeze({ sessionId, freezeToken: token, canvasIds: [canvasId] })).resolves.toMatchObject({ snapshots: [{ canvasId }], closed: 0 });
    expect(sut.accounting().captureBytes).toBeLessThanOrEqual(48 * 1024 * 1024);
  });

  test("does not report a frozen bundle until the installed Document save mutex releases, then closes its registered connections and reports their exact count", async () => {
    const { createCanvasLifecycle } = await lifecycle();
    const { Document } = await import("@hocuspocus/server");
    const document = new Document(`canvas:${canvasId}`);
    const closed: string[] = [];
    for (const label of ["first", "second"]) {
      document.connections.set({} as never, { clients: new Set(), connection: { close: () => closed.push(label) } } as never);
    }
    const releaseSave = await document.saveMutex.acquire();
    const sut = createCanvasLifecycle({ validateLease: async () => ({ allowed: true, remainingMs: 2_000 }), documents: new Map([[document.name, document]]) });
    let settled = false;
    const frozen = sut.freeze({ sessionId, freezeToken: token, canvasIds: [canvasId] }).then((result: unknown) => { settled = true; return result; });
    await Promise.resolve();
    expect(settled).toBe(false);
    expect(closed).toEqual([]);
    releaseSave();
    await expect(frozen).resolves.toMatchObject({ closed: 2, snapshots: [{ canvasId }] });
    expect(closed).toEqual(["first", "second"]);
  });

  test("rejects the ninth canvas mutation before allocating a waiter and settles all eight on cancellation", async () => {
    const { createCanvasLifecycle } = await lifecycle();
    const blocked = Promise.withResolvers<{ allowed: boolean; readOnly: boolean }>();
    const sut = createCanvasLifecycle({ authorizeMutation: async () => blocked.promise });
    const eight = Array.from({ length: 8 }, () => sut.admitMutation({ documentName: `canvas:${canvasId}`, sessionId, connection: { close() {} }, update: updateWith(randomUUID()) }));
    await expect(sut.admitMutation({ documentName: `canvas:${canvasId}`, sessionId, connection: { close() {} }, update: updateWith("ninth") })).rejects.toMatchObject({ closeCode: 1013 });
    blocked.resolve({ allowed: false, readOnly: false });
    await Promise.allSettled(eight);
    expect(sut.inspectDocument(`canvas:${canvasId}`)?.admissions).toBe(0);
  });

  test("rolls back shadow, accounting, and turnstile for auth deny, timeout, cancel, local fence, and apply failure", async () => {
    const { createCanvasLifecycle } = await lifecycle();
    const sut = createCanvasLifecycle({ authorizeMutation: async () => ({ allowed: false, readOnly: false }) });
    await expect(sut.admitMutation({ documentName: `canvas:${canvasId}`, sessionId, connection: { close() {} }, update: updateWith("denied") })).rejects.toThrow();
    expect(sut.inspectDocument(`canvas:${canvasId}`)).toMatchObject({ admissions: 0, turnstileLocked: false, shadowMatchesAuthoritative: true });
  });

  test("settles deadline, close cancellation, and unload cancellation before a late authorization grant can touch a generation", async () => {
    const { createCanvasLifecycle } = await lifecycle();
    const pending = Promise.withResolvers<{ allowed: boolean; readOnly: boolean }>();
    const sut = createCanvasLifecycle({ authorizeMutation: async () => pending.promise, authorizationDeadlineMs: 1 });
    const documentName = `canvas:${canvasId}`;
    const connection = { close() {} };
    const admission = sut.admitMutation({ documentName, sessionId, connection, update: updateWith("late-grant") });
    await sut.cancelAdmissions({ documentName, reason: "connection_closed" });
    pending.resolve({ allowed: true, readOnly: false });
    await expect(admission).rejects.toMatchObject({ code: "connection_closed" });
    expect(sut.inspectDocument(documentName)).toMatchObject({ admissions: 0, turnstileLocked: false, shadowMatchesAuthoritative: true });
  });

  test("handles partial-overlap Yjs updates by identity and actual-state fallback, never byte digest", async () => {
    const { createCanvasLifecycle } = await lifecycle();
    const authoritative = new Y.Doc();
    const sut = createCanvasLifecycle({ documents: new Map([[`canvas:${canvasId}`, authoritative]]), authorizeMutation: async () => ({ allowed: true, readOnly: false }) });
    const first = updateWith("first");
    Y.applyUpdate(authoritative, first);
    await expect(sut.admitMutation({ documentName: `canvas:${canvasId}`, sessionId, connection: { close() {} }, update: Y.mergeUpdates([first, updateWith("second")]) })).resolves.toMatchObject({ handoff: true });
    expect(sut.inspectDocument(`canvas:${canvasId}`)?.shadowMatchesAuthoritative).toBe(true);
  });

  test("enforces one MiB parsed canvas updates while preserving the shared 100 MiB websocket cap for non-canvas documents", async () => {
    const { createCanvasLifecycle, HOCUSPOCUS_SHARED_MAX_PAYLOAD } = await lifecycle();
    const sut = createCanvasLifecycle();
    expect(HOCUSPOCUS_SHARED_MAX_PAYLOAD).toBe(100 * 1024 * 1024);
    await expect(sut.validateParsedFrame({ documentName: `canvas:${canvasId}`, update: new Uint8Array(1_048_577) })).rejects.toMatchObject({ code: "canvas_update_too_large" });
    await expect(sut.validateParsedFrame({ documentName: `attempt:${randomUUID()}`, update: new Uint8Array(2 * 1024 * 1024) })).resolves.toBeUndefined();
  });

  test("uses generation-keyed load watchdogs, makes unload reversible, and frees resources only on actual destroy", async () => {
    const { createCanvasLifecycle } = await lifecycle();
    const sut = createCanvasLifecycle();
    const document = new Y.Doc();
    const generation = await sut.beginLoad({ documentName: `canvas:${canvasId}`, document, persistedUpdate: updateWith("loaded") });
    await sut.beforeUnload({ documentName: `canvas:${canvasId}`, document, generation });
    expect(sut.inspectDocument(`canvas:${canvasId}`)?.destroyed).toBe(false);
    document.destroy();
    expect(sut.inspectDocument(`canvas:${canvasId}`)).toBeUndefined();
  });

  test("releases a failed unregistered load from its next-turn watchdog without releasing a later generation", async () => {
    const { createCanvasLifecycle } = await lifecycle();
    const sut = createCanvasLifecycle();
    const first = new Y.Doc();
    const second = new Y.Doc();
    await sut.beginLoad({ documentName: `canvas:${canvasId}`, document: first, persistedUpdate: updateWith("first") });
    await sut.beginLoad({ documentName: `canvas:${canvasId}`, document: second, persistedUpdate: updateWith("second") });
    await new Promise<void>((resolve) => setImmediate(resolve));
    expect(sut.inspectDocument(`canvas:${canvasId}`)?.document).toBe(second);
    expect(sut.accounting().residentBytes).toBeGreaterThan(0);
  });

  test("decodes the installed nested Yjs sync envelope and enforces the exact 1 MiB decoded-update boundary", async () => {
    const { decodeCanvasMutationUpdate } = await lifecycle();
    // This is intentionally not a raw-buffer-size check: Hocuspocus wraps the
    // Yjs update in a document-name and sync envelope before the hook sees it.
    const nested = new Uint8Array(1_048_576);
    const { OutgoingMessage } = await import("@hocuspocus/server");
    const frame = new OutgoingMessage(`canvas:${canvasId}`).createSyncMessage().writeUpdate(nested).toUint8Array();
    expect(decodeCanvasMutationUpdate({ documentName: `canvas:${canvasId}`, frame })).toEqual(nested);
    const oversized = new OutgoingMessage(`canvas:${canvasId}`).createSyncMessage().writeUpdate(new Uint8Array(1_048_577)).toUint8Array();
    expect(() => decodeCanvasMutationUpdate({ documentName: `canvas:${canvasId}`, frame: oversized })).toThrow(/1 MiB|too large/i);
  });

  test("maps the real Go 409 session_freezing response to a retryable client condition rather than a permanent read-only decision", async () => {
    const { decodeCanvasAuthorizationResponse } = await lifecycle();
    expect(decodeCanvasAuthorizationResponse({ status: 409, json: { code: "session_freezing" } })).toMatchObject({ code: "session_freezing", retryable: true, readOnly: false });
    expect(decodeCanvasAuthorizationResponse({ status: 409, json: { code: "session_end_in_progress" } })).not.toMatchObject({ code: "session_freezing" });
  });

  test("turnstile admission owns a deadline and AbortSignal for all eight entries and settles every waiter on close", async () => {
    const { createCanvasLifecycle } = await lifecycle();
    const pending = Promise.withResolvers<{ allowed: boolean; readOnly: boolean }>();
    const sut = createCanvasLifecycle({ authorizeMutation: ({ signal }: { signal: AbortSignal }) => new Promise((resolve, reject) => {
      signal.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")), { once: true });
      pending.promise.then(resolve, reject);
    }) });
    const documentName = `canvas:${canvasId}`;
    const admissions = Array.from({ length: 8 }, () => sut.admitMutation({ documentName, sessionId, connection: { close() {} }, update: updateWith(randomUUID()) }));
    await sut.cancelAdmissions({ documentName, reason: "socket_closed" });
    pending.resolve({ allowed: true, readOnly: false });
    const settled = await Promise.allSettled(admissions);
    expect(settled.every((item) => item.status === "rejected")).toBe(true);
    expect(sut.inspectDocument(documentName)).toMatchObject({ admissions: 0, turnstileLocked: false });
  });

  test("a token-keyed zero-snapshot freeze is immutable and terminal cleanup rejects post-terminal reuse without recapture", async () => {
    const { createCanvasLifecycle } = await lifecycle();
    const sut = createCanvasLifecycle({ validateLease: async () => ({ allowed: true, remainingMs: 2_000 }) });
    const first = await sut.freeze({ sessionId, freezeToken: token, canvasIds: [] });
    await expect(sut.freeze({ sessionId, freezeToken: token, canvasIds: [canvasId] })).resolves.toEqual(first);
    await sut.complete({ sessionId, freezeToken: token });
    await expect(sut.freeze({ sessionId, freezeToken: token, canvasIds: [] })).rejects.toMatchObject({ code: "operation_completed" });
  });
});

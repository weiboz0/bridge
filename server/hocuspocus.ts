import { IncomingMessage, MessageType, Server } from "@hocuspocus/server";
import { createServer as createHttpServer, type IncomingMessage as HttpIncomingMessage, type ServerResponse } from "node:http";
import { createServer as createHttpsServer } from "node:https";
import { readFileSync } from "node:fs";
import { isIP } from "node:net";
import { sql } from "drizzle-orm";
import { messageYjsSyncStep2, messageYjsUpdate } from "y-protocols/sync";
import * as Y from "yjs";
import { loadDocumentState, storeDocumentState } from "./documents";
import { serverDb } from "./db";
import { createE2EStackAttestation } from "./e2e-stack";
import {
  loadAttemptYjsState,
  storeAttemptYjsState,
} from "./attempts";
import { rechckDocumentAccess, verifyRealtimeJwt } from "./realtime-jwt";
import {
  CanvasLifecycle,
  CanvasLifecycleError,
  createCanvasControlServer,
  createCanvasLifecycle,
  decodeCanvasAuthorizationResponse,
  decodeCanvasMutationUpdate,
  HOCUSPOCUS_SHARED_MAX_PAYLOAD,
  parseCanvasControlRequest,
} from "./canvas-lifecycle";

// Plan 072 phase 2 config (legacy path fully removed):
// - HOCUSPOCUS_TOKEN_SECRET: shared HMAC secret with the Go API.
//   REQUIRED. Boot fails (process.exit(1)) if unset.
// - BRIDGE_HOST_EXPOSURE: "" / "localhost" (default) or "exposed".
//   Mirrors the Go API's semantics (platform/cmd/api/main.go line 504).
// - GO_INTERNAL_API_URL: base URL Hocuspocus uses to call the
//   server-to-server recheck (`POST /api/internal/realtime/auth`).
//   Defaults to localhost:8002 (the Go API's local port). NOT
//   browser-reachable. A warning is logged at boot when this is the
//   default value under BRIDGE_HOST_EXPOSURE=exposed.
const TOKEN_SECRET = process.env.HOCUSPOCUS_TOKEN_SECRET ?? "";
const BRIDGE_HOST_EXPOSURE = (process.env.BRIDGE_HOST_EXPOSURE ?? "").toLowerCase().trim();
// For local dev the Go port alone (PLATFORM_PORT, default 8002) is enough — the
// internal URL is derived from it. Set GO_INTERNAL_API_URL explicitly to reach
// a non-localhost Go API (e.g. when Hocuspocus runs on a different host).
const GO_DEFAULT_INTERNAL_URL = `http://127.0.0.1:${process.env.PLATFORM_PORT ?? "8002"}`;
const GO_INTERNAL_API_URL = process.env.GO_INTERNAL_API_URL ?? GO_DEFAULT_INTERNAL_URL;

/** Control bearers are never sent to an arbitrary plaintext destination. */
export function validateGoInternalApiUrl(value: string): string {
  let url: URL;
  try { url = new URL(value); } catch { throw new Error("GO_INTERNAL_API_URL must be an absolute URL"); }
  if (url.username || url.password || url.hash) throw new Error("GO_INTERNAL_API_URL must not contain credentials or a fragment");
  if (url.protocol === "https:") return url.toString().replace(/\/$/, "");
  const host = url.hostname.replace(/^\[|\]$/g, "");
  if (url.protocol !== "http:" || !isIP(host) || !(host === "::1" || host.startsWith("127."))) {
    throw new Error("GO_INTERNAL_API_URL must be numeric loopback HTTP or HTTPS");
  }
  return url.toString().replace(/\/$/, "");
}

// HOCUSPOCUS_PORT: TCP port the collaboration server listens on. Defaults to
// 4000; override via .env. An invalid / out-of-range value aborts boot rather
// than silently binding the wrong port (which would surface only as failed
// browser WebSocket connections later).
function parseHocuspocusPort(raw: string | undefined): number {
  if (raw === undefined || raw.trim() === "") {
    return 4000;
  }
  const port = Number(raw);
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error(`HOCUSPOCUS_PORT=${JSON.stringify(raw)} is not a valid TCP port (1-65535)`);
  }
  return port;
}
const HOCUSPOCUS_PORT = parseHocuspocusPort(process.env.HOCUSPOCUS_PORT);
const HOCUSPOCUS_CONTROL_PORT = parseHocuspocusPort(process.env.HOCUSPOCUS_CONTROL_PORT ?? "4001");
const HOCUSPOCUS_CONTROL_HOST = process.env.HOCUSPOCUS_CONTROL_HOST ?? "127.0.0.1";
const HOCUSPOCUS_CONTROL_SECRET = process.env.HOCUSPOCUS_CONTROL_SECRET ?? "";

function isLoopbackControlHost(host: string): boolean {
  const family = isIP(host);
  if (family === 4) return host.startsWith("127.");
  return family === 6 && host === "::1";
}

function validateControlEnv(): { host: string; port: number; secret: string; tls?: { key: Buffer; cert: Buffer } } {
  if (!/^[0-9a-f]{64}$/.test(HOCUSPOCUS_CONTROL_SECRET)) {
    throw new Error("HOCUSPOCUS_CONTROL_SECRET must be 64 lowercase hexadecimal characters");
  }
  if (HOCUSPOCUS_CONTROL_SECRET === TOKEN_SECRET) {
    throw new Error("HOCUSPOCUS_CONTROL_SECRET must differ from HOCUSPOCUS_TOKEN_SECRET");
  }
  if (isLoopbackControlHost(HOCUSPOCUS_CONTROL_HOST)) {
    return { host: HOCUSPOCUS_CONTROL_HOST, port: HOCUSPOCUS_CONTROL_PORT, secret: HOCUSPOCUS_CONTROL_SECRET };
  }
  const keyPath = process.env.HOCUSPOCUS_CONTROL_TLS_KEY_FILE;
  const certPath = process.env.HOCUSPOCUS_CONTROL_TLS_CERT_FILE;
  if (!keyPath || !certPath) {
    throw new Error("A non-loopback Hocuspocus control listener requires verified TLS key and certificate paths");
  }
  return {
    host: HOCUSPOCUS_CONTROL_HOST,
    port: HOCUSPOCUS_CONTROL_PORT,
    secret: HOCUSPOCUS_CONTROL_SECRET,
    tls: { key: readFileSync(keyPath), cert: readFileSync(certPath) },
  };
}

function validateRealtimeAuthEnv(): void {
  // Plan 072 phase 2 — JWT-only boot check. TOKEN_SECRET is required; no
  // legacy fallback. Mirrors platform/cmd/api/main.go::validateDevAuthEnv.

  // Empty-string BRIDGE_HOST_EXPOSURE is treated as "localhost" — matches
  // Go's main.go:504 semantics so a dev box without the env var
  // explicitly exported to the Node process behaves consistently.
  const isLocalhost = BRIDGE_HOST_EXPOSURE === "" || BRIDGE_HOST_EXPOSURE === "localhost";
  const isExposed = BRIDGE_HOST_EXPOSURE === "exposed";

  // Validate BRIDGE_HOST_EXPOSURE enum: only "", "localhost", "exposed" allowed.
  if (!isLocalhost && !isExposed) {
    console.error(
      `[hocuspocus] refusing to start: BRIDGE_HOST_EXPOSURE=${JSON.stringify(BRIDGE_HOST_EXPOSURE)} is unrecognized. Allowed values are "localhost" (default) and "exposed".`
    );
    process.exit(1);
  }

  // Hard fail: signing secret is required.
  if (!TOKEN_SECRET) {
    console.error(
      "[hocuspocus] refusing to start: HOCUSPOCUS_TOKEN_SECRET is unset. Set the shared HMAC secret with the Go API."
    );
    process.exit(1);
  }
  validateControlEnv();
  validateGoInternalApiUrl(GO_INTERNAL_API_URL);

  // Operational warning: GO_INTERNAL_API_URL default localhost in an
  // exposed environment will make every onLoadDocument recheck fail.
  if (isExposed && GO_INTERNAL_API_URL === GO_DEFAULT_INTERNAL_URL) {
    console.warn(
      `[hocuspocus] WARNING: GO_INTERNAL_API_URL is the localhost default (${GO_DEFAULT_INTERNAL_URL}) in a BRIDGE_HOST_EXPOSURE=exposed environment. Realtime document loads will fail unless this points at a reachable Go API. Set GO_INTERNAL_API_URL explicitly.`
    );
  }

  // Startup mode log — visible in ops at boot.
  console.log(`[hocuspocus] realtime auth mode: JWT only; exposure=${BRIDGE_HOST_EXPOSURE || "localhost (default)"}`);
}

const e2eStackAttestation = createE2EStackAttestation();

if (import.meta.main) {
  validateRealtimeAuthEnv();
  e2eStackAttestation.assertBootAllowed();
}

interface AuthContext {
  userId: string;
  role: string;
  attemptId?: string;
  canvasId?: string;
  sessionId?: string;
  readOnly?: boolean;
  jwtExpiry?: number;
}

type CanvasAuthorization = (input: {
  documentName: string;
  sub: string;
  sessionId: string;
}) => Promise<{ allowed: boolean; readOnly: boolean; code?: string; reason?: string }>;

type CanvasConnection = {
  readOnly: boolean;
  close?: (event?: { code?: number; reason?: string }) => void;
};

function retryableFreezeError(): CanvasLifecycleError {
  return new CanvasLifecycleError("session_freezing", "Session whiteboards are temporarily freezing", { retryable: true });
}

function lifecycleErrorFromDecision(decision: { code?: string; reason?: string }): CanvasLifecycleError {
  if (decision.code === "session_freezing") return retryableFreezeError();
  return new CanvasLifecycleError(decision.code ?? "canvas_access_denied", decision.reason ?? "Canvas access was denied");
}

async function currentCanvasAuthorization({ documentName, sub, sessionId }: { documentName: string; sub: string; sessionId: string }) {
  // validateRealtimeAuthEnv validates this before production startup. Keeping
  // request construction here direct preserves the no-listen hook seam.
  const response = await fetch(`${GO_INTERNAL_API_URL}/api/internal/realtime/auth`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${TOKEN_SECRET}` },
    body: JSON.stringify({ documentName, sub, sessionId }),
    redirect: "error",
  });
  let body: unknown;
  try { body = await response.json(); } catch { throw new CanvasLifecycleError("invalid_authorization_response", "Canvas authorization response is invalid"); }
  const decision = decodeCanvasAuthorizationResponse({ status: response.status, json: body });
  if (typeof decision.readOnly !== "boolean") throw new Error("Canvas authorization omitted readOnly");
  return { allowed: decision.allowed, readOnly: decision.readOnly, code: decision.code, reason: decision.reason };
}

/**
 * Canvas-only hooks are deliberately separate from the legacy document hooks.
 * onAuthenticate is the per-connection authorization path; onLoadDocument is
 * only persistence and cannot authorize a second socket joining a hot doc.
 */
export function createCanvasLifecycleHooks({
  lifecycle = createCanvasLifecycle(),
  authorize = currentCanvasAuthorization,
}: {
  lifecycle?: Partial<Pick<CanvasLifecycle, "beginAdmission" | "commitAdmission" | "rollbackAdmission" | "cancelAdmissions" | "beforeUnload">>;
  authorize?: CanvasAuthorization;
} = {}) {
  async function authorizeConnection({ documentName, context, connectionConfig, connection }: {
    documentName: string;
    context: { userId?: string; sessionId?: string };
    connectionConfig: { readOnly: boolean };
    connection?: CanvasConnection;
  }) {
    if (!documentName.startsWith("canvas:")) return;
    if (!context.userId || !context.sessionId) {
      throw new CanvasLifecycleError("missing_authenticated_context", "Canvas connection is missing authenticated context");
    }
    const decision = await authorize({ documentName, sub: context.userId, sessionId: context.sessionId });
    if (decision.code === "session_freezing") throw retryableFreezeError();
    if (!decision.allowed) throw lifecycleErrorFromDecision(decision);
    if (decision.readOnly) {
      connectionConfig.readOnly = true;
      if (connection) connection.readOnly = true;
    }
  }

  return {
    async onAuthenticate(input: {
      documentName: string;
      context: { userId?: string; sessionId?: string };
      connectionConfig: { readOnly: boolean };
    }) {
      await authorizeConnection(input);
    },

    async beforeHandleMessage(input: {
      documentName: string;
      document?: Y.Doc;
      connection: CanvasConnection;
      update: Uint8Array;
      context: { userId?: string; sessionId?: string };
    }) {
      if (!input.documentName.startsWith("canvas:") || input.connection.readOnly || !isYjsMutationFrame(input.update)) return;
      if (!lifecycle.beginAdmission) {
        const config = { readOnly: input.connection.readOnly };
        await authorizeConnection({ ...input, connectionConfig: config });
        return;
      }
      if (!input.context.sessionId) throw new CanvasLifecycleError("missing_authenticated_context", "Canvas connection is missing authenticated session context");
      const decodedUpdate = decodeCanvasMutationUpdate({ documentName: input.documentName, frame: input.update });
      const admission = await lifecycle.beginAdmission?.({
        documentName: input.documentName,
        sessionId: input.context.sessionId,
        userId: input.context.userId,
        connection: input.connection,
        update: decodedUpdate,
      } as never);
      if (input.connection.readOnly) return;

      if (!input.document || !lifecycle.commitAdmission) return;
      let settled = false;
      const commit = (_update: Uint8Array, origin: unknown) => {
        // Hocuspocus passes the Connection instance as Yjs origin.  The
        // installed 3.4.4 apply seam can wrap that object, so the pending
        // admission is correlated to this exact document/turnstile rather
        // than a byte digest or a fragile wrapper identity.
        if (settled || origin !== input.connection) return;
        settled = true;
        input.document?.off("update", commit);
        lifecycle.commitAdmission?.({ documentName: input.documentName, admission });
      };
      input.document.on("update", commit);
      setImmediate(() => {
        if (settled) return;
        settled = true;
        input.document?.off("update", commit);
        lifecycle.rollbackAdmission?.({ documentName: input.documentName, admission });
      });
    },

    async onDisconnect(input: { documentName: string; connection?: unknown }) {
      if (input.documentName.startsWith("canvas:")) {
        await lifecycle.cancelAdmissions?.({ documentName: input.documentName, connection: input.connection, reason: "disconnect" });
      }
    },

    async beforeUnloadDocument(input: { documentName: string; document: Y.Doc }) {
      if (input.documentName.startsWith("canvas:")) {
        await lifecycle.cancelAdmissions?.({ documentName: input.documentName, reason: "before_unload" });
      }
    },

    async afterLoadDocument({ documentName, document }: { documentName: string; document: Y.Doc }) {
      // The real registry retains the exact document until destroy. This hook
      // exists to make that installed lifecycle boundary explicit.
      void documentName;
      void document;
    },
  };
}

/**
 * Production canvas-load adapter for Hocuspocus's pre-registry hook order.
 * Earlier extensions run before Bridge claims the pending load, so their
 * failure leaves its watchdog armed. Startup separately enforces that Bridge
 * is final because pinned Hocuspocus cannot roll back a post-claim extension.
 */
export function createCanvasLoadLifecycleAdapter({ lifecycle = canvasLifecycle }: {
  lifecycle?: CanvasLifecycle;
} = {}) {
  return {
    async prepare({ documentName, persistedUpdate = new Uint8Array() }: { documentName: string; persistedUpdate?: Uint8Array }) {
      return lifecycle.prepareLoad({ documentName, persistedUpdate });
    },
    async afterLoadDocument({ documentName, document, instance }: { documentName: string; document: Y.Doc; instance: { documents: Map<string, Y.Doc> } }) {
      if (!documentName.startsWith("canvas:")) return;
      lifecycle.claimPreparedLoad({ documentName, document, registry: instance.documents });
    },
    async beforeUnloadDocument({ documentName, document, instance }: { documentName: string; document: Y.Doc; instance: { documents: Map<string, Y.Doc> } }) {
      if (!documentName.startsWith("canvas:")) return;
      if (instance.documents.get(documentName) !== document) return;
      const state = lifecycle.inspectDocument(documentName);
      if (state) await lifecycle.beforeUnload({ documentName, document, generation: state.generation, registry: instance.documents });
    },
  };
}

/** Close both reader and writer sockets when their signed canvas JWT expires. */
export function scheduleCanvasJwtExpiry({
  exp,
  connection,
  now = () => Date.now(),
}: {
  exp: number;
  connection: { close: (event?: { code?: number; reason?: string }) => void };
  now?: () => number;
}) {
  let cancelled = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const check = () => {
    if (cancelled) return;
    const remaining = exp * 1_000 - now();
    if (remaining <= 0) {
      connection.close({ code: 4001, reason: "canvas_jwt_expired" });
      return;
    }
    timer = setTimeout(check, Math.min(remaining, 2_147_483_647));
  };
  if (exp * 1_000 <= now()) {
    check();
  } else {
    timer = setTimeout(check, Math.min(exp * 1_000 - now(), 2_147_483_647));
  }
  return {
    cancel() {
      cancelled = true;
      if (timer) clearTimeout(timer);
    },
  };
}

const canvasLifecycle = createCanvasLifecycle({
  authorizeMutation: async ({ documentName, sessionId, userId, signal }) => {
    const response = await fetch(`${GO_INTERNAL_API_URL}/api/internal/realtime/auth`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${TOKEN_SECRET}` },
      body: JSON.stringify({ documentName, sub: userId, sessionId }),
      signal,
      redirect: "error",
    });
    let body: unknown;
    try { body = await response.json(); } catch { throw new CanvasLifecycleError("invalid_authorization_response", "Canvas authorization response is invalid"); }
    return decodeCanvasAuthorizationResponse({ status: response.status, json: body });
  },
  validateLease: async ({ sessionId, freezeToken, signal }) => {
    const response = await fetch(`${GO_INTERNAL_API_URL}/api/internal/canvas-sessions/freeze-auth`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${HOCUSPOCUS_CONTROL_SECRET}` },
      body: JSON.stringify({ sessionId, freezeToken }),
      signal,
      redirect: "error",
    });
    if (response.status !== 200) return { allowed: false };
    const body = await response.json() as { allowed?: unknown; remainingMs?: unknown };
    return {
      allowed: body.allowed === true && Number.isInteger(body.remainingMs) && Number(body.remainingMs) > 0,
      remainingMs: typeof body.remainingMs === "number" ? body.remainingMs : undefined,
    };
  },
});

const canvasLifecycleHooks = createCanvasLifecycleHooks({ lifecycle: canvasLifecycle });
const canvasLoadLifecycle = createCanvasLoadLifecycleAdapter({
  lifecycle: canvasLifecycle,
});
const canvasConnections = new Map<string, unknown>();

export function canvasAuthenticationContext({
  documentName,
  claims,
  connectionConfig,
}: {
  documentName: string;
  claims: { sub: string; role: string; readOnly: boolean; sessionId?: string; exp?: number };
  connectionConfig: { readOnly: boolean };
}): AuthContext {
  if (!documentName.startsWith("canvas:")) {
    throw new Error("Canvas authentication requires a canvas document");
  }
  if (!claims.sessionId) {
    throw new Error("Canvas authentication requires a verified sessionId");
  }
  connectionConfig.readOnly = claims.readOnly;
  return {
    userId: claims.sub,
    role: claims.role,
    canvasId: documentName.slice("canvas:".length),
    sessionId: claims.sessionId,
    readOnly: claims.readOnly,
    jwtExpiry: claims.exp,
  };
}

export function isYjsMutationFrame(update: Uint8Array): boolean {
  const message = new IncomingMessage(update);
  message.readVarString();
  const messageType = message.readVarUint();
  if (messageType !== MessageType.Sync && messageType !== MessageType.SyncReply) {
    return false;
  }
  const syncType = message.readVarUint();
  return syncType === messageYjsSyncStep2 || syncType === messageYjsUpdate;
}

type DocumentRecheck = typeof rechckDocumentAccess;

export async function guardCanvasMutationFrame({
  documentName,
  update,
  connection,
  userId,
  sessionId,
  recheck = rechckDocumentAccess,
}: {
  documentName: string;
  update: Uint8Array;
  connection: { readOnly: boolean };
  userId: string;
  sessionId?: string;
  recheck?: DocumentRecheck;
}): Promise<void> {
  if (!documentName.startsWith("canvas:") || connection.readOnly || !isYjsMutationFrame(update)) {
    return;
  }
  if (!userId) {
    throw new Error("Canvas mutation is missing authenticated user context");
  }
  if (!sessionId) {
    throw new Error("Canvas mutation is missing authenticated session context");
  }
  const decision = await recheck({
    apiBaseUrl: GO_INTERNAL_API_URL,
    secret: TOKEN_SECRET,
    documentName,
    sub: userId,
    sessionId,
  });
  if (!decision.allowed) {
    throw new Error(`Access denied (mutation recheck): ${decision.reason ?? "unauthorized"}`);
  }
  if (typeof decision.readOnly !== "boolean") {
    throw new Error("Canvas mutation recheck omitted readOnly");
  }
  if (decision.readOnly) {
    connection.readOnly = true;
  }
}

export async function loadCanvasYjsState(canvasId: string): Promise<string | null> {
  const rows = await serverDb.execute<{ yjs_state: string | null }>(sql`
    SELECT yjs_state FROM session_canvases WHERE id = ${canvasId}::uuid
  `);
  if (rows.length !== 1) {
    throw new Error("Canvas does not exist");
  }
  return rows[0].yjs_state;
}

// The joined status predicate is the durable archive backstop. A debounce
// that fires after a session ends cannot overwrite the pre-end snapshot.
export async function storeCanvasYjsState(canvasId: string, yjsState: string): Promise<boolean> {
  const rows = await serverDb.execute<{ id: string }>(sql`
    UPDATE session_canvases AS canvas
    SET yjs_state = ${yjsState}, updated_at = now()
    FROM sessions
    WHERE canvas.id = ${canvasId}::uuid
      AND sessions.id = canvas.session_id
      AND sessions.status <> 'ended'
    RETURNING canvas.id
  `);
  return rows.length === 1;
}

// Exporting the configuration makes the installed callbacks independently
// executable in a no-listen test process. The production Server receives this
// exact object below; it is not a parallel test-only implementation.
export const hocuspocusHooks = {
  port: HOCUSPOCUS_PORT,
  debounce: 30000, // Save to DB every 30 seconds (also saves on disconnect)
  ...(e2eStackAttestation.enabled ? { onRequest: e2eStackAttestation.onRequest } : {}),

  async onAuthenticate({ token, documentName, connectionConfig }) {
    // noop documents don't carry collaboration content — short-circuit
    // before JWT verification so connection probes don't require a token.
    // Codex code-review BLOCKER: must NOT also bypass on missing-token for
    // real documents; otherwise an unauthenticated WebSocket can load any
    // document. noop is the only doc that bypasses; everything else
    // requires a JWT.
    if (documentName === "noop") {
      return { userId: "", role: "" } satisfies AuthContext;
    }
    if (!token) {
      throw new Error("Authentication required");
    }

    // Plan 072 phase 2 — JWT is the ONLY auth path. Legacy userId:role
    // tokens are gone. TOKEN_SECRET is guaranteed non-empty at boot
    // (validateRealtimeAuthEnv enforces this).
    const claims = verifyRealtimeJwt(token, TOKEN_SECRET);
    if (claims.scope !== documentName) {
      throw new Error("JWT scope does not match documentName");
    }
    if (documentName.startsWith("canvas:")) {
      const context = canvasAuthenticationContext({ documentName, claims, connectionConfig });
      await canvasLifecycleHooks.onAuthenticate({ documentName, context, connectionConfig });
      return context;
    }
    const ctx: AuthContext = {
      userId: claims.sub,
      role: claims.role,
    };
    // Carry attempt readOnly into the context. JWT scope `attempt:{aid}`
    // is minted owner-only; readOnly = false until a teacher-watch JWT
    // path is added.
    if (documentName.startsWith("attempt:")) {
      ctx.attemptId = documentName.slice("attempt:".length);
      ctx.readOnly = false;
    }
    return ctx;
  },

  async onLoadDocument({
    document,
    documentName,
    context,
  }: {
    document: Y.Doc;
    documentName: string;
    context: AuthContext;
  }) {
    // Plan 072 phase 2 — defense-in-depth DB recheck, now unconditional.
    // TOKEN_SECRET is guaranteed non-empty at boot. The Go mint endpoint
    // enforced access at mint time, but a user could be demoted in the
    // 25-min window before the connection lands here. Re-asks the Go API
    // "does this user STILL have access to this doc?" If no, throw —
    // Hocuspocus tears down the connection.
    // Skip the recheck for noop documents (no userId, no meaningful access).
    if (context?.userId) {
      const decision = await rechckDocumentAccess({
        apiBaseUrl: GO_INTERNAL_API_URL,
        secret: TOKEN_SECRET,
        documentName,
        sub: context.userId,
        sessionId: documentName.startsWith("canvas:") ? context.sessionId : undefined,
      });
      if (!decision.allowed) {
        throw new Error(`Access denied (recheck): ${decision.reason ?? "unauthorized"}`);
      }
    }

    // chapter:* documents are not persisted via Hocuspocus — realtime sync only.
    // The editor saves to the teaching-unit API on demand.
    if (documentName.startsWith("broadcast:") || documentName === "noop" || documentName.startsWith("chapter:")) return document;

    try {
      let yjsState: string | null = null;
      if (documentName.startsWith("attempt:")) {
        const attemptId = documentName.slice("attempt:".length);
        yjsState = await loadAttemptYjsState(attemptId);
      } else if (documentName.startsWith("canvas:")) {
        yjsState = await loadCanvasYjsState(documentName.slice("canvas:".length));
      } else {
        yjsState = await loadDocumentState(documentName);
      }
      if (yjsState !== null) {
        const update = Buffer.from(yjsState, "base64");
        if (documentName.startsWith("canvas:")) {
          return await canvasLoadLifecycle.prepare({ documentName, persistedUpdate: update });
        } else {
          Y.applyUpdate(document, update);
        }
        console.log(`[hocuspocus] Loaded state for: ${documentName}`);
      } else if (documentName.startsWith("canvas:")) {
        return await canvasLoadLifecycle.prepare({ documentName });
      }
    } catch (err) {
      console.error(`[hocuspocus] Failed to load state for ${documentName}:`, err);
      if (documentName.startsWith("canvas:")) {
        throw err;
      }
    }

    return document;
  },

  async beforeHandleMessage({ documentName, document, connection, update, context }) {
    await canvasLifecycleHooks.beforeHandleMessage({
      documentName,
      document,
      connection,
      update,
      context: context as AuthContext,
    });
  },

  async onStoreDocument({ document, documentName }: { document: Y.Doc; documentName: string }) {
    // chapter:* documents are not persisted via Hocuspocus — realtime sync only.
    // The editor saves to the teaching-unit API on demand.
    if (documentName.startsWith("broadcast:") || documentName === "noop" || documentName.startsWith("chapter:")) return;

    try {
      const update = Y.encodeStateAsUpdate(document);
      const yjsState = Buffer.from(update).toString("base64");
      const plainText = document.getText("content").toString();

      if (documentName.startsWith("attempt:")) {
        const attemptId = documentName.slice("attempt:".length);
        await storeAttemptYjsState(attemptId, yjsState, plainText);
      } else if (documentName.startsWith("canvas:")) {
        const stored = await storeCanvasYjsState(documentName.slice("canvas:".length), yjsState);
        if (!stored) {
          console.warn(`[hocuspocus] Dropped canvas snapshot after session ended: ${documentName}`);
          return;
        }
      } else {
        await storeDocumentState(documentName, yjsState, plainText);
      }
      console.log(`[hocuspocus] Stored state for: ${documentName} (${plainText.length} chars)`);
    } catch (err) {
      console.error(`[hocuspocus] Failed to store state for ${documentName}:`, err);
    }
  },

  async onConnect({ documentName }: { documentName: string }) {
    console.log(`[hocuspocus] Client connected to: ${documentName}`);
  },

  async onDisconnect({ documentName, socketId }: { documentName: string; socketId?: string }) {
    console.log(`[hocuspocus] Client disconnected from: ${documentName}`);
    const connection = socketId ? canvasConnections.get(socketId) : undefined;
    if (socketId) canvasConnections.delete(socketId);
    await canvasLifecycleHooks.onDisconnect({ documentName, connection });
  },

  async afterLoadDocument({ documentName, document, instance }: { documentName: string; document: Y.Doc; instance: { documents: Map<string, Y.Doc> } }) {
    if (documentName.startsWith("canvas:")) {
      await canvasLoadLifecycle.afterLoadDocument({ documentName, document, instance });
      return;
    }
    await canvasLifecycleHooks.afterLoadDocument({ documentName, document });
  },

  async beforeUnloadDocument({ documentName, document, instance }: { documentName: string; document: Y.Doc; instance: { documents: Map<string, Y.Doc> } }) {
    if (documentName.startsWith("canvas:")) {
      await canvasLoadLifecycle.beforeUnloadDocument({ documentName, document, instance });
      return;
    }
    await canvasLifecycleHooks.beforeUnloadDocument({ documentName, document });
  },

  async connected({ documentName, context, connection, socketId }: { documentName: string; context: AuthContext; socketId?: string; connection: { onClose: (callback: () => void) => unknown; close: (event?: { code?: number; reason?: string }) => void } }) {
    if (socketId) canvasConnections.set(socketId, connection);
    if (!documentName.startsWith("canvas:") || !context.jwtExpiry) return;
    const expiry = scheduleCanvasJwtExpiry({ exp: context.jwtExpiry, connection });
    connection.onClose(() => expiry.cancel());
  },
};

/** Spec 013 relies on Bridge owning the final pre-registry after-load step. */
export function assertCanvasAfterLoadIsFinal(extensions: Array<{ afterLoadDocument?: unknown }>, finalHook: unknown): void {
  const afterLoadExtensions = extensions.filter((extension) => typeof extension.afterLoadDocument === "function");
  if (afterLoadExtensions.at(-1)?.afterLoadDocument !== finalHook) {
    throw new Error("Bridge canvas afterLoadDocument must be the final after-load extension");
  }
}

const server = new Server(hocuspocusHooks, { maxPayload: HOCUSPOCUS_SHARED_MAX_PAYLOAD });
assertCanvasAfterLoadIsFinal(server.hocuspocus.configuration.extensions, hocuspocusHooks.afterLoadDocument);

function writeControlResponse(response: ServerResponse, status: number, body: Record<string, unknown>): void {
  response.writeHead(status, { "Content-Type": "application/json", "Cache-Control": "no-store" });
  response.end(JSON.stringify(body));
}

async function readStrictJson(request: HttpIncomingMessage): Promise<unknown> {
  const chunks: Buffer[] = [];
  let size = 0;
  for await (const chunk of request) {
    const data = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk);
    size += data.length;
    if (size > 64 * 1024) throw new CanvasLifecycleError("control_body_too_large", "Control request body is too large");
    chunks.push(data);
  }
  try {
    return JSON.parse(Buffer.concat(chunks).toString("utf8"));
  } catch {
    throw new CanvasLifecycleError("invalid_control_request", "Control request JSON is invalid");
  }
}

/**
 * This listener is intentionally not a Hocuspocus hook: the browser websocket
 * server has no control routes and never receives the control bearer.
 */
export function createCanvasControlListener({
  secret,
  lifecycle = canvasLifecycle,
  host = HOCUSPOCUS_CONTROL_HOST,
  port = HOCUSPOCUS_CONTROL_PORT,
  tls,
}: {
  secret: string;
  lifecycle?: CanvasLifecycle;
  host?: string;
  port?: number;
  tls?: { key: Buffer; cert: Buffer };
}) {
  const control = createCanvasControlServer({ secret, lifecycle });
  const handler = async (request: HttpIncomingMessage, response: ServerResponse) => {
    const writer = new AbortController();
    let responseFinished = false;
    const abortWriter = () => writer.abort();
    const abortIncompleteRequest = () => {
      if (request.aborted) abortWriter();
    };
    const abortOpenResponse = () => {
      if (!responseFinished) abortWriter();
    };
    request.once("aborted", abortWriter);
    request.once("close", abortIncompleteRequest);
    response.once("close", abortOpenResponse);
    let json: unknown;
    try {
      json = request.method === "POST" ? await readStrictJson(request) : undefined;
    } catch (error) {
      const known = error instanceof CanvasLifecycleError;
      if (writer.signal.aborted) response.destroy?.();
      else writeControlResponse(response, known ? 400 : 500, { code: known ? error.code : "control_failure" });
      responseFinished = true;
      request.off("aborted", abortWriter);
      request.off("close", abortIncompleteRequest);
      response.off("close", abortOpenResponse);
      return;
    }
    const path = new URL(request.url ?? "/", "http://127.0.0.1").pathname;
    const result = await control.dispatch({
      method: request.method ?? "",
      path,
      authorization: request.headers.authorization,
      json,
    });
    if (writer.signal.aborted) {
      responseFinished = true;
      response.destroy?.();
      request.off("aborted", abortWriter);
      request.off("close", abortIncompleteRequest);
      response.off("close", abortOpenResponse);
      return;
    }
    if (path === "/internal/canvas-sessions/freeze" && result.status === 200 && json !== undefined && lifecycle.stream) {
      const parsed = parseCanvasControlRequest("freeze", json) as { sessionId: string; freezeToken: string };
      response.writeHead(200, { "Content-Type": "application/json", "Cache-Control": "no-store" });
      try {
        await lifecycle.stream({
          ...parsed,
          write: (chunk: string) => response.write(chunk),
          onceDrain: () => new Promise<void>((resolve) => response.once("drain", resolve)),
          signal: writer.signal,
        });
        if (writer.signal.aborted) {
          responseFinished = true;
          response.destroy();
          return;
        }
        responseFinished = true;
        response.end();
      } catch {
        responseFinished = true;
        response.destroy();
      } finally {
        request.off("aborted", abortWriter);
        request.off("close", abortIncompleteRequest);
        response.off?.("close", abortOpenResponse);
      }
      return;
    }
    responseFinished = true;
    writeControlResponse(response, result.status, result.json);
    request.off("aborted", abortWriter);
    request.off("close", abortIncompleteRequest);
    response.off?.("close", abortOpenResponse);
  };
  const listener = tls ? createHttpsServer(tls, handler) : createHttpServer(handler);
  return {
    listener,
    close() {
      return new Promise<void>((resolve, reject) => listener.close((error) => error ? reject(error) : resolve()));
    },
    listen() {
      return new Promise<void>((resolve, reject) => {
        const onError = (error: Error) => {
          listener.off("listening", onListening);
          reject(error);
        };
        const onListening = () => {
          listener.off("error", onError);
          resolve();
        };
        listener.once("error", onError);
        listener.once("listening", onListening);
        listener.listen({ host, port });
      });
    },
  };
}

/** Bind order is transactional: a failed websocket bind closes the private listener. */
export async function startHocuspocusListeners({ control, websocket }: { control: { listen: () => Promise<void>; close?: () => Promise<void> | void }; websocket: { listen: () => Promise<void> } }): Promise<void> {
  await control.listen();
  try {
    await websocket.listen();
  } catch (error) {
    await control.close?.();
    throw error;
  }
}

if (import.meta.main) {
  try {
    validateRealtimeAuthEnv();
    const config = validateControlEnv();
    const control = createCanvasControlListener({ secret: config.secret, host: config.host, port: config.port, tls: config.tls });
    await startHocuspocusListeners({ control, websocket: server });
    console.log(`[hocuspocus] WebSocket server running on ws://127.0.0.1:${HOCUSPOCUS_PORT}`);
    console.log(`[hocuspocus] Canvas control listener running on ${config.tls ? "https" : "http"}://${config.host}:${config.port}`);
  } catch (error) {
    console.error("[hocuspocus] refusing to start:", error);
    process.exitCode = 1;
  }
}

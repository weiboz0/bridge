import { IncomingMessage, MessageType, Server } from "@hocuspocus/server";
import { sql } from "drizzle-orm";
import { messageYjsSyncStep2, messageYjsUpdate } from "y-protocols/sync";
import * as Y from "yjs";
import { loadDocumentState, storeDocumentState } from "./documents";
import { serverDb } from "./db";
import {
  loadAttemptYjsState,
  storeAttemptYjsState,
} from "./attempts";
import { rechckDocumentAccess, verifyRealtimeJwt } from "./realtime-jwt";

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
const GO_DEFAULT_INTERNAL_URL = `http://localhost:${process.env.PLATFORM_PORT ?? "8002"}`;
const GO_INTERNAL_API_URL = process.env.GO_INTERNAL_API_URL ?? GO_DEFAULT_INTERNAL_URL;

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
    console.error(
      `[hocuspocus] refusing to start: HOCUSPOCUS_PORT=${JSON.stringify(raw)} is not a valid TCP port (1-65535).`
    );
    process.exit(1);
  }
  return port;
}
const HOCUSPOCUS_PORT = parseHocuspocusPort(process.env.HOCUSPOCUS_PORT);

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

if (import.meta.main) {
  validateRealtimeAuthEnv();
}

interface AuthContext {
  userId: string;
  role: string;
  attemptId?: string;
  canvasId?: string;
  readOnly?: boolean;
}

export function canvasAuthenticationContext({
  documentName,
  claims,
  connectionConfig,
}: {
  documentName: string;
  claims: { sub: string; role: string; readOnly: boolean };
  connectionConfig: { readOnly: boolean };
}): AuthContext {
  if (!documentName.startsWith("canvas:")) {
    throw new Error("Canvas authentication requires a canvas document");
  }
  connectionConfig.readOnly = claims.readOnly;
  return {
    userId: claims.sub,
    role: claims.role,
    canvasId: documentName.slice("canvas:".length),
    readOnly: claims.readOnly,
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
  recheck = rechckDocumentAccess,
}: {
  documentName: string;
  update: Uint8Array;
  connection: { readOnly: boolean };
  userId: string;
  recheck?: DocumentRecheck;
}): Promise<void> {
  if (!documentName.startsWith("canvas:") || connection.readOnly || !isYjsMutationFrame(update)) {
    return;
  }
  if (!userId) {
    throw new Error("Canvas mutation is missing authenticated user context");
  }
  const decision = await recheck({
    apiBaseUrl: GO_INTERNAL_API_URL,
    secret: TOKEN_SECRET,
    documentName,
    sub: userId,
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

const server = new Server({
  port: HOCUSPOCUS_PORT,
  debounce: 30000, // Save to DB every 30 seconds (also saves on disconnect)

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
      return canvasAuthenticationContext({ documentName, claims, connectionConfig });
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
        Y.applyUpdate(document, update);
        console.log(`[hocuspocus] Loaded state for: ${documentName}`);
      }
    } catch (err) {
      console.error(`[hocuspocus] Failed to load state for ${documentName}:`, err);
      if (documentName.startsWith("canvas:")) {
        throw err;
      }
    }

    return document;
  },

  async beforeHandleMessage({ documentName, connection, update, context }) {
    await guardCanvasMutationFrame({
      documentName,
      connection,
      update,
      userId: (context as AuthContext | undefined)?.userId ?? "",
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

  async onDisconnect({ documentName }: { documentName: string }) {
    console.log(`[hocuspocus] Client disconnected from: ${documentName}`);
  },
});

if (import.meta.main) {
  server.listen().then(() => {
    console.log(`[hocuspocus] WebSocket server running on ws://127.0.0.1:${HOCUSPOCUS_PORT}`);
  });
}

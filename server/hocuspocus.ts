import { Server } from "@hocuspocus/server";
import * as Y from "yjs";
import { loadDocumentState, storeDocumentState } from "./documents";
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
const GO_INTERNAL_API_URL = process.env.GO_INTERNAL_API_URL ?? "http://localhost:8002";

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
  if (isExposed && GO_INTERNAL_API_URL === "http://localhost:8002") {
    console.warn(
      "[hocuspocus] WARNING: GO_INTERNAL_API_URL is the default http://localhost:8002 in a BRIDGE_HOST_EXPOSURE=exposed environment. Realtime document loads will fail unless this points at a reachable Go API. Set GO_INTERNAL_API_URL explicitly."
    );
  }

  // Startup mode log — visible in ops at boot.
  console.log(`[hocuspocus] realtime auth mode: JWT only; exposure=${BRIDGE_HOST_EXPOSURE || "localhost (default)"}`);
}

validateRealtimeAuthEnv();

interface AuthContext {
  userId: string;
  role: string;
  attemptId?: string;
  readOnly?: boolean;
}

const server = new Server({
  port: 4000,
  debounce: 30000, // Save to DB every 30 seconds (also saves on disconnect)

  async onAuthenticate({ token, documentName }: { token: string; documentName: string }) {
    // noop documents don't carry collaboration content — short-circuit
    // before JWT verification so connection probes don't require a token.
    if (!token || documentName === "noop") {
      return { userId: "", role: "" } satisfies AuthContext;
    }

    // Plan 072 phase 2 — JWT is the ONLY auth path. Legacy userId:role
    // tokens are gone. TOKEN_SECRET is guaranteed non-empty at boot
    // (validateRealtimeAuthEnv enforces this).
    const claims = verifyRealtimeJwt(token, TOKEN_SECRET);
    if (claims.scope !== documentName) {
      throw new Error("JWT scope does not match documentName");
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

    // unit:* documents are not persisted via Hocuspocus — realtime sync only.
    // The editor saves to the teaching-unit API on demand.
    if (documentName.startsWith("broadcast:") || documentName === "noop" || documentName.startsWith("unit:")) return document;

    try {
      let yjsState: string | null = null;
      if (documentName.startsWith("attempt:")) {
        const attemptId = documentName.slice("attempt:".length);
        yjsState = await loadAttemptYjsState(attemptId);
      } else {
        yjsState = await loadDocumentState(documentName);
      }
      if (yjsState) {
        const update = Buffer.from(yjsState, "base64");
        Y.applyUpdate(document, update);
        console.log(`[hocuspocus] Loaded state for: ${documentName}`);
      }
    } catch (err) {
      console.error(`[hocuspocus] Failed to load state for ${documentName}:`, err);
    }

    return document;
  },

  async onStoreDocument({ document, documentName }: { document: Y.Doc; documentName: string }) {
    // unit:* documents are not persisted via Hocuspocus — realtime sync only.
    // The editor saves to the teaching-unit API on demand.
    if (documentName.startsWith("broadcast:") || documentName === "noop" || documentName.startsWith("unit:")) return;

    try {
      const update = Y.encodeStateAsUpdate(document);
      const yjsState = Buffer.from(update).toString("base64");
      const plainText = document.getText("content").toString();

      if (documentName.startsWith("attempt:")) {
        const attemptId = documentName.slice("attempt:".length);
        await storeAttemptYjsState(attemptId, yjsState, plainText);
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

server.listen().then(() => {
  console.log(`[hocuspocus] WebSocket server running on ws://127.0.0.1:4000`);
});

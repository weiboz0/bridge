import { Server } from "@hocuspocus/server";
import * as Y from "yjs";
import { loadDocumentState, storeDocumentState } from "./documents";
import {
  loadAttemptOwner,
  loadAttemptYjsState,
  storeAttemptYjsState,
  teacherCanViewAttempt,
} from "./attempts";
import { isLikelyJwt, rechckDocumentAccess, verifyRealtimeJwt } from "./realtime-jwt";

// Plan 072 phase 1 config (inverted from plan 053 defaults):
// - HOCUSPOCUS_TOKEN_SECRET: shared HMAC secret with the Go API.
//   REQUIRED in production. Boot fails (process.exit(1)) if unset
//   and HOCUSPOCUS_ALLOW_LEGACY_TOKEN is not set.
// - HOCUSPOCUS_ALLOW_LEGACY_TOKEN=1: dev-only escape hatch. Re-enables
//   the legacy `userId:role` token path for local development. Only
//   honored when BRIDGE_HOST_EXPOSURE is "" or "localhost"; an exposed
//   host refuses to start with this flag set (defense-in-depth).
// - BRIDGE_HOST_EXPOSURE: "" / "localhost" (default) or "exposed".
//   Mirrors the Go API's semantics (platform/cmd/api/main.go line 504).
// - GO_INTERNAL_API_URL: base URL Hocuspocus uses to call the
//   server-to-server recheck (`POST /api/internal/realtime/auth`).
//   Defaults to localhost:8002 (the Go API's local port). NOT
//   browser-reachable. A warning is logged at boot when this is the
//   default value under BRIDGE_HOST_EXPOSURE=exposed.
const TOKEN_SECRET = process.env.HOCUSPOCUS_TOKEN_SECRET ?? "";
const ALLOW_LEGACY_TOKEN = process.env.HOCUSPOCUS_ALLOW_LEGACY_TOKEN === "1";
const BRIDGE_HOST_EXPOSURE = (process.env.BRIDGE_HOST_EXPOSURE ?? "").toLowerCase().trim();
const GO_INTERNAL_API_URL = process.env.GO_INTERNAL_API_URL ?? "http://localhost:8002";

function validateRealtimeAuthEnv(): void {
  // Plan 072 phase 1 — secure-by-default realtime auth boot check.
  // Mirrors platform/cmd/api/main.go::validateDevAuthEnv pattern.

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

  // Hard fail: no signing secret AND legacy not opted-in.
  if (!TOKEN_SECRET && !ALLOW_LEGACY_TOKEN) {
    console.error(
      "[hocuspocus] refusing to start: HOCUSPOCUS_TOKEN_SECRET is unset. Set the shared HMAC secret with the Go API, or set HOCUSPOCUS_ALLOW_LEGACY_TOKEN=1 (dev-only, requires BRIDGE_HOST_EXPOSURE=localhost) to enable the legacy token path."
    );
    process.exit(1);
  }

  // Hard fail: ALLOW_LEGACY_TOKEN=1 only honored under localhost exposure.
  if (ALLOW_LEGACY_TOKEN && !isLocalhost) {
    console.error(
      `[hocuspocus] refusing to start: HOCUSPOCUS_ALLOW_LEGACY_TOKEN=1 is only honored under BRIDGE_HOST_EXPOSURE=localhost (or empty). Current BRIDGE_HOST_EXPOSURE=${JSON.stringify(BRIDGE_HOST_EXPOSURE)}.`
    );
    process.exit(1);
  }

  // Operational warning (Kimi K2.6 finding): GO_INTERNAL_API_URL default
  // localhost in an exposed environment will make every onLoadDocument
  // recheck fail post-Phase-2. Surface a warning at boot for ops.
  if (isExposed && GO_INTERNAL_API_URL === "http://localhost:8002") {
    console.warn(
      "[hocuspocus] WARNING: GO_INTERNAL_API_URL is the default http://localhost:8002 in a BRIDGE_HOST_EXPOSURE=exposed environment. Realtime document loads will fail unless this points at a reachable Go API. Set GO_INTERNAL_API_URL explicitly."
    );
  }

  // Startup mode log — visible in ops at boot.
  const mode = TOKEN_SECRET
    ? (ALLOW_LEGACY_TOKEN ? "JWT + legacy fallback (dev escape hatch)" : "JWT only")
    : "legacy only (no signing secret — dev only)";
  console.log(`[hocuspocus] realtime auth mode: ${mode}; exposure=${BRIDGE_HOST_EXPOSURE || "localhost (default)"}`);
}

validateRealtimeAuthEnv();

interface AuthContext {
  userId: string;
  role: string;
  attemptId?: string;
  readOnly?: boolean;
  // tokenKind tracks how onAuthenticate resolved the token so
  // onLoadDocument knows whether to run the DB recheck (only
  // meaningful when a verified JWT carried the access claim).
  tokenKind: "jwt" | "legacy";
}

const server = new Server({
  port: 4000,
  debounce: 30000, // Save to DB every 30 seconds (also saves on disconnect)

  async onAuthenticate({ token, documentName }: { token: string; documentName: string }) {
    if (!token) {
      throw new Error("Authentication required");
    }

    // Plan 053 phase 2 — JWT path. If the token looks like a JWT
    // (starts with `ey` and has 3 dot-separated parts), verify it
    // signed by the Go API. Scope MUST equal documentName
    // byte-for-byte. The Go mint endpoint already enforced per-doc
    // access at mint time; the recheck in onLoadDocument catches
    // the "user demoted between mint and connect" race.
    if (isLikelyJwt(token)) {
      if (!TOKEN_SECRET) {
        throw new Error("Realtime tokens not configured");
      }
      const claims = verifyRealtimeJwt(token, TOKEN_SECRET);
      if (claims.scope !== documentName) {
        throw new Error("JWT scope does not match documentName");
      }
      const ctx: AuthContext = {
        userId: claims.sub,
        role: claims.role,
        tokenKind: "jwt",
      };
      // Carry attempt readOnly into the context for parity with the
      // legacy attempt path. JWT scope `attempt:{aid}` was minted
      // owner-only in phase 1, so readOnly = false until phase 2/3
      // adds the teacher-watch path.
      if (documentName.startsWith("attempt:")) {
        ctx.attemptId = documentName.slice("attempt:".length);
        ctx.readOnly = false;
      }
      return ctx;
    }

    // Plan 072 phase 1 — inverted gate. Legacy userId:role tokens are now
    // opt-in (HOCUSPOCUS_ALLOW_LEGACY_TOKEN=1) rather than the default.
    // Boot validation already enforces that the flag is only honored under
    // localhost exposure, so this check at authenticate-time is the runtime
    // guard per connection.
    if (!ALLOW_LEGACY_TOKEN) {
      throw new Error("Legacy userId:role tokens are disabled. Set HOCUSPOCUS_ALLOW_LEGACY_TOKEN=1 (dev-only, requires BRIDGE_HOST_EXPOSURE=localhost) to enable.");
    }

    // Backward-compat path — unchanged from pre-053 behavior.
    const [userId, role] = token.split(":");
    if (!userId || !role) {
      throw new Error("Invalid token format");
    }

    const parts = documentName.split(":");

    // Existing session document — student/teacher live editor.
    //
    // TODO(030c): Direct-add access was extended in 030c. The Hocuspocus auth check should verify
    // session membership via Go's CanAccessSession endpoint in a follow-up (same gap noted in 030b).
    //
    // TODO (030b follow-up): This check is permissive. It allows any authenticated
    // user whose token matches the `userId:role` format to open any session document
    // they are nominally the owner of, without verifying they actually have a
    // `session_participants` row or a valid invite token. Plan 030b added token-based
    // session joins (via `POST /api/s/{token}/join`), which creates the participant
    // row on the Go side — so a properly joined user will already have access at the
    // DB level. However, Hocuspocus cannot verify that here because it has no direct
    // access to the Go session store. A forged `userId:role` token could theoretically
    // open any session document whose `docOwner` matches the forged userId.
    //
    // The real fix is to call Go's `CanAccessSession` helper from here (or a purpose-
    // built lightweight Go endpoint) on every WebSocket upgrade. That network hop was
    // deferred in 030b because the risk is pre-existing and not introduced by the
    // invite-token flow. Track this as a Hocuspocus auth hardening follow-up (030c+).
    if (parts[0] === "session" && parts[2] === "user") {
      const docOwner = parts[3];
      if (role !== "teacher" && role !== "user" && role !== "parent" && userId !== docOwner) {
        throw new Error("Access denied");
      }
      return { userId, role, tokenKind: "legacy" } satisfies AuthContext;
    }

    // Plan 025b — attempt:{attemptId} room. Owner = read+write. A teacher who
    // shares a class with the student in the problem's course = read-only.
    if (parts[0] === "attempt" && parts[1]) {
      const attemptId = parts[1];
      const owner = await loadAttemptOwner(attemptId);
      if (!owner) {
        throw new Error("Attempt not found");
      }
      if (owner.userId === userId) {
        return { userId, role, attemptId, readOnly: false, tokenKind: "legacy" } satisfies AuthContext;
      }
      if (role === "teacher" || role === "user") {
        const allowed = await teacherCanViewAttempt(userId, attemptId);
        if (allowed) {
          return { userId, role, attemptId, readOnly: true, tokenKind: "legacy" } satisfies AuthContext;
        }
      }
      throw new Error("Access denied");
    }

    if (parts[0] === "broadcast") {
      // Anyone in the session can read broadcast documents
      return { userId, role, tokenKind: "legacy" } satisfies AuthContext;
    }

    // Plan 035 — unit:{unitId} namespace for teaching unit collaborative editing.
    // The Hocuspocus server provides realtime CRDT sync only; persistence happens
    // via the teaching-unit API (save button), not Hocuspocus's onStoreDocument.
    // Auth: role === "teacher" check only. Per-unit scope/ownership validation
    // (e.g., verifying the teacher belongs to the unit's org) requires calling
    // Go's canEditUnit — deferred to a follow-up that adds a purpose-built Go
    // auth endpoint for Hocuspocus (same gap as session auth noted in 030b/030c).
    if (parts[0] === "unit" && parts[1]) {
      if (role !== "teacher") {
        throw new Error("Access denied: only teachers may collaborate on unit documents");
      }
      return { userId, role, tokenKind: "legacy" } satisfies AuthContext;
    }

    if (documentName === "noop") {
      return { userId, role, tokenKind: "legacy" } satisfies AuthContext;
    }

    throw new Error("Invalid document name format");
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
    // Plan 053 phase 2 — defense-in-depth DB recheck. Only meaningful
    // for the JWT path: the Go mint endpoint enforced access at mint
    // time, but a user could be demoted in the 25-min window before
    // the connection lands here. Re-asks the Go API "does this user
    // STILL have access to this doc?" If the Go side says no, throw
    // — Hocuspocus tears down the connection.
    //
    // Legacy `userId:role` path skips the recheck (it's already the
    // pre-053 behavior; the recheck only applies to JWT-authed
    // connections during the rollout window).
    if (context?.tokenKind === "jwt" && TOKEN_SECRET) {
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

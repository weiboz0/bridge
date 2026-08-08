import { afterEach, describe, expect, test } from "bun:test";
import { Document, IncomingMessage, MessageReceiver, OutgoingMessage } from "@hocuspocus/server";
import { createHmac, randomUUID } from "node:crypto";
import postgres from "postgres";
import { Awareness } from "y-protocols/awareness";
import { writeSyncStep2 } from "y-protocols/sync";
import * as Y from "yjs";

import { JwtVerifyError, rechckDocumentAccess, verifyRealtimeJwt } from "./realtime-jwt";

const secret = "canvas-test-secret";

function signedClaims(claims: Record<string, unknown>): string {
  const header = Buffer.from(JSON.stringify({ alg: "HS256", typ: "JWT" })).toString("base64url");
  const payload = Buffer.from(JSON.stringify({
    sub: "11111111-1111-4111-8111-111111111111",
    role: "user",
    scope: "canvas:22222222-2222-4222-8222-222222222222",
    iss: "bridge-platform",
    iat: Math.floor(Date.now() / 1000),
    exp: Math.floor(Date.now() / 1000) + 60,
    ...claims,
  })).toString("base64url");
  const signature = createHmac("sha256", secret).update(`${header}.${payload}`).digest("base64url");
  return `${header}.${payload}.${signature}`;
}

afterEach(() => {
  globalThis.fetch = originalFetch;
});

const originalFetch = globalThis.fetch;
const canvasTestDatabaseUrl = "postgresql://work@127.0.0.1:5432/bridge_test";

function pinnedCanvasTestDatabaseUrl(): string {
  expect(process.env.DATABASE_URL).toBe(canvasTestDatabaseUrl);
  expect(process.env.TEST_DATABASE_URL).toBe(canvasTestDatabaseUrl);
  expect(new URL(canvasTestDatabaseUrl).pathname.slice(1).endsWith("_test")).toBe(true);
  return canvasTestDatabaseUrl;
}

describe("hocuspocus canvas hook test seam", () => {
  test("importing the server module does not validate configuration or bind a port", () => {
    const databaseUrl = process.env.DATABASE_URL ?? "";
    const databaseName = new URL(databaseUrl).pathname.slice(1);
    expect(databaseName.endsWith("_test")).toBe(true);

    const child = Bun.spawnSync({
      cmd: [process.execPath, "-e", 'await import("./server/hocuspocus.ts")'],
      cwd: process.cwd(),
      env: {
        ...process.env,
        DATABASE_URL: databaseUrl,
        HOCUSPOCUS_TOKEN_SECRET: "",
      },
      stdout: "pipe",
      stderr: "pipe",
    });

    expect(child.exitCode).toBe(0);
  });

  test("classifies only mutation-bearing Yjs sync frames", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const classify = runtime.isYjsMutationFrame;
    expect(typeof classify).toBe("function");
    if (typeof classify !== "function") return;

    const documentName = "canvas:22222222-2222-4222-8222-222222222222";
    const changed = new Y.Doc();
    changed.getMap("elements").set("shape", { type: "rectangle" });
    const updateFrame = new OutgoingMessage(documentName)
      .createSyncMessage()
      .writeUpdate(Y.encodeStateAsUpdate(changed))
      .toUint8Array();
    const step2 = new OutgoingMessage(documentName).createSyncMessage();
    writeSyncStep2(step2.encoder, changed);

    expect(classify(updateFrame)).toBe(true);
    expect(classify(step2.toUint8Array())).toBe(true);
    expect(classify(new OutgoingMessage(documentName).writeQueryAwareness().toUint8Array())).toBe(false);
    expect(classify(new OutgoingMessage(documentName).createAwarenessUpdateMessage(new Awareness(changed)).toUint8Array())).toBe(false);
  });

  test("rechecks writable canvas mutations and flips the connection before apply", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const guard = runtime.guardCanvasMutationFrame;
    expect(typeof guard).toBe("function");
    if (typeof guard !== "function") return;

    const documentName = "canvas:22222222-2222-4222-8222-222222222222";
    const changed = new Y.Doc();
    changed.getMap("elements").set("shape", { type: "rectangle" });
    const mutation = new OutgoingMessage(documentName)
      .createSyncMessage()
      .writeUpdate(Y.encodeStateAsUpdate(changed))
      .toUint8Array();
    const awareness = new OutgoingMessage(documentName).writeQueryAwareness().toUint8Array();
    const connection = { readOnly: false };
    let rechecks = 0;
    const recheck = async () => {
      rechecks += 1;
      return { allowed: true, readOnly: true };
    };

    await guard({ documentName, update: awareness, connection, userId: "user-1", recheck });
    expect(rechecks).toBe(0);
    await guard({ documentName, update: mutation, connection, userId: "user-1", recheck });
    expect(rechecks).toBe(1);
    expect(connection.readOnly).toBe(true);
    await guard({ documentName, update: mutation, connection, userId: "user-1", recheck });
    expect(rechecks).toBe(1);
  });

  test("fails closed when a canvas mutation recheck denies or errors", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const guard = runtime.guardCanvasMutationFrame;
    expect(typeof guard).toBe("function");
    if (typeof guard !== "function") return;

    const documentName = "canvas:22222222-2222-4222-8222-222222222222";
    const changed = new Y.Doc();
    changed.getMap("elements").set("shape", { type: "ellipse" });
    const mutation = new OutgoingMessage(documentName)
      .createSyncMessage()
      .writeUpdate(Y.encodeStateAsUpdate(changed))
      .toUint8Array();

    await expect(guard({
      documentName,
      update: mutation,
      connection: { readOnly: false },
      userId: "user-1",
      recheck: async () => ({ allowed: false, readOnly: true, reason: "removed" }),
    })).rejects.toThrow(/removed/);
    await expect(guard({
      documentName,
      update: mutation,
      connection: { readOnly: false },
      userId: "user-1",
      recheck: async () => { throw new Error("database unavailable"); },
    })).rejects.toThrow(/database unavailable/);
  });

  test("fails closed when a malformed raw frame reaches the canvas mutation guard", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const guard = runtime.guardCanvasMutationFrame;
    expect(typeof guard).toBe("function");
    if (typeof guard !== "function") return;

    let rechecks = 0;
    await expect(guard({
      documentName: "canvas:22222222-2222-4222-8222-222222222222",
      update: new Uint8Array([0xff]),
      connection: { readOnly: false },
      userId: "owner",
      recheck: async () => {
        rechecks += 1;
        return { allowed: true, readOnly: false };
      },
    })).rejects.toThrow();
    expect(rechecks).toBe(0);
  });

  test("fails closed when a canvas mutation lacks authenticated user context", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const guard = runtime.guardCanvasMutationFrame;
    expect(typeof guard).toBe("function");
    if (typeof guard !== "function") return;

    const source = new Y.Doc();
    source.getMap("elements").set("shape", { type: "rectangle" });
    const mutation = new OutgoingMessage("canvas:22222222-2222-4222-8222-222222222222")
      .createSyncMessage()
      .writeUpdate(Y.encodeStateAsUpdate(source))
      .toUint8Array();
    let rechecks = 0;

    await expect(guard({
      documentName: "canvas:22222222-2222-4222-8222-222222222222",
      update: mutation,
      connection: { readOnly: false },
      userId: "",
      recheck: async () => {
        rechecks += 1;
        return { allowed: true, readOnly: false };
      },
    })).rejects.toThrow(/missing authenticated user context/);
    expect(rechecks).toBe(0);
  });

  test("applies the signed canvas readOnly claim to the enforcing connection flag", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const authenticate = runtime.canvasAuthenticationContext;
    expect(typeof authenticate).toBe("function");
    if (typeof authenticate !== "function") return;

    const connectionConfig = { readOnly: false, isAuthenticated: false };
    const context = authenticate({
      documentName: "canvas:22222222-2222-4222-8222-222222222222",
      claims: {
        sub: "11111111-1111-4111-8111-111111111111",
        role: "user",
        readOnly: true,
      },
      connectionConfig,
    }) as { canvasId?: string; readOnly?: boolean };

    expect(connectionConfig.readOnly).toBe(true);
    expect(context.readOnly).toBe(true);
    expect(context.canvasId).toBe("22222222-2222-4222-8222-222222222222");
  });

  test("a read-only canvas connection cannot apply or relay an update", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const authenticate = runtime.canvasAuthenticationContext;
    expect(typeof authenticate).toBe("function");
    if (typeof authenticate !== "function") return;

    const documentName = "canvas:22222222-2222-4222-8222-222222222222";
    const source = new Y.Doc();
    source.getMap("elements").set("shape", { type: "rectangle" });
    const frame = new OutgoingMessage(documentName)
      .createSyncMessage()
      .writeUpdate(Y.encodeStateAsUpdate(source))
      .toUint8Array();
    const config = { readOnly: false, isAuthenticated: false };
    authenticate({ documentName, claims: { sub: "viewer", role: "user", readOnly: true }, connectionConfig: config });

    const target = new Document(documentName);
    let relayedUpdates = 0;
    target.onUpdate(() => { relayedUpdates += 1; });
    const connection = {
      readOnly: config.readOnly,
      send: () => undefined,
      callbacks: { beforeSync: () => undefined },
    };
    const incoming = new IncomingMessage(frame);
    expect(incoming.readVarString()).toBe(documentName);
    incoming.writeVarString(documentName);
    new MessageReceiver(incoming).apply(target, connection as never);

    expect(target.getMap("elements").size).toBe(0);
    expect(relayedUpdates).toBe(0);
  });

  test("the first owner mutation after end is flipped read-only before apply or relay", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const guard = runtime.guardCanvasMutationFrame;
    expect(typeof guard).toBe("function");
    if (typeof guard !== "function") return;

    const documentName = "canvas:22222222-2222-4222-8222-222222222222";
    const source = new Y.Doc();
    source.getMap("elements").set("late", { type: "diamond" });
    const frame = new OutgoingMessage(documentName)
      .createSyncMessage()
      .writeUpdate(Y.encodeStateAsUpdate(source))
      .toUint8Array();
    const target = new Document(documentName);
    let relayedUpdates = 0;
    target.onUpdate(() => { relayedUpdates += 1; });
    const connection = {
      readOnly: false,
      send: () => undefined,
      callbacks: { beforeSync: () => undefined },
    };

    await guard({
      documentName,
      update: frame,
      connection,
      userId: "owner",
      recheck: async () => ({ allowed: true, readOnly: true }),
    });
    const incoming = new IncomingMessage(frame);
    incoming.readVarString();
    incoming.writeVarString(documentName);
    new MessageReceiver(incoming).apply(target, connection as never);

    expect(connection.readOnly).toBe(true);
    expect(target.getMap("elements").size).toBe(0);
    expect(relayedUpdates).toBe(0);
  });

  test("surfaces a canvas persistence load query failure instead of returning blank state", async () => {
    const databaseUrl = pinnedCanvasTestDatabaseUrl();
    const db = postgres(databaseUrl, { max: 1 });
    try {
      const [{ currentDatabase }] = await db<{ currentDatabase: string }[]>`SELECT current_database() AS "currentDatabase"`;
      expect(currentDatabase.endsWith("_test")).toBe(true);

      const runtime = await import("./hocuspocus") as Record<string, unknown>;
      const load = runtime.loadCanvasYjsState;
      expect(typeof load).toBe("function");
      if (typeof load !== "function") return;

      await expect(load("not-a-uuid")).rejects.toThrow();
    } finally {
      await db.end();
    }
  });

  test("persists a live owner Yjs update, restores its map content, and refuses a late ended-session write", async () => {
    const databaseUrl = pinnedCanvasTestDatabaseUrl();
    const db = postgres(databaseUrl, { max: 1 });
    const userId = randomUUID();
    const sessionId = randomUUID();
    const canvasId = randomUUID();
    try {
      const [{ currentDatabase }] = await db<{ currentDatabase: string }[]>`SELECT current_database() AS "currentDatabase"`;
      expect(currentDatabase.endsWith("_test")).toBe(true);
      await db`INSERT INTO users (id, name, email) VALUES (${userId}::uuid, 'Canvas persistence', ${`canvas-${canvasId}@example.test`})`;
      await db`INSERT INTO sessions (id, teacher_id, title) VALUES (${sessionId}::uuid, ${userId}::uuid, 'Canvas persistence')`;
      await db`INSERT INTO session_canvases (id, session_id, owner_id, title, visibility) VALUES (${canvasId}::uuid, ${sessionId}::uuid, ${userId}::uuid, 'Canvas', 'private')`;

      const runtime = await import("./hocuspocus") as Record<string, unknown>;
      const store = runtime.storeCanvasYjsState;
      const load = runtime.loadCanvasYjsState;
      expect(typeof store).toBe("function");
      expect(typeof load).toBe("function");
      if (typeof store !== "function" || typeof load !== "function") return;

      const beforeEnd = new Y.Doc();
      beforeEnd.getMap("elements").set("owner-element", { type: "rectangle", owner: "owner" });
      const firstState = Buffer.from(Y.encodeStateAsUpdate(beforeEnd)).toString("base64");
      expect(await store(canvasId, firstState)).toBe(true);
      const loadedState = await load(canvasId);
      expect(loadedState).toBe(firstState);
      const restored = new Y.Doc();
      Y.applyUpdate(restored, Buffer.from(loadedState ?? "", "base64"));
      expect(restored.getMap("elements").toJSON()).toEqual({
        "owner-element": { type: "rectangle", owner: "owner" },
      });

      await db`UPDATE sessions SET status = 'ended', ended_at = now() WHERE id = ${sessionId}::uuid`;
      beforeEnd.getMap("elements").set("late", { type: "diamond" });
      const lateState = Buffer.from(Y.encodeStateAsUpdate(beforeEnd)).toString("base64");
      expect(await store(canvasId, lateState)).toBe(false);
      expect(await load(canvasId)).toBe(firstState);
    } finally {
      await db`DELETE FROM session_canvases WHERE id = ${canvasId}::uuid`;
      await db`DELETE FROM sessions WHERE id = ${sessionId}::uuid`;
      await db`DELETE FROM users WHERE id = ${userId}::uuid`;
      await db.end();
    }
  });
});

describe("canvas realtime JWT compatibility", () => {
  test("defaults a missing readOnly claim to false", () => {
    expect(verifyRealtimeJwt(signedClaims({}), secret).readOnly).toBe(false);
  });

  test("rejects a present non-boolean readOnly claim", () => {
    expect(() => verifyRealtimeJwt(signedClaims({ readOnly: "false" }), secret)).toThrow(JwtVerifyError);
  });

  test("fails closed when the internal auth 200 body is malformed", async () => {
    globalThis.fetch = async () => new Response(JSON.stringify({ allowed: true, readOnly: "false" }), { status: 200 });
    await expect(rechckDocumentAccess({ apiBaseUrl: "http://api.example", secret, documentName: "canvas:22222222-2222-4222-8222-222222222222", sub: "11111111-1111-4111-8111-111111111111" })).rejects.toThrow(JwtVerifyError);
  });

  test("keeps non-canvas internal responses without readOnly compatible", async () => {
    globalThis.fetch = async () => new Response(JSON.stringify({ allowed: true }), { status: 200 });
    await expect(rechckDocumentAccess({ apiBaseUrl: "http://api.example", secret, documentName: "session:22222222-2222-4222-8222-222222222222:user:11111111-1111-4111-8111-111111111111", sub: "11111111-1111-4111-8111-111111111111" })).resolves.toEqual({ allowed: true, reason: undefined });
  });

  test("fails closed when allowed is not a boolean", async () => {
    globalThis.fetch = async () => new Response(JSON.stringify({ allowed: "true", readOnly: false }), { status: 200 });
    await expect(rechckDocumentAccess({ apiBaseUrl: "http://api.example", secret, documentName: "session:22222222-2222-4222-8222-222222222222:user:11111111-1111-4111-8111-111111111111", sub: "11111111-1111-4111-8111-111111111111" })).rejects.toThrow(JwtVerifyError);
  });
});

import { afterEach, describe, expect, test } from "bun:test";
import { Document, Hocuspocus, IncomingMessage, MessageReceiver, OutgoingMessage } from "@hocuspocus/server";
import { createHmac, randomUUID } from "node:crypto";
import postgres from "postgres";
import { Awareness } from "y-protocols/awareness";
import { writeSyncStep2 } from "y-protocols/sync";
import * as Y from "yjs";

import { JwtVerifyError, rechckDocumentAccess, verifyRealtimeJwt } from "./realtime-jwt";

const secret = "canvas-test-secret";
const phase10CanvasId = "22222222-2222-4222-8222-222222222222";

function signedClaims(claims: Record<string, unknown>): string {
  const header = Buffer.from(JSON.stringify({ alg: "HS256", typ: "JWT" })).toString("base64url");
  const payload = Buffer.from(JSON.stringify({
    sub: "11111111-1111-4111-8111-111111111111",
    role: "user",
    scope: "canvas:22222222-2222-4222-8222-222222222222",
    sessionId: "11111111-1111-4111-8111-111111111111",
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

function pinnedCanvasTestDatabaseUrl(): string {
  const databaseUrl = process.env.DATABASE_URL ?? "";
  const databaseName = new URL(databaseUrl).pathname.slice(1);
  expect(databaseName.endsWith("_test")).toBe(true);
  return databaseUrl;
}

type CanvasHooks = {
  beforeHandleMessage(input: {
    documentName: string;
    connection: { readOnly: boolean };
    update: Uint8Array;
    context: { userId: string; sessionId?: string };
  }): Promise<void>;
  onLoadDocument(input: {
    document: Y.Doc;
    documentName: string;
    context: { userId: string; sessionId?: string };
  }): Promise<Y.Doc>;
  onStoreDocument(input: {
    document: Y.Doc;
    documentName: string;
  }): Promise<void>;
};

function registeredCanvasHooks(runtime: Record<string, unknown>): CanvasHooks {
  const hooks = runtime.hocuspocusHooks;
  expect(hooks).toBeDefined();
  return hooks as CanvasHooks;
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

    await guard({ documentName, update: awareness, connection, userId: "user-1", sessionId: "11111111-1111-4111-8111-111111111111", recheck });
    expect(rechecks).toBe(0);
    await guard({ documentName, update: mutation, connection, userId: "user-1", sessionId: "11111111-1111-4111-8111-111111111111", recheck });
    expect(rechecks).toBe(1);
    expect(connection.readOnly).toBe(true);
    await guard({ documentName, update: mutation, connection, userId: "user-1", sessionId: "11111111-1111-4111-8111-111111111111", recheck });
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
      sessionId: "11111111-1111-4111-8111-111111111111",
      recheck: async () => ({ allowed: false, readOnly: true, reason: "removed" }),
    })).rejects.toThrow(/removed/);
    await expect(guard({
      documentName,
      update: mutation,
      connection: { readOnly: false },
      userId: "user-1",
      sessionId: "11111111-1111-4111-8111-111111111111",
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
        sessionId: "11111111-1111-4111-8111-111111111111",
      },
      connectionConfig,
    }) as { canvasId?: string; readOnly?: boolean };

    expect(connectionConfig.readOnly).toBe(true);
    expect(context.readOnly).toBe(true);
    expect(context.canvasId).toBe("22222222-2222-4222-8222-222222222222");
  });

  test("retains verified canvas sessionId in context and propagates it to admission and mutation rechecks", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const authenticate = runtime.canvasAuthenticationContext as ((input: unknown) => { sessionId?: string }) | undefined;
    expect(authenticate).toBeTypeOf("function");
    const documentName = "canvas:22222222-2222-4222-8222-222222222222";
    const sessionId = "11111111-1111-4111-8111-111111111111";
    const context = authenticate!({ documentName, claims: { sub: "user", role: "user", readOnly: false, sessionId }, connectionConfig: { readOnly: false } });
    expect(context.sessionId).toBe(sessionId);

    const calls: unknown[] = [];
    globalThis.fetch = async (_url, init) => {
      calls.push(JSON.parse(String(init?.body)));
      return new Response(JSON.stringify({ allowed: true, readOnly: false }));
    };
    const hooks = registeredCanvasHooks(runtime);
    await expect(hooks.onLoadDocument({ document: new Y.Doc(), documentName, context: context as { userId: string } })).rejects.toThrow(/Canvas does not exist/);
    expect(calls.at(-1)).toMatchObject({ documentName, sub: "user", sessionId });

    const changed = new Y.Doc();
    changed.getMap("elements").set("shape", { type: "rectangle" });
    const update = new OutgoingMessage(documentName)
      .createSyncMessage()
      .writeUpdate(Y.encodeStateAsUpdate(changed))
      .toUint8Array();
    const recheckCalls: unknown[] = [];
    const guard = runtime.guardCanvasMutationFrame as ((input: Record<string, unknown>) => Promise<void>) | undefined;
    await guard!({
      documentName,
      update,
      connection: { readOnly: false },
      userId: "user",
      sessionId,
      recheck: async (input: unknown) => {
        recheckCalls.push(input);
        return { allowed: true, readOnly: false };
      },
    });
    expect(recheckCalls.at(-1)).toMatchObject({ documentName, sub: "user", sessionId });
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
    authenticate({ documentName, claims: { sub: "viewer", role: "user", readOnly: true, sessionId: "11111111-1111-4111-8111-111111111111" }, connectionConfig: config });

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
      sessionId: "11111111-1111-4111-8111-111111111111",
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

  test("the registered beforeHandleMessage hook blocks a post-end mutation before observer relay", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const hooks = registeredCanvasHooks(runtime);
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
    let rechecks = 0;
    globalThis.fetch = async () => {
      rechecks += 1;
      return new Response(JSON.stringify({ allowed: true, readOnly: true }));
    };

    await hooks.beforeHandleMessage({
      documentName,
      connection,
      update: frame,
      context: { userId: "owner", sessionId: "11111111-1111-4111-8111-111111111111" },
    });
    const incoming = new IncomingMessage(frame);
    incoming.readVarString();
    incoming.writeVarString(documentName);
    new MessageReceiver(incoming).apply(target, connection as never);

    expect(rechecks).toBe(1);
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

  test("the registered load hook rejects a valid canvas ID whose persisted Yjs state is missing", async () => {
    const databaseUrl = pinnedCanvasTestDatabaseUrl();
    const db = postgres(databaseUrl, { max: 1 });
    try {
      const [{ currentDatabase }] = await db<{ currentDatabase: string }[]>`SELECT current_database() AS "currentDatabase"`;
      expect(currentDatabase.endsWith("_test")).toBe(true);

      const runtime = await import("./hocuspocus") as Record<string, unknown>;
      const hooks = registeredCanvasHooks(runtime);
      globalThis.fetch = async () => new Response(JSON.stringify({ allowed: true, readOnly: false }));

      await expect(hooks.onLoadDocument({
        document: new Y.Doc(),
        documentName: `canvas:${randomUUID()}`,
        context: { userId: "owner", sessionId: "11111111-1111-4111-8111-111111111111" },
      })).rejects.toThrow(/Canvas does not exist/);
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
    let databaseVerified = false;
    try {
      const [{ currentDatabase }] = await db<{ currentDatabase: string }[]>`SELECT current_database() AS "currentDatabase"`;
      expect(currentDatabase.endsWith("_test")).toBe(true);
      databaseVerified = true;
      await db`INSERT INTO users (id, name, email) VALUES (${userId}::uuid, 'Canvas persistence', ${`canvas-${canvasId}@example.test`})`;
      await db`INSERT INTO sessions (id, teacher_id, title) VALUES (${sessionId}::uuid, ${userId}::uuid, 'Canvas persistence')`;
      await db`INSERT INTO session_canvases (id, session_id, owner_id, title, visibility) VALUES (${canvasId}::uuid, ${sessionId}::uuid, ${userId}::uuid, 'Canvas', 'private')`;

      const runtime = await import("./hocuspocus") as Record<string, unknown>;
      const store = runtime.storeCanvasYjsState;
      const load = runtime.loadCanvasYjsState;
      const hooks = registeredCanvasHooks(runtime);
      expect(typeof store).toBe("function");
      expect(typeof load).toBe("function");
      if (typeof store !== "function" || typeof load !== "function") return;

      const beforeEnd = new Y.Doc();
      beforeEnd.getMap("elements").set("owner-element", { type: "rectangle", owner: "owner" });
      const firstState = Buffer.from(Y.encodeStateAsUpdate(beforeEnd)).toString("base64");
      expect(await store(canvasId, firstState)).toBe(true);
      const restored = new Y.Doc();
      globalThis.fetch = async () => new Response(JSON.stringify({ allowed: true, readOnly: false }));
      await hooks.onLoadDocument({
        document: restored,
        documentName: `canvas:${canvasId}`,
        context: { userId, sessionId },
      });
      expect(restored.getMap("elements").toJSON()).toEqual({
        "owner-element": { type: "rectangle", owner: "owner" },
      });

      await db`UPDATE sessions SET status = 'ended', ended_at = now() WHERE id = ${sessionId}::uuid`;
      beforeEnd.getMap("elements").set("late", { type: "diamond" });
      await hooks.onStoreDocument({ document: beforeEnd, documentName: `canvas:${canvasId}` });
      expect(await load(canvasId)).toBe(firstState);
    } finally {
      if (databaseVerified) {
        await db`DELETE FROM session_canvases WHERE id = ${canvasId}::uuid`;
        await db`DELETE FROM sessions WHERE id = ${sessionId}::uuid`;
        await db`DELETE FROM users WHERE id = ${userId}::uuid`;
      }
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

describe("Phase 10 installed Hocuspocus hook RED contract", () => {
  test("propagates an actual Go 409 session_freezing response as the retryable client condition", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const hooks = runtime.createCanvasLifecycleHooks as ((input: Record<string, unknown>) => Record<string, (input: Record<string, unknown>) => Promise<unknown>>) | undefined;
    expect(hooks).toBeTypeOf("function");
    globalThis.fetch = async () => new Response(JSON.stringify({ code: "session_freezing" }), { status: 409 });
    const changed = new Y.Doc();
    changed.getMap("elements").set("fenced", "shape");
    const frame = new OutgoingMessage(`canvas:${phase10CanvasId}`).createSyncMessage().writeUpdate(Y.encodeStateAsUpdate(changed)).toUint8Array();
    const installed = hooks!({ lifecycle: {} });
    await expect(installed.beforeHandleMessage({ documentName: `canvas:${phase10CanvasId}`, connection: { readOnly: false }, update: frame, context: { userId: "writer", sessionId: randomUUID() } })).rejects.toMatchObject({ code: "session_freezing", retryable: true });
  });

  test("validates the outbound Go internal URL before any bearer can be sent", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const validate = runtime.validateGoInternalApiUrl as ((value: string) => string) | undefined;
    expect(validate).toBeTypeOf("function");
    expect(validate!("http://127.0.0.1:8002")).toBe("http://127.0.0.1:8002");
    expect(validate!("http://[::1]:8002")).toBe("http://[::1]:8002");
    expect(validate!("https://api.bridge.example/internal")).toBe("https://api.bridge.example/internal");
    for (const unsafe of ["http://localhost:8002", "http://api.bridge.example", "http://10.0.0.1:8002", "ftp://127.0.0.1", "https://127.0.0.1:8002@evil.example"]) {
      expect(() => validate!(unsafe)).toThrow();
    }
  });

  test("the registered connected hook closes an installed canvas connection at expiry so provider reauthentication can begin", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const hooks = registeredCanvasHooks(runtime) as CanvasHooks & { connected?: (input: Record<string, unknown>) => Promise<void> };
    let closed = 0;
    const callbacks: Array<() => void> = [];
    await hooks.connected?.({ documentName: `canvas:${phase10CanvasId}`, context: { jwtExpiry: Math.floor((Date.now() - 1) / 1_000) }, connection: { close: () => closed++, onClose: (callback: () => void) => callbacks.push(callback) } });
    await Bun.sleep(0);
    expect(closed).toBe(1);
    callbacks.forEach((callback) => callback());
  });

  test("installs after-load and before-unload hooks around the real Hocuspocus registry instead of releasing a generation from a logical load alone", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const hooks = registeredCanvasHooks(runtime) as CanvasHooks & {
      afterLoadDocument?: (input: Record<string, unknown>) => Promise<void>;
      beforeUnloadDocument?: (input: Record<string, unknown>) => Promise<void>;
    };
    expect(hooks.afterLoadDocument).toBeTypeOf("function");
    expect(hooks.beforeUnloadDocument).toBeTypeOf("function");
    const document = new Document(`canvas:${phase10CanvasId}`);
    const registry = new Map([[document.name, document]]);
    const instance = { documents: registry };
    await hooks.afterLoadDocument!({ documentName: document.name, document, instance });
    await hooks.beforeUnloadDocument!({ documentName: document.name, document, instance });
    // Pinned 3.4.4 can cancel unload after its hook; instrumentation must
    // survive that path until the installed document's destroy event.
    expect(registry.get(document.name)).toBe(document);
    document.destroy();
  });

  test("reclaims a failed installed createDocument before registry insertion and does not let a stale watchdog release its replacement generation", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const hooks = runtime.createCanvasLoadLifecycleHooks as ((input: Record<string, unknown>) => Record<string, unknown>) | undefined;
    expect(hooks).toBeTypeOf("function");
    const lifecycle = hooks!({ failAfterLoad: true });
    const instance = new Hocuspocus({ ...lifecycle });
    const documentName = `canvas:${phase10CanvasId}`;
    await expect(instance.createDocument(documentName, {}, "socket", { readOnly: false }, { userId: "writer", sessionId: randomUUID() })).rejects.toThrow();
    expect(instance.documents.has(documentName)).toBe(false);
    const replacement = await instance.createDocument(documentName, {}, "socket-2", { readOnly: false }, { userId: "writer", sessionId: randomUUID() });
    await new Promise<void>((resolve) => setImmediate(resolve));
    expect(instance.documents.get(documentName)).toBe(replacement);
    await instance.unloadDocument(replacement);
  });

  test("rolls back the authenticated control listener if the installed websocket listener fails after control bind", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const start = runtime.startHocuspocusListeners as ((input: Record<string, unknown>) => Promise<void>) | undefined;
    expect(start).toBeTypeOf("function");
    let controlClosed = 0;
    await expect(start!({
      control: { listen: async () => undefined, close: async () => { controlClosed += 1; } },
      websocket: { listen: async () => { throw new Error("websocket bind failed"); } },
    })).rejects.toThrow("websocket bind failed");
    expect(controlClosed).toBe(1);
  });

  test("streams the frozen bundle incrementally through the real control response, yields after backpressure, and settles reader ownership on response destroy", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const stream = runtime.streamCanvasFreezeResponse as ((input: Record<string, unknown>) => Promise<void>) | undefined;
    expect(stream).toBeTypeOf("function");
    const writes: string[] = [];
    let destroyed: (() => void) | undefined;
    await stream!({
      result: { snapshots: [{ canvasId: phase10CanvasId, stateBase64: "AA==", sha256: "0".repeat(64) }], closed: 0 },
      response: { write: (part: string) => { writes.push(part); return false; }, once: (event: string, callback: () => void) => { if (event === "close") destroyed = callback; }, end() {} },
      waitForDrain: async () => { destroyed?.(); },
    });
    expect(writes.length).toBeGreaterThan(1);
  });

  test("runs current authorization for every canvas admission, including an already-loaded document", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const install = runtime.createCanvasLifecycleHooks as ((input: Record<string, unknown>) => Record<string, (input: Record<string, unknown>) => Promise<unknown>>) | undefined;
    expect(install).toBeTypeOf("function");
    const calls: unknown[] = [];
    const hooks = install!({ authorize: async (input: unknown) => { calls.push(input); return { allowed: true, readOnly: false }; }, lifecycle: {} });
    await hooks.onAuthenticate({ documentName: `canvas:${randomUUID()}`, context: { userId: "writer", sessionId: randomUUID() }, connectionConfig: { readOnly: false } });
    await hooks.onAuthenticate({ documentName: `canvas:${randomUUID()}`, context: { userId: "writer", sessionId: randomUUID() }, connectionConfig: { readOnly: false } });
    expect(calls).toHaveLength(2);
  });

  test("turns only permanent ended/viewer admission decisions read-only and returns retryable session_freezing to an established writer", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const install = runtime.createCanvasLifecycleHooks as ((input: Record<string, unknown>) => Record<string, (input: Record<string, unknown>) => Promise<unknown>>) | undefined;
    expect(install).toBeTypeOf("function");
    const connection = { readOnly: false, close() {} };
    const hooks = install!({ authorize: async () => ({ allowed: true, readOnly: false, code: "session_freezing" }), lifecycle: {} });
    const changed = new Y.Doc();
    changed.getMap("elements").set("fenced", "shape");
    const frame = new OutgoingMessage(`canvas:${phase10CanvasId}`).createSyncMessage().writeUpdate(Y.encodeStateAsUpdate(changed)).toUint8Array();
    await expect(hooks.beforeHandleMessage({ documentName: `canvas:${phase10CanvasId}`, connection, update: frame, context: { userId: "writer", sessionId: randomUUID() } })).rejects.toMatchObject({ code: "session_freezing", retryable: true });
    expect(connection.readOnly).toBe(false);
  });

  test("closes writable and read-only canvas sockets at their verified JWT expiry and clears early-close timers", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const schedule = runtime.scheduleCanvasJwtExpiry as ((input: Record<string, unknown>) => { cancel(): void }) | undefined;
    expect(schedule).toBeTypeOf("function");
    let writableClosed = 0;
    let readonlyClosed = 0;
    const now = Date.now();
    schedule!({ exp: Math.floor((now - 1) / 1000), now: () => now, connection: { close: () => writableClosed++ } });
    const early = schedule!({ exp: Math.floor((now + 10_000) / 1000), now: () => now, connection: { readOnly: true, close: () => readonlyClosed++ } });
    early.cancel();
    await Bun.sleep(0);
    expect(writableClosed).toBe(1);
    expect(readonlyClosed).toBe(0);
  });

  test("commits the exact pending admission during Hocuspocus 3.4.4 MessageReceiver.apply, including partial overlap", async () => {
    const runtime = await import("./hocuspocus") as Record<string, unknown>;
    const install = runtime.createCanvasLifecycleHooks as ((input: Record<string, unknown>) => Record<string, (input: Record<string, unknown>) => Promise<unknown>>) | undefined;
    expect(install).toBeTypeOf("function");
    const documentName = `canvas:${phase10CanvasId}`;
    const target = new Document(documentName);
    const first = new Y.Doc();
    first.getMap("elements").set("first", "a");
    Y.applyUpdate(target, Y.encodeStateAsUpdate(first));
    const second = new Y.Doc();
    Y.applyUpdate(second, Y.encodeStateAsUpdate(first));
    second.getMap("elements").set("second", "b");
    const frame = new OutgoingMessage(documentName).createSyncMessage().writeUpdate(Y.encodeStateAsUpdate(second)).toUint8Array();
    const lifecycleCalls: string[] = [];
    const hooks = install!({ lifecycle: { beginAdmission: () => lifecycleCalls.push("reserve"), commitAdmission: () => lifecycleCalls.push("commit"), rollbackAdmission: () => lifecycleCalls.push("rollback") }, authorize: async () => ({ allowed: true, readOnly: false }) });
    await hooks.beforeHandleMessage({ documentName, document: target, connection: { readOnly: false }, update: frame, context: { userId: "writer", sessionId: randomUUID() } });
    const incoming = new IncomingMessage(frame);
    incoming.readVarString();
    incoming.writeVarString(documentName);
    new MessageReceiver(incoming).apply(target, { readOnly: false, send() {}, callbacks: { beforeSync() {} } } as never);
    expect(target.getMap("elements").toJSON()).toMatchObject({ first: "a", second: "b" });
    expect(lifecycleCalls).toEqual(["reserve", "commit"]);
  });
});

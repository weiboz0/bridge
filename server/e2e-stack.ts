import { createHash, randomBytes as nodeRandomBytes } from "node:crypto";
import type { IncomingMessage, ServerResponse } from "node:http";
import { sql } from "drizzle-orm";
import { serverDb } from "./db";

export const E2E_STACK_DATABASE_URL_ENV = "DATABASE_URL";
export const E2E_STACK_LOCK_CLASS = 0x42523245;
export const E2E_STACK_NONCE_PATTERN = /^[0-9a-f]{64}$/;

const E2E_STACK_PATH = "/e2e-stack";
const E2E_STACK_QUERY_TIMEOUT_MS = 2_000;
const E2E_STACK_REFUSAL_LOG_INTERVAL_MS = 10_000;
const E2E_STACK_INSTANCE_GLOBAL_KEY = "__bridgeE2EStackAttestationInstance";
const queryTimedOut = Symbol("e2e-stack-query-timeout");

type E2EStackQueryResult = { database: string; seen: boolean };

type E2EStackRequestPayload = {
  request: IncomingMessage;
  response: ServerResponse;
};

export type E2EStackAttestationDeps = {
  env: Record<string, string | undefined>;
  query: (lockClass: number, objid: number) => Promise<E2EStackQueryResult>;
  now: () => number;
  log: (message: string) => void;
  randomBytes: (size: number) => Uint8Array;
};

type E2EStackGlobal = typeof globalThis & {
  [E2E_STACK_INSTANCE_GLOBAL_KEY]?: string;
};

/** Derives the low word of the reserved one-key advisory-lock namespace. */
export function deriveE2EStackObjid(nonce: string): number {
  if (!E2E_STACK_NONCE_PATTERN.test(nonce)) {
    throw new Error("nonce must be 64 lowercase hexadecimal characters");
  }
  const digest = createHash("sha256").update(nonce, "ascii").digest("hex");
  return Number.parseInt(digest.slice(0, 8), 16) & 0x7fffffff;
}

/** Hashes the exact nonce/database byte sequence shared by all attestations. */
export function e2eStackFingerprint(nonce: string, database: string): string {
  if (!E2E_STACK_NONCE_PATTERN.test(nonce)) {
    throw new Error("nonce must be 64 lowercase hexadecimal characters");
  }
  return createHash("sha256")
    .update(Buffer.concat([Buffer.from(nonce, "ascii"), Buffer.from([0]), Buffer.from(database, "utf8")]))
    .digest("hex");
}

function parsedTestDatabaseName(databaseUrl: string | undefined): string | null {
  if (!databaseUrl) return null;
  try {
    const parsed = new URL(databaseUrl);
    if (parsed.protocol !== "postgres:" && parsed.protocol !== "postgresql:") return null;
    const database = decodeURIComponent(parsed.pathname.replace(/^\//, ""));
    return database && !database.includes("/") && database.endsWith("_test") ? database : null;
  } catch {
    return null;
  }
}

function e2eStackInstance(randomBytes: (size: number) => Uint8Array): string {
  const globalState = globalThis as E2EStackGlobal;
  if (!globalState[E2E_STACK_INSTANCE_GLOBAL_KEY]) {
    globalState[E2E_STACK_INSTANCE_GLOBAL_KEY] = `hocuspocus-${process.pid}-${Buffer.from(randomBytes(16)).toString("hex")}`;
  }
  return globalState[E2E_STACK_INSTANCE_GLOBAL_KEY];
}

async function observeE2EStackLock(lockClass: number, objid: number): Promise<E2EStackQueryResult> {
  const rows = await serverDb.execute<E2EStackQueryResult>(sql`
    SELECT current_database() AS database, EXISTS (
      SELECT 1
      FROM pg_locks l
      JOIN pg_database d ON d.oid = l.database
      WHERE d.datname = current_database()
        AND l.locktype = 'advisory'
        AND l.classid = ${lockClass}::int8::oid
        AND l.objid = ${objid}::int8::oid
        AND l.objsubid = 1
        AND l.granted
    ) AS seen
  `);
  if (rows.length !== 1) throw new Error("e2e stack observation returned an unexpected row count");
  return rows[0];
}

async function observeWithinTimeout<T>(query: () => Promise<T>): Promise<T> {
  let timeout: ReturnType<typeof setTimeout> | undefined;
  try {
    return await Promise.race([
      query(),
      new Promise<never>((_, reject) => {
        timeout = setTimeout(() => reject(queryTimedOut), E2E_STACK_QUERY_TIMEOUT_MS);
      }),
    ]);
  } finally {
    if (timeout !== undefined) clearTimeout(timeout);
  }
}

/**
 * Builds the optional client-facing Hocuspocus E2E-stack attestation hook.
 * All observable dependencies are injected so refusal and success paths can
 * be exercised without querying a database or starting a listener.
 */
export function createE2EStackAttestation({
  env = process.env,
  query = observeE2EStackLock,
  now = Date.now,
  log = console.warn,
  randomBytes = nodeRandomBytes,
}: Partial<E2EStackAttestationDeps> = {}) {
  const enabled = env.BRIDGE_E2E_STACK === "1";
  const lastRefusalLog = new Map<string, number>();

  function logRefusal(reason: string): void {
    const timestamp = now();
    const lastLogged = lastRefusalLog.get(reason);
    if (lastLogged !== undefined && timestamp - lastLogged < E2E_STACK_REFUSAL_LOG_INTERVAL_MS) return;
    lastRefusalLog.set(reason, timestamp);
    log(`[hocuspocus] e2e stack attestation refused: ${reason}`);
  }

  return {
    enabled,

    assertBootAllowed(): void {
      const isExposed = (env.BRIDGE_HOST_EXPOSURE ?? "").toLowerCase().trim() === "exposed";
      if (enabled && isExposed && env.ALLOW_E2E_STACK_OVER_TUNNEL !== "true") {
        throw new Error(
          "[hocuspocus] refusing to start: BRIDGE_E2E_STACK=1 with BRIDGE_HOST_EXPOSURE=exposed requires ALLOW_E2E_STACK_OVER_TUNNEL=true"
        );
      }
    },

    async onRequest({ request, response }: E2EStackRequestPayload): Promise<void> {
      if (!enabled || request.method !== "GET") return;

      let url: URL;
      try {
        url = new URL(request.url ?? "/", "http://hocuspocus.invalid");
      } catch {
        return;
      }
      if (url.pathname !== E2E_STACK_PATH) return;

      const nonce = url.searchParams.get("nonce");
      if (!nonce || !E2E_STACK_NONCE_PATTERN.test(nonce)) {
        logRefusal("malformed_nonce");
        return;
      }
      if (!parsedTestDatabaseName(env[E2E_STACK_DATABASE_URL_ENV])) {
        logRefusal("parsed_database_not_test");
        return;
      }

      let observed: E2EStackQueryResult;
      try {
        observed = await observeWithinTimeout(() => query(E2E_STACK_LOCK_CLASS, deriveE2EStackObjid(nonce)));
      } catch (error) {
        logRefusal(error === queryTimedOut ? "query_timeout" : "query_error");
        return;
      }
      if (!observed.database.endsWith("_test")) {
        logRefusal("live_database_not_test");
        return;
      }
      if (!observed.seen) {
        logRefusal("lock_not_seen");
        return;
      }

      response.writeHead(200, { "Content-Type": "application/json", "Cache-Control": "no-store" });
      response.end(JSON.stringify({ fingerprint: e2eStackFingerprint(nonce, observed.database), instance: e2eStackInstance(randomBytes) }));

      // Hocuspocus treats a falsy rejected hook as handled and skips its
      // default text response. See its requestHandler's empty-error branch.
      return Promise.reject();
    },
  };
}

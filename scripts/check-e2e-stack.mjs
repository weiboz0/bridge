#!/usr/bin/env node
// Proves that the running E2E stack is connected to the gate's validated _test
// database before anything mutates it (Plan 094 Phase 14).
//
// ci-local.sh validates ITS database URL, but Playwright drives a separately
// running stack chosen by E2E_BASE_URL, and e2e/seed.setup.ts creates classes,
// enrolls users, and ends sessions. This verifier holds a transaction-scoped
// advisory lock in the validated database and requires the Go API, Next.js,
// and Hocuspocus — each on the origin E2E traffic really uses — to observe
// that lock through their own pools. A clone, a standby, or another cluster
// holding its own bridge_test cannot show it.
//
// The protocol is pinned in scripts/tests/e2e-stack-vector.json and asserted
// by Go, Bun, Vitest, and scripts/tests/test-guards.sh.
//
// Import-safe: importing this module does no work and opens no connection.
// e2e/seed.setup.ts imports verifyE2EStack() for its own fresh-nonce re-check.

import { createHash, randomBytes } from "node:crypto";
import net from "node:net";
import { pathToFileURL } from "node:url";

export const E2E_STACK_LOCK_CLASS = 0x42523245;
export const NONCE_PATTERN = /^[0-9a-f]{64}$/;
export const SERVICES = ["next", "go", "hocuspocus"];
export const INSTANCE_ENV = {
  go: "E2E_STACK_INSTANCE_GO",
  next: "E2E_STACK_INSTANCE_NEXT",
  hocuspocus: "E2E_STACK_INSTANCE_HOCUSPOCUS",
};

const PATHS = { go: "/api/health/e2e-stack", next: "/api/e2e-stack", hocuspocus: "/e2e-stack" };
const FINGERPRINT_PATTERN = /^[0-9a-f]{64}$/;
// Instance ids are exported through a shell environment block, so they are
// restricted to characters that need no quoting.
const INSTANCE_PATTERN = /^[A-Za-z0-9._-]{1,128}$/;
const DEFAULT_SAMPLES = 5;
const DEFAULT_FETCH_TIMEOUT_MS = 5_000;
const DEFAULT_HOLD_DEADLINE_MS = 30_000;
// Every database phase — BEGIN, the lock, each re-read, ROLLBACK, close — is
// bounded too; otherwise a server that black-holes after the lock is taken
// would hang the gate while the lock stayed held.
const DEFAULT_DATABASE_PHASE_TIMEOUT_MS = 5_000;
// Server-side bound on the lock-holding transaction, set inside it before the
// lock is taken. PostgreSQL then ends the transaction — and releases the lock —
// on its own if the client vanishes, whatever the client's socket does.
const IDLE_IN_TRANSACTION_TIMEOUT_MS = 30_000;

export function deriveObjid(nonce) {
  if (!NONCE_PATTERN.test(nonce)) throw new Error("nonce must be 64 lowercase hex characters");
  const digest = createHash("sha256").update(nonce, "ascii").digest("hex");
  return Number.parseInt(digest.slice(0, 8), 16) & 0x7fffffff;
}

/** The one-key advisory lock key as a decimal string (it exceeds 2^53). */
export function composeKey(objid) {
  if (!Number.isInteger(objid) || objid < 0 || objid > 0x7fffffff) {
    throw new Error("objid must be a non-negative 31-bit integer");
  }
  return ((BigInt(E2E_STACK_LOCK_CLASS) << 32n) | BigInt(objid)).toString();
}

export function fingerprint(nonce, database) {
  if (!NONCE_PATTERN.test(nonce)) throw new Error("nonce must be 64 lowercase hex characters");
  return createHash("sha256")
    .update(Buffer.concat([Buffer.from(nonce, "ascii"), Buffer.from([0]), Buffer.from(database, "utf8")]))
    .digest("hex");
}

/**
 * ws→http, wss→https; anything else (including http itself) is not a realtime URL.
 * The base PATH is preserved, not just the origin: a single-port reverse proxy
 * commonly mounts Hocuspocus under a prefix (e.g. `ws://host:3100/hocuspocus`,
 * per deploy/nginx/bridge.conf), and the attestation GET must be probed under
 * that same prefix. The caller appends `/e2e-stack` to this base, so a browser
 * URL of `ws://host:3100/hocuspocus` is probed at `http://host:3100/hocuspocus/e2e-stack`.
 * A trailing slash is trimmed so the join never doubles it.
 */
export function realtimeOriginFrom(realtimeUrl) {
  if (typeof realtimeUrl !== "string" || realtimeUrl === "") return null;
  let parsed;
  try {
    parsed = new URL(realtimeUrl);
  } catch {
    return null;
  }
  if (parsed.protocol !== "ws:" && parsed.protocol !== "wss:") return null;
  if (parsed.username || parsed.password) return null;
  const scheme = parsed.protocol === "wss:" ? "https:" : "http:";
  const basePath = parsed.pathname.replace(/\/+$/, "");
  return `${scheme}//${parsed.host}${basePath}`;
}

/** The verifier never opens a connection to anything but a parsed _test name. */
export function testDatabaseName(databaseUrl) {
  let parsed;
  try {
    parsed = new URL(databaseUrl);
  } catch {
    throw new Error("the attestation database URL is not a URL");
  }
  if (parsed.protocol !== "postgres:" && parsed.protocol !== "postgresql:") {
    throw new Error("the attestation database URL is not a PostgreSQL URL");
  }
  const name = decodeURIComponent(parsed.pathname.replace(/^\//, ""));
  if (!name || name.includes("/") || !name.endsWith("_test")) {
    throw new Error("the attestation database name must end in _test");
  }
  return name;
}

export const REMEDIATION = {
  "not attested":
    "start all three services with BRIDGE_E2E_STACK=1 against the gate's _test database, or point E2E_BASE_URL at such a stack. Without the flag the Go API and Hocuspocus have no endpoint and log nothing, so silence there most likely means the flag is missing; Next.js always has the route and logs `flag_disabled`. A flagged service logs the refusal category (database name or lock), at most once per ten seconds",
  mismatch:
    "the service is connected to a database with a different name than the gate's; restart it against the gate's _test database",
  "multiple instances":
    "run exactly one process per service with live reload off (no air, no file-watch restarts), reached without load balancing; a service restarted mid-check also reads as a second instance",
  "no realtime origin":
    "set NEXT_PUBLIC_HOCUSPOCUS_URL to a ws:// or wss:// URL and rebuild/restart Next.js — it is baked into the client bundle at build time, and without it browsers fall back to port 4000",
  "lock lost":
    "the gate's own lock-holding transaction died mid-check (reaped backend, pooler, or idle_in_transaction_session_timeout); the stack was not at fault — re-run",
  "database timeout":
    "the gate's own connection to its _test database stopped answering, so no verdict on the stack was produced (it may or may not have been sampled) — check PostgreSQL and re-run",
  unchecked:
    "Hocuspocus could not be checked because Next.js did not attest, and only an attested Next.js reports the realtime origin browsers use; fix Next.js first",
  unreachable:
    "the service did not answer, or a proxy in front of it answered 5xx; check that it is running and that E2E_BASE_URL is right",
  redirect: "the origin redirected; point E2E_BASE_URL at the final origin — redirects are never followed",
  timeout: "the service did not answer in time; check that it is running and not overloaded",
};

/**
 * postgres-js closes with `socket.end()`, a graceful FIN that a black-holed
 * server never answers, so a forced close must destroy the raw socket itself.
 * Same pattern as scripts/check-test-database-url.mjs: hand the driver a
 * socket factory, keep the one socket it makes, and destroy() it on demand.
 * One attempt only — the verifier holds one backend for its whole run.
 */
function createOneShotSocketController() {
  let attempted = false;
  let rawSocket;
  return {
    createSocket(options) {
      if (attempted) throw new Error("the E2E stack verifier permits one connection attempt");
      attempted = true;
      rawSocket = net.createConnection({ host: options.host[0], port: options.port[0] });
      return new Promise((resolve, reject) => {
        let connected = false;
        rawSocket.once("connect", () => {
          connected = true;
          rawSocket.host = options.host[0];
          rawSocket.port = options.port[0];
          resolve(rawSocket);
        });
        rawSocket.once("error", reject);
        rawSocket.once("close", () => {
          if (!connected) reject(new Error("socket closed before connect"));
        });
      });
    },
    destroy() {
      rawSocket?.destroy();
    },
    get destroyed() {
      return rawSocket === undefined || rawSocket.destroyed;
    },
  };
}

/** Exported for the live regressions only; the verifier always uses it through `connect`. */
export async function defaultConnect(databaseUrl, { socketController = createOneShotSocketController() } = {}) {
  // Imported lazily so importing this module never loads a driver or connects.
  const { default: postgres } = await import("postgres");
  const sql = postgres(databaseUrl, {
    max: 1,
    idle_timeout: 0,
    connect_timeout: 5,
    onnotice: () => {},
    socket: socketController.createSocket,
  });
  // One reserved backend carries BEGIN, the lock, both re-reads, and ROLLBACK.
  // A transaction-scoped lock is released by rollback or by disconnect, so it
  // cannot be stranded and needs no unlock that could land on another backend.
  let reserved;
  try {
    reserved = await sql.reserve();
  } catch (error) {
    // The client exists even though no connection was handed back; destroy the
    // socket and end it so a failed handshake never leaves anything behind.
    socketController.destroy();
    await sql.end({ timeout: 0 }).catch(() => {});
    throw error;
  }
  return {
    begin: async () => {
      await reserved`BEGIN`;
      // SET LOCAL lasts exactly as long as this transaction. Whatever happens
      // to the client, the server drops an idle transaction — and its lock —
      // after this long.
      await reserved.unsafe(`SET LOCAL idle_in_transaction_session_timeout = ${IDLE_IN_TRANSACTION_TIMEOUT_MS}`);
    },
    /** Server-side view of the bound, for the live regressions. */
    idleInTransactionTimeout: async () =>
      (await reserved`SHOW idle_in_transaction_session_timeout`)[0].idle_in_transaction_session_timeout,
    backendPid: async () => (await reserved`SELECT pg_backend_pid() AS pid`)[0].pid,
    lock: (key) => reserved`SELECT pg_advisory_xact_lock(${key}::bigint)`,
    currentDatabase: async () => (await reserved`SELECT current_database() AS name`)[0].name,
    ownLockGranted: async (objid) =>
      (
        await reserved`
          SELECT EXISTS (
            SELECT 1 FROM pg_locks l JOIN pg_database d ON d.oid = l.database
            WHERE d.datname = current_database() AND l.locktype = 'advisory'
              AND l.classid = ${E2E_STACK_LOCK_CLASS}::int8::oid AND l.objid = ${objid}::int8::oid
              AND l.objsubid = 1 AND l.granted AND l.pid = pg_backend_pid()
          ) AS granted`
      )[0].granted === true,
    rollback: () => reserved`ROLLBACK`,
    // `force` destroys the socket without waiting for in-flight queries. A
    // transaction-scoped lock cannot outlive its connection, so this releases
    // the lock even when ROLLBACK itself never returned.
    close: async ({ force = false } = {}) => {
      try {
        if (!force) reserved.release();
      } finally {
        // A forced close destroys the raw socket FIRST: `sql.end` only sends a
        // graceful FIN, which a server that stopped answering never completes.
        // Destroying the socket ends the backend's session on the server side
        // as soon as the RST arrives, which releases the transaction lock.
        if (force) socketController.destroy();
        await sql.end({ timeout: force ? 0 : 2 });
        if (!socketController.destroyed) socketController.destroy();
      }
    },
    /** True once the raw socket is destroyed, for the live regressions. */
    socketDestroyed: () => socketController.destroyed,
  };
}

class DatabasePhaseTimeout extends Error {
  constructor(phase) {
    super(`database phase timed out: ${phase}`);
    this.name = "DatabasePhaseTimeout";
    this.phase = phase;
  }
}

/**
 * Races one database phase against a timer. The timer is deliberately NOT
 * unref'd: if the phase never settles and nothing else holds the event loop
 * open, an unref'd timer would let Node exit before the deadline could fire.
 * It is always cleared in `finally`, so it never outlives the phase.
 */
function bounded(phase, ms, work) {
  let timer;
  const timeout = new Promise((_, reject) => {
    timer = setTimeout(() => reject(new DatabasePhaseTimeout(phase)), ms);
  });
  return Promise.race([Promise.resolve().then(work), timeout]).finally(() => clearTimeout(timer));
}

function classifyThrown(error) {
  const name = error?.name ?? "";
  if (name === "TimeoutError" || name === "AbortError") return "timeout";
  return "unreachable";
}

async function sampleOnce({ url, expected, fetchImpl, fetchTimeoutMs, deadlineSignal }) {
  let response;
  try {
    response = await fetchImpl(url, {
      method: "GET",
      redirect: "manual",
      cache: "no-store",
      headers: { Connection: "close", "Cache-Control": "no-store", Accept: "application/json" },
      signal: AbortSignal.any([AbortSignal.timeout(fetchTimeoutMs), deadlineSignal]),
    });
  } catch (error) {
    return { failure: classifyThrown(error) };
  }
  if (response.status >= 300 && response.status < 400) return { failure: "redirect" };
  // A refusal is deliberately indistinguishable from the path not existing:
  // Go and Next.js answer their ordinary 404, Hocuspocus its default 200 text.
  // A 5xx is an intermediary saying the upstream is down (the Next.js proxy
  // when Go is not running); flag/database/lock advice would mislead there.
  if (response.status >= 500) return { failure: "unreachable" };
  if (response.status !== 200) return { failure: "not attested" };
  let body;
  try {
    body = await response.json();
  } catch {
    return { failure: "not attested" };
  }
  if (!body || typeof body !== "object" || !FINGERPRINT_PATTERN.test(body.fingerprint ?? "")) {
    return { failure: "not attested" };
  }
  if (typeof body.instance !== "string" || !INSTANCE_PATTERN.test(body.instance)) {
    return { failure: "not attested" };
  }
  if (body.fingerprint !== expected) return { failure: "mismatch" };
  return { instance: body.instance, realtimeUrl: body.realtimeUrl };
}

async function sampleService({ service, origin, nonce, samples, ...rest }) {
  const url = `${origin}${PATHS[service]}?nonce=${nonce}`;
  let instance;
  let realtimeUrl;
  for (let i = 0; i < samples; i += 1) {
    const result = await sampleOnce({ url, ...rest });
    if (result.failure) return { service, failure: result.failure };
    if (instance !== undefined && result.instance !== instance) return { service, failure: "multiple instances" };
    if (i > 0 && result.realtimeUrl !== realtimeUrl) return { service, failure: "multiple instances" };
    instance = result.instance;
    realtimeUrl = result.realtimeUrl;
  }
  return { service, instance, realtimeUrl };
}

/**
 * @returns {Promise<{ok: boolean, baseUrl: string, realtimeOrigin: string|null,
 *   instances: Record<string,string>, failures: {service: string, class: string}[]}>}
 */
export async function verifyE2EStack({
  baseUrl,
  databaseUrl,
  expectedInstances,
  nonce = randomBytes(32).toString("hex"),
  samples = DEFAULT_SAMPLES,
  fetchTimeoutMs = DEFAULT_FETCH_TIMEOUT_MS,
  holdDeadlineMs = DEFAULT_HOLD_DEADLINE_MS,
  databasePhaseTimeoutMs = DEFAULT_DATABASE_PHASE_TIMEOUT_MS,
  fetchImpl = globalThis.fetch,
  connect = defaultConnect,
} = {}) {
  let base;
  try {
    base = new URL(baseUrl);
  } catch {
    throw new Error("E2E_BASE_URL is not a URL");
  }
  if (base.protocol !== "http:" && base.protocol !== "https:") throw new Error("E2E_BASE_URL must be http or https");
  const origin = base.origin;
  const parsedName = testDatabaseName(databaseUrl);
  const objid = deriveObjid(nonce);

  const result = { ok: false, baseUrl: origin, realtimeOrigin: null, instances: {}, failures: [] };
  const deadline = AbortSignal.timeout(holdDeadlineMs);
  const db = (phase, work) => bounded(phase, databasePhaseTimeoutMs, work);
  let connection;
  let timedOut = false;
  try {
    // connect() is the one phase with nothing to clean up if it times out:
    // `connection` is still unset, so the `finally` below would skip it. Keep
    // the promise, and if it settles AFTER the deadline, dispose of whatever it
    // produced instead of leaving a reserved socket open.
    const connecting = Promise.resolve().then(() => connect(databaseUrl));
    try {
      connection = await db("connect", () => connecting);
    } catch (error) {
      if (error instanceof DatabasePhaseTimeout) {
        // Wrapped so even a close() that throws synchronously, or a non-function,
        // can never surface as an unhandled rejection from this handler.
        connecting
          .then((late) => {
            try {
              return Promise.resolve(late?.close?.({ force: true })).catch(() => {});
            } catch {
              return undefined;
            }
          })
          .catch(() => {});
      }
      throw error;
    }
    await db("begin", () => connection.begin());
    const pid = await db("backend pid", () => connection.backendPid());
    await db("lock", () => connection.lock(composeKey(objid)));
    const liveName = await db("current database", () => connection.currentDatabase());
    if (!liveName.endsWith("_test") || liveName !== parsedName) {
      throw new Error("the attestation database is not the parsed _test database");
    }
    if (
      (await db("backend pid", () => connection.backendPid())) !== pid ||
      !(await db("own lock", () => connection.ownLockGranted(objid)))
    ) {
      result.failures.push({ service: "gate", class: "lock lost" });
      return result;
    }

    const expected = fingerprint(nonce, liveName);
    const shared = { nonce, samples, expected, fetchImpl, fetchTimeoutMs, deadlineSignal: deadline };
    const outcomes = [];
    const next = await sampleService({ service: "next", origin, ...shared });
    outcomes.push(next);
    outcomes.push(await sampleService({ service: "go", origin, ...shared }));
    // Hocuspocus is attested on the origin the BROWSER is handed, which only
    // the attested Next.js process knows; it is never taken from configuration.
    if (!next.failure) {
      const realtimeOrigin = realtimeOriginFrom(next.realtimeUrl);
      if (!realtimeOrigin) {
        outcomes.push({ service: "hocuspocus", failure: "no realtime origin" });
      } else {
        result.realtimeOrigin = realtimeOrigin;
        outcomes.push(await sampleService({ service: "hocuspocus", origin: realtimeOrigin, ...shared }));
      }
    } else {
      // Its origin is only known from an attested Next.js, so it could not be
      // checked at all; blaming NEXT_PUBLIC_HOCUSPOCUS_URL here would mislead.
      outcomes.push({ service: "hocuspocus", failure: "unchecked" });
    }

    // If our own transaction died, every service correctly saw no lock; say so
    // instead of blaming a stack that may be perfectly configured.  An expired
    // hold deadline is NOT that: the lock is re-read, and if it is still held
    // the per-service timeouts stand, because a hung service is the stack's
    // fault and "re-run, the stack was fine" would be exactly wrong.
    if (
      (await db("backend pid", () => connection.backendPid())) !== pid ||
      !(await db("own lock", () => connection.ownLockGranted(objid)))
    ) {
      result.failures = [{ service: "gate", class: "lock lost" }];
      return result;
    }

    for (const outcome of outcomes) {
      if (outcome.failure) result.failures.push({ service: outcome.service, class: outcome.failure });
      else result.instances[outcome.service] = outcome.instance;
    }
    if (expectedInstances) {
      for (const service of SERVICES) {
        const want = expectedInstances[service];
        const got = result.instances[service];
        if (got !== undefined && want !== got && !result.failures.some((f) => f.service === service)) {
          result.failures.push({ service, class: "multiple instances" });
        }
      }
    }
    result.ok = result.failures.length === 0 && SERVICES.every((s) => result.instances[s]);
    return result;
  } catch (error) {
    if (!(error instanceof DatabasePhaseTimeout)) throw error;
    // Fail closed with a class of its own: nothing about the stack was learned.
    timedOut = true;
    result.ok = false;
    result.instances = {};
    result.failures = [{ service: "gate", class: "database timeout" }];
    return result;
  } finally {
    // Rollback releases the lock. If rollback fails or never returns, the
    // connection is destroyed instead: a transaction-scoped lock cannot outlive
    // its connection, so the lock is released either way and the gate never
    // hangs on a server that stopped answering.
    if (connection) {
      let force = timedOut;
      if (!force) {
        try {
          await db("rollback", () => connection.rollback());
        } catch {
          force = true;
        }
      }
      // Nothing here may throw out of `finally`: that would discard a result
      // already decided, and the process exits regardless, which ends the
      // connection and with it the transaction-scoped lock.
      try {
        await db("close", () => connection.close({ force }));
      } catch {
        if (!force) await db("close", () => connection.close({ force: true })).catch(() => {});
      }
    }
  }
}

export function formatReport(result) {
  const lines = [`E2E stack target: ${result.baseUrl}`];
  lines.push(`E2E realtime origin: ${result.realtimeOrigin ?? "(not derived)"}`);
  if (result.ok) {
    lines.push("E2E stack attested: next, go, and hocuspocus each observed the gate's lock.");
    return lines;
  }
  for (const failure of result.failures) {
    lines.push(`E2E stack NOT attested — ${failure.service}: ${failure.class}`);
    lines.push(`  → ${REMEDIATION[failure.class] ?? "see docs/testing.md"}`);
  }
  return lines;
}

/** One machine-readable line ci-local.sh captures; a child cannot export into its parent. */
export function instancesLine(instances) {
  return `E2E_STACK_INSTANCES ${SERVICES.map((s) => `${s}=${instances[s]}`).join(" ")}`;
}

export function expectedInstancesFromEnv(env) {
  const values = Object.fromEntries(SERVICES.map((s) => [s, env[INSTANCE_ENV[s]]]));
  return SERVICES.every((s) => values[s]) ? values : undefined;
}

async function main() {
  const baseUrl = process.env.E2E_BASE_URL;
  const databaseUrl = process.env.CHECK_E2E_STACK_DATABASE_URL;
  if (!baseUrl || !databaseUrl) {
    console.error("check-e2e-stack: E2E_BASE_URL and CHECK_E2E_STACK_DATABASE_URL are required");
    return 2;
  }
  // When the gate has already attested and exported the per-process instance
  // ids (E2E_STACK_INSTANCE_*), enforce them here too: a re-run (e.g. from
  // e2e/seed.setup.ts) then fails closed if any service was restarted between
  // the gate's check and this one. Absent (the gate's own first run), this is
  // undefined and the instance check is skipped.
  const expectedInstances = expectedInstancesFromEnv(process.env);
  let result;
  try {
    result = await verifyE2EStack({ baseUrl, databaseUrl, expectedInstances });
  } catch (error) {
    // Never echo the database URL: postgres errors can embed it.
    console.error(`check-e2e-stack: ${error?.code ?? error?.name ?? "error"}: verification could not run`);
    if (error instanceof Error && !/postgres(ql)?:\/\//i.test(error.message)) console.error(`  ${error.message}`);
    return 1;
  }
  for (const line of formatReport(result)) (result.ok ? console.log : console.error)(line);
  if (!result.ok) return 1;
  console.log(instancesLine(result.instances));
  return 0;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  // No top-level await: this module is imported by e2e/seed.setup.ts, which
  // Playwright loads via require(), and Node refuses to require() an ESM graph
  // that contains a top-level await. Set the exit code from the promise instead.
  main().then(
    (code) => {
      process.exitCode = code;
    },
    (error) => {
      console.error(`check-e2e-stack: ${error?.message ?? error}`);
      process.exitCode = 1;
    },
  );
}

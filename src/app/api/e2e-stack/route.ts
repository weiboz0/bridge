import { createHash, randomUUID } from "node:crypto";
import { notFound } from "next/navigation";
import { NextRequest, NextResponse } from "next/server";
import { sql } from "drizzle-orm";
import { db } from "@/lib/db";

/**
 * E2E stack attestation — Plan 094 Phase 14.
 *
 * The local test gate (`scripts/check-e2e-stack.mjs`) proves that the running
 * Next.js process is connected to the SAME validated `_test` database it
 * holds an advisory lock in, before Playwright is allowed to mutate anything
 * through this stack. This route answers that proof for the legacy
 * TypeScript API's `db` pool, and additionally reports the realtime origin
 * baked into the client bundle so the gate can attest Hocuspocus on the
 * origin browsers actually use.
 *
 * The cross-language contract (flag name, nonce pattern, lock class, key
 * derivation, observe query, fingerprint, paths, response/refusal shape) is
 * pinned in `scripts/tests/e2e-stack-vector.json` and asserted verbatim by
 * Go, Bun, and this route's Vitest suite — do not change a constant here
 * without updating that file and its siblings.
 *
 * Every refusal path (flag off, exposed without opt-in, malformed nonce,
 * non-test parsed or live database name, unseen lock, or a failed/timed-out
 * observe query) must be indistinguishable from this route not existing, so
 * they all funnel through `notFound()`.
 */

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

/** The exact env var name `src/lib/db/index.ts` reads for its pool. Exported so a regression can assert the two never diverge. */
export const DATABASE_URL_ENV_VAR = "DATABASE_URL";

const E2E_STACK_FLAG = "BRIDGE_E2E_STACK";
const E2E_STACK_FLAG_ENABLED_VALUE = "1";
const HOST_EXPOSURE_FLAG = "BRIDGE_HOST_EXPOSURE";
const HOST_EXPOSURE_EXPOSED_VALUE = "exposed";
const TUNNEL_OPT_IN_FLAG = "ALLOW_E2E_STACK_OVER_TUNNEL";
const TUNNEL_OPT_IN_ENABLED_VALUE = "true";

/** `0x42523245` — reserved one-key advisory lock class, disjoint from Bridge's lifecycle locks. See `platform/internal/store/session_lifecycle.go`. */
export const E2E_STACK_LOCK_CLASS = 0x42523245;

export const E2E_STACK_NONCE_PATTERN = /^[0-9a-f]{64}$/;

const TEST_DATABASE_SUFFIX = "_test";

const OBSERVE_QUERY_TIMEOUT_MS = 2000;
const REFUSAL_LOG_INTERVAL_MS = 10_000;

/** `contract.observeConcurrency.maxInFlightPerProcess` — at most this many observe queries run at once, per process. */
export const E2E_STACK_OBSERVE_CONCURRENCY_LIMIT = 2;

type EnvLike = Record<string, string | undefined>;

export type E2EStackRefusalReason =
  | "flag_disabled"
  | "exposed_without_opt_in"
  | "malformed_nonce"
  | "non_test_parsed_database"
  | "non_test_live_database"
  | "lock_unseen"
  | "observe_query_failed"
  | "observe_busy";

export interface E2EStackObserveResult {
  database: string;
  lockSeen: boolean;
}

/**
 * The observe query's concurrency gate. `tryAcquire` returns `false` (and
 * takes no slot) when the process is already at the cap; a caller that
 * acquired a slot MUST call `release` exactly once, including on error or
 * timeout. Injectable so a test can drive contention deterministically
 * without real concurrent requests — see `contract.observeConcurrency` in
 * `scripts/tests/e2e-stack-vector.json`.
 */
export interface E2EStackObserveLimiter {
  tryAcquire: () => boolean;
  release: () => void;
}

export interface E2EStackSuccessBody {
  fingerprint: string;
  instance: string;
  realtimeUrl?: string;
}

export type E2EStackEvaluation =
  | { ok: true; body: E2EStackSuccessBody }
  | { ok: false; reason: E2EStackRefusalReason };

// ---------------------------------------------------------------------------
// Pure key derivation / fingerprint — asserted against scripts/tests/e2e-stack-vector.json
// ---------------------------------------------------------------------------

/**
 * `objid = parseInt(sha256(ASCII(nonce)).hex[0..8], 16) & 0x7fffffff`
 * The low half of the advisory-lock key; kept a 31-bit non-negative value so
 * it always widens to a positive `bigint` when combined with the lock class.
 */
export function deriveE2EStackObjid(nonce: string): number {
  assertE2EStackNonce(nonce);
  const digest = createHash("sha256").update(Buffer.from(nonce, "ascii")).digest("hex");
  return parseInt(digest.slice(0, 8), 16) & 0x7fffffff;
}

/** `sha256( ASCII(nonce) || 0x00 || UTF-8(database) )`, lowercase hex. */
export function e2eStackFingerprint(nonce: string, database: string): string {
  assertE2EStackNonce(nonce);
  const payload = Buffer.concat([
    Buffer.from(nonce, "ascii"),
    Buffer.from([0x00]),
    Buffer.from(database, "utf8"),
  ]);
  return createHash("sha256").update(payload).digest("hex");
}

// ---------------------------------------------------------------------------
// Process-scoped state — cached on globalThis so Next.js dev-server module
// re-evaluation (fast refresh) never mints a new instance id or resets the
// refusal-log rate limiter.
// ---------------------------------------------------------------------------

declare global {
  var bridgeE2EStackInstance: string | undefined;
  var bridgeE2EStackRefusalLog: Map<string, number> | undefined;
  var bridgeE2EStackObserveInFlight: number | undefined;
}

/** Per-process id, generated once and cached on `globalThis`; includes `process.pid` per the contract's single-instance topology check. */
export function getE2EStackInstanceId(): string {
  if (!globalThis.bridgeE2EStackInstance) {
    globalThis.bridgeE2EStackInstance = `${process.pid}-${randomUUID()}`;
  }
  return globalThis.bridgeE2EStackInstance;
}

/** Logs a refusal reason at most once per 10s per reason. Never logs the database URL, nonce, or derived key. */
function logRefusalReason(reason: E2EStackRefusalReason, now: () => number = Date.now): void {
  const timestamp = now();
  const log = (globalThis.bridgeE2EStackRefusalLog ??= new Map<string, number>());
  const last = log.get(reason);
  if (last !== undefined && timestamp - last < REFUSAL_LOG_INTERVAL_MS) {
    return;
  }
  log.set(reason, timestamp);
  console.warn(`[e2e-stack] refusal: ${reason}`);
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * `contract.hostExposure.normalization`: trim ASCII whitespace, then compare
 * case-insensitively; only the exact word "exposed" declares exposure (so
 * "exposedish" and "not exposed" do not). Matches the Go (`strings.EqualFold`
 * + `strings.TrimSpace`) and Hocuspocus (`.trim().toLowerCase()`)
 * implementations of the same contract — asserted against `hostExposureCases`
 * in `scripts/tests/e2e-stack-vector.json`. The tunnel opt-in is deliberately
 * NOT normalized this way; see `TUNNEL_OPT_IN_ENABLED_VALUE`.
 */
export function isExposedHost(value: string | undefined): boolean {
  if (value === undefined) return false;
  return value.trim().toLowerCase() === HOST_EXPOSURE_EXPOSED_VALUE;
}

/**
 * Default observe-query concurrency gate, backed by a counter on
 * `globalThis` for the same reason as the instance id and refusal log: dev-
 * server module re-evaluation must not reset an in-flight count to zero
 * while queries are still running.
 */
function getGlobalE2EStackObserveLimiter(): E2EStackObserveLimiter {
  return {
    tryAcquire: () => {
      const current = globalThis.bridgeE2EStackObserveInFlight ?? 0;
      if (current >= E2E_STACK_OBSERVE_CONCURRENCY_LIMIT) return false;
      globalThis.bridgeE2EStackObserveInFlight = current + 1;
      return true;
    },
    release: () => {
      const current = globalThis.bridgeE2EStackObserveInFlight ?? 0;
      globalThis.bridgeE2EStackObserveInFlight = Math.max(0, current - 1);
    },
  };
}

// Both pure functions are exported, so they validate for themselves rather than
// rely on the handler having checked first — as the Hocuspocus and gate-side
// implementations of the same contract do.
function assertE2EStackNonce(nonce: string): void {
  if (!E2E_STACK_NONCE_PATTERN.test(nonce)) {
    throw new Error("nonce must be 64 lowercase hex characters");
  }
}

/**
 * Parses the database name from a Postgres connection string, or null if it
 * cannot be parsed. Same acceptance as `server/e2e-stack.ts` and
 * `scripts/check-e2e-stack.mjs`: a PostgreSQL scheme and a single-segment name.
 */
function parseDatabaseName(connectionString: string | undefined): string | null {
  if (!connectionString) return null;
  try {
    const url = new URL(connectionString);
    if (url.protocol !== "postgres:" && url.protocol !== "postgresql:") return null;
    const name = decodeURIComponent(url.pathname.replace(/^\//, ""));
    return name.length > 0 && !name.includes("/") ? name : null;
  } catch {
    return null;
  }
}

function withTimeout<T>(promise: Promise<T>, ms: number, now: () => number): Promise<T> {
  const startedAt = now();
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => {
      reject(new Error(`e2e-stack observe query exceeded ${ms}ms (elapsed ${now() - startedAt}ms)`));
    }, ms);
    promise.then(
      (value) => {
        clearTimeout(timer);
        resolve(value);
      },
      (error: unknown) => {
        clearTimeout(timer);
        reject(error instanceof Error ? error : new Error(String(error)));
      }
    );
  });
}

// ---------------------------------------------------------------------------
// Decision logic — pure aside from the injected `observe` and the one
// literal `NEXT_PUBLIC_HOCUSPOCUS_URL` read the build-time inlining
// requirement carves out (see module doc).
// ---------------------------------------------------------------------------

export interface EvaluateE2EStackRequestParams {
  env: EnvLike;
  nonce: string | null;
  observe: (args: { lockClass: number; objid: number }) => Promise<E2EStackObserveResult>;
  now?: () => number;
  /** Defaults to the process-wide `globalThis`-backed gate; injectable so a test can drive contention deterministically. */
  limiter?: E2EStackObserveLimiter;
}

export async function evaluateE2EStackRequest({
  env,
  nonce,
  observe,
  now = Date.now,
  limiter = getGlobalE2EStackObserveLimiter(),
}: EvaluateE2EStackRequestParams): Promise<E2EStackEvaluation> {
  if (env[E2E_STACK_FLAG] !== E2E_STACK_FLAG_ENABLED_VALUE) {
    return { ok: false, reason: "flag_disabled" };
  }

  if (isExposedHost(env[HOST_EXPOSURE_FLAG]) && env[TUNNEL_OPT_IN_FLAG] !== TUNNEL_OPT_IN_ENABLED_VALUE) {
    return { ok: false, reason: "exposed_without_opt_in" };
  }

  if (!nonce || !E2E_STACK_NONCE_PATTERN.test(nonce)) {
    return { ok: false, reason: "malformed_nonce" };
  }

  const parsedDatabase = parseDatabaseName(env[DATABASE_URL_ENV_VAR]);
  if (!parsedDatabase || !parsedDatabase.endsWith(TEST_DATABASE_SUFFIX)) {
    return { ok: false, reason: "non_test_parsed_database" };
  }

  const objid = deriveE2EStackObjid(nonce);

  // contract.observeConcurrency: a request that would exceed the per-process
  // cap is refused like any other refusal and runs NO query — no slot is
  // held, so there is nothing to release on this path.
  if (!limiter.tryAcquire()) {
    return { ok: false, reason: "observe_busy" };
  }

  let observed: E2EStackObserveResult;
  try {
    observed = await withTimeout(observe({ lockClass: E2E_STACK_LOCK_CLASS, objid }), OBSERVE_QUERY_TIMEOUT_MS, now);
  } catch {
    return { ok: false, reason: "observe_query_failed" };
  } finally {
    limiter.release();
  }

  if (!observed.database.endsWith(TEST_DATABASE_SUFFIX)) {
    return { ok: false, reason: "non_test_live_database" };
  }

  if (!observed.lockSeen) {
    return { ok: false, reason: "lock_unseen" };
  }

  const body: E2EStackSuccessBody = {
    fingerprint: e2eStackFingerprint(nonce, observed.database),
    instance: getE2EStackInstanceId(),
  };

  // Literal reference required verbatim — Next.js inlines NEXT_PUBLIC_* at
  // build time only where this exact expression appears in the bundle. No
  // destructuring, no dynamic key, no helper indirection: that would read
  // the runtime environment instead and reintroduce the build-vs-runtime
  // split this attestation exists to catch.
  const realtimeUrl = process.env.NEXT_PUBLIC_HOCUSPOCUS_URL;
  if (realtimeUrl) {
    body.realtimeUrl = realtimeUrl;
  }

  return { ok: true, body };
}

// ---------------------------------------------------------------------------
// Real observe implementation — runs on this process's own `db` pool.
// ---------------------------------------------------------------------------

async function observeE2EStackLock({
  lockClass,
  objid,
}: {
  lockClass: number;
  objid: number;
}): Promise<E2EStackObserveResult> {
  const rows = await db.execute<{ database: string; lock_seen: boolean }>(sql`
    SELECT current_database() AS database,
           EXISTS (
             SELECT 1
             FROM pg_locks l
             JOIN pg_database d ON d.oid = l.database
             WHERE d.datname = current_database()
               AND l.locktype = 'advisory'
               AND l.classid = ${lockClass}::int8::oid
               AND l.objid = ${objid}::int8::oid
               AND l.objsubid = 1
               AND l.granted
           ) AS lock_seen
  `);

  const row = rows[0];
  if (!row) {
    throw new Error("e2e-stack observe query returned no rows");
  }

  return { database: row.database, lockSeen: row.lock_seen };
}

// ---------------------------------------------------------------------------
// Route handler — thin: wires real env, the real pool, and notFound().
// ---------------------------------------------------------------------------

export async function GET(request: NextRequest) {
  const nonce = request.nextUrl.searchParams.get("nonce");

  const evaluation = await evaluateE2EStackRequest({
    env: process.env,
    nonce,
    observe: observeE2EStackLock,
  });

  if (!evaluation.ok) {
    logRefusalReason(evaluation.reason);
    notFound();
  }

  return NextResponse.json(evaluation.body, {
    headers: { "Cache-Control": "no-store" },
  });
}

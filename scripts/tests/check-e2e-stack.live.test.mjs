// Plan 094 Phase 14 — live regressions for the gate-side E2E stack verifier.
//
// scripts/check-e2e-stack.mjs is the only thing standing between an
// unattested stack and a Playwright run that creates classes, enrolls users,
// and ends live sessions. Its safety rests on PostgreSQL behaviour that a
// mock cannot express: a transaction-scoped advisory lock taken on ONE
// reserved backend, visible to every other connection to the same database
// while it is held, and gone the instant that transaction ends — by rollback,
// by failure, or by the connection disappearing.
//
// These tests therefore use the verifier's REAL default connection path and
// mock only fetch. They take advisory locks and read pg_locks; they touch no
// table, so a concurrent suite truncating this database cannot disturb them.
//
// They refuse to connect to anything but a parsed _test database and skip
// themselves otherwise.

import { afterAll, expect, test } from "bun:test";
import postgres from "postgres";

import {
  composeKey,
  deriveObjid,
  E2E_STACK_LOCK_CLASS,
  fingerprint,
  REMEDIATION,
  realtimeOriginFrom,
  SERVICES,
  testDatabaseName,
  verifyE2EStack,
} from "../check-e2e-stack.mjs";

const DATABASE_URL = process.env.DATABASE_URL ?? "";

/** The one guard that is allowed to skip: never open a non-_test database. */
function parsedTestName(url) {
  try {
    return testDatabaseName(url);
  } catch {
    return null;
  }
}

const PARSED_NAME = parsedTestName(DATABASE_URL);
const live = PARSED_NAME ? test : test.skip;

const BASE_URL = "http://127.0.0.1:65535";
const REALTIME_URL = "ws://127.0.0.1:64999";

let observer = null;
function sql() {
  if (!observer) {
    observer = postgres(DATABASE_URL, { max: 1, idle_timeout: 0, connect_timeout: 5, onnotice: () => {} });
  }
  return observer;
}

afterAll(async () => {
  if (observer) await observer.end({ timeout: 5 });
});

async function liveDatabaseName() {
  return (await sql()`SELECT current_database() AS name`)[0].name;
}

/** Every granted one-key gate lock for this object id in THIS database. */
async function gateLockPids(objid) {
  const rows = await sql()`
    SELECT l.pid AS pid FROM pg_locks l JOIN pg_database d ON d.oid = l.database
    WHERE d.datname = current_database() AND l.locktype = 'advisory'
      AND l.classid = ${E2E_STACK_LOCK_CLASS}::int8::oid AND l.objid = ${objid}::int8::oid
      AND l.objsubid = 1 AND l.granted`;
  return rows.map((row) => row.pid);
}

async function waitForNoLock(objid, timeoutMs = 5_000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const pids = await gateLockPids(objid);
    if (pids.length === 0 || Date.now() > deadline) return pids;
    await new Promise((resolve) => setTimeout(resolve, 25));
  }
}

async function inOpenTransaction(pid) {
  const rows = await sql()`SELECT xact_start IS NOT NULL AS open FROM pg_stat_activity WHERE pid = ${pid}`;
  return rows.length === 1 && rows[0].open === true;
}

/**
 * A stand-in for the three services. Only fetch is mocked; the verifier's own
 * connection, lock, and re-reads stay real.
 */
function mockStack({ expected, status = {}, instances = {}, realtimeUrl = REALTIME_URL, onFetch }) {
  const calls = [];
  const impl = async (url, init) => {
    const parsed = new URL(url);
    const service =
      parsed.pathname === "/api/e2e-stack"
        ? "next"
        : parsed.pathname === "/api/health/e2e-stack"
          ? "go"
          : parsed.pathname === "/e2e-stack"
            ? "hocuspocus"
            : null;
    calls.push({ service, url: parsed.href, nonce: parsed.searchParams.get("nonce") });
    if (onFetch) await onFetch({ service, init, index: calls.length - 1 });
    const code = status[service] ?? 200;
    if (code !== 200) {
      // Exactly what a refusing service answers: an ordinary 404 body.
      return new Response("404 page not found\n", { status: code, headers: { "content-type": "text/plain" } });
    }
    const body = { fingerprint: expected, instance: instances[service] ?? `${service}-1` };
    if (service === "next" && realtimeUrl) body.realtimeUrl = realtimeUrl;
    return new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
  };
  return { impl, calls };
}

// ---------------------------------------------------------------------------
// One reserved backend; visible while held; gone on success
// ---------------------------------------------------------------------------

live("the lock and every later statement run on one backend, and the lock is released on success", async () => {
  const nonce = "1".repeat(64);
  const objid = deriveObjid(nonce);
  const database = await liveDatabaseName();
  expect(database.endsWith("_test")).toBe(true);
  expect(await gateLockPids(objid)).toEqual([]);

  const samples = [];
  const { impl, calls } = mockStack({
    expected: fingerprint(nonce, database),
    // Sampled from a SECOND client while a mocked fetch is in flight, i.e.
    // exactly while a real service would be running its own observe query.
    onFetch: async () => {
      const pids = await gateLockPids(objid);
      samples.push({ pids, open: pids.length === 1 ? await inOpenTransaction(pids[0]) : false });
    },
  });

  const result = await verifyE2EStack({
    baseUrl: BASE_URL,
    databaseUrl: DATABASE_URL,
    nonce,
    samples: 2,
    fetchImpl: impl,
  });

  expect(result.failures).toEqual([]);
  expect(result.ok).toBe(true);
  expect(result.baseUrl).toBe(BASE_URL);
  expect(result.realtimeOrigin).toBe(realtimeOriginFrom(REALTIME_URL));
  expect(Object.keys(result.instances).sort()).toEqual([...SERVICES].sort());

  // 3 services x 2 samples, every one of them carrying the same fresh nonce.
  expect(calls).toHaveLength(6);
  for (const call of calls) expect(call.nonce).toBe(nonce);

  // The lock was visible to the second client throughout, always granted to
  // exactly one backend, and that backend always had an open transaction.
  expect(samples).toHaveLength(6);
  const holders = new Set();
  for (const sample of samples) {
    expect(sample.pids).toHaveLength(1);
    expect(sample.open).toBe(true);
    holders.add(sample.pids[0]);
  }
  expect([...holders]).toHaveLength(1);

  // The composed key is the contract's, not an accident of the driver.
  expect(composeKey(objid)).toBe(((BigInt(E2E_STACK_LOCK_CLASS) << 32n) | BigInt(objid)).toString());

  // Rollback happens in the verifier's finally, before it resolves.
  expect(await gateLockPids(objid)).toEqual([]);
});

// ---------------------------------------------------------------------------
// Failure paths still release the lock, and report the right class
// ---------------------------------------------------------------------------

live("a service that reports no lock fails as 'not attested' and the lock is still released", async () => {
  const nonce = "2".repeat(64);
  const objid = deriveObjid(nonce);
  const database = await liveDatabaseName();

  const { impl } = mockStack({ expected: fingerprint(nonce, database), status: { go: 404 } });
  const result = await verifyE2EStack({
    baseUrl: BASE_URL,
    databaseUrl: DATABASE_URL,
    nonce,
    samples: 2,
    fetchImpl: impl,
  });

  expect(result.ok).toBe(false);
  expect(result.failures).toContainEqual({ service: "go", class: "not attested" });
  expect(result.instances.go).toBeUndefined();
  expect(REMEDIATION["not attested"]).toContain("BRIDGE_E2E_STACK=1");
  // The gate itself is healthy, so it must not be blamed.
  expect(result.failures.some((failure) => failure.service === "gate")).toBe(false);

  expect(await gateLockPids(objid)).toEqual([]);
});

live("an unattested Next.js leaves Hocuspocus 'unchecked' rather than blaming its URL", async () => {
  const nonce = "3".repeat(64);
  const objid = deriveObjid(nonce);
  const database = await liveDatabaseName();

  const { impl, calls } = mockStack({ expected: fingerprint(nonce, database), status: { next: 404 } });
  const result = await verifyE2EStack({
    baseUrl: BASE_URL,
    databaseUrl: DATABASE_URL,
    nonce,
    samples: 1,
    fetchImpl: impl,
  });

  expect(result.ok).toBe(false);
  expect(result.failures).toContainEqual({ service: "next", class: "not attested" });
  expect(result.failures).toContainEqual({ service: "hocuspocus", class: "unchecked" });
  expect(result.realtimeOrigin).toBeNull();
  // Hocuspocus is never fetched from configuration when Next.js did not attest.
  expect(calls.filter((call) => call.service === "hocuspocus")).toHaveLength(0);
  // Go is still sampled, so one broken service does not mask another.
  expect(calls.filter((call) => call.service === "go")).toHaveLength(1);

  expect(await gateLockPids(objid)).toEqual([]);
});

// ---------------------------------------------------------------------------
// The lock cannot be stranded
// ---------------------------------------------------------------------------

live("a transaction-scoped lock is gone when the connection is destroyed without a rollback", async () => {
  const nonce = "4".repeat(64);
  const objid = deriveObjid(nonce);
  expect(await gateLockPids(objid)).toEqual([]);

  // The same statements defaultConnect issues, on its own reserved backend.
  const holder = postgres(DATABASE_URL, { max: 1, idle_timeout: 0, connect_timeout: 5, onnotice: () => {} });
  const reserved = await holder.reserve();
  await reserved`BEGIN`;
  const pid = (await reserved`SELECT pg_backend_pid() AS pid`)[0].pid;
  await reserved`SELECT pg_advisory_xact_lock(${composeKey(objid)}::bigint)`;

  // Held, granted, and visible to the observer — on that one backend.
  expect(await gateLockPids(objid)).toEqual([pid]);

  // No ROLLBACK: the connection simply goes away, as it would if the gate
  // process were killed or the backend reaped mid-run.
  await holder.end({ timeout: 0 });

  expect(await waitForNoLock(objid)).toEqual([]);
});

// ---------------------------------------------------------------------------
// The gate refuses before it connects
// ---------------------------------------------------------------------------

test("the verifier refuses a database URL whose name lacks _test without connecting", async () => {
  for (const url of [
    "postgresql://bridge@127.0.0.1:5432/bridge",
    "postgresql://bridge@127.0.0.1:5432/bridge_dev",
    "postgresql://bridge@127.0.0.1:5432/",
    "mysql://bridge@127.0.0.1:3306/bridge_test",
    "not a url",
  ]) {
    let connected = false;
    let fetched = false;
    await expect(
      verifyE2EStack({
        baseUrl: BASE_URL,
        databaseUrl: url,
        fetchImpl: async () => {
          fetched = true;
          throw new Error("must not fetch");
        },
        connect: async () => {
          connected = true;
          throw new Error("must not connect");
        },
      }),
    ).rejects.toThrow();
    expect(connected).toBe(false);
    expect(fetched).toBe(false);
  }

  // And the same rule is enforced by the exported parser the CLI uses.
  expect(testDatabaseName("postgresql://bridge@127.0.0.1:5432/bridge_test")).toBe("bridge_test");
  expect(() => testDatabaseName("postgresql://bridge@127.0.0.1:5432/bridge")).toThrow(/_test/);
});

// ---------------------------------------------------------------------------
// The total-hold deadline
// ---------------------------------------------------------------------------

live("the total-hold deadline fires, blames the hung service rather than the gate, and releases the lock", async () => {
  const nonce = "5".repeat(64);
  const objid = deriveObjid(nonce);
  const database = await liveDatabaseName();

  const { impl, calls } = mockStack({
    expected: fingerprint(nonce, database),
    // A service that never answers; only the abort signal ends the wait.
    onFetch: ({ init }) =>
      new Promise((_resolve, reject) => {
        const signal = init.signal;
        if (signal.aborted) {
          reject(signal.reason);
          return;
        }
        signal.addEventListener("abort", () => reject(signal.reason), { once: true });
      }),
  });

  const started = Date.now();
  const result = await verifyE2EStack({
    baseUrl: BASE_URL,
    databaseUrl: DATABASE_URL,
    nonce,
    samples: 1,
    // Much shorter than the per-fetch timeout, so the TOTAL-hold deadline is
    // unambiguously what fires.
    holdDeadlineMs: 80,
    fetchTimeoutMs: 10_000,
    fetchImpl: impl,
  });
  const elapsed = Date.now() - started;

  expect(result.ok).toBe(false);
  // An expired hold deadline is not a lost lock. The verifier re-reads its own
  // lock, finds it still held, and lets the per-service `timeout` stand: a
  // service that hangs is the stack's fault, so telling the operator "the
  // stack was not at fault — re-run" (the `lock lost` remediation) would be
  // exactly wrong. Next.js timed out, so Hocuspocus could not be checked.
  expect(result.failures).toEqual([
    { service: "next", class: "timeout" },
    { service: "go", class: "timeout" },
    { service: "hocuspocus", class: "unchecked" },
  ]);
  expect(result.failures.some((f) => f.service === "gate")).toBe(false);
  expect(REMEDIATION.timeout).not.toContain("re-run");
  expect(calls.length).toBeGreaterThan(0);
  expect(elapsed).toBeLessThan(10_000);

  expect(await gateLockPids(objid)).toEqual([]);
});

// ---------------------------------------------------------------------------
// A lock that disappears mid-check is the gate's fault, not the stack's
// ---------------------------------------------------------------------------

/**
 * defaultConnect is module-private, so this reproduces it statement for
 * statement (reserve → BEGIN → one-key xact lock → re-reads → ROLLBACK) and
 * lets a test decide what the re-read reports.
 */
function inlineConnect(databaseUrl, { grantedResults }) {
  return async () => {
    const client = postgres(databaseUrl, { max: 1, idle_timeout: 0, connect_timeout: 5, onnotice: () => {} });
    const reserved = await client.reserve();
    let granted = 0;
    return {
      begin: () => reserved`BEGIN`,
      backendPid: async () => (await reserved`SELECT pg_backend_pid() AS pid`)[0].pid,
      lock: (key) => reserved`SELECT pg_advisory_xact_lock(${key}::bigint)`,
      currentDatabase: async () => (await reserved`SELECT current_database() AS name`)[0].name,
      ownLockGranted: async (objid) => {
        const real =
          (
            await reserved`
              SELECT EXISTS (
                SELECT 1 FROM pg_locks l JOIN pg_database d ON d.oid = l.database
                WHERE d.datname = current_database() AND l.locktype = 'advisory'
                  AND l.classid = ${E2E_STACK_LOCK_CLASS}::int8::oid AND l.objid = ${objid}::int8::oid
                  AND l.objsubid = 1 AND l.granted AND l.pid = pg_backend_pid()
              ) AS granted`
          )[0].granted === true;
        const forced = grantedResults[granted];
        granted += 1;
        return forced === undefined ? real : forced;
      },
      rollback: () => reserved`ROLLBACK`,
      close: async () => {
        try {
          reserved.release();
        } finally {
          await client.end({ timeout: 2 });
        }
      },
    };
  };
}

live("a lock the gate cannot see before sampling aborts without fetching anything", async () => {
  const nonce = "6".repeat(64);
  const objid = deriveObjid(nonce);
  let fetched = 0;

  const result = await verifyE2EStack({
    baseUrl: BASE_URL,
    databaseUrl: DATABASE_URL,
    nonce,
    samples: 1,
    fetchImpl: async () => {
      fetched += 1;
      throw new Error("must not fetch after the gate lost its own lock");
    },
    connect: inlineConnect(DATABASE_URL, { grantedResults: [false] }),
  });

  expect(result.ok).toBe(false);
  expect(result.failures).toEqual([{ service: "gate", class: "lock lost" }]);
  expect(fetched).toBe(0);
  expect(await gateLockPids(objid)).toEqual([]);
});

live("a lock lost after sampling is reported as 'lock lost', not as a broken stack", async () => {
  const nonce = "7".repeat(64);
  const objid = deriveObjid(nonce);
  const database = await liveDatabaseName();

  // The first re-read (before sampling) is honest; the second (after
  // sampling) reports the lock gone, as a reaped backend or a pooler would.
  const { impl, calls } = mockStack({ expected: fingerprint(nonce, database), status: { go: 404 } });
  const result = await verifyE2EStack({
    baseUrl: BASE_URL,
    databaseUrl: DATABASE_URL,
    nonce,
    samples: 1,
    fetchImpl: impl,
    connect: inlineConnect(DATABASE_URL, { grantedResults: [undefined, false] }),
  });

  expect(result.ok).toBe(false);
  expect(calls.length).toBeGreaterThan(0);
  expect(result.failures).toEqual([{ service: "gate", class: "lock lost" }]);
  expect(await gateLockPids(objid)).toEqual([]);
});

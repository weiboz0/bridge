// @vitest-environment node

/**
 * Plan 094 Phase 14 — the Next.js E2E stack attestation route.
 *
 * `src/app/api/e2e-stack/route.ts` proves that THIS Next.js process's own `db`
 * pool sees the advisory lock the gate holds in its validated `_test` database,
 * and reports the `NEXT_PUBLIC_HOCUSPOCUS_URL` baked into the client bundle so
 * the gate can attest Hocuspocus on the origin browsers are actually handed.
 *
 * Every refusal — flag off, exposed host without the opt-in, malformed nonce,
 * non-`_test` parsed or live database name, unseen lock, failed observe query —
 * must be byte-identical to the route not existing, so they all funnel through
 * a single `notFound()`.
 *
 * `@/lib/db` is mocked, so nothing here opens a connection.
 */

import { readFileSync } from "node:fs";
import path from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";

import {
  DATABASE_URL_ENV_VAR,
  E2E_STACK_LOCK_CLASS,
  E2E_STACK_NONCE_PATTERN,
  E2E_STACK_OBSERVE_CONCURRENCY_LIMIT,
  GET,
  deriveE2EStackObjid,
  dynamic,
  e2eStackFingerprint,
  evaluateE2EStackRequest,
  getE2EStackInstanceId,
  isExposedHost,
  type E2EStackObserveLimiter,
  type E2EStackObserveResult,
} from "@/app/api/e2e-stack/route";

// ---------------------------------------------------------------------------
// Module mocks — hoisted so the factories can reference them, and so importing
// the route never reaches a real database or a real Next.js router.
// ---------------------------------------------------------------------------

const { NotFoundSentinel, notFoundMock, executeMock } = vi.hoisted(() => {
  class NotFoundSentinel extends Error {
    constructor() {
      super("NEXT_NOT_FOUND");
      this.name = "NotFoundSentinel";
    }
  }
  return {
    NotFoundSentinel,
    notFoundMock: vi.fn(() => {
      throw new NotFoundSentinel();
    }),
    executeMock: vi.fn(),
  };
});

vi.mock("next/navigation", () => ({
  notFound: (...args: unknown[]) => notFoundMock(...(args as [])),
}));

vi.mock("@/lib/db", () => ({
  db: { execute: (...args: unknown[]) => executeMock(...(args as [])) },
}));

// ---------------------------------------------------------------------------
// Shared cross-language contract + sources
// ---------------------------------------------------------------------------

const repoRoot = path.resolve(__dirname, "../..");

interface VectorCase {
  nonce: string;
  database: string;
  objid: number;
  key: string;
  fingerprint: string;
}

interface Vector {
  contract: {
    flag: { name: string; enabledValue: string };
    tunnelOptIn: { name: string; enabledValue: string };
    noncePattern: string;
    lockClass: number;
    lockClassHex: string;
    objsubid: number;
    testDatabaseSuffix: string;
    successBody: Record<string, string[]>;
    successHeaders: Record<string, string>;
    paths: Record<string, string>;
    nonceQueryParam: string;
    hostExposure: { name: string; exposedValue: string; normalization: string; rule: string };
    observeConcurrency: {
      maxInFlightPerProcess: number;
      rule: string;
      refusalReason: string;
    };
  };
  cases: VectorCase[];
  hostExposureCases: { value: string; exposed: boolean }[];
  composeKeyBoundaries: { objid: number; key: string }[];
  disjointFrom: {
    sessionLifecycleLockClass: number;
    classReplacementLockClass: number;
    hashtextOneKeyHighWords: number[];
  };
}

const vector = JSON.parse(
  readFileSync(path.join(repoRoot, "scripts/tests/e2e-stack-vector.json"), "utf8"),
) as Vector;

const routeSource = readFileSync(
  path.join(repoRoot, "src/app/api/e2e-stack/route.ts"),
  "utf8",
);
const providerSource = readFileSync(
  path.join(repoRoot, "src/lib/yjs/use-yjs-provider.ts"),
  "utf8",
);
const libDbSource = readFileSync(path.join(repoRoot, "src/lib/db/index.ts"), "utf8");

const FLAG = vector.contract.flag.name;
const FLAG_ON = vector.contract.flag.enabledValue;
const OPT_IN = vector.contract.tunnelOptIn.name;
const OPT_IN_ON = vector.contract.tunnelOptIn.enabledValue;
const NONCE_PARAM = vector.contract.nonceQueryParam;
const ROUTE_PATH = vector.contract.paths.next;

const NONCE = vector.cases[0].nonce;
const TEST_DATABASE_URL = "postgresql://bridge:hunter2@127.0.0.1:5432/bridge_test";
const LIVE_DATABASE = "bridge_test";
const REALTIME_URL = "ws://127.0.0.1:4111";

function composeKey(objid: number): string {
  return ((BigInt(E2E_STACK_LOCK_CLASS) << BigInt(32)) | BigInt(objid)).toString();
}

// ---------------------------------------------------------------------------
// Env + global state management
// ---------------------------------------------------------------------------

const MANAGED_ENV_KEYS = [
  FLAG,
  OPT_IN,
  "BRIDGE_HOST_EXPOSURE",
  DATABASE_URL_ENV_VAR,
  "NEXT_PUBLIC_HOCUSPOCUS_URL",
];

let savedEnv: Record<string, string | undefined> = {};
let savedInstance: string | undefined;
let savedRefusalLog: Map<string, number> | undefined;
let savedObserveInFlight: number | undefined;
let warnCalls: string[] = [];
const originalWarn = console.warn;

function attestedEnv(overrides: Record<string, string | undefined> = {}) {
  return {
    [FLAG]: FLAG_ON,
    [DATABASE_URL_ENV_VAR]: TEST_DATABASE_URL,
    ...overrides,
  } as Record<string, string | undefined>;
}

function applyEnv(env: Record<string, string | undefined>): void {
  for (const key of MANAGED_ENV_KEYS) delete process.env[key];
  for (const [key, value] of Object.entries(env)) {
    if (value !== undefined) process.env[key] = value;
  }
}

function request(nonce: string | null = NONCE): NextRequest {
  const url = new URL(`http://127.0.0.1:3000${ROUTE_PATH}`);
  if (nonce !== null) url.searchParams.set(NONCE_PARAM, nonce);
  return new NextRequest(url);
}

function observing(result: E2EStackObserveResult) {
  return vi.fn(async () => result);
}

beforeEach(() => {
  savedEnv = Object.fromEntries(MANAGED_ENV_KEYS.map((key) => [key, process.env[key]]));
  savedInstance = globalThis.bridgeE2EStackInstance;
  savedRefusalLog = globalThis.bridgeE2EStackRefusalLog;
  savedObserveInFlight = globalThis.bridgeE2EStackObserveInFlight;
  globalThis.bridgeE2EStackRefusalLog = new Map<string, number>();
  globalThis.bridgeE2EStackObserveInFlight = undefined;
  notFoundMock.mockClear();
  executeMock.mockReset();
  executeMock.mockResolvedValue([{ database: LIVE_DATABASE, lock_seen: true }]);
  warnCalls = [];
  console.warn = (...args: unknown[]) => {
    warnCalls.push(args.map((arg) => String(arg)).join(" "));
  };
});

afterEach(() => {
  for (const key of MANAGED_ENV_KEYS) {
    const value = savedEnv[key];
    if (value === undefined) delete process.env[key];
    else process.env[key] = value;
  }
  if (savedInstance === undefined) delete globalThis.bridgeE2EStackInstance;
  else globalThis.bridgeE2EStackInstance = savedInstance;
  if (savedRefusalLog === undefined) delete globalThis.bridgeE2EStackRefusalLog;
  else globalThis.bridgeE2EStackRefusalLog = savedRefusalLog;
  if (savedObserveInFlight === undefined) delete globalThis.bridgeE2EStackObserveInFlight;
  else globalThis.bridgeE2EStackObserveInFlight = savedObserveInFlight;
  console.warn = originalWarn;
});

// ---------------------------------------------------------------------------

describe("e2e-stack route", () => {
  it("e2e-stack route returns 404 without the flag", async () => {
    for (const flagValue of [undefined, "", "0", "true", " 1"]) {
      executeMock.mockClear();
      notFoundMock.mockClear();
      applyEnv(attestedEnv({ [FLAG]: flagValue }));

      await expect(GET(request())).rejects.toBeInstanceOf(NotFoundSentinel);
      expect(notFoundMock).toHaveBeenCalledTimes(1);
      // Nothing about the database is disclosed and no query is run.
      expect(executeMock).not.toHaveBeenCalled();
    }

    const evaluation = await evaluateE2EStackRequest({
      env: { [DATABASE_URL_ENV_VAR]: TEST_DATABASE_URL },
      nonce: NONCE,
      observe: observing({ database: LIVE_DATABASE, lockSeen: true }),
    });
    expect(evaluation).toEqual({ ok: false, reason: "flag_disabled" });
  });

  it("e2e-stack route refuses a non-test database", async () => {
    // Parsed name is not _test: refuse before running any query.
    const observe = observing({ database: LIVE_DATABASE, lockSeen: true });
    const parsed = await evaluateE2EStackRequest({
      env: attestedEnv({ [DATABASE_URL_ENV_VAR]: "postgresql://bridge@127.0.0.1:5432/bridge" }),
      nonce: NONCE,
      observe,
    });
    expect(parsed).toEqual({ ok: false, reason: "non_test_parsed_database" });
    expect(observe).not.toHaveBeenCalled();

    // A name that merely contains _test is not a _test database.
    expect(
      await evaluateE2EStackRequest({
        env: attestedEnv({
          [DATABASE_URL_ENV_VAR]: "postgresql://bridge@127.0.0.1:5432/bridge_test_prod",
        }),
        nonce: NONCE,
        observe,
      }),
    ).toEqual({ ok: false, reason: "non_test_parsed_database" });

    // The parsed name says _test but the live pool is somewhere else — the
    // clone / standby / misconfigured-pool case this attestation exists for.
    const liveObserve = observing({ database: "bridge", lockSeen: true });
    expect(
      await evaluateE2EStackRequest({ env: attestedEnv(), nonce: NONCE, observe: liveObserve }),
    ).toEqual({ ok: false, reason: "non_test_live_database" });
    expect(liveObserve).toHaveBeenCalledTimes(1);

    // Through the real handler, with the real pool call mocked.
    applyEnv(attestedEnv());
    executeMock.mockResolvedValue([{ database: "bridge", lock_seen: true }]);
    await expect(GET(request())).rejects.toBeInstanceOf(NotFoundSentinel);
    expect(executeMock).toHaveBeenCalledTimes(1);
  });

  it("e2e-stack route refuses an unseen lock", async () => {
    const observe = observing({ database: LIVE_DATABASE, lockSeen: false });
    expect(
      await evaluateE2EStackRequest({ env: attestedEnv(), nonce: NONCE, observe }),
    ).toEqual({ ok: false, reason: "lock_unseen" });
    // The query DID run: the right database was reached, the lock was not there.
    expect(observe).toHaveBeenCalledWith({
      lockClass: E2E_STACK_LOCK_CLASS,
      objid: deriveE2EStackObjid(NONCE),
    });

    applyEnv(attestedEnv());
    executeMock.mockResolvedValue([{ database: LIVE_DATABASE, lock_seen: false }]);
    await expect(GET(request())).rejects.toBeInstanceOf(NotFoundSentinel);

    // An observe query that fails or returns nothing refuses identically.
    executeMock.mockResolvedValue([]);
    await expect(GET(request())).rejects.toBeInstanceOf(NotFoundSentinel);
    executeMock.mockRejectedValue(new Error("connection terminated"));
    await expect(GET(request())).rejects.toBeInstanceOf(NotFoundSentinel);
  });

  it("e2e-stack route reports the baked realtime URL", async () => {
    applyEnv(attestedEnv({ NEXT_PUBLIC_HOCUSPOCUS_URL: REALTIME_URL }));

    const response = await GET(request());
    const body = (await response.json()) as Record<string, string>;
    expect(body.realtimeUrl).toBe(REALTIME_URL);
    expect(Object.keys(body).sort()).toEqual([...vector.contract.successBody.next].sort());
    expect(body.fingerprint).toBe(e2eStackFingerprint(NONCE, LIVE_DATABASE));
    expect(body.instance).toBe(getE2EStackInstanceId());

    // It is read from the process environment (where Next.js inlines the build
    // value), NEVER from the injected env object.
    const evaluation = await evaluateE2EStackRequest({
      env: attestedEnv({ NEXT_PUBLIC_HOCUSPOCUS_URL: "ws://injected.invalid:9999" }),
      nonce: NONCE,
      observe: observing({ database: LIVE_DATABASE, lockSeen: true }),
    });
    expect(evaluation.ok).toBe(true);
    if (evaluation.ok) expect(evaluation.body.realtimeUrl).toBe(REALTIME_URL);
  });

  it("e2e-stack route reports no realtime URL when it is unset", async () => {
    applyEnv(attestedEnv());
    delete process.env.NEXT_PUBLIC_HOCUSPOCUS_URL;

    const response = await GET(request());
    const body = (await response.json()) as Record<string, string>;
    expect("realtimeUrl" in body).toBe(false);
    expect(Object.keys(body).sort()).toEqual(["fingerprint", "instance"]);

    // An empty value is "unset" too — an empty origin would be unusable.
    process.env.NEXT_PUBLIC_HOCUSPOCUS_URL = "";
    const evaluation = await evaluateE2EStackRequest({
      env: attestedEnv(),
      nonce: NONCE,
      observe: observing({ database: LIVE_DATABASE, lockSeen: true }),
    });
    expect(evaluation.ok).toBe(true);
    if (evaluation.ok) expect("realtimeUrl" in evaluation.body).toBe(false);
  });

  it("e2e-stack route value equals the provider hook's realtime URL", () => {
    // Next.js inlines NEXT_PUBLIC_* at build time only where this exact literal
    // appears. A destructured or dynamic read would return the RUNTIME value and
    // reintroduce the build-vs-runtime split the attestation exists to catch.
    const literal = "process.env.NEXT_PUBLIC_HOCUSPOCUS_URL";
    expect(routeSource).toContain(literal);
    expect(providerSource).toContain(literal);

    // Not read through the injected env object, nor through a dynamic key.
    expect(routeSource).not.toMatch(/env\s*\[\s*["'`]?NEXT_PUBLIC_HOCUSPOCUS_URL/);
    expect(routeSource).not.toMatch(/(?<!process\.)\benv\.NEXT_PUBLIC_HOCUSPOCUS_URL/);
    expect(routeSource).not.toMatch(/process\.env\s*\[\s*[^\]]*HOCUSPOCUS/);
    expect(routeSource).not.toMatch(/\{[^}]*NEXT_PUBLIC_HOCUSPOCUS_URL[^}]*\}\s*=\s*process\.env/);

    // Exactly one read in the route, so there is no second divergent source.
    const reads = routeSource.split(literal).length - 1;
    expect(reads).toBe(1);
  });

  it("e2e-stack route is force-dynamic and no-store", async () => {
    expect(dynamic).toBe("force-dynamic");
    expect(routeSource).toMatch(/export const dynamic = "force-dynamic"/);

    applyEnv(attestedEnv({ NEXT_PUBLIC_HOCUSPOCUS_URL: REALTIME_URL }));
    const response = await GET(request());
    expect(response.status).toBe(200);
    expect(response.headers.get("Cache-Control")).toBe(
      vector.contract.successHeaders["Cache-Control"],
    );
    expect(response.headers.get("Content-Type")).toContain(
      vector.contract.successHeaders["Content-Type"],
    );
  });

  it("e2e-stack route refusals are byte-identical including a malformed nonce", async () => {
    // Every refusal takes the SAME single notFound() path, so the flag is
    // indistinguishable from the route not existing.
    expect(routeSource.match(/^\s*notFound\(\);\s*$/gm) ?? []).toHaveLength(1);

    const refusals: {
      name: string;
      env: Record<string, string | undefined>;
      nonce: string | null;
      execute?: () => Promise<unknown>;
      queryExpected: boolean;
    }[] = [
      { name: "flag off", env: attestedEnv({ [FLAG]: undefined }), nonce: NONCE, queryExpected: false },
      {
        name: "exposed without opt-in",
        env: attestedEnv({ BRIDGE_HOST_EXPOSURE: "exposed" }),
        nonce: NONCE,
        queryExpected: false,
      },
      { name: "missing nonce", env: attestedEnv(), nonce: null, queryExpected: false },
      { name: "malformed nonce", env: attestedEnv(), nonce: "not-a-nonce", queryExpected: false },
      {
        name: "uppercase nonce",
        env: attestedEnv(),
        nonce: vector.cases[4].nonce.toUpperCase(),
        queryExpected: false,
      },
      { name: "short nonce", env: attestedEnv(), nonce: NONCE.slice(0, 63), queryExpected: false },
      {
        name: "non-test parsed database",
        env: attestedEnv({ [DATABASE_URL_ENV_VAR]: "postgresql://bridge@127.0.0.1:5432/bridge" }),
        nonce: NONCE,
        queryExpected: false,
      },
      {
        name: "missing database url",
        env: attestedEnv({ [DATABASE_URL_ENV_VAR]: undefined }),
        nonce: NONCE,
        queryExpected: false,
      },
      {
        name: "non-test live database",
        env: attestedEnv(),
        nonce: NONCE,
        execute: async () => [{ database: "bridge", lock_seen: true }],
        queryExpected: true,
      },
      {
        name: "unseen lock",
        env: attestedEnv(),
        nonce: NONCE,
        execute: async () => [{ database: LIVE_DATABASE, lock_seen: false }],
        queryExpected: true,
      },
      {
        name: "observe query fails",
        env: attestedEnv(),
        nonce: NONCE,
        execute: async () => {
          throw new Error("connection terminated");
        },
        queryExpected: true,
      },
    ];

    const thrown: unknown[] = [];
    for (const refusal of refusals) {
      executeMock.mockReset();
      executeMock.mockImplementation(
        refusal.execute ?? (async () => [{ database: LIVE_DATABASE, lock_seen: true }]),
      );
      notFoundMock.mockClear();
      applyEnv(refusal.env);

      const error = await GET(request(refusal.nonce)).then(
        () => new Error(`${refusal.name} did not refuse`),
        (caught: unknown) => caught,
      );
      expect(error, refusal.name).toBeInstanceOf(NotFoundSentinel);
      thrown.push(error);

      expect(notFoundMock, refusal.name).toHaveBeenCalledTimes(1);
      expect(notFoundMock, refusal.name).toHaveBeenCalledWith();
      expect(executeMock.mock.calls.length > 0, `${refusal.name} query usage`).toBe(
        refusal.queryExpected,
      );
    }

    // Identical outcome for all of them: same constructor, same message.
    const messages = new Set(thrown.map((error) => (error as Error).message));
    expect(messages.size).toBe(1);
  });

  it("e2e-stack route refuses on an exposed host without the opt-in", async () => {
    for (const optIn of [undefined, "", "TRUE", "1", "yes"]) {
      expect(
        await evaluateE2EStackRequest({
          env: attestedEnv({ BRIDGE_HOST_EXPOSURE: "exposed", [OPT_IN]: optIn }),
          nonce: NONCE,
          observe: observing({ database: LIVE_DATABASE, lockSeen: true }),
        }),
      ).toEqual({ ok: false, reason: "exposed_without_opt_in" });
    }

    const allowed = await evaluateE2EStackRequest({
      env: attestedEnv({ BRIDGE_HOST_EXPOSURE: "exposed", [OPT_IN]: OPT_IN_ON }),
      nonce: NONCE,
      observe: observing({ database: LIVE_DATABASE, lockSeen: true }),
    });
    expect(allowed.ok).toBe(true);

    // Not exposed: the opt-in is irrelevant.
    const local = await evaluateE2EStackRequest({
      env: attestedEnv(),
      nonce: NONCE,
      observe: observing({ database: LIVE_DATABASE, lockSeen: true }),
    });
    expect(local.ok).toBe(true);

    applyEnv(attestedEnv({ BRIDGE_HOST_EXPOSURE: "exposed" }));
    await expect(GET(request())).rejects.toBeInstanceOf(NotFoundSentinel);
    expect(executeMock).not.toHaveBeenCalled();
  });

  it("e2e-stack route refusal logging is rate limited and omits the URL", async () => {
    applyEnv(attestedEnv({ [FLAG]: "0" }));

    await expect(GET(request())).rejects.toBeInstanceOf(NotFoundSentinel);
    await expect(GET(request())).rejects.toBeInstanceOf(NotFoundSentinel);
    await expect(GET(request())).rejects.toBeInstanceOf(NotFoundSentinel);
    expect(warnCalls).toHaveLength(1);

    // Once the 10s window has passed, the reason is logged again.
    globalThis.bridgeE2EStackRefusalLog?.set("flag_disabled", Date.now() - 10_001);
    await expect(GET(request())).rejects.toBeInstanceOf(NotFoundSentinel);
    expect(warnCalls).toHaveLength(2);

    // A different reason keeps its own window.
    applyEnv(attestedEnv());
    await expect(GET(request("not-a-nonce"))).rejects.toBeInstanceOf(NotFoundSentinel);
    expect(warnCalls).toHaveLength(3);

    const logged = warnCalls.join("\n");
    expect(logged).not.toContain(TEST_DATABASE_URL);
    expect(logged).not.toContain("hunter2");
    expect(logged).not.toMatch(/postgres(ql)?:\/\//i);
    expect(logged).not.toContain(NONCE);
    expect(logged).not.toContain(String(deriveE2EStackObjid(NONCE)));
    expect(logged).not.toContain(composeKey(deriveE2EStackObjid(NONCE)));
    expect(logged).toContain("flag_disabled");
    expect(logged).toContain("malformed_nonce");
  });

  it("e2e-stack route instance id survives module re-evaluation", async () => {
    const first = getE2EStackInstanceId();
    expect(first).toContain(`${process.pid}-`);
    expect(first.startsWith(`${process.pid}-`)).toBe(true);

    // A Next.js dev recompile / fast refresh re-evaluates the module body. The
    // id is cached on globalThis precisely so that does not mint a new one.
    vi.resetModules();
    const reloaded = (await import("@/app/api/e2e-stack/route")) as {
      getE2EStackInstanceId: () => string;
    };
    expect(reloaded.getE2EStackInstanceId).not.toBe(getE2EStackInstanceId);
    expect(reloaded.getE2EStackInstanceId()).toBe(first);
    expect(globalThis.bridgeE2EStackInstance).toBe(first);

    // And a process that has no cached id mints exactly one.
    delete globalThis.bridgeE2EStackInstance;
    const minted = getE2EStackInstanceId();
    expect(minted).not.toBe(first);
    expect(getE2EStackInstanceId()).toBe(minted);
    expect(minted.startsWith(`${process.pid}-`)).toBe(true);
  });

  it("e2e-stack route reads the same DATABASE_URL variable as src/lib/db", () => {
    expect(DATABASE_URL_ENV_VAR).toBe("DATABASE_URL");
    // src/lib/db/index.ts builds the pool this route speaks for; if it ever
    // read a different variable the route would attest the wrong pool.
    expect(libDbSource).toContain(`process.env.${DATABASE_URL_ENV_VAR}`);
    // The route must reference the exported constant, not a divergent literal.
    expect(routeSource).toContain("env[DATABASE_URL_ENV_VAR]");
  });


  it("e2e-stack route pure functions reject a malformed nonce like the other implementations", async () => {
    const route = await import("@/app/api/e2e-stack/route");
    for (const bad of ["", "0".repeat(63), "0".repeat(65), "A".repeat(64), "g".repeat(64)]) {
      expect(() => route.deriveE2EStackObjid(bad)).toThrow(/64 lowercase hex/);
      expect(() => route.e2eStackFingerprint(bad, "bridge_test")).toThrow(/64 lowercase hex/);
    }
  });

  it("e2e-stack route parses only a single-segment PostgreSQL database name", async () => {
    const route = await import("@/app/api/e2e-stack/route");
    const observe = vi.fn(async () => ({ database: "bridge_test", lockSeen: true }));
    for (const url of ["https://example.com/bridge_test", "mysql://u@h/bridge_test", "postgresql://u@h/a/bridge_test", "postgresql://u@h/a%2Fbridge_test"]) {
      const outcome = await route.evaluateE2EStackRequest({
        env: { BRIDGE_E2E_STACK: "1", [route.DATABASE_URL_ENV_VAR]: url },
        nonce: "0".repeat(64),
        observe,
      });
      expect(outcome.ok).toBe(false);
    }
    expect(observe).not.toHaveBeenCalled();
  });
  // -------------------------------------------------------------------------
  // R2-15: exposure normalization
  // -------------------------------------------------------------------------

  it("e2e-stack route exposure check matches the shared vector", async () => {
    const exposure = vector.contract.hostExposure;
    expect(exposure.name).toBe("BRIDGE_HOST_EXPOSURE");
    expect(exposure.exposedValue).toBe("exposed");
    expect(vector.hostExposureCases.length).toBeGreaterThan(0);

    // The exported predicate answers every shared row, and an unset variable.
    for (const row of vector.hostExposureCases) {
      expect(isExposedHost(row.value), JSON.stringify(row.value)).toBe(row.exposed);
    }
    expect(isExposedHost(undefined)).toBe(false);

    // Behaviourally: R2-15 was that this route compared the value EXACTLY
    // while Go and Hocuspocus trimmed and case-folded, so "Exposed" made the
    // two local services refuse while the internet-facing one kept serving.
    for (const row of vector.hostExposureCases) {
      const refused = await evaluateE2EStackRequest({
        env: attestedEnv({ BRIDGE_HOST_EXPOSURE: row.value }),
        nonce: NONCE,
        observe: observing({ database: LIVE_DATABASE, lockSeen: true }),
      });
      if (row.exposed) {
        expect(refused, JSON.stringify(row.value)).toEqual({
          ok: false,
          reason: "exposed_without_opt_in",
        });
      } else {
        expect(refused.ok, JSON.stringify(row.value)).toBe(true);
      }

      // The recorded opt-in allows every value.
      const allowed = await evaluateE2EStackRequest({
        env: attestedEnv({ BRIDGE_HOST_EXPOSURE: row.value, [OPT_IN]: OPT_IN_ON }),
        nonce: NONCE,
        observe: observing({ database: LIVE_DATABASE, lockSeen: true }),
      });
      expect(allowed.ok, JSON.stringify(row.value)).toBe(true);
    }

    // The two spellings R2-15 named explicitly now refuse without the opt-in.
    for (const value of ["Exposed", " exposed"]) {
      const observe = observing({ database: LIVE_DATABASE, lockSeen: true });
      expect(
        await evaluateE2EStackRequest({
          env: attestedEnv({ BRIDGE_HOST_EXPOSURE: value }),
          nonce: NONCE,
          observe,
        }),
      ).toEqual({ ok: false, reason: "exposed_without_opt_in" });
      expect(observe).not.toHaveBeenCalled();
    }

    // The opt-in itself is deliberately NOT normalized: it must be exactly
    // "true", so a truthy-looking value never unlocks an exposed host.
    for (const wrong of ["TRUE", "1", " true", "True", "yes"]) {
      expect(
        await evaluateE2EStackRequest({
          env: attestedEnv({ BRIDGE_HOST_EXPOSURE: "Exposed", [OPT_IN]: wrong }),
          nonce: NONCE,
          observe: observing({ database: LIVE_DATABASE, lockSeen: true }),
        }),
      ).toEqual({ ok: false, reason: "exposed_without_opt_in" });
    }
  });

  // -------------------------------------------------------------------------
  // R2-16: the observe query is capped per process
  // -------------------------------------------------------------------------

  it("e2e-stack route refuses when the observe cap is reached and runs no query", async () => {
    const release = vi.fn();
    const limiter: E2EStackObserveLimiter = { tryAcquire: () => false, release };
    const observe = observing({ database: LIVE_DATABASE, lockSeen: true });

    expect(
      await evaluateE2EStackRequest({ env: attestedEnv(), nonce: NONCE, observe, limiter }),
    ).toEqual({ ok: false, reason: vector.contract.observeConcurrency.refusalReason });

    expect(observe).not.toHaveBeenCalled();
    // No slot was taken, so nothing may be released on this path.
    expect(release).not.toHaveBeenCalled();
  });

  it("e2e-stack route releases its observe slot on success, error, and timeout", async () => {
    const counting = () => {
      const state = { acquired: 0, released: 0 };
      const limiter: E2EStackObserveLimiter = {
        tryAcquire: () => {
          state.acquired += 1;
          return true;
        },
        release: () => {
          state.released += 1;
        },
      };
      return { state, limiter };
    };

    // Success.
    {
      const { state, limiter } = counting();
      const result = await evaluateE2EStackRequest({
        env: attestedEnv(),
        nonce: NONCE,
        observe: observing({ database: LIVE_DATABASE, lockSeen: true }),
        limiter,
      });
      expect(result.ok).toBe(true);
      expect(state).toEqual({ acquired: 1, released: 1 });
    }

    // Error.
    {
      const { state, limiter } = counting();
      const result = await evaluateE2EStackRequest({
        env: attestedEnv(),
        nonce: NONCE,
        observe: vi.fn(async () => {
          throw new Error("connection terminated");
        }),
        limiter,
      });
      expect(result).toEqual({ ok: false, reason: "observe_query_failed" });
      expect(state).toEqual({ acquired: 1, released: 1 });
    }

    // Timeout: the abandoned query must not strand its slot either.
    {
      const { state, limiter } = counting();
      vi.useFakeTimers();
      try {
        const pending = evaluateE2EStackRequest({
          env: attestedEnv(),
          nonce: NONCE,
          observe: vi.fn(() => new Promise<E2EStackObserveResult>(() => {})),
          limiter,
        });
        await vi.advanceTimersByTimeAsync(2_001);
        expect(await pending).toEqual({ ok: false, reason: "observe_query_failed" });
      } finally {
        vi.useRealTimers();
      }
      expect(state).toEqual({ acquired: 1, released: 1 });
    }
  });

  it("e2e-stack route default limiter is process-wide and capped at the contract value", async () => {
    const limit = vector.contract.observeConcurrency.maxInFlightPerProcess;
    expect(limit).toBeGreaterThan(0);
    expect(E2E_STACK_OBSERVE_CONCURRENCY_LIMIT).toBe(limit);

    // The counter lives on globalThis so a dev-server module re-evaluation
    // cannot reset an in-flight count to zero; beforeEach/afterEach save and
    // restore it around this test.
    globalThis.bridgeE2EStackObserveInFlight = undefined;

    const resolvers: ((result: E2EStackObserveResult) => void)[] = [];
    const observe = vi.fn(
      () =>
        new Promise<E2EStackObserveResult>((resolve) => {
          resolvers.push(resolve);
        }),
    );

    // Each evaluation builds its OWN default limiter, yet they share one count.
    const held = [];
    for (let index = 0; index < limit; index += 1) {
      held.push(evaluateE2EStackRequest({ env: attestedEnv(), nonce: NONCE, observe }));
    }
    expect(globalThis.bridgeE2EStackObserveInFlight).toBe(limit);
    expect(observe).toHaveBeenCalledTimes(limit);

    const over = await evaluateE2EStackRequest({
      env: attestedEnv(),
      nonce: NONCE,
      observe,
    });
    expect(over).toEqual({ ok: false, reason: vector.contract.observeConcurrency.refusalReason });
    expect(observe).toHaveBeenCalledTimes(limit);
    expect(globalThis.bridgeE2EStackObserveInFlight).toBe(limit);

    // Through the real handler, the busy refusal takes the SAME single
    // notFound() path as every other refusal, and runs no query.
    applyEnv(attestedEnv());
    executeMock.mockClear();
    notFoundMock.mockClear();
    const busyError = await GET(request()).then(
      () => new Error("the busy request did not refuse"),
      (caught: unknown) => caught,
    );
    expect(busyError).toBeInstanceOf(NotFoundSentinel);
    expect(notFoundMock).toHaveBeenCalledTimes(1);
    expect(notFoundMock).toHaveBeenCalledWith();
    expect(executeMock).not.toHaveBeenCalled();
    // ...and it was refused for THAT reason, not another.
    expect(warnCalls.join("\n")).toContain(vector.contract.observeConcurrency.refusalReason);

    // Byte-identical to the flag-off refusal.
    globalThis.bridgeE2EStackObserveInFlight = 0;
    applyEnv(attestedEnv({ [FLAG]: undefined }));
    const flagOffError = await GET(request()).then(
      () => new Error("the flag-off request did not refuse"),
      (caught: unknown) => caught,
    );
    expect((busyError as Error).message).toBe((flagOffError as Error).message);

    // Releasing every held query returns the process-wide count to zero.
    globalThis.bridgeE2EStackObserveInFlight = limit;
    for (const resolve of resolvers) resolve({ database: LIVE_DATABASE, lockSeen: true });
    for (const pending of held) expect((await pending).ok).toBe(true);
    expect(globalThis.bridgeE2EStackObserveInFlight).toBe(0);
  });

  it("e2e-stack route matches the shared vector", () => {
    expect(E2E_STACK_LOCK_CLASS).toBe(vector.contract.lockClass);
    expect(`0x${E2E_STACK_LOCK_CLASS.toString(16)}`).toBe(vector.contract.lockClassHex);
    expect(E2E_STACK_NONCE_PATTERN.source).toBe(vector.contract.noncePattern);
    expect(ROUTE_PATH).toBe("/api/e2e-stack");

    for (const testCase of vector.cases) {
      expect(deriveE2EStackObjid(testCase.nonce)).toBe(testCase.objid);
      expect(composeKey(testCase.objid)).toBe(testCase.key);
      // The non-ASCII name pins the UTF-8 encoding of the second field.
      expect(e2eStackFingerprint(testCase.nonce, testCase.database)).toBe(testCase.fingerprint);
    }

    for (const boundary of vector.composeKeyBoundaries) {
      expect(composeKey(boundary.objid)).toBe(boundary.key);
      expect(BigInt(boundary.key) > BigInt(0)).toBe(true);
    }

    expect(E2E_STACK_LOCK_CLASS).not.toBe(vector.disjointFrom.sessionLifecycleLockClass);
    expect(E2E_STACK_LOCK_CLASS).not.toBe(vector.disjointFrom.classReplacementLockClass);
    for (const highWord of vector.disjointFrom.hashtextOneKeyHighWords) {
      expect(E2E_STACK_LOCK_CLASS).not.toBe(highWord);
    }

    // The observe query this route runs on its own pool must match the shared
    // shape: one-key advisory lock, objsubid = 1, granted, current database.
    const normalised = routeSource.replace(/\s+/g, " ");
    for (const fragment of [
      "SELECT current_database()",
      "FROM pg_locks l",
      "JOIN pg_database d ON d.oid = l.database",
      "d.datname = current_database()",
      "l.locktype = 'advisory'",
      "::int8::oid",
      `l.objsubid = ${vector.contract.objsubid}`,
      "l.granted",
    ]) {
      expect(normalised, `observe query missing ${fragment}`).toContain(fragment);
    }

    // The derived key reaches the query as the class/objid pair, not a literal.
    expect(routeSource).toContain("lockClass: E2E_STACK_LOCK_CLASS");
    expect(vector.contract.testDatabaseSuffix).toBe("_test");
  });
});

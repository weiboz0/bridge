/**
 * Plan 094 Phase 14 — Hocuspocus E2E stack attestation.
 *
 * The client-facing realtime port is the origin browsers are actually handed,
 * so it is the origin the gate attests. Hocuspocus answers every unhandled HTTP
 * request with its own `200 text/plain`, which means a refusal here must
 * *decline to handle* the request rather than write a 404: only then is the
 * flag indistinguishable from the path not existing.
 *
 * Everything observable in `createE2EStackAttestation` is injected, so these
 * tests exercise both the success and every refusal path without a database and
 * without binding a port. The flag-conditional hook registration in
 * `server/hocuspocus.ts` is checked in a child process, because the ambient
 * environment of this test process cannot be changed after module load.
 */

import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import type { IncomingMessage, ServerResponse } from "node:http";
import path from "node:path";

import {
  E2E_STACK_DATABASE_URL_ENV,
  E2E_STACK_LOCK_CLASS,
  E2E_STACK_MAX_IN_FLIGHT_OBSERVE_QUERIES,
  E2E_STACK_NONCE_PATTERN,
  createE2EStackAttestation,
  deriveE2EStackObjid,
  e2eStackFingerprint,
} from "./e2e-stack";

// ---------------------------------------------------------------------------
// Shared cross-language contract
// ---------------------------------------------------------------------------

const repoRoot = path.resolve(import.meta.dir, "..");

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
    observeQuery: string;
    fingerprint: string;
    testDatabaseSuffix: string;
    successHeaders: Record<string, string>;
    successBody: Record<string, string[]>;
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

const e2eStackSource = readFileSync(path.join(repoRoot, "server/e2e-stack.ts"), "utf8");
const hocuspocusSource = readFileSync(path.join(repoRoot, "server/hocuspocus.ts"), "utf8");
const serverDbSource = readFileSync(path.join(repoRoot, "server/db.ts"), "utf8");

const NONCE = vector.cases[0].nonce;
const OTHER_NONCE = vector.cases[4].nonce;
const TEST_DATABASE_URL = "postgres://bridge:pw@127.0.0.1:5432/bridge_test";
const LIVE_DATABASE = "bridge_test";
const HOOK_PATH = vector.contract.paths.hocuspocus;
const NONCE_PARAM = vector.contract.nonceQueryParam;

function composeKey(objid: number): string {
  return ((BigInt(E2E_STACK_LOCK_CLASS) << 32n) | BigInt(objid)).toString();
}

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

interface Spy<A extends unknown[], R> {
  (...args: A): R;
  calls: A[];
}

function spy<A extends unknown[], R>(impl: (...args: A) => R): Spy<A, R> {
  const calls: A[] = [];
  const fn = (...args: A): R => {
    calls.push(args);
    return impl(...args);
  };
  return Object.assign(fn, { calls });
}

function fakeRequest(method: string, url: string): IncomingMessage {
  return { method, url } as unknown as IncomingMessage;
}

interface FakeResponse {
  response: ServerResponse;
  heads: { status: number; headers: Record<string, string> }[];
  bodies: string[];
}

function fakeResponse(): FakeResponse {
  const heads: { status: number; headers: Record<string, string> }[] = [];
  const bodies: string[] = [];
  const response = {
    writeHead(status: number, headers: Record<string, string>) {
      heads.push({ status, headers });
      return response;
    },
    end(body?: string) {
      bodies.push(body ?? "");
      return response;
    },
  };
  return { response: response as unknown as ServerResponse, heads, bodies };
}

type QueryResult = { database: string; seen: boolean };
type QueryFn = (lockClass: number, objid: number) => Promise<QueryResult>;

interface Harness {
  attestation: ReturnType<typeof createE2EStackAttestation>;
  query: Spy<[number, number], Promise<QueryResult>>;
  logs: string[];
  clock: { value: number };
}

function harness(
  overrides: {
    env?: Record<string, string | undefined>;
    query?: QueryFn;
    randomBytes?: (size: number) => Uint8Array;
    maxInFlightObserveQueries?: number;
  } = {},
): Harness {
  const logs: string[] = [];
  const clock = { value: 0 };
  const query = spy<[number, number], Promise<QueryResult>>(
    overrides.query ?? (async () => ({ database: LIVE_DATABASE, seen: true })),
  );
  const attestation = createE2EStackAttestation({
    env: overrides.env ?? {
      [vector.contract.flag.name]: vector.contract.flag.enabledValue,
      [E2E_STACK_DATABASE_URL_ENV]: TEST_DATABASE_URL,
    },
    query,
    now: () => clock.value,
    log: (message: string) => logs.push(message),
    randomBytes: overrides.randomBytes ?? ((size: number) => new Uint8Array(size).fill(0xab)),
    maxInFlightObserveQueries: overrides.maxInFlightObserveQueries,
  });
  return { attestation, query, logs, clock };
}

function attestationUrl(nonce: string = NONCE): string {
  return `${HOOK_PATH}?${NONCE_PARAM}=${nonce}`;
}

const INSTANCE_GLOBAL_KEY = "__bridgeE2EStackAttestationInstance";

type InstanceGlobal = typeof globalThis & { [INSTANCE_GLOBAL_KEY]?: string };

let savedInstanceId: string | undefined;

beforeEach(() => {
  savedInstanceId = (globalThis as InstanceGlobal)[INSTANCE_GLOBAL_KEY];
  delete (globalThis as InstanceGlobal)[INSTANCE_GLOBAL_KEY];
});

afterEach(() => {
  if (savedInstanceId === undefined) delete (globalThis as InstanceGlobal)[INSTANCE_GLOBAL_KEY];
  else (globalThis as InstanceGlobal)[INSTANCE_GLOBAL_KEY] = savedInstanceId;
});

// ---------------------------------------------------------------------------

describe("hocuspocus e2e stack attestation", () => {
  test("e2e stack attestation is absent without the flag", async () => {
    for (const flagValue of [undefined, "", "0", "true", "1 ", " 1", "11"]) {
      const env: Record<string, string | undefined> = {
        [E2E_STACK_DATABASE_URL_ENV]: TEST_DATABASE_URL,
      };
      if (flagValue !== undefined) env[vector.contract.flag.name] = flagValue;

      const { attestation, query, logs } = harness({ env });
      expect(attestation.enabled).toBe(false);

      const { response, heads, bodies } = fakeResponse();
      await expect(
        attestation.onRequest({ request: fakeRequest("GET", attestationUrl()), response }),
      ).resolves.toBeUndefined();

      expect(heads).toEqual([]);
      expect(bodies).toEqual([]);
      expect(query.calls).toEqual([]);
      // A refusal reason would itself disclose that the surface exists.
      expect(logs).toEqual([]);
    }

    const enabled = harness().attestation;
    expect(enabled.enabled).toBe(true);
  });

  test("e2e stack hook is not registered without the flag", () => {
    // The production wiring is a conditional spread; assert the exact shape so
    // an unconditional registration cannot slip in. R2-21: the condition is
    // `registrable`, NOT `enabled` — the hook used to attach whenever the flag
    // was on, while the exposed-host boot refusal ran only under
    // `import.meta.main`, so a non-main importer got the endpoint anyway.
    const spread = hocuspocusSource.replace(/\s+/g, " ");
    expect(spread).toContain(
      "...(e2eStackAttestation.registrable ? { onRequest: e2eStackAttestation.onRequest } : {})",
    );
    expect(spread).not.toContain(
      "...(e2eStackAttestation.enabled ? { onRequest: e2eStackAttestation.onRequest } : {})",
    );

    // ...and prove it in a real process, where the module actually evaluates.
    expect(hookRegisteredInChild("0")).toBe(false);
    expect(hookRegisteredInChild("1")).toBe(true);
  });

  test("e2e stack attestation is reachable on the client-facing realtime origin", async () => {
    const { attestation, query } = harness();
    const { response, heads, bodies } = fakeResponse();

    // A handled request REJECTS with a falsy value: that is how Hocuspocus is
    // told to suppress its default unhandled response.
    await expect(
      attestation.onRequest({ request: fakeRequest("GET", attestationUrl()), response }),
    ).rejects.toBeUndefined();

    expect(query.calls).toEqual([[E2E_STACK_LOCK_CLASS, deriveE2EStackObjid(NONCE)]]);
    expect(heads).toHaveLength(1);
    expect(heads[0].status).toBe(200);
    expect(bodies).toHaveLength(1);

    const body = JSON.parse(bodies[0]) as { fingerprint: string; instance: string };
    expect(Object.keys(body).sort()).toEqual([...vector.contract.successBody.hocuspocus].sort());
    expect(body.fingerprint).toBe(e2eStackFingerprint(NONCE, LIVE_DATABASE));
    expect(body.fingerprint).toBe(vector.cases[0].fingerprint);
    expect(body.instance).toContain(`-${process.pid}-`);

    // Extra query parameters and a repeated nonce must not change the answer.
    const second = fakeResponse();
    await expect(
      attestation.onRequest({
        request: fakeRequest("GET", `${attestationUrl()}&extra=1`),
        response: second.response,
      }),
    ).rejects.toBeUndefined();
    expect(JSON.parse(second.bodies[0])).toEqual(body);

    // A different nonce derives a different key and a different fingerprint.
    const third = fakeResponse();
    await expect(
      attestation.onRequest({
        request: fakeRequest("GET", attestationUrl(OTHER_NONCE)),
        response: third.response,
      }),
    ).rejects.toBeUndefined();
    expect(query.calls.at(-1)).toEqual([E2E_STACK_LOCK_CLASS, deriveE2EStackObjid(OTHER_NONCE)]);
    expect((JSON.parse(third.bodies[0]) as { fingerprint: string }).fingerprint).toBe(
      vector.cases[4].fingerprint,
    );
  });

  test("e2e stack attestation refuses a non-test live database", async () => {
    // The parsed URL says _test but the pool is really connected elsewhere:
    // exactly the clone/standby/misconfiguration case this attestation exists
    // to catch.
    const { attestation, query, logs } = harness({
      query: async () => ({ database: "bridge", seen: true }),
    });
    const { response, heads, bodies } = fakeResponse();

    await expect(
      attestation.onRequest({ request: fakeRequest("GET", attestationUrl()), response }),
    ).resolves.toBeUndefined();

    expect(query.calls).toHaveLength(1);
    expect(heads).toEqual([]);
    expect(bodies).toEqual([]);
    expect(logs.join("\n")).toContain("live_database_not_test");

    // A name that merely contains "_test" without ending in it is refused too.
    const near = harness({ query: async () => ({ database: "bridge_test_prod", seen: true }) });
    const nearResponse = fakeResponse();
    await expect(
      near.attestation.onRequest({
        request: fakeRequest("GET", attestationUrl()),
        response: nearResponse.response,
      }),
    ).resolves.toBeUndefined();
    expect(nearResponse.heads).toEqual([]);
  });

  test("e2e stack attestation refuses an unseen lock", async () => {
    const { attestation, query, logs } = harness({
      query: async () => ({ database: LIVE_DATABASE, seen: false }),
    });
    const { response, heads, bodies } = fakeResponse();

    await expect(
      attestation.onRequest({ request: fakeRequest("GET", attestationUrl()), response }),
    ).resolves.toBeUndefined();

    // The query DID run — the right database was reached, the lock was not there.
    expect(query.calls).toEqual([[E2E_STACK_LOCK_CLASS, deriveE2EStackObjid(NONCE)]]);
    expect(heads).toEqual([]);
    expect(bodies).toEqual([]);
    expect(logs.join("\n")).toContain("lock_not_seen");
  });

  test("e2e stack refusals fall through to the default unhandled response", async () => {
    const never = () => new Promise<QueryResult>(() => {});

    const refusals: {
      name: string;
      method?: string;
      url?: string;
      env?: Record<string, string | undefined>;
      query?: QueryFn;
      queryExpected: boolean;
    }[] = [
      { name: "wrong method", method: "POST", queryExpected: false },
      { name: "wrong method (HEAD)", method: "HEAD", queryExpected: false },
      { name: "other path", url: "/collaboration", queryExpected: false },
      { name: "path prefix only", url: `${HOOK_PATH}x?${NONCE_PARAM}=${NONCE}`, queryExpected: false },
      { name: "missing nonce", url: HOOK_PATH, queryExpected: false },
      { name: "malformed nonce", url: `${HOOK_PATH}?${NONCE_PARAM}=NOTHEX`, queryExpected: false },
      {
        name: "uppercase nonce",
        url: `${HOOK_PATH}?${NONCE_PARAM}=${NONCE.toUpperCase().replace(/0/g, "A")}`,
        queryExpected: false,
      },
      {
        name: "non-test parsed name",
        env: {
          [vector.contract.flag.name]: vector.contract.flag.enabledValue,
          [E2E_STACK_DATABASE_URL_ENV]: "postgres://bridge@127.0.0.1:5432/bridge",
        },
        queryExpected: false,
      },
      {
        name: "missing database url",
        env: { [vector.contract.flag.name]: vector.contract.flag.enabledValue },
        queryExpected: false,
      },
      {
        name: "non-test live name",
        query: async () => ({ database: "bridge", seen: true }),
        queryExpected: true,
      },
      {
        name: "unseen lock",
        query: async () => ({ database: LIVE_DATABASE, seen: false }),
        queryExpected: true,
      },
      {
        name: "query throws",
        query: async () => {
          throw new Error("connection terminated");
        },
        queryExpected: true,
      },
      { name: "query times out", query: never, queryExpected: true },
    ];

    for (const refusal of refusals) {
      const { attestation, query } = harness({ env: refusal.env, query: refusal.query });
      const { response, heads, bodies } = fakeResponse();

      await expect(
        attestation.onRequest({
          request: fakeRequest(refusal.method ?? "GET", refusal.url ?? attestationUrl()),
          response,
        }),
      ).resolves.toBeUndefined();

      expect(heads, `${refusal.name} wrote a response head`).toEqual([]);
      expect(bodies, `${refusal.name} wrote a response body`).toEqual([]);
      expect(query.calls.length > 0, `${refusal.name} query usage`).toBe(refusal.queryExpected);
    }
  }, 20_000);

  test("e2e stack refuses to boot on an exposed host without the opt-in", () => {
    const optIn = vector.contract.tunnelOptIn;
    const flag = vector.contract.flag;

    const boot = (env: Record<string, string | undefined>) => () =>
      createE2EStackAttestation({
        env,
        query: async () => ({ database: LIVE_DATABASE, seen: true }),
        now: () => 0,
        log: () => {},
        randomBytes: (size: number) => new Uint8Array(size),
      }).assertBootAllowed();

    expect(boot({ [flag.name]: flag.enabledValue, BRIDGE_HOST_EXPOSURE: "exposed" })).toThrow(
      new RegExp(optIn.name),
    );
    expect(
      boot({ [flag.name]: flag.enabledValue, BRIDGE_HOST_EXPOSURE: "  EXPOSED " }),
    ).toThrow(new RegExp(optIn.name));
    // The opt-in must be exactly "true".
    for (const wrong of ["TRUE", "1", "yes", ""]) {
      expect(
        boot({
          [flag.name]: flag.enabledValue,
          BRIDGE_HOST_EXPOSURE: "exposed",
          [optIn.name]: wrong,
        }),
      ).toThrow(new RegExp(optIn.name));
    }

    expect(
      boot({
        [flag.name]: flag.enabledValue,
        BRIDGE_HOST_EXPOSURE: "exposed",
        [optIn.name]: optIn.enabledValue,
      }),
    ).not.toThrow();
    // Not exposed, or flag off: the opt-in is irrelevant.
    expect(boot({ [flag.name]: flag.enabledValue })).not.toThrow();
    expect(boot({ BRIDGE_HOST_EXPOSURE: "exposed" })).not.toThrow();
  });

  test("e2e stack refusal logging is rate limited and omits the URL", async () => {
    const { attestation, logs, clock } = harness({
      query: async () => ({ database: LIVE_DATABASE, seen: false }),
    });

    const refuse = async (url = attestationUrl()) => {
      const { response } = fakeResponse();
      await attestation.onRequest({ request: fakeRequest("GET", url), response });
    };

    await refuse();
    expect(logs).toHaveLength(1);

    clock.value = 9_999;
    await refuse();
    await refuse();
    expect(logs).toHaveLength(1);

    clock.value = 10_000;
    await refuse();
    expect(logs).toHaveLength(2);

    // A different reason has its own window, so rate limiting cannot hide one
    // reason behind another.
    clock.value = 10_001;
    await refuse(`${HOOK_PATH}?${NONCE_PARAM}=bogus`);
    expect(logs).toHaveLength(3);

    const text = logs.join("\n");
    expect(text).not.toContain(TEST_DATABASE_URL);
    expect(text).not.toContain("bridge:pw");
    expect(text).not.toContain(NONCE);
    expect(text).not.toContain(String(deriveE2EStackObjid(NONCE)));
    expect(text).not.toContain(composeKey(deriveE2EStackObjid(NONCE)));
  });

  test("e2e stack attestation is no-store", async () => {
    const { attestation } = harness();
    const { response, heads } = fakeResponse();

    await expect(
      attestation.onRequest({ request: fakeRequest("GET", attestationUrl()), response }),
    ).rejects.toBeUndefined();

    expect(heads[0].headers).toEqual(vector.contract.successHeaders);
    expect(heads[0].headers["Cache-Control"]).toBe("no-store");
  });

  test("e2e stack instance id survives module re-evaluation", async () => {
    // The id is cached on globalThis, which is the ONLY state that survives a
    // module re-evaluation (a watch restart re-running the module body, or a
    // second import of a re-resolved specifier). Two independent factory
    // instances — each with its own closure state and its own randomBytes —
    // must therefore agree.
    const first = harness({ randomBytes: (size: number) => new Uint8Array(size).fill(0x11) });
    const second = harness({ randomBytes: (size: number) => new Uint8Array(size).fill(0x22) });

    const read = async (h: Harness) => {
      const { response, bodies } = fakeResponse();
      await expect(
        h.attestation.onRequest({ request: fakeRequest("GET", attestationUrl()), response }),
      ).rejects.toBeUndefined();
      return (JSON.parse(bodies[0]) as { instance: string }).instance;
    };

    const firstId = await read(first);
    const secondId = await read(second);

    expect(secondId).toBe(firstId);
    expect(firstId).toContain(`-${process.pid}-`);
    expect(firstId).toMatch(/^hocuspocus-\d+-[0-9a-f]{32}$/);
    expect((globalThis as InstanceGlobal)[INSTANCE_GLOBAL_KEY]).toBe(firstId);
    // Not derived from randomBytes of the second factory.
    expect(firstId).toContain("11".repeat(16));

    // A module evaluated afresh finds the cached id and reuses it verbatim.
    delete (globalThis as InstanceGlobal)[INSTANCE_GLOBAL_KEY];
    (globalThis as InstanceGlobal)[INSTANCE_GLOBAL_KEY] = "hocuspocus-preexisting-id";
    const third = harness({ randomBytes: (size: number) => new Uint8Array(size).fill(0x33) });
    expect(await read(third)).toBe("hocuspocus-preexisting-id");
  });

  test("e2e stack reads the same DATABASE_URL variable as server/db", () => {
    expect(E2E_STACK_DATABASE_URL_ENV).toBe("DATABASE_URL");
    // server/db.ts builds the pool this attestation speaks for. If it ever
    // reads a different variable, the attestation would prove the wrong pool.
    expect(serverDbSource).toContain(`process.env.${E2E_STACK_DATABASE_URL_ENV}`);
    // The attestation must read that variable by the exported name, not a
    // hardcoded literal that could drift from it.
    expect(e2eStackSource).toContain("env[E2E_STACK_DATABASE_URL_ENV]");

    // And behaviourally: the attestation refuses when THAT variable is not a
    // _test URL, even though another variable holds one.
    const { attestation, query } = harness({
      env: {
        [vector.contract.flag.name]: vector.contract.flag.enabledValue,
        [E2E_STACK_DATABASE_URL_ENV]: "postgres://bridge@127.0.0.1:5432/bridge",
        TEST_DATABASE_URL,
      },
    });
    const { response, heads } = fakeResponse();
    return attestation
      .onRequest({ request: fakeRequest("GET", attestationUrl()), response })
      .then(() => {
        expect(heads).toEqual([]);
        expect(query.calls).toEqual([]);
      });
  });

  test("e2e stack key derivation and fingerprint match the shared vector", () => {
    expect(E2E_STACK_LOCK_CLASS).toBe(vector.contract.lockClass);
    expect(`0x${E2E_STACK_LOCK_CLASS.toString(16)}`).toBe(vector.contract.lockClassHex);
    expect(E2E_STACK_NONCE_PATTERN.source).toBe(vector.contract.noncePattern);

    for (const testCase of vector.cases) {
      expect(deriveE2EStackObjid(testCase.nonce)).toBe(testCase.objid);
      expect(composeKey(testCase.objid)).toBe(testCase.key);
      // The non-ASCII database name pins the UTF-8 encoding of the second field.
      expect(e2eStackFingerprint(testCase.nonce, testCase.database)).toBe(testCase.fingerprint);
    }

    for (const boundary of vector.composeKeyBoundaries) {
      expect(composeKey(boundary.objid)).toBe(boundary.key);
      expect(BigInt(boundary.key) > 0n).toBe(true);
    }

    // Structurally disjoint from Bridge's other advisory-lock namespaces.
    expect(E2E_STACK_LOCK_CLASS).not.toBe(vector.disjointFrom.sessionLifecycleLockClass);
    expect(E2E_STACK_LOCK_CLASS).not.toBe(vector.disjointFrom.classReplacementLockClass);
    for (const highWord of vector.disjointFrom.hashtextOneKeyHighWords) {
      expect(E2E_STACK_LOCK_CLASS).not.toBe(highWord);
    }

    // A malformed nonce can never reach the key derivation.
    for (const bad of ["", "abc", OTHER_NONCE.toUpperCase(), `${NONCE}0`, NONCE.slice(0, 63)]) {
      expect(() => deriveE2EStackObjid(bad)).toThrow();
      expect(() => e2eStackFingerprint(bad, LIVE_DATABASE)).toThrow();
    }

    // The observe query the default wiring uses must match the shared shape:
    // one-key advisory lock, objsubid = 1, granted, in the CURRENT database.
    const observed = e2eStackSource.replace(/\s+/g, " ");
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
      expect(observed, `observe query missing ${fragment}`).toContain(fragment);
    }
  });

  // -------------------------------------------------------------------------
  // R2-21: the hook registers only when boot would be allowed
  // -------------------------------------------------------------------------

  test("e2e stack hook is not registered on an exposed host without the opt-in", () => {
    const flag = vector.contract.flag;
    const optIn = vector.contract.tunnelOptIn;
    const exposure = vector.contract.hostExposure;

    const factory = (env: Record<string, string | undefined>) =>
      createE2EStackAttestation({
        env,
        query: async () => ({ database: LIVE_DATABASE, seen: true }),
        now: () => 0,
        log: () => {},
        randomBytes: (size: number) => new Uint8Array(size),
      });

    // Enabled but NOT registrable: the flag is on, so the process knows the
    // surface exists, yet the hook must not attach.
    const exposed = factory({
      [flag.name]: flag.enabledValue,
      [exposure.name]: exposure.exposedValue,
    });
    expect(exposed.enabled).toBe(true);
    expect(exposed.registrable).toBe(false);

    // Every exposed spelling from the shared vector behaves the same way.
    for (const row of vector.hostExposureCases) {
      const attestation = factory({
        [flag.name]: flag.enabledValue,
        [exposure.name]: row.value,
      });
      expect(attestation.enabled, `${JSON.stringify(row.value)} enabled`).toBe(true);
      expect(attestation.registrable, `${JSON.stringify(row.value)} registrable`).toBe(!row.exposed);
    }

    // The opt-in is exact: only "true" re-enables registration.
    for (const wrong of [undefined, "", "TRUE", "1", "yes"]) {
      expect(
        factory({
          [flag.name]: flag.enabledValue,
          [exposure.name]: exposure.exposedValue,
          [optIn.name]: wrong,
        }).registrable,
      ).toBe(false);
    }
    expect(
      factory({
        [flag.name]: flag.enabledValue,
        [exposure.name]: exposure.exposedValue,
        [optIn.name]: optIn.enabledValue,
      }).registrable,
    ).toBe(true);

    // Flag off: nothing is registrable regardless of exposure.
    expect(factory({ [exposure.name]: exposure.exposedValue }).registrable).toBe(false);

    // And in a real process: importing server/hocuspocus.ts (which is NOT
    // `import.meta.main`, so the boot refusal never runs) must not hand the
    // importer an onRequest hook on an exposed host without the opt-in.
    expect(
      hookRegisteredInChild(flag.enabledValue, {
        [exposure.name]: exposure.exposedValue,
      }),
    ).toBe(false);
    expect(
      hookRegisteredInChild(flag.enabledValue, {
        [exposure.name]: exposure.exposedValue,
        [optIn.name]: optIn.enabledValue,
      }),
    ).toBe(true);
  });

  // -------------------------------------------------------------------------
  // R2-15: exposure normalization is the shared vector's, exactly
  // -------------------------------------------------------------------------

  test("e2e stack exposure check matches the shared vector", () => {
    const flag = vector.contract.flag;
    const optIn = vector.contract.tunnelOptIn;
    const exposure = vector.contract.hostExposure;
    expect(exposure.name).toBe("BRIDGE_HOST_EXPOSURE");
    expect(exposure.exposedValue).toBe("exposed");
    expect(vector.hostExposureCases.length).toBeGreaterThan(0);

    const boot = (env: Record<string, string | undefined>) => () =>
      createE2EStackAttestation({
        env,
        query: async () => ({ database: LIVE_DATABASE, seen: true }),
        now: () => 0,
        log: () => {},
        randomBytes: (size: number) => new Uint8Array(size),
      }).assertBootAllowed();

    for (const row of vector.hostExposureCases) {
      const env = { [flag.name]: flag.enabledValue, [exposure.name]: row.value };
      const label = JSON.stringify(row.value);
      if (row.exposed) {
        expect(boot(env), `${label} must refuse to boot`).toThrow(new RegExp(optIn.name));
      } else {
        expect(boot(env), `${label} must boot`).not.toThrow();
      }
      // The recorded opt-in allows every value; the flag being off dormant-ises
      // every value.
      expect(boot({ ...env, [optIn.name]: optIn.enabledValue })).not.toThrow();
      expect(boot({ [exposure.name]: row.value })).not.toThrow();
    }

    // An absent variable is not exposure.
    expect(boot({ [flag.name]: flag.enabledValue })).not.toThrow();
  });

  // -------------------------------------------------------------------------
  // R2-16: the observe query is capped per process
  // -------------------------------------------------------------------------

  test("e2e stack refuses when the observe cap is reached and runs no query", async () => {
    // Each call parks until the test resolves it, so the cap is reached with a
    // query genuinely in flight rather than by racing the event loop.
    const pending: ((result: QueryResult) => void)[] = [];
    const query: QueryFn = () =>
      new Promise<QueryResult>((resolve) => {
        pending.push(resolve);
      });
    const { attestation, query: observed, logs } = harness({
      query,
      maxInFlightObserveQueries: 1,
    });

    const first = fakeResponse();
    const firstCall = attestation.onRequest({
      request: fakeRequest("GET", attestationUrl()),
      response: first.response,
    });
    expect(observed.calls).toHaveLength(1);

    // The only slot is occupied: refuse, write nothing, and run NO query.
    const busy = fakeResponse();
    await expect(
      attestation.onRequest({ request: fakeRequest("GET", attestationUrl()), response: busy.response }),
    ).resolves.toBeUndefined();
    expect(observed.calls).toHaveLength(1);
    expect(busy.heads).toEqual([]);
    expect(busy.bodies).toEqual([]);
    expect(logs.filter((line) => line.includes(vector.contract.observeConcurrency.refusalReason))).toHaveLength(1);

    // Refusal logging is rate limited, so a flood cannot fill the log.
    const flooded = fakeResponse();
    await attestation.onRequest({
      request: fakeRequest("GET", attestationUrl()),
      response: flooded.response,
    });
    expect(observed.calls).toHaveLength(1);
    expect(logs.filter((line) => line.includes(vector.contract.observeConcurrency.refusalReason))).toHaveLength(1);

    // Releasing the holder completes it and frees the slot.
    pending[0]({ database: LIVE_DATABASE, seen: true });
    await expect(firstCall).rejects.toBeUndefined();
    expect(first.heads[0].status).toBe(200);

    const third = fakeResponse();
    const thirdCall = attestation.onRequest({
      request: fakeRequest("GET", attestationUrl()),
      response: third.response,
    });
    expect(observed.calls).toHaveLength(2);
    pending[1]({ database: LIVE_DATABASE, seen: true });
    await expect(thirdCall).rejects.toBeUndefined();
    expect(third.heads[0].status).toBe(200);
  });

  // -------------------------------------------------------------------------
  // R2-25: the slot follows the QUERY, not the request timeout
  // -------------------------------------------------------------------------

  test("e2e stack holds its observe slot until the query settles, not until the request times out", async () => {
    // A controllable query: every call parks and hands the test its settlers,
    // so the slot's lifetime can be observed independently of the 2s request
    // timeout. The cap is 1 throughout, so "the slot" is unambiguous.
    interface Settler {
      resolve: (result: QueryResult) => void;
      reject: (error: unknown) => void;
    }
    const controllable = () => {
      const settlers: Settler[] = [];
      const query: QueryFn = () =>
        new Promise<QueryResult>((resolve, reject) => {
          settlers.push({ resolve, reject });
        });
      return { settlers, query };
    };
    const ok: QueryResult = { database: LIVE_DATABASE, seen: true };
    // The release handler runs in a microtask on the settled query; a macrotask
    // turn is strictly more than enough for it to have run.
    const settle = () => new Promise<void>((resolve) => setTimeout(resolve, 0));

    // R2-25 also required that the abandoned query's LATE rejection stays
    // handled: nothing may surface as an unhandled rejection after the request
    // has already returned.
    const unhandled: unknown[] = [];
    const onUnhandledRejection = (reason: unknown) => {
      unhandled.push(reason);
    };
    process.on("unhandledRejection", onUnhandledRejection);

    try {
      // 1. A query that SUCCEEDS frees the slot.
      {
        const { settlers, query } = controllable();
        const { attestation, query: observed } = harness({ maxInFlightObserveQueries: 1, query });
        const first = fakeResponse();
        const firstCall = attestation.onRequest({ request: fakeRequest("GET", attestationUrl()), response: first.response });
        expect(observed.calls).toHaveLength(1);
        settlers[0].resolve(ok);
        await expect(firstCall).rejects.toBeUndefined();
        expect(first.heads[0].status).toBe(200);

        const next = fakeResponse();
        const nextCall = attestation.onRequest({ request: fakeRequest("GET", attestationUrl()), response: next.response });
        expect(observed.calls).toHaveLength(2);
        settlers[1].resolve(ok);
        await expect(nextCall).rejects.toBeUndefined();
        expect(next.heads[0].status).toBe(200);
      }

      // 2. A query that REJECTS frees the slot.
      {
        const { settlers, query } = controllable();
        const { attestation, query: observed, logs } = harness({ maxInFlightObserveQueries: 1, query });
        const failed = fakeResponse();
        const failedCall = attestation.onRequest({ request: fakeRequest("GET", attestationUrl()), response: failed.response });
        expect(observed.calls).toHaveLength(1);
        settlers[0].reject(new Error("connection terminated"));
        await expect(failedCall).resolves.toBeUndefined();
        expect(failed.heads).toEqual([]);
        expect(failed.bodies).toEqual([]);
        expect(logs.join("\n")).toContain("query_error");

        const next = fakeResponse();
        const nextCall = attestation.onRequest({ request: fakeRequest("GET", attestationUrl()), response: next.response });
        expect(observed.calls).toHaveLength(2);
        settlers[1].resolve(ok);
        await expect(nextCall).rejects.toBeUndefined();
        expect(next.heads[0].status).toBe(200);
      }

      // 3. The regression itself, in both late outcomes: the request times out
      //    while its query is STILL RUNNING. The request refuses and writes
      //    nothing, but the database work it started has not stopped, so the
      //    slot must stay occupied until that query itself settles. Releasing
      //    on the timeout instead let a stalled database accumulate running
      //    queries without limit.
      for (const late of ["resolve", "reject"] as const) {
        const { settlers, query } = controllable();
        const { attestation, query: observed, logs } = harness({ maxInFlightObserveQueries: 1, query });

        const timedOut = fakeResponse();
        await expect(
          attestation.onRequest({ request: fakeRequest("GET", attestationUrl()), response: timedOut.response }),
        ).resolves.toBeUndefined();
        expect(timedOut.heads, late).toEqual([]);
        expect(timedOut.bodies, late).toEqual([]);
        expect(logs.join("\n"), late).toContain("query_timeout");
        expect(observed.calls, late).toHaveLength(1);

        // The abandoned query is still in flight: the next request is refused
        // `observe_busy` and runs NO query of its own.
        const busy = fakeResponse();
        await expect(
          attestation.onRequest({ request: fakeRequest("GET", attestationUrl()), response: busy.response }),
        ).resolves.toBeUndefined();
        expect(observed.calls, late).toHaveLength(1);
        expect(busy.heads, late).toEqual([]);
        expect(busy.bodies, late).toEqual([]);
        expect(logs.join("\n"), late).toContain(vector.contract.observeConcurrency.refusalReason);

        // Now the abandoned query settles — however it settles — and THAT is
        // what frees the slot.
        if (late === "resolve") settlers[0].resolve(ok);
        else settlers[0].reject(new Error("connection terminated after the request gave up"));
        await settle();

        const after = fakeResponse();
        const afterCall = attestation.onRequest({ request: fakeRequest("GET", attestationUrl()), response: after.response });
        expect(observed.calls, late).toHaveLength(2);

        // Exactly ONE slot came back: a second concurrent request is still
        // refused, so the release cannot have run twice for one acquisition.
        const stillBusy = fakeResponse();
        await expect(
          attestation.onRequest({ request: fakeRequest("GET", attestationUrl()), response: stillBusy.response }),
        ).resolves.toBeUndefined();
        expect(observed.calls, late).toHaveLength(2);
        expect(stillBusy.heads, late).toEqual([]);

        settlers[1].resolve(ok);
        await expect(afterCall).rejects.toBeUndefined();
        expect(after.heads[0].status, late).toBe(200);
      }

      // The late rejection above was handled by the production code, not by a
      // race that had already lost.
      await settle();
      expect(unhandled).toEqual([]);
    } finally {
      process.off("unhandledRejection", onUnhandledRejection);
    }
  }, 30_000);

  test("e2e stack cannot exceed the cap however many requests time out", async () => {
    // With a database that never answers, every request times out. Before
    // R2-25 each timeout handed its slot back while its query kept running, so
    // `cap + 5` requests started `cap + 5` queries. Now the cap holds.
    const cap = vector.contract.observeConcurrency.maxInFlightPerProcess;
    const { attestation, query, logs } = harness({
      maxInFlightObserveQueries: cap,
      query: () => new Promise<QueryResult>(() => {}),
    });

    for (let i = 0; i < cap + 5; i += 1) {
      const response = fakeResponse();
      await expect(
        attestation.onRequest({ request: fakeRequest("GET", attestationUrl()), response: response.response }),
      ).resolves.toBeUndefined();
      expect(response.heads, `request ${i} wrote a response head`).toEqual([]);
      expect(response.bodies, `request ${i} wrote a response body`).toEqual([]);
      expect(query.calls.length, `after request ${i}`).toBeLessThanOrEqual(cap);
    }

    expect(query.calls).toHaveLength(cap);
    const text = logs.join("\n");
    expect(text).toContain("query_timeout");
    expect(text).toContain(vector.contract.observeConcurrency.refusalReason);
  }, 30_000);

  test("e2e stack default observe cap matches the shared vector", async () => {
    const limit = vector.contract.observeConcurrency.maxInFlightPerProcess;
    expect(limit).toBeGreaterThan(0);
    expect(E2E_STACK_MAX_IN_FLIGHT_OBSERVE_QUERIES).toBe(limit);
    expect(vector.contract.observeConcurrency.refusalReason).toBe("observe_busy");

    // Behaviourally, with NO injected cap: exactly `limit` queries fit.
    const pending: ((result: QueryResult) => void)[] = [];
    const { attestation, query, logs } = harness({
      query: () =>
        new Promise<QueryResult>((resolve) => {
          pending.push(resolve);
        }),
    });

    const held = [];
    for (let i = 0; i < limit; i += 1) {
      const response = fakeResponse();
      held.push({
        response,
        call: attestation.onRequest({ request: fakeRequest("GET", attestationUrl()), response: response.response }),
      });
    }
    expect(query.calls).toHaveLength(limit);

    const over = fakeResponse();
    await expect(
      attestation.onRequest({ request: fakeRequest("GET", attestationUrl()), response: over.response }),
    ).resolves.toBeUndefined();
    expect(query.calls).toHaveLength(limit);
    expect(over.heads).toEqual([]);
    expect(logs.join("\n")).toContain(vector.contract.observeConcurrency.refusalReason);

    for (let i = 0; i < limit; i += 1) {
      pending[i]({ database: LIVE_DATABASE, seen: true });
      await expect(held[i].call).rejects.toBeUndefined();
      expect(held[i].response.heads[0].status).toBe(200);
    }
  });
});

// ---------------------------------------------------------------------------
// Child-process probe for the flag-conditional hook registration.
// ---------------------------------------------------------------------------

function hookRegisteredInChild(
  flagValue: string,
  extraEnv: Record<string, string> = {},
): boolean {
  const script = `
    const mod = await import(${JSON.stringify(path.join(repoRoot, "server/hocuspocus.ts"))});
    const hooks = mod.hocuspocusHooks;
    console.log(JSON.stringify({ registered: Object.prototype.hasOwnProperty.call(hooks, "onRequest") }));
  `;
  const child = Bun.spawnSync({
    cmd: [process.execPath, "-e", script],
    cwd: repoRoot,
    env: {
      PATH: process.env.PATH ?? "",
      HOME: process.env.HOME ?? "",
      // An explicit value wins over anything .env would supply.
      [vector.contract.flag.name]: flagValue,
      // Parsed but never connected: the module body opens no connection.
      DATABASE_URL: TEST_DATABASE_URL,
      HOCUSPOCUS_TOKEN_SECRET: "e2e-stack-test-secret",
      HOCUSPOCUS_PORT: "4199",
      HOCUSPOCUS_CONTROL_PORT: "4198",
      HOCUSPOCUS_CONTROL_SECRET: "e2e-stack-test-control-secret",
      BRIDGE_HOST_EXPOSURE: "",
      ALLOW_E2E_STACK_OVER_TUNNEL: "",
      ...extraEnv,
    },
    stdout: "pipe",
    stderr: "pipe",
  });

  const stdout = child.stdout.toString().trim();
  const stderr = child.stderr.toString().trim();
  expect(child.exitCode, `child failed: ${stderr}`).toBe(0);
  const line = stdout.split("\n").at(-1) ?? "";
  return (JSON.parse(line) as { registered: boolean }).registered;
}

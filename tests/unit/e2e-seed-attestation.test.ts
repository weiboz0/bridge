// @vitest-environment node

/**
 * Plan 094 Phase 14 — the E2E seed's time-of-check-to-time-of-use re-attestation.
 *
 * `e2e/seed.setup.ts` creates classes, enrolls users, and ends live sessions.
 * Before any of that it must re-run the gate's verifier with a FRESH nonce, and
 * it must refuse to run at all when the gate did not attest the stack. These
 * tests drive `assertAttestedE2EStack` — the exact function `seed.setup.ts`
 * calls as its first statement — with an injected verifier, so no stack, no
 * database, and no Playwright are involved.
 */

import { readFileSync } from "node:fs";
import path from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  assertAttestedE2EStack,
  type E2EStackResult,
  type EnvLike,
  type VerifyE2EStackArgs,
} from "../../e2e/helpers/e2e-stack";
import { INSTANCE_ENV, REMEDIATION } from "../../scripts/check-e2e-stack.mjs";

const repoRoot = path.resolve(__dirname, "../..");

const DATABASE_URL = "postgresql://bridge:hunter2@127.0.0.1:5432/bridge_test";
const BASE_URL = "http://127.0.0.1:3000";

const instanceEnv = INSTANCE_ENV as Record<string, string>;
const remediation = REMEDIATION as Record<string, string>;

function attestedEnv(overrides: EnvLike = {}): EnvLike {
  return {
    E2E_BASE_URL: BASE_URL,
    DATABASE_URL,
    [instanceEnv.go]: "go-instance-1",
    [instanceEnv.next]: "next-instance-1",
    [instanceEnv.hocuspocus]: "hocuspocus-instance-1",
    ...overrides,
  };
}

function okResult(): E2EStackResult {
  return {
    ok: true,
    baseUrl: BASE_URL,
    realtimeOrigin: "http://127.0.0.1:4000",
    instances: { next: "next-instance-1", go: "go-instance-1", hocuspocus: "hocuspocus-instance-1" },
    failures: [],
  };
}

function failedResult(failures: { service: string; class: string }[]): E2EStackResult {
  return { ok: false, baseUrl: BASE_URL, realtimeOrigin: null, instances: {}, failures };
}

function makeVerify() {
  return vi.fn<(args: VerifyE2EStackArgs) => Promise<E2EStackResult>>(async () => okResult());
}

describe("e2e seed stack attestation", () => {
  let verify: ReturnType<typeof makeVerify>;
  let mutate: ReturnType<typeof vi.fn<() => void>>;

  beforeEach(() => {
    verify = makeVerify();
    mutate = vi.fn<() => void>();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  /** Mirrors seed.setup.ts: attest first, only then touch the stack. */
  async function seedLikeFlow(env: EnvLike): Promise<void> {
    await assertAttestedE2EStack({ env, verify });
    mutate();
  }

  it("seed setup aborts before its first mutation when attestation fails", async () => {
    verify.mockResolvedValue(failedResult([{ service: "go", class: "not attested" }]));

    await expect(seedLikeFlow(attestedEnv())).rejects.toThrow(
      /refuses to write without an attested stack/,
    );

    expect(verify).toHaveBeenCalledTimes(1);
    expect(mutate).not.toHaveBeenCalled();
  });

  it("seed setup uses a fresh nonce", async () => {
    // The seed must never pass a nonce, and must never take one from the
    // environment: the verifier mints a fresh one per call, so a cached or
    // replayed answer to the GATE's nonce cannot satisfy this re-check.
    const env = attestedEnv({
      E2E_STACK_NONCE: "a".repeat(64),
      NONCE: "b".repeat(64),
      E2E_STACK_INSTANCE_NONCE: "c".repeat(64),
    });

    await seedLikeFlow(env);

    expect(verify).toHaveBeenCalledTimes(1);
    const args: object = verify.mock.calls[0][0];
    expect(Object.keys(args).sort()).toEqual(["baseUrl", "databaseUrl", "expectedInstances"]);
    expect("nonce" in args).toBe(false);
    expect(JSON.stringify(args)).not.toContain("a".repeat(64));
    expect(JSON.stringify(args)).not.toContain("b".repeat(64));
    expect(JSON.stringify(args)).not.toContain("c".repeat(64));

    // The helper itself must not read a nonce out of the environment.
    const helperSource = readFileSync(path.join(repoRoot, "e2e/helpers/e2e-stack.ts"), "utf8");
    expect(helperSource).not.toMatch(/\bnonce\s*[:=][^=]/i);
    expect(helperSource).not.toMatch(/env\s*\[[^\]]*nonce/i);
    expect(helperSource).not.toMatch(/env\.\w*nonce/i);
    expect(mutate).toHaveBeenCalledTimes(1);
  });

  it("seed setup fails closed when E2E_BASE_URL is missing", async () => {
    const env = attestedEnv();
    delete env.E2E_BASE_URL;

    await expect(seedLikeFlow(env)).rejects.toThrow(/missing E2E_BASE_URL/);
    await expect(seedLikeFlow(env)).rejects.toThrow(/scripts\/ci-local\.sh/);
    await expect(seedLikeFlow(env)).rejects.toThrow(/docs\/testing\.md/);
    expect(verify).not.toHaveBeenCalled();
    expect(mutate).not.toHaveBeenCalled();
  });

  it("seed setup fails closed when DATABASE_URL is missing", async () => {
    const env = attestedEnv();
    delete env.DATABASE_URL;

    await expect(seedLikeFlow(env)).rejects.toThrow(/missing DATABASE_URL/);
    expect(verify).not.toHaveBeenCalled();
    expect(mutate).not.toHaveBeenCalled();
  });

  it("seed setup fails closed when the gate did not export an instance id", async () => {
    // A missing instance variable means the gate never ran the attestation, so
    // there is nothing to re-check against: refuse rather than silently skip.
    for (const variable of Object.values(instanceEnv)) {
      const env = attestedEnv();
      delete env[variable];

      await expect(seedLikeFlow(env)).rejects.toThrow(
        /refuses to write without an attested stack/,
      );
      await expect(seedLikeFlow(env)).rejects.toThrow(new RegExp(variable));
      expect(verify).not.toHaveBeenCalled();
      expect(mutate).not.toHaveBeenCalled();
    }

    // An empty value is missing too.
    const blank = attestedEnv({ [instanceEnv.next]: "" });
    await expect(seedLikeFlow(blank)).rejects.toThrow(
      /refuses to write without an attested stack/,
    );
    expect(verify).not.toHaveBeenCalled();
  });

  // R2-23: the refusal used to list all three instance variables whenever one
  // was missing, sending the reader after two variables that were already set.
  it("seed setup names only the instance variables that are actually missing", async () => {
    const variables = Object.values(instanceEnv);
    for (const missing of variables) {
      for (const [label, blank] of [["absent", undefined], ["empty", ""]] as const) {
        const env = attestedEnv();
        if (blank === undefined) delete env[missing];
        else env[missing] = blank;

        const error = await seedLikeFlow(env).catch((thrown: unknown) => thrown as Error);
        expect(error, `${missing} ${label}`).toBeInstanceOf(Error);
        const message = (error as Error).message;

        expect(message, `${missing} ${label}`).toContain(missing);
        for (const present of variables.filter((name) => name !== missing)) {
          expect(message, `${missing} ${label} must not name ${present}`).not.toContain(present);
        }
        // Still fails closed, and still says exactly how to fix it.
        expect(message).toContain("refuses to write without an attested stack");
        expect(message).toContain("scripts/ci-local.sh");
        expect(message).toContain("docs/testing.md");
        expect(verify).not.toHaveBeenCalled();
        expect(mutate).not.toHaveBeenCalled();
      }
    }

    // Two missing: exactly those two are named, and not the third.
    const twoMissing = attestedEnv();
    delete twoMissing[instanceEnv.go];
    twoMissing[instanceEnv.next] = "";
    const twoError = await seedLikeFlow(twoMissing).catch((thrown: unknown) => thrown as Error);
    const twoMessage = (twoError as Error).message;
    expect(twoMessage).toContain(instanceEnv.go);
    expect(twoMessage).toContain(instanceEnv.next);
    expect(twoMessage).not.toContain(instanceEnv.hocuspocus);

    // All three missing: all three are named.
    const allMissing = attestedEnv();
    for (const variable of variables) delete allMissing[variable];
    const allError = await seedLikeFlow(allMissing).catch((thrown: unknown) => thrown as Error);
    for (const variable of variables) {
      expect((allError as Error).message).toContain(variable);
    }
    expect(verify).not.toHaveBeenCalled();
    expect(mutate).not.toHaveBeenCalled();
  });

  it("seed setup passes the gate's three instance ids as expectedInstances", async () => {
    await seedLikeFlow(attestedEnv());

    expect(verify).toHaveBeenCalledWith({
      baseUrl: BASE_URL,
      databaseUrl: DATABASE_URL,
      expectedInstances: {
        go: "go-instance-1",
        next: "next-instance-1",
        hocuspocus: "hocuspocus-instance-1",
      },
    });
    expect(mutate).toHaveBeenCalledTimes(1);
  });

  it("seed setup reports every failure class with its remediation and never the database URL", async () => {
    verify.mockResolvedValue(
      failedResult([
        { service: "next", class: "mismatch" },
        { service: "go", class: "multiple instances" },
        { service: "hocuspocus", class: "no realtime origin" },
      ]),
    );

    const error = await seedLikeFlow(attestedEnv()).catch((thrown: unknown) => thrown as Error);
    expect(error).toBeInstanceOf(Error);
    const message = (error as Error).message;

    for (const failureClass of ["mismatch", "multiple instances", "no realtime origin"]) {
      expect(message).toContain(failureClass);
      expect(message).toContain(remediation[failureClass]);
    }
    expect(message).toContain("next");
    expect(message).toContain("go");
    expect(message).toContain("hocuspocus");

    expect(message).not.toContain(DATABASE_URL);
    expect(message).not.toContain("hunter2");
    expect(message).not.toMatch(/postgres(ql)?:\/\//i);
    expect(mutate).not.toHaveBeenCalled();
  });

  it("seed setup refuses without leaking the database URL when the verifier throws", async () => {
    verify.mockRejectedValue(
      new Error(`connect ECONNREFUSED for ${DATABASE_URL}`),
    );

    const error = await seedLikeFlow(attestedEnv()).catch((thrown: unknown) => thrown as Error);
    expect((error as Error).message).toContain("refuses to write without an attested stack");
    expect((error as Error).message).not.toContain(DATABASE_URL);
    expect((error as Error).message).not.toContain("hunter2");
    expect(mutate).not.toHaveBeenCalled();
  });

  it("seed setup calls the attestation before any other step", () => {
    // Guards the ordering in the real fixture: the attestation must be the
    // first statement of the setup body, ahead of every request.
    const seedSource = readFileSync(path.join(repoRoot, "e2e/seed.setup.ts"), "utf8");
    const bodyStart = seedSource.indexOf('setup("seed fixture data"');
    expect(bodyStart).toBeGreaterThan(-1);

    const body = seedSource.slice(bodyStart);
    const attestIndex = body.indexOf("assertAttestedE2EStack(");
    expect(attestIndex).toBeGreaterThan(-1);

    for (const marker of ["page.request.", "loginWithCredentials(", "logout(", "writeFileSync("]) {
      const markerIndex = body.indexOf(marker);
      if (markerIndex === -1) continue;
      expect(attestIndex, `${marker} runs before the attestation`).toBeLessThan(markerIndex);
    }
  });
});

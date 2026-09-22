// @vitest-environment node

/**
 * Plan 094 Phase 14 — the E2E seed's time-of-check-to-time-of-use re-attestation.
 *
 * `e2e/seed.setup.ts` creates classes, enrolls users, and ends live sessions.
 * Before any of that it must re-run the gate's verifier with a FRESH nonce, and
 * it must refuse to run at all when the gate did not attest the stack.
 *
 * The verifier (`scripts/check-e2e-stack.mjs`) is now run as a SUBPROCESS, not
 * imported: importing that ESM/Node module into Playwright's TS graph fails with
 * "exports is not defined". `assertAttestedE2EStack` therefore takes an injected
 * `run` dependency, so these tests drive the fail-closed and no-leak logic with a
 * mocked verifier — no stack, no database, and no Playwright are involved.
 */

import { readFileSync } from "node:fs";
import path from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  assertAttestedE2EStack,
  type EnvLike,
  type RunVerifier,
  type VerifierRun,
} from "../../e2e/helpers/e2e-stack";

const repoRoot = path.resolve(__dirname, "../..");

// The gate exports these three per-process instance ids into the Playwright
// child; the helper duplicates the literals rather than importing the .mjs.
const INSTANCE_VARS = [
  "E2E_STACK_INSTANCE_GO",
  "E2E_STACK_INSTANCE_NEXT",
  "E2E_STACK_INSTANCE_HOCUSPOCUS",
] as const;
const REQUIRED_VARS = ["E2E_BASE_URL", "DATABASE_URL", ...INSTANCE_VARS] as const;

const DATABASE_URL = "postgresql://bridge:hunter2@127.0.0.1:5432/bridge_test";
const BASE_URL = "http://127.0.0.1:3000";

const GO_INSTANCE = "go-instance-1";
const NEXT_INSTANCE = "next-instance-1";
const HOCUSPOCUS_INSTANCE = "hocuspocus-instance-1";

const OK_STDOUT = [
  `E2E stack target: ${BASE_URL}`,
  "E2E realtime origin: http://127.0.0.1:4000",
].join("\n");

/** A fully attested environment; overrides let a test drop or blank a variable. */
function attestedEnv(overrides: EnvLike = {}): EnvLike {
  return {
    E2E_BASE_URL: BASE_URL,
    DATABASE_URL,
    E2E_STACK_INSTANCE_GO: GO_INSTANCE,
    E2E_STACK_INSTANCE_NEXT: NEXT_INSTANCE,
    E2E_STACK_INSTANCE_HOCUSPOCUS: HOCUSPOCUS_INSTANCE,
    ...overrides,
  };
}

function okRun(stdout: string = OK_STDOUT): VerifierRun {
  return { code: 0, stdout, stderr: "" };
}

describe("e2e seed stack attestation", () => {
  let run: ReturnType<typeof vi.fn<RunVerifier>>;
  let mutate: ReturnType<typeof vi.fn<() => void>>;

  beforeEach(() => {
    run = vi.fn<RunVerifier>(async () => okRun());
    mutate = vi.fn<() => void>();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  /** Mirrors seed.setup.ts: attest first, only then touch the stack. */
  async function seedLikeFlow(env: EnvLike): Promise<void> {
    await assertAttestedE2EStack({ env, run });
    mutate();
  }

  it("seed setup aborts before its first mutation when attestation fails", async () => {
    run.mockResolvedValue({ code: 1, stdout: "go service: not attested", stderr: "" });

    const error = await seedLikeFlow(attestedEnv()).catch((thrown: unknown) => thrown as Error);
    expect(error).toBeInstanceOf(Error);
    const message = (error as Error).message;

    // The seed's own `await` therefore rejects before mutate() ever runs.
    expect(message).toContain("refuses to write without an attested stack");
    expect(message).toContain("not attested"); // the verifier's failure detail
    expect(message).toContain("scripts/ci-local.sh");
    expect(message).toContain("docs/testing.md"); // remediation

    expect(run).toHaveBeenCalledTimes(1);
    expect(mutate).not.toHaveBeenCalled();
  });

  it("seed setup uses a fresh nonce", async () => {
    // The helper must never pass a nonce: the CLI mints a fresh one per call, so
    // a cached or replayed answer to the GATE's nonce cannot satisfy this check.
    await seedLikeFlow(attestedEnv());

    expect(run).toHaveBeenCalledTimes(1);
    const childEnv = run.mock.calls[0][1];
    const nonceKeys = Object.keys(childEnv).filter((key) => /nonce/i.test(key));
    expect(nonceKeys).toEqual([]);

    // The helper source must not read a nonce out of the environment either.
    const helperSource = readFileSync(path.join(repoRoot, "e2e/helpers/e2e-stack.ts"), "utf8");
    expect(helperSource).not.toMatch(/env\s*\[[^\]]*nonce/i);
    expect(helperSource).not.toMatch(/env\.\w*nonce/i);
    expect(helperSource).not.toMatch(/NONCE/); // no *_NONCE env-var literal

    expect(mutate).toHaveBeenCalledTimes(1);
  });

  it("seed setup fails closed when a required variable is missing", async () => {
    for (const missing of REQUIRED_VARS) {
      for (const [label, blank] of [["absent", undefined], ["empty", ""]] as const) {
        run.mockClear();
        mutate.mockClear();

        const env = attestedEnv();
        if (blank === undefined) delete env[missing];
        else env[missing] = blank;

        const error = await seedLikeFlow(env).catch((thrown: unknown) => thrown as Error);
        expect(error, `${missing} ${label}`).toBeInstanceOf(Error);
        const message = (error as Error).message;

        // Rejects, names ONLY this variable, and never runs the verifier.
        expect(message, `${missing} ${label}`).toContain("refuses to write without an attested stack");
        expect(message, `${missing} ${label}`).toContain(missing);
        for (const other of REQUIRED_VARS.filter((name) => name !== missing)) {
          expect(message, `${missing} ${label} must not name ${other}`).not.toContain(other);
        }
        expect(message).toContain("scripts/ci-local.sh");
        expect(message).toContain("docs/testing.md");
        expect(run, `${missing} ${label}`).not.toHaveBeenCalled();
        expect(mutate, `${missing} ${label}`).not.toHaveBeenCalled();
      }
    }
  });

  it("seed setup names only the instance variables that are actually missing", async () => {
    // Two missing (one absent, one blank): exactly those two are named.
    const twoMissing = attestedEnv();
    delete twoMissing.E2E_STACK_INSTANCE_GO;
    twoMissing.E2E_STACK_INSTANCE_NEXT = "";
    const twoError = await seedLikeFlow(twoMissing).catch((thrown: unknown) => thrown as Error);
    const twoMessage = (twoError as Error).message;
    expect(twoMessage).toContain("E2E_STACK_INSTANCE_GO");
    expect(twoMessage).toContain("E2E_STACK_INSTANCE_NEXT");
    expect(twoMessage).not.toContain("E2E_STACK_INSTANCE_HOCUSPOCUS");
    expect(run).not.toHaveBeenCalled();
    expect(mutate).not.toHaveBeenCalled();

    // All three missing: all three are named.
    const allMissing = attestedEnv();
    for (const variable of INSTANCE_VARS) delete allMissing[variable];
    const allError = await seedLikeFlow(allMissing).catch((thrown: unknown) => thrown as Error);
    for (const variable of INSTANCE_VARS) {
      expect((allError as Error).message).toContain(variable);
    }
    expect(run).not.toHaveBeenCalled();
    expect(mutate).not.toHaveBeenCalled();
  });

  it("seed setup passes the gate database and instance ids to the verifier and no nonce", async () => {
    await seedLikeFlow(attestedEnv());

    expect(run).toHaveBeenCalledTimes(1);
    const [scriptArg, childEnv] = run.mock.calls[0];

    // The verifier is spawned from the .mjs, given the gate database under its
    // own variable, and handed the three instance ids plus the base URL.
    expect(scriptArg).toMatch(/scripts\/check-e2e-stack\.mjs$/);
    expect(childEnv.CHECK_E2E_STACK_DATABASE_URL).toBe(DATABASE_URL);
    expect(childEnv.E2E_BASE_URL).toBe(BASE_URL);
    expect(childEnv.E2E_STACK_INSTANCE_GO).toBe(GO_INSTANCE);
    expect(childEnv.E2E_STACK_INSTANCE_NEXT).toBe(NEXT_INSTANCE);
    expect(childEnv.E2E_STACK_INSTANCE_HOCUSPOCUS).toBe(HOCUSPOCUS_INSTANCE);

    // Never a nonce — the CLI mints its own.
    expect(Object.keys(childEnv).filter((key) => /nonce/i.test(key))).toEqual([]);
    expect(mutate).toHaveBeenCalledTimes(1);
  });

  it("seed setup returns the parsed target and realtime origin on success", async () => {
    run.mockResolvedValue(
      okRun(
        [
          "some preamble",
          "E2E stack target: http://localhost:3101",
          "E2E realtime origin: http://localhost:3100/hocuspocus",
        ].join("\n"),
      ),
    );

    const result = await assertAttestedE2EStack({ env: attestedEnv(), run });
    expect(result).toEqual({
      ok: true,
      baseUrl: "http://localhost:3101",
      realtimeOrigin: "http://localhost:3100/hocuspocus",
    });
  });

  it("seed setup never leaks the database URL when the verifier errors", async () => {
    const leakyUrl = "postgresql://user:s3cr3tpw@dbhost:5432/bridge_test";

    // Case A: the verifier exits non-zero and prints the connection string.
    run.mockResolvedValue({
      code: 1,
      stdout: `go: not attested\nusing ${leakyUrl}`,
      stderr: "",
    });
    const exitError = await seedLikeFlow(attestedEnv()).catch((thrown: unknown) => thrown as Error);
    const exitMessage = (exitError as Error).message;
    expect(exitMessage).toContain("refuses to write without an attested stack");
    expect(exitMessage).toContain("not attested"); // non-URL detail survives the scrub
    expect(exitMessage).not.toContain(leakyUrl);
    expect(exitMessage).not.toContain("s3cr3tpw");
    expect(exitMessage).not.toMatch(/postgres(ql)?:\/\//i);
    expect(mutate).not.toHaveBeenCalled();

    // Case B: the verifier subprocess itself rejects with a URL-bearing error.
    run.mockReset();
    mutate.mockReset();
    run.mockRejectedValue(new Error(`connect ECONNREFUSED ${leakyUrl}`));
    const rejectError = await seedLikeFlow(attestedEnv()).catch((thrown: unknown) => thrown as Error);
    const rejectMessage = (rejectError as Error).message;
    expect(rejectMessage).toContain("refuses to write without an attested stack");
    expect(rejectMessage).not.toContain(leakyUrl);
    expect(rejectMessage).not.toContain("s3cr3tpw");
    expect(rejectMessage).not.toMatch(/postgres(ql)?:\/\//i);
    expect(mutate).not.toHaveBeenCalled();
  });

  it("seed setup calls the attestation before any other step", () => {
    // Guards the ordering in the real fixture: the attestation must run ahead of
    // every request, login, logout, and state write.
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

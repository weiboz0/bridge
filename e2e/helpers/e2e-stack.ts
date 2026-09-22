/**
 * Time-of-check to time-of-use re-attestation for the E2E seed — Plan 094 Phase 14.
 *
 * `scripts/ci-local.sh` verifies the running stack before it launches Playwright,
 * but `e2e/seed.setup.ts` then creates classes, enrolls users, and ends live
 * sessions. Between the gate's check and the seed's first write the stack could
 * have been restarted against another database, and an intermediary could replay
 * the gate's answer, so the seed re-runs the SAME verifier with a FRESH nonce and
 * its own lock before its first mutating request.
 *
 * The verifier (`scripts/check-e2e-stack.mjs`) is run as a SUBPROCESS, not
 * imported: it is an ESM module that uses Node built-ins, and Playwright's TS
 * transform mishandles importing a `.mjs` into its graph (`exports is not
 * defined` / require-of-ESM). Running it out of process is also how
 * `ci-local.sh` invokes it, so there is exactly one attestation code path.
 *
 * The CLI mints its own fresh nonce and, because the gate exported the
 * per-process instance ids into this child's environment, enforces that the same
 * three processes answer (see `main()` in the verifier). Everything observable
 * here (`env`, `run`) is injected so the fail-closed logic is unit-testable
 * without spawning anything.
 */

import { execFile } from "node:child_process";
import * as path from "node:path";

// Mirrors INSTANCE_ENV in scripts/check-e2e-stack.mjs. Duplicated as three
// literals rather than imported, because importing that module into Playwright's
// graph fails; the verifier's own tests pin these names.
const INSTANCE_VARS = [
  "E2E_STACK_INSTANCE_GO",
  "E2E_STACK_INSTANCE_NEXT",
  "E2E_STACK_INSTANCE_HOCUSPOCUS",
] as const;

export type EnvLike = Record<string, string | undefined>;

/** The outcome of running the verifier subprocess. */
export interface VerifierRun {
  code: number;
  stdout: string;
  stderr: string;
}

/** Runs the verifier CLI with the given environment; resolves with its exit code and output. */
export type RunVerifier = (scriptPath: string, env: EnvLike) => Promise<VerifierRun>;

export interface AssertAttestedE2EStackDeps {
  env?: EnvLike;
  run?: RunVerifier;
  scriptPath?: string;
}

/** What the seed logs on success. */
export interface E2EStackAttestation {
  ok: true;
  baseUrl: string;
  realtimeOrigin: string | null;
}

const REFUSAL_PREFIX = "e2e seed refuses to write without an attested stack";
const REMEDIATION =
  "Run the E2E tier through `bash scripts/ci-local.sh`, which verifies the running stack " +
  "against its validated _test database and exports these variables into the Playwright child. " +
  "See docs/testing.md.";

/** Postgres client errors can embed the whole connection string; never surface one that does. */
function scrub(text: string): string {
  return text
    .split("\n")
    .filter((line) => !/postgres(ql)?:\/\//i.test(line))
    .join("\n")
    .trim();
}

function parseLine(stdout: string, prefix: string): string | null {
  const line = stdout.split("\n").find((l) => l.startsWith(prefix));
  return line ? line.slice(prefix.length).trim() : null;
}

const defaultRun: RunVerifier = (scriptPath, env) =>
  new Promise((resolve) => {
    execFile(
      process.execPath,
      [scriptPath],
      { env: env as NodeJS.ProcessEnv, timeout: 60_000, maxBuffer: 1024 * 1024 },
      (error, stdout, stderr) => {
        const code = error && typeof error.code === "number" ? error.code : error ? 1 : 0;
        resolve({ code, stdout: stdout ?? "", stderr: stderr ?? "" });
      },
    );
  });

/**
 * Re-verifies the E2E stack with a fresh nonce (via the verifier subprocess) and
 * throws unless every service attests and matches the gate's instance ids. Fails
 * CLOSED: a missing `E2E_BASE_URL`, a missing `DATABASE_URL`, or missing
 * gate-exported instance ids all refuse rather than skip the check. The thrown
 * message carries the verifier's per-service failure lines and never the URL.
 */
export async function assertAttestedE2EStack({
  env = process.env,
  run = defaultRun,
  scriptPath = path.resolve(process.cwd(), "scripts/check-e2e-stack.mjs"),
}: AssertAttestedE2EStackDeps = {}): Promise<E2EStackAttestation> {
  const baseUrl = env.E2E_BASE_URL;
  const databaseUrl = env.DATABASE_URL;

  const missing: string[] = [];
  if (!baseUrl) missing.push("E2E_BASE_URL");
  if (!databaseUrl) missing.push("DATABASE_URL");
  // Name only the instance vars actually missing/empty (R2-23).
  for (const v of INSTANCE_VARS) if (!env[v]) missing.push(v);

  if (missing.length > 0) {
    throw new Error(`${REFUSAL_PREFIX}: missing ${missing.join(", ")}. ${REMEDIATION}`);
  }

  // The verifier reads its target database from CHECK_E2E_STACK_DATABASE_URL and
  // mints its own fresh nonce. The gate-exported E2E_STACK_INSTANCE_* pass
  // through so the CLI enforces the same three processes answered.
  const childEnv: EnvLike = { ...env, CHECK_E2E_STACK_DATABASE_URL: databaseUrl };

  let outcome: VerifierRun;
  try {
    outcome = await run(scriptPath, childEnv);
  } catch (error) {
    const cause = scrub(error instanceof Error ? error.message : String(error));
    throw new Error(`${REFUSAL_PREFIX}: the attestation could not run.${cause ? ` ${cause}` : ""} ${REMEDIATION}`);
  }

  if (outcome.code !== 0) {
    const detail = scrub(`${outcome.stdout}\n${outcome.stderr}`);
    throw new Error([`${REFUSAL_PREFIX}.`, detail, REMEDIATION].filter(Boolean).join("\n"));
  }

  return {
    ok: true,
    baseUrl: parseLine(outcome.stdout, "E2E stack target:") ?? (baseUrl as string),
    realtimeOrigin: parseLine(outcome.stdout, "E2E realtime origin:"),
  };
}

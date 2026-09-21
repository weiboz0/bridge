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
 * The logic lives here rather than in `seed.setup.ts` so it is unit-testable
 * without Playwright: everything observable (`env`, `verify`, `report`) is
 * injected. Importing this module — and the verifier it wraps — performs no work
 * and opens no connection.
 */

import {
  INSTANCE_ENV,
  expectedInstancesFromEnv as expectedInstancesFromEnvImpl,
  formatReport as formatReportImpl,
  verifyE2EStack as verifyE2EStackImpl,
} from "../../scripts/check-e2e-stack.mjs";

/** One per-service failure recorded by the verifier (`{service, class}`). */
export interface E2EStackFailure {
  service: string;
  class: string;
}

/** The verifier's result shape (`scripts/check-e2e-stack.mjs`). */
export interface E2EStackResult {
  ok: boolean;
  baseUrl: string;
  realtimeOrigin: string | null;
  instances: Record<string, string>;
  failures: E2EStackFailure[];
}

/**
 * The exact argument object the seed hands the verifier. `nonce` is deliberately
 * absent: the verifier must mint a fresh one per call, and the seed must never
 * inherit the gate's nonce through the environment.
 */
export interface VerifyE2EStackArgs {
  baseUrl: string;
  databaseUrl: string;
  expectedInstances: Record<string, string>;
}

export type VerifyE2EStack = (args: VerifyE2EStackArgs) => Promise<E2EStackResult>;

export type EnvLike = Record<string, string | undefined>;

export interface AssertAttestedE2EStackDeps {
  env?: EnvLike;
  verify?: VerifyE2EStack;
  report?: (result: E2EStackResult) => string[];
}

const verifyE2EStack = verifyE2EStackImpl as unknown as VerifyE2EStack;
const formatReport = formatReportImpl as unknown as (result: E2EStackResult) => string[];
const expectedInstancesFromEnv = expectedInstancesFromEnvImpl as unknown as (
  env: EnvLike,
) => Record<string, string> | undefined;

const INSTANCE_VARS = [INSTANCE_ENV.go, INSTANCE_ENV.next, INSTANCE_ENV.hocuspocus] as string[];

const REFUSAL_PREFIX =
  "e2e seed refuses to write without an attested stack";
const REMEDIATION =
  "Run the E2E tier through `bash scripts/ci-local.sh`, which verifies the running stack " +
  "against its validated _test database and exports these variables into the Playwright child. " +
  "See docs/testing.md.";

/** Postgres client errors can embed the whole connection string; never re-throw one that does. */
function safeCauseLine(error: unknown): string {
  if (!(error instanceof Error)) return "";
  if (/postgres(ql)?:\/\//i.test(error.message)) return "";
  return error.message;
}

/**
 * Re-verifies the E2E stack with a fresh nonce and throws unless every service
 * attested. Fails CLOSED: a missing `E2E_BASE_URL`, a missing `DATABASE_URL`, or
 * missing gate-exported instance ids all refuse rather than skip the check.
 *
 * The thrown message carries the per-service failure class and its remediation
 * line, and never the database URL.
 */
export async function assertAttestedE2EStack({
  env = process.env,
  verify = verifyE2EStack,
  report = formatReport,
}: AssertAttestedE2EStackDeps = {}): Promise<E2EStackResult> {
  const baseUrl = env.E2E_BASE_URL;
  const databaseUrl = env.DATABASE_URL;
  const expectedInstances = expectedInstancesFromEnv(env);

  const missing: string[] = [];
  if (!baseUrl) missing.push("E2E_BASE_URL");
  if (!databaseUrl) missing.push("DATABASE_URL");
  if (!expectedInstances) missing.push(INSTANCE_VARS.join(", "));

  if (!baseUrl || !databaseUrl || !expectedInstances) {
    throw new Error(
      `${REFUSAL_PREFIX}: missing ${missing.join(", ")}. ${REMEDIATION}`,
    );
  }

  let result: E2EStackResult;
  try {
    // No `nonce` key: the verifier mints a fresh one, so a cached or replayed
    // gate answer cannot satisfy this re-check.
    result = await verify({ baseUrl, databaseUrl, expectedInstances });
  } catch (error) {
    const cause = safeCauseLine(error);
    throw new Error(
      `${REFUSAL_PREFIX}: the attestation could not run.${cause ? ` ${cause}` : ""} ${REMEDIATION}`,
    );
  }

  if (!result.ok) {
    throw new Error([`${REFUSAL_PREFIX}.`, ...report(result), REMEDIATION].join("\n"));
  }

  return result;
}

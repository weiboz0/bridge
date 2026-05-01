/**
 * sandbox-runner.ts — TypeScript wrapper around `platform/cmd/run-piston`.
 *
 * Plan 049 phase 2: the importer pre-flights every problem's reference
 * solution against every test case before opening the DB transaction.
 * This module spawns the Go CLI and parses its JSON response. The
 * Piston URL is configurable via `PISTON_URL`; the CLI's default
 * matches `sandbox.NewPistonClient`'s default.
 *
 * The CLI is invoked via `go run ./cmd/run-piston` rather than a
 * pre-built binary, so the importer doesn't need a separate build
 * step. For larger imports this is fine — the Go toolchain caches the
 * compiled artifact, so successive runs reuse it.
 */

import { spawn } from "node:child_process";
import { join } from "node:path";

export interface SandboxRequest {
  language: string;
  source: string;
  stdin: string;
}

export interface SandboxResult {
  stdout: string;
  stderr: string;
  exitCode: number;
  signal?: string;
  error?: string;
  timeoutMs: number;
}

export interface NormalizedComparison {
  matched: boolean;
  expectedNormalized: string;
  actualNormalized: string;
}

const REPO_ROOT = (() => {
  // The runner is invoked from arbitrary cwd (vitest, the importer
  // CLI, manual smoke). Walk up from this file's directory to find
  // the platform/ sibling. Use import.meta.dirname when available
  // (Bun + Node 22+); fall back to process.cwd() heuristics.
  const here =
    typeof import.meta !== "undefined" && (import.meta as { dirname?: string }).dirname
      ? (import.meta as { dirname: string }).dirname
      : process.cwd();
  // here is .../scripts/python-101 — repo root is two up.
  return join(here, "..", "..");
})();

const PLATFORM_DIR = join(REPO_ROOT, "platform");

/**
 * Match the Go grader's normalization (see
 * `platform/internal/handlers/attempt_run_test_handler.go:222-260`):
 *   • strip trailing whitespace from each line
 *   • strip a single trailing newline
 *
 * Authors don't write trailing newlines in `expectedStdout` so this
 * mostly handles Python's `print()` adding one. Keeping the rule
 * symmetric for both expected and actual prevents false negatives.
 */
export function normalizeOutput(s: string): string {
  return s
    .split(/\r?\n/)
    .map((line) => line.replace(/[ \t]+$/u, ""))
    .join("\n")
    .replace(/\n$/u, "");
}

export function compareOutputs(
  expected: string,
  actual: string,
): NormalizedComparison {
  const expectedNormalized = normalizeOutput(expected);
  const actualNormalized = normalizeOutput(actual);
  return {
    matched: expectedNormalized === actualNormalized,
    expectedNormalized,
    actualNormalized,
  };
}

export class SandboxError extends Error {
  readonly stage: "spawn" | "exit" | "decode" | "transport";
  readonly exitCode?: number;
  constructor(
    message: string,
    stage: "spawn" | "exit" | "decode" | "transport",
    exitCode?: number,
  ) {
    super(message);
    this.name = "SandboxError";
    this.stage = stage;
    this.exitCode = exitCode;
  }
}

/**
 * Runs one (source, stdin) pair through Piston via the Go CLI.
 *
 * Throws SandboxError when the CLI itself fails to run, decode, or
 * report a transport-level Piston error. A non-zero exit code from
 * the user's source code (e.g., a Python NameError) is NOT a sandbox
 * error — it lands in `result.exitCode` for the importer to inspect.
 */
export async function runInSandbox(
  req: SandboxRequest,
  opts: { goBin?: string; cwd?: string; timeoutMs?: number } = {},
): Promise<SandboxResult> {
  const goBin = opts.goBin ?? "go";
  const cwd = opts.cwd ?? PLATFORM_DIR;
  const timeoutMs = opts.timeoutMs ?? 90_000; // total wall-clock budget

  return await new Promise<SandboxResult>((resolve, reject) => {
    const child = spawn(goBin, ["run", "./cmd/run-piston/"], {
      cwd,
      stdio: ["pipe", "pipe", "pipe"],
      env: process.env,
    });

    let stdout = "";
    let stderr = "";
    let resolved = false;

    const finishOk = (r: SandboxResult) => {
      if (resolved) return;
      resolved = true;
      clearTimeout(killer);
      resolve(r);
    };
    const finishErr = (e: Error) => {
      if (resolved) return;
      resolved = true;
      clearTimeout(killer);
      reject(e);
    };

    const killer = setTimeout(() => {
      child.kill("SIGKILL");
      finishErr(
        new SandboxError(
          `run-piston wall-clock timeout after ${timeoutMs}ms`,
          "transport",
        ),
      );
    }, timeoutMs);

    child.stdout.on("data", (chunk: Buffer) => {
      stdout += chunk.toString("utf8");
    });
    child.stderr.on("data", (chunk: Buffer) => {
      stderr += chunk.toString("utf8");
    });

    child.on("error", (err: Error) => {
      finishErr(new SandboxError(`spawn ${goBin}: ${err.message}`, "spawn"));
    });

    child.on("close", (code) => {
      // The CLI emits JSON to stdout regardless of exit code (0, 1,
      // or 2). Parse first; surface stderr only if parsing fails.
      let parsed: SandboxResult | null = null;
      try {
        parsed = JSON.parse(stdout) as SandboxResult;
      } catch (e) {
        finishErr(
          new SandboxError(
            `decode CLI stdout (exit=${code}): ${(e as Error).message}; stderr=${stderr.slice(0, 400)}`,
            "decode",
            code ?? undefined,
          ),
        );
        return;
      }
      if (parsed.error) {
        finishErr(
          new SandboxError(parsed.error, "transport", code ?? undefined),
        );
        return;
      }
      if (code !== 0) {
        // Code 1 = invocation error, code 2 = transport error. Both
        // should have surfaced via parsed.error already. Defensive
        // catch for unexpected exit codes.
        finishErr(
          new SandboxError(
            `run-piston exited ${code} without parsed error; stderr=${stderr.slice(0, 400)}`,
            "exit",
            code ?? undefined,
          ),
        );
        return;
      }
      finishOk(parsed);
    });

    child.stdin.write(JSON.stringify(req));
    child.stdin.end();
  });
}

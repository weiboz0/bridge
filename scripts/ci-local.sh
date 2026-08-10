#!/usr/bin/env bash
# The authoritative local gate. Any future CI must run this same script, so the
# two cannot drift.
#
# Two safety properties this script exists to guarantee, both of which were found
# the hard way during plan 091's review:
#
#   1. It never bills API calls. tests/llm/*.test.ts hit real provider endpoints and
#      are gated only by the presence of an API key — and bun AUTO-LOADS .env, which
#      carries real keys. `unset ANTHROPIC_API_KEY` does not work; the key is re-read
#      from .env. Only --env-file=/dev/null actually suppresses them.
#
#   2. It never touches a real database or a foreign service. Migrations read
#      DATABASE_URL with no test-only path, and Playwright's baseURL defaults to a
#      port that hosts an unrelated service on the primary dev machine while its seed
#      fixture creates classes and enrolls users. Both fail closed here.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

FAST=0
[[ "${1:-}" == "--fast" ]] && FAST=1

# Machine-local, gitignored under .claude/*. See the Attestation block at the end.
ATTESTATION="$REPO_ROOT/.claude/ci-local-attestation.json"

FAILED=()
step() {
  local name="$1"; shift
  echo ""
  echo "══ $name"
  if "$@"; then
    echo "── $name OK"
  else
    echo "── $name FAILED" >&2
    FAILED+=("$name")
  fi
}

# ── Safety preconditions ─────────────────────────────────────────────────────

# Validate an inherited URL before it can become the selected gate URL.  The
# validator parses the pathname and then probes the live database name without
# printing credentials or the URL; it is the only database validation authority.
if [[ -n "${DATABASE_URL:-}" ]]; then
  if ! env CHECK_TEST_DATABASE_URL="$DATABASE_URL" node scripts/check-test-database-url.mjs; then
    echo "REFUSING TO RUN: inherited DATABASE_URL did not validate as a live test database." >&2
    exit 2
  fi
fi

GATE_DATABASE_URL="${TEST_DATABASE_URL:-${DATABASE_URL:-postgresql://work@127.0.0.1:5432/bridge_test}}"
if ! env CHECK_TEST_DATABASE_URL="$GATE_DATABASE_URL" node scripts/check-test-database-url.mjs; then
  echo "REFUSING TO RUN: resolved gate database URL did not validate as a live test database." >&2
  exit 2
fi

echo "Bridge local gate — repo $REPO_ROOT"
[[ $FAST -eq 1 ]] && echo "MODE: --fast (E2E skipped; NOT accepted by pre-merge-guard.sh)"

# ── Static ───────────────────────────────────────────────────────────────────

# Lint runs as a RATCHET, not a pass/fail on the raw count: main carries ~145
# pre-existing violations across ~83 files. The ratchet fails on anything new and
# tolerates what is recorded, so the gate is meaningful today instead of red until
# plan 093 finishes the cleanup. See scripts/check-lint-baseline.sh.
step "lint (ratchet)" bash scripts/check-lint-baseline.sh
step "type-check"     bunx tsc --noEmit

# ── Guards ───────────────────────────────────────────────────────────────────

step "guard: plans"      bash scripts/check-plan-uniqueness.sh
step "guard: specs"      bash scripts/check-spec-uniqueness.sh
step "guard: migrations" bash scripts/check-migration-uniqueness.sh
step "guard: decisions"  bash scripts/check-decisions-uniqueness.sh
step "guard: conflicts"  bash scripts/check-conflict-markers.sh
step "guard: self-test"  bash scripts/tests/test-guards.sh

# ── Tests ────────────────────────────────────────────────────────────────────

# Two independent hazards, two explicit overrides. Do NOT simplify this to
# `bun run test`, and do not drop either half.
#
#   DATABASE_URL — .env points at the DEVELOPMENT database, bun auto-loads .env,
#     and tests/helpers.ts truncates every table in afterEach. Pinning it to the
#     test DB is what keeps the suite off real data. (tests/helpers.ts now also
#     refuses a non-_test database outright, so this is belt and braces.)
#
#   *_API_KEY — tests/llm/*.test.ts call live provider endpoints, gated only by
#     `skipIf(!key)`. Exporting them EMPTY makes skipIf fire; `unset` does not
#     work, because bun re-reads the real values from .env.
step "vitest" env \
  DATABASE_URL="$GATE_DATABASE_URL" \
  TEST_DATABASE_URL="$GATE_DATABASE_URL" \
  ANTHROPIC_API_KEY= \
  OPENAI_API_KEY= \
  GEMINI_API_KEY= \
  DASHSCOPE_API_KEY= \
  OPENROUTER_API_KEY= \
  bun run test

step "go test" env \
  DATABASE_URL="$GATE_DATABASE_URL" \
  TEST_DATABASE_URL="$GATE_DATABASE_URL" \
  bash -c 'cd platform && go test ./... -count=1 -timeout 120s'

# ── E2E ──────────────────────────────────────────────────────────────────────

if [[ $FAST -eq 1 ]]; then
  echo ""
  echo "══ e2e SKIPPED (--fast)"
elif [[ -z "${E2E_BASE_URL:-}" ]]; then
  echo ""
  echo "REFUSING TO RUN E2E: E2E_BASE_URL is unset." >&2
  echo "  playwright.config.ts would fall back to http://localhost:3003, which on this" >&2
  echo "  machine is an unrelated service — and e2e/seed.setup.ts CREATES CLASSES and" >&2
  echo "  ENROLLS USERS against whatever answers." >&2
  echo "  Export E2E_BASE_URL pointing at your own stack, or use --fast." >&2
  FAILED+=("e2e (E2E_BASE_URL unset)")
else
  step "e2e" env \
    DATABASE_URL="$GATE_DATABASE_URL" \
    TEST_DATABASE_URL="$GATE_DATABASE_URL" \
    bun run test:e2e
fi

# ── Result ───────────────────────────────────────────────────────────────────

echo ""
if (( ${#FAILED[@]} )); then
  echo "GATE FAILED — ${#FAILED[@]} step(s):" >&2
  printf '  ✗ %s\n' "${FAILED[@]}" >&2
  # Remove any stale attestation: a previous pass must not vouch for this tree.
  rm -f "$ATTESTATION"
  exit 1
fi

# ── Attestation ──────────────────────────────────────────────────────────────
#
# Bridge runs no cloud CI, so this file IS the merge evidence. It records WHICH
# commit passed, because "the gate passed" is worthless without that — a run
# against different code proves nothing about what is being merged.
#
# br-autopilot refuses to auto-merge unless this names HEAD exactly and reports a
# full (non---fast) run. Machine-local by design: .claude/ is gitignored, so the
# attestation cannot be committed and then trusted on another machine.
mkdir -p "$(dirname "$ATTESTATION")"
cat > "$ATTESTATION" <<JSON
{
  "commit": "$(git rev-parse HEAD)",
  "branch": "$(git rev-parse --abbrev-ref HEAD)",
  "tree_dirty": $(if [[ -n "$(git status --porcelain)" ]]; then echo true; else echo false; fi),
  "fast": $(if (( FAST )); then echo true; else echo false; fi),
  "e2e": $(if (( FAST )) || [[ -z "${E2E_BASE_URL:-}" ]]; then echo false; else echo true; fi),
  "passed_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
JSON

echo "GATE PASSED"
echo "attestation: $ATTESTATION ($(git rev-parse --short HEAD))"
if (( FAST )); then
  echo "NOTE: --fast run. E2E did not run, so this is NOT sufficient for merge."
fi
exit 0

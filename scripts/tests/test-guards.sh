#!/usr/bin/env bash
# Executable tests for the guard scripts.
#
# A guard that has never been observed FAILING on a real collision is not a guard —
# it is a script that exits 0. Each case below builds a throwaway fixture tree and
# asserts the checker's exit status in both directions.
#
# The letter-suffix case is the important one: Bridge uses 025b / 030a-e / 079b
# deliberately, and a naive bare-NNN check flags every one of them.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$REPO_ROOT/scripts/lib/uniqueness.sh"

PASS=0; FAIL=0
FIXTURES="$(mktemp -d)"
trap 'rm -rf "$FIXTURES"' EXIT

ok()   { echo "  ✓ $1"; PASS=$((PASS+1)); }
bad()  { echo "  ✗ $1" >&2; FAIL=$((FAIL+1)); }

# expect <expected-rc> <description> <command...>
expect() {
  local want="$1" desc="$2"; shift 2
  local got=0
  "$@" >/dev/null 2>&1 || got=$?
  if [[ "$got" == "$want" ]]; then ok "$desc"; else bad "$desc (expected rc=$want, got rc=$got)"; fi
}

PLAN_RE='s/^([0-9]{3}[a-z]?).*\.md$/\1/p'
MIGR_RE='s/^([0-9]{4}).*\.sql$/\1/p'

echo "guard tests"

# ── 1. clean tree passes ─────────────────────────────────────────────────────
d="$FIXTURES/clean"; mkdir -p "$d"
touch "$d/001-alpha.md" "$d/002-beta.md" "$d/003-gamma.md"
expect 0 "clean numbering passes" check_uniqueness t "$d" '*.md' "$PLAN_RE"

# ── 2. real collision fails ──────────────────────────────────────────────────
d="$FIXTURES/collide"; mkdir -p "$d"
touch "$d/001-alpha.md" "$d/001-also-alpha.md"
expect 1 "duplicate NNN fails" check_uniqueness t "$d" '*.md' "$PLAN_RE"

# ── 3. letter suffixes are DISTINCT, not collisions ──────────────────────────
# The regression this whole file exists to prevent.
d="$FIXTURES/letters"; mkdir -p "$d"
touch "$d/030-base.md" "$d/030a-sub.md" "$d/030b-sub.md" "$d/030c-sub.md"
expect 0 "030 vs 030a/b/c are distinct tokens" check_uniqueness t "$d" '*.md' "$PLAN_RE"

# ── 4. two different letter-suffixed sub-plans DO collide with each other ────
d="$FIXTURES/letters-dup"; mkdir -p "$d"
touch "$d/030a-one.md" "$d/030a-two.md"
expect 1 "030a twice is still a collision" check_uniqueness t "$d" '*.md' "$PLAN_RE"

# ── 5. baseline suppresses a known collision, but only that one ──────────────
d="$FIXTURES/baseline"; mkdir -p "$d"
touch "$d/012-one.md" "$d/012-two.md"
bl="$FIXTURES/baseline.txt"; printf '# comment\n\n012\n' > "$bl"
expect 0 "baselined collision is accepted" check_uniqueness t "$d" '*.md' "$PLAN_RE" "$bl"
touch "$d/049-one.md" "$d/049-two.md"
expect 1 "NEW collision still fails despite baseline" check_uniqueness t "$d" '*.md' "$PLAN_RE" "$bl"

# ── 6. migrations use 4 digits with no letter suffix ─────────────────────────
d="$FIXTURES/migr"; mkdir -p "$d"
touch "$d/0001_a.sql" "$d/0002_b.sql"
expect 0 "clean migrations pass" check_uniqueness t "$d" '*.sql' "$MIGR_RE"
touch "$d/0002_c.sql"
expect 1 "duplicate migration prefix fails" check_uniqueness t "$d" '*.sql' "$MIGR_RE"

# ── 7. missing directory is a skip, not a crash ──────────────────────────────
expect 0 "absent directory skips cleanly" check_uniqueness t "$FIXTURES/nope" '*.md' "$PLAN_RE"

# ── 8. conflict markers ──────────────────────────────────────────────────────
d="$FIXTURES/conflict"; mkdir -p "$d"
{ printf '<<<<<<< HEAD\n'; printf 'a\n'; printf '=======\n'; printf 'b\n'; printf '>>>>>>> other\n'; } > "$d/f.txt"
if grep -qE '^(<{7} |={7}$|>{7} )' "$d/f.txt"; then ok "conflict-marker pattern matches real markers"; else bad "conflict-marker pattern missed real markers"; fi
printf 'x <<<<<<< inline, not a marker\n' > "$d/g.txt"
if grep -qE '^(<{7} |={7}$|>{7} )' "$d/g.txt"; then bad "conflict-marker pattern false-positives mid-line"; else ok "conflict-marker pattern ignores mid-line text"; fi

# ── 9. the real repo's guards pass, and their selftests trip ─────────────────
expect 0 "live plan guard passes"      bash "$REPO_ROOT/scripts/check-plan-uniqueness.sh"
expect 0 "live spec guard passes"      bash "$REPO_ROOT/scripts/check-spec-uniqueness.sh"
expect 0 "live migration guard passes" bash "$REPO_ROOT/scripts/check-migration-uniqueness.sh"
expect 0 "live decisions guard passes" bash "$REPO_ROOT/scripts/check-decisions-uniqueness.sh"
expect 0 "plan guard selftest trips"      bash "$REPO_ROOT/scripts/check-plan-uniqueness.sh" --selftest
expect 0 "migration guard selftest trips" bash "$REPO_ROOT/scripts/check-migration-uniqueness.sh" --selftest

# ── 10. pre-merge-guard rejects bad arguments rather than proceeding ─────────
expect 2 "pre-merge-guard rejects unknown args" bash "$REPO_ROOT/scripts/pre-merge-guard.sh" --bogus

# ── 11. lint ratchet ─────────────────────────────────────────────────────────
# The selftest writes a violating file and asserts the ratchet trips. It caught a
# real hole when first run: filtering to `git ls-files` alone ignored violations in
# NEW untracked files, which is precisely the case an agent writing fresh code hits.
expect 0 "lint ratchet passes on current tree" bash "$REPO_ROOT/scripts/check-lint-baseline.sh"
expect 0 "lint ratchet trips on a new violation" bash "$REPO_ROOT/scripts/check-lint-baseline.sh" --selftest

# ── 12. test database URL validator parser contracts ────────────────────────
# Parser selftests deliberately use the validator's dedicated input variable;
# they must never open a database connection.
VALIDATOR="$REPO_ROOT/scripts/check-test-database-url.mjs"
parse_test_url() {
  env CHECK_TEST_DATABASE_URL="$1" node "$VALIDATOR" --parse-only
}

expect 0 "test database path is accepted" \
  parse_test_url "postgresql://guard:guard@127.0.0.1:5432/bridge_test"
expect 0 "percent-decoded test database path is accepted" \
  parse_test_url "postgresql://guard:guard@127.0.0.1:5432/bridge%5Ftest"
expect 1 "empty database path is rejected" \
  parse_test_url "postgresql://guard:guard@127.0.0.1:5432/"
expect 1 "non-Postgres scheme is rejected" \
  parse_test_url "mysql://guard:guard@127.0.0.1:3306/bridge_test"
expect 1 "non-test database path is rejected" \
  parse_test_url "postgresql://guard:guard@127.0.0.1:5432/bridge"
expect 1 "query-suffix test-name decoy is rejected" \
  parse_test_url "postgresql://guard:guard@127.0.0.1:5432/bridge?dbname=bridge_test"
expect 0 "safe test pathname ignores production query override in parser mode" \
  parse_test_url "postgresql://guard:guard@127.0.0.1:5432/bridge_test?dbname=production"
expect 1 "comma-separated database hosts are rejected" \
  parse_test_url "postgresql://guard:guard@primary,replica:5432/bridge_test"
expect 1 "target session attributes are rejected" \
  parse_test_url "postgresql://guard:guard@127.0.0.1:5432/bridge_test?target_session_attrs=read-write"

# ── 13. database gate wiring is explicit and complete ───────────────────────
if rg -q 'check-test-database-url\.mjs' "$REPO_ROOT/scripts/ci-local.sh"; then
  ok "ci-local invokes the test database URL validator"
else
  bad "ci-local invokes the test database URL validator"
fi

if rg -q 'CHECK_TEST_DATABASE_URL="\$DATABASE_URL"' "$REPO_ROOT/scripts/ci-local.sh" \
  && rg -q 'GATE_DATABASE_URL=' "$REPO_ROOT/scripts/ci-local.sh"; then
  ok "ci-local validates ambient and resolved database URLs separately"
else
  bad "ci-local validates ambient and resolved database URLs separately"
fi

runner_pins_database_urls() {
  local runner="$1" active=0 database_url=0 test_database_url=0 line
  while IFS= read -r line; do
    if [[ "$line" == *step\ \"* ]]; then
      active=0
      [[ "$line" == *"$runner"* ]] && active=1
    fi
    if (( active )); then
      [[ "$line" == *' DATABASE_URL="$GATE_DATABASE_URL" \' ]] && database_url=1
      [[ "$line" == *' TEST_DATABASE_URL="$GATE_DATABASE_URL" \' ]] && test_database_url=1
    fi
  done < "$REPO_ROOT/scripts/ci-local.sh"
  (( database_url && test_database_url ))
}

for runner in vitest 'go test' e2e; do
  if runner_pins_database_urls "$runner"; then
    ok "$runner pins both database URL variables"
  else
    bad "$runner pins both database URL variables"
  fi
done

for key in ANTHROPIC_API_KEY OPENAI_API_KEY GEMINI_API_KEY DASHSCOPE_API_KEY OPENROUTER_API_KEY; do
  if rg -q "^  $key= \\\\$" "$REPO_ROOT/scripts/ci-local.sh"; then
    ok "vitest empties $key"
  else
    bad "vitest empties $key"
  fi
done

# ── 14. validator and governance hardening cannot silently regress ──────────
if rg -q 'max: 1,' "$VALIDATOR" \
  && rg -q 'connect_timeout: 5,' "$VALIDATOR" \
  && rg -q 'fetch_types: false,' "$VALIDATOR" \
  && rg -q 'prepare: false,' "$VALIDATOR"; then
  ok "validator limits connections and disables implicit type/prepared queries"
else
  bad "validator limits connections and disables implicit type/prepared queries"
fi

if rg -Fq 'const query = sql`SELECT current_database()`;' "$VALIDATOR" \
  && rg -Fq 'const TIMEOUT_MS = 5_000;' "$VALIDATOR" \
  && rg -q 'deadline = setTimeout' "$VALIDATOR" \
  && rg -Fq '}, TIMEOUT_MS);' "$VALIDATOR" \
  && rg -Fq 'sql.end({ timeout: 0 })' "$VALIDATOR" \
  && ! rg -Fq 'sql.end({ timeout: 5 })' "$VALIDATOR" \
  && [[ "$(rg -c '5_000' "$VALIDATOR")" == "1" ]]; then
  ok "validator destroys the active query at its five-second deadline"
else
  bad "validator destroys the active query at its five-second deadline"
fi

ambient_clear_line="$(rg -n -F 'delete process.env.PGTARGETSESSIONATTRS;' "$VALIDATOR" | cut -d: -f1 || true)"
client_create_line="$(rg -n -F 'const sql = postgres(value, {' "$VALIDATOR" | cut -d: -f1 || true)"
if [[ -n "$ambient_clear_line" && -n "$client_create_line" \
  && "$ambient_clear_line" -lt "$client_create_line" \
  && $(rg -c 'sql`' "$VALIDATOR") == "1" \
  && $(rg -c 'target_session_attrs' "$VALIDATOR") -ge "1" \
  && $(rg -Fc 'includes(",")' "$VALIDATOR") -ge "1" ]]; then
  ok "validator rejects routing controls and clears ambient session routing"
else
  bad "validator rejects routing controls and clears ambient session routing"
fi

if rg -Fq 'import net from "node:net";' "$VALIDATOR" \
  && rg -Fq 'socket: createOneShotSocket(),' "$VALIDATOR" \
  && rg -Fq 'if (attempted)' "$VALIDATOR" \
  && rg -Fq 'throw new Error("test database probe permits one connection attempt")' "$VALIDATOR" \
  && rg -Fq 'host: options.host[0],' "$VALIDATOR" \
  && rg -Fq 'port: options.port[0],' "$VALIDATOR" \
  && rg -Fq 'socket.once("connect", () => resolve(socket));' "$VALIDATOR"; then
  ok "validator uses one connected one-shot socket without replacing SSL handling"
else
  bad "validator uses one connected one-shot socket without replacing SSL handling"
fi

governance_block="$(sed -n '/\*\*Governance docs\*\*/,/The hook and the gate script/p' "$REPO_ROOT/AGENTS.md")"
if [[ "$governance_block" == *'scripts/check-test-database-url.mjs'* \
  && "$governance_block" == *'scripts/tests/test-guards.sh'* \
  && "$governance_block" == *'scripts/ci-local.sh'* ]]; then
  ok "AGENTS governance safeguard names every test gate artifact"
else
  bad "AGENTS governance safeguard names every test gate artifact"
fi

llm_block="$(sed -n '/\*\*LLM-touching tests bill real money\.\*\*/,/## Documentation/p' "$REPO_ROOT/AGENTS.md")"
if [[ "$llm_block" == *'ANTHROPIC_API_KEY='* \
  && "$llm_block" == *'OPENAI_API_KEY='* \
  && "$llm_block" == *'GEMINI_API_KEY='* \
  && "$llm_block" == *'DASHSCOPE_API_KEY='* \
  && "$llm_block" == *'OPENROUTER_API_KEY='* ]]; then
  ok "AGENTS documents five explicit empty provider-key exports"
else
  bad "AGENTS documents five explicit empty provider-key exports"
fi

ci_header="$(sed -n '1,16p' "$REPO_ROOT/scripts/ci-local.sh")"
if [[ "$ci_header" == *'ANTHROPIC_API_KEY='* \
  && "$ci_header" == *'OPENAI_API_KEY='* \
  && "$ci_header" == *'GEMINI_API_KEY='* \
  && "$ci_header" == *'DASHSCOPE_API_KEY='* \
  && "$ci_header" == *'OPENROUTER_API_KEY='* ]]; then
  ok "ci-local header documents five explicit empty provider-key exports"
else
  bad "ci-local header documents five explicit empty provider-key exports"
fi

# Task 2 creates the fixture and moves direct calls into it.  Once that file
# exists this guard automatically becomes strict: every other handler test must
# use the fixture rather than calling RegisterUser directly.
USER_FIXTURE="$REPO_ROOT/platform/internal/handlers/user_fixture_test.go"
if [[ ! -e "$USER_FIXTURE" ]]; then
  ok "RegisterUser fixture enforcement deferred until Phase 6 Task 2 creates it"
else
  direct_register_files="$(rg -l 'RegisterUser\(' "$REPO_ROOT/platform/internal/handlers" -g '*_test.go' | grep -Fvx "$USER_FIXTURE" || true)"
  if [[ -z "$direct_register_files" ]]; then
    ok "only user_fixture_test.go calls RegisterUser directly"
  else
    bad "only user_fixture_test.go calls RegisterUser directly"
  fi
fi

echo ""
echo "guard tests: $PASS passed, $FAIL failed"
(( FAIL == 0 ))

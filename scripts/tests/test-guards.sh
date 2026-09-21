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
expect 0 "lowercase percent-decoded test database path is accepted" \
  parse_test_url "postgresql://guard:guard@127.0.0.1:5432/bridge%5ftest"
expect 1 "unsupported percent-encoded database pathname is rejected" \
  parse_test_url "postgresql://guard:guard@127.0.0.1:5432/bridge%5F%74est"
expect 1 "encoded test database pathname with a fragment is rejected" \
  parse_test_url "postgresql://guard:guard@127.0.0.1:5432/bridge%5Ftest#fragment"
expect 1 "literal test database pathname with a fragment is rejected" \
  parse_test_url "postgresql://guard:guard@127.0.0.1:5432/bridge_test#fragment"
expect 1 "encoded test database pathname with an empty fragment is rejected" \
  parse_test_url "postgresql://guard:guard@127.0.0.1:5432/bridge%5Ftest#"
expect 1 "literal test database pathname with an empty fragment is rejected" \
  parse_test_url "postgresql://guard:guard@127.0.0.1:5432/bridge_test#"
expect 1 "empty database path is rejected" \
  parse_test_url "postgresql://guard:guard@127.0.0.1:5432/"
expect 1 "non-Postgres scheme is rejected" \
  parse_test_url "mysql://guard:guard@127.0.0.1:3306/bridge_test"
expect 1 "non-test database path is rejected" \
  parse_test_url "postgresql://guard:guard@127.0.0.1:5432/bridge"
expect 1 "query-suffix test-name decoy is rejected" \
  parse_test_url "postgresql://guard:guard@127.0.0.1:5432/bridge?dbname=bridge_test"
for option in host hostaddr port dbname database user password service servicefile target_session_attrs load_balance_hosts HOST '%68ost' '%64bname'; do
  expect 1 "libpq routing option $option is rejected" \
    parse_test_url "postgresql://guard:guard@127.0.0.1:5432/bridge_test?$option=foreign"
done
expect 0 "nonrouting ssl and application options remain accepted" \
  parse_test_url "postgresql://guard:guard@127.0.0.1:5432/bridge_test?application_name=guard&sslmode=require"
expect 1 "comma-separated database hosts are rejected" \
  parse_test_url "postgresql://guard:guard@primary,replica:5432/bridge_test"
expect 1 "encoded comma-separated database hosts are rejected" \
  parse_test_url "postgresql://guard:guard@safe%2Cforeign:5432/bridge_test"

# Extract only the canonicalizer function.  It must not run ci-local or invoke
# the validator, and it must communicate exclusively through its output variable.
CANONICALIZER="$FIXTURES/canonicalize-test-database-url.sh"
sed -n '/^canonicalize_test_database_url() {/,/^}$/p' "$REPO_ROOT/scripts/ci-local.sh" > "$CANONICALIZER"
canonicalize_and_expect() {
  local input="$1" expected="$2" capture="$FIXTURES/canonicalizer.stdout"
  : > "$capture"
  bash -c 'source "$1"; canonicalize_test_database_url "$2" canonicalized; [[ -n "$canonicalized" && "$canonicalized" == "$3" ]]' \
    _ "$CANONICALIZER" "$input" "$expected" > "$capture"
  [[ ! -s "$capture" ]]
}
expect 0 "Bash canonicalizer normalizes uppercase encoded suffix without stdout" \
  canonicalize_and_expect \
  "postgresql://guard:guard@127.0.0.1:5432/bridge%5Ftest?application_name=guard" \
  "postgresql://guard:guard@127.0.0.1:5432/bridge_test?application_name=guard"
expect 0 "Bash canonicalizer normalizes lowercase encoded suffix without stdout" \
  canonicalize_and_expect \
  "postgresql://guard:guard@127.0.0.1:5432/bridge%5ftest?application_name=guard" \
  "postgresql://guard:guard@127.0.0.1:5432/bridge_test?application_name=guard"

# ── 13. database gate wiring is explicit and complete ───────────────────────
if rg -q 'check-test-database-url\.mjs' "$REPO_ROOT/scripts/ci-local.sh"; then
  ok "ci-local invokes the test database URL validator"
else
  bad "ci-local invokes the test database URL validator"
fi

if rg -Fq 'canonicalize_test_database_url "$DATABASE_URL" AMBIENT_DATABASE_URL' "$REPO_ROOT/scripts/ci-local.sh" \
  && rg -Fq 'CHECK_TEST_DATABASE_URL="$AMBIENT_DATABASE_URL"' "$REPO_ROOT/scripts/ci-local.sh" \
  && rg -Fq 'canonicalize_test_database_url "$GATE_DATABASE_URL" GATE_DATABASE_URL' "$REPO_ROOT/scripts/ci-local.sh"; then
  ok "ci-local canonicalizes ambient and resolved database URLs before validation"
else
  bad "ci-local canonicalizes ambient and resolved database URLs before validation"
fi

gate_canonical_line="$(rg -n -F 'canonicalize_test_database_url "$GATE_DATABASE_URL" GATE_DATABASE_URL' "$REPO_ROOT/scripts/ci-local.sh" | cut -d: -f1 || true)"
gate_validate_line="$(rg -n -F 'CHECK_TEST_DATABASE_URL="$GATE_DATABASE_URL"' "$REPO_ROOT/scripts/ci-local.sh" | cut -d: -f1 || true)"
vitest_line="$(rg -n -F 'step "vitest" env' "$REPO_ROOT/scripts/ci-local.sh" | cut -d: -f1 || true)"
if [[ -n "$gate_canonical_line" && -n "$gate_validate_line" && -n "$vitest_line" \
  && "$gate_canonical_line" -lt "$gate_validate_line" \
  && "$gate_canonical_line" -lt "$vitest_line" ]]; then
  ok "ci-local canonicalizes the gate URL before validation and runner pinning"
else
  bad "ci-local canonicalizes the gate URL before validation and runner pinning"
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

# ── 14. full-gate E2E fixture restoration is ordered and fail-closed ────────
# Extract the executable branch and run it with shell-function mocks.  This
# proves the gate cannot reach Playwright before the destructive-suite recovery
# seed succeeds, without starting a stack or touching a database.
E2E_GATE_FUNCTIONS="$FIXTURES/e2e-gate-functions.sh"
sed -n '/^restore_e2e_demo_seed() {/,/^}$/p' "$REPO_ROOT/scripts/ci-local.sh" > "$E2E_GATE_FUNCTIONS"
sed -n '/^load_persistent_e2e_base_url() {/,/^}$/p' "$REPO_ROOT/scripts/ci-local.sh" >> "$E2E_GATE_FUNCTIONS"
sed -n '/^attest_e2e_stack() {/,/^}$/p' "$REPO_ROOT/scripts/ci-local.sh" >> "$E2E_GATE_FUNCTIONS"
sed -n '/^run_e2e_gate() {/,/^}$/p' "$REPO_ROOT/scripts/ci-local.sh" >> "$E2E_GATE_FUNCTIONS"

run_e2e_gate_case() {
  local fast="$1" base_url="$2" loaded_base_url="$3" restore_rc="$4" expected="$5" attest_rc="${6:-0}" gate_rc
  local trace="$FIXTURES/e2e-gate-trace" attestation="$FIXTURES/e2e-gate-attestation"
  : > "$trace"
  : > "$attestation"
  # shellcheck disable=SC1090
  source "$E2E_GATE_FUNCTIONS"
  ATTESTATION="$attestation"
  attest_e2e_stack() {
    echo attest >> "$trace"
    E2E_STACK_INSTANCE_GO="g" E2E_STACK_INSTANCE_NEXT="n" E2E_STACK_INSTANCE_HOCUSPOCUS="h"
    return "$attest_rc"
  }
  restore_e2e_demo_seed() { echo restore >> "$trace"; return "$restore_rc"; }
  load_persistent_e2e_base_url() {
    [[ -n "${E2E_BASE_URL:-}" ]] && return
    echo load >> "$trace"
    E2E_BASE_URL="$loaded_base_url"
  }
  step() { echo "step:$1" >> "$trace"; return 0; }
  FAILED=()
  FAST="$fast"
  E2E_BASE_URL="$base_url"
  GATE_DATABASE_URL="postgresql://guard:guard@127.0.0.1:5432/bridge_test"
  if run_e2e_gate >/dev/null 2>&1; then
    gate_rc=0
  else
    gate_rc=$?
  fi
  if [[ "$gate_rc" == 0 && "$(tr '\n' ' ' < "$trace")|${FAILED[*]}" == "$expected" ]]; then
    return 0
  fi
  return 1
}

expect 0 "fast gate never restores fixtures or starts E2E" \
  run_e2e_gate_case 1 "http://pinned.test" "http://dotenv.test" 0 "|"
expect 0 "unpinned gate refuses after persistent lookup but before fixture restoration" \
  run_e2e_gate_case 0 "" "" 0 "load |e2e (E2E_BASE_URL unset)"
expect 0 "full pinned gate attests the stack, then restores fixtures, then runs E2E" \
  run_e2e_gate_case 0 "http://pinned.test" "http://dotenv.test" 0 "attest restore step:e2e |"
expect 0 "persistent E2E URL restores fixtures before E2E when shell is unset" \
  run_e2e_gate_case 0 "" "http://dotenv.test" 0 "load attest restore step:e2e |"
expect 0 "failed fixture restoration blocks E2E fail-closed" \
  run_e2e_gate_case 0 "http://pinned.test" "http://dotenv.test" 1 "attest restore |e2e demo seed restore"

restore_e2e_seed_uses_validated_url() {
  local trace="$FIXTURES/e2e-seed-command"
  : > "$trace"
  # shellcheck disable=SC1090
  source "$E2E_GATE_FUNCTIONS"
  REPO_ROOT="$REPO_ROOT"
  GATE_DATABASE_URL="postgresql://guard:guard@127.0.0.1:5432/bridge_test"
  psql() { printf '<%s>\n' "$@" >> "$trace"; }
  restore_e2e_demo_seed
  [[ "$(cat "$trace")" == $'<-v>\n<ON_ERROR_STOP=1>\n<-d>\n<'"$GATE_DATABASE_URL"$'>\n<-f>\n<'"$REPO_ROOT"$'/scripts/seed_problem_demo.sql>' ]]
}
expect 0 "fixture restore applies the canonical seed only through GATE_DATABASE_URL" \
  restore_e2e_seed_uses_validated_url

production_dotenv_loader_reads_fixture_and_preserves_shell() {
  local fixture="$FIXTURES/e2e-env-fixture"
  printf 'E2E_BASE_URL=http://dotenv.test:3999\n' > "$fixture"
  # shellcheck disable=SC1090
  source "$E2E_GATE_FUNCTIONS"
  unset E2E_BASE_URL
  load_persistent_e2e_base_url "$fixture"
  [[ "$E2E_BASE_URL" == "http://dotenv.test:3999" ]]
  E2E_BASE_URL="http://shell.test:4888"
  load_persistent_e2e_base_url "$fixture"
  [[ "$E2E_BASE_URL" == "http://shell.test:4888" ]]
}
expect 0 "production dotenv loader reads a non-.env fixture and preserves a shell override" \
  production_dotenv_loader_reads_fixture_and_preserves_shell

dotenv_load_failure_blocks_e2e() {
  local attestation="$FIXTURES/stale-attestation"
  : > "$attestation"
  # shellcheck disable=SC1090
  source "$E2E_GATE_FUNCTIONS"
  node() { return 1; }
  FAILED=()
  E2E_BASE_URL=""
  ATTESTATION="$attestation"
  if load_persistent_e2e_base_url "$FIXTURES/e2e-env-fixture" >/dev/null 2>&1; then return 1; fi
  [[ ! -e "$attestation" && "${FAILED[*]}" == "e2e (persistent E2E_BASE_URL load failed)" ]]
}
expect 0 "production dotenv loader failure removes stale attestation and records failure" \
  dotenv_load_failure_blocks_e2e

rejected_gate_url_never_invokes_consumers() {
  local fakebin="$FIXTURES/rejected-gate-bin" trace="$FIXTURES/rejected-gate-trace"
  mkdir -p "$fakebin"
  : > "$trace"
  printf '#!/usr/bin/env bash\nprintf psql >> "%s"\n' "$trace" > "$fakebin/psql"
  printf '#!/usr/bin/env bash\nprintf bun >> "%s"\n' "$trace" > "$fakebin/bun"
  chmod +x "$fakebin/psql" "$fakebin/bun"
  local rc=0
  env PATH="$fakebin:/usr/bin:/bin" \
    DATABASE_URL="postgresql://guard:guard@127.0.0.1:5432/bridge_test?dbname=foreign" \
    TEST_DATABASE_URL="postgresql://guard:guard@127.0.0.1:5432/bridge_test?dbname=foreign" \
    bash "$REPO_ROOT/scripts/ci-local.sh" --fast >/dev/null 2>&1 || rc=$?
  [[ "$rc" == 2 && ! -s "$trace" ]]
}
expect 0 "rejected routing URL invokes neither seed nor test consumers" \
  rejected_gate_url_never_invokes_consumers

e2e_gate_call_line="$(rg -n '^run_e2e_gate$' "$REPO_ROOT/scripts/ci-local.sh" | tail -1 | cut -d: -f1 || true)"
if [[ -n "$gate_validate_line" && -n "$e2e_gate_call_line" && "$gate_validate_line" -lt "$e2e_gate_call_line" ]]; then
  ok "live _test validation completes before the E2E restore branch can run"
else
  bad "live _test validation completes before the E2E restore branch can run"
fi

for key in ANTHROPIC_API_KEY OPENAI_API_KEY GEMINI_API_KEY DASHSCOPE_API_KEY OPENROUTER_API_KEY; do
  if sed -n '/step "e2e" env \\/,/bun run test:e2e/p' "$REPO_ROOT/scripts/ci-local.sh" | rg -q "^    $key= \\\\$"; then
    ok "e2e empties $key"
  else
    bad "e2e empties $key"
  fi
done

e2e_env_assignments_are_exact() {
  local block key count
  block="$(sed -n '/step "e2e" env \\/,/bun run test:e2e/p' "$REPO_ROOT/scripts/ci-local.sh")"
  for key in DATABASE_URL TEST_DATABASE_URL ANTHROPIC_API_KEY OPENAI_API_KEY GEMINI_API_KEY DASHSCOPE_API_KEY OPENROUTER_API_KEY; do
    count="$(printf '%s\n' "$block" | rg -c "^    $key=" || true)"
    [[ "$count" == 1 ]] || return 1
  done
  [[ "$block" == *'    DATABASE_URL="$GATE_DATABASE_URL" \'* ]] \
    && [[ "$block" == *'    TEST_DATABASE_URL="$GATE_DATABASE_URL" \'* ]] \
    && [[ "$block" == *$'    ANTHROPIC_API_KEY= \\\n    OPENAI_API_KEY= \\\n    GEMINI_API_KEY= \\\n    DASHSCOPE_API_KEY= \\\n    OPENROUTER_API_KEY= \\'* ]]
}
expect 0 "E2E command has one exact empty-or-gate assignment per protected variable" \
  e2e_env_assignments_are_exact

# ── 14b. E2E stack attestation (Plan 094 Phase 14) ──────────────────────────
expect 0 "failed stack attestation blocks the seed restore and Playwright" \
  run_e2e_gate_case 0 "http://pinned.test" "http://dotenv.test" 0 "attest |e2e (stack attestation)" 1

failed_attestation_removes_stale_attestation() {
  run_e2e_gate_case 0 "http://pinned.test" "http://dotenv.test" 0 "attest |e2e (stack attestation)" 1 || return 1
  [[ ! -e "$FIXTURES/e2e-gate-attestation" ]]
}
expect 0 "failed stack attestation removes a stale gate attestation" \
  failed_attestation_removes_stale_attestation

attest_function_case() {
  # $1 = fake verifier stdout, $2 = fake verifier rc, $3 = expected rc,
  # $4 = expected "go|next|hocuspocus" ids after the call.
  local stdout="$1" fake_rc="$2" want_rc="$3" want_ids="$4" rc=0 trace="$FIXTURES/attest-node-trace"
  : > "$trace"
  # shellcheck disable=SC1090
  source "$E2E_GATE_FUNCTIONS"
  REPO_ROOT="$REPO_ROOT"
  E2E_BASE_URL="http://pinned.test:3100"
  GATE_DATABASE_URL="postgresql://guard:guard@127.0.0.1:5432/bridge_test"
  node() { printf '%s|%s|%s\n' "$E2E_BASE_URL" "$CHECK_E2E_STACK_DATABASE_URL" "$1" >> "$trace"; printf '%s' "$stdout"; return "$fake_rc"; }
  attest_e2e_stack >/dev/null 2>&1 || rc=$?
  [[ "$rc" == "$want_rc" ]] || return 1
  [[ "$E2E_STACK_INSTANCE_GO|$E2E_STACK_INSTANCE_NEXT|$E2E_STACK_INSTANCE_HOCUSPOCUS" == "$want_ids" ]] || return 1
  [[ "$(cat "$trace")" == "http://pinned.test:3100|$GATE_DATABASE_URL|$REPO_ROOT/scripts/check-e2e-stack.mjs" ]]
}
expect 0 "attestation passes only the pinned URL and the validated gate database to the verifier, and captures all three instance ids" \
  attest_function_case $'E2E stack target: http://pinned.test:3100\nE2E_STACK_INSTANCES next=n-1 go=g-1 hocuspocus=h-1\n' 0 0 "g-1|n-1|h-1"
expect 0 "a verifier failure fails the attestation and exports no instance ids" \
  attest_function_case $'E2E stack NOT attested — go: not attested\n' 1 1 "||"
expect 0 "a verifier success without the machine-readable instance line fails closed" \
  attest_function_case $'E2E stack attested\n' 0 1 "||"
expect 0 "a verifier success missing one instance id fails closed" \
  attest_function_case $'E2E_STACK_INSTANCES next=n-1 go=g-1\n' 0 1 "g-1|n-1|"

e2e_block="$(sed -n '/step "e2e" env \\/,/bun run test:e2e/p' "$REPO_ROOT/scripts/ci-local.sh")"
if [[ "$e2e_block" == *'    E2E_BASE_URL="$E2E_BASE_URL" \'* \
  && "$e2e_block" == *'    E2E_STACK_INSTANCE_GO="$E2E_STACK_INSTANCE_GO" \'* \
  && "$e2e_block" == *'    E2E_STACK_INSTANCE_NEXT="$E2E_STACK_INSTANCE_NEXT" \'* \
  && "$e2e_block" == *'    E2E_STACK_INSTANCE_HOCUSPOCUS="$E2E_STACK_INSTANCE_HOCUSPOCUS" \'* ]]; then
  ok "the pinned URL and all three instance ids reach the Playwright child environment"
else
  bad "the pinned URL and all three instance ids reach the Playwright child environment"
fi

attest_line="$(rg -n '^  if attest_e2e_stack; then$' "$REPO_ROOT/scripts/ci-local.sh" | cut -d: -f1 || true)"
restore_line="$(rg -n '^  if restore_e2e_demo_seed; then$' "$REPO_ROOT/scripts/ci-local.sh" | cut -d: -f1 || true)"
if [[ -n "$attest_line" && -n "$restore_line" && "$attest_line" -lt "$restore_line" ]]; then
  ok "stack attestation is ordered before the demo-seed restore in the gate source"
else
  bad "stack attestation is ordered before the demo-seed restore in the gate source"
fi

# The verifier's own logic, driven through injected database and fetch mocks:
# no network, no database, no driver import.
E2E_STACK_SELFTEST="$FIXTURES/e2e-stack-selftest.mjs"
cat > "$E2E_STACK_SELFTEST" <<'NODE'
import { readFileSync } from "node:fs";
const [, , modulePath, vectorPath, scenario] = process.argv;
const m = await import(modulePath);
const vector = JSON.parse(readFileSync(vectorPath, "utf8"));
const nonce = vector.cases[0].nonce;
const good = m.fingerprint(nonce, "bridge_test");
const fail = (why) => { console.error(why); process.exit(1); };

function connection(options = {}) {
  const trace = [];
  let locked = false;
  return {
    trace,
    connect: async () => ({
      begin: async () => trace.push("begin"),
      backendPid: async () => 7,
      lock: async (key) => { locked = true; trace.push(`lock:${key}`); },
      currentDatabase: async () => options.database ?? "bridge_test",
      ownLockGranted: async () => (options.loseLock && trace.includes("sampled") ? false : locked),
      rollback: async () => trace.push("rollback"),
      close: async () => trace.push("close"),
    }),
  };
}
function fetcher(overrides, trace) {
  const calls = [];
  const fetchImpl = async (url, init) => {
    calls.push({ url, init });
    if (!trace.includes("sampled")) trace.push("sampled");
    const parsed = new URL(url);
    const service = parsed.port === "4100" ? "hocuspocus" : parsed.pathname.startsWith("/api/health") ? "go" : "next";
    const nth = calls.filter((c) => c.url === url).length;
    const reply = overrides[service]?.(nth) ?? {
      status: 200,
      body: { fingerprint: good, instance: `${service}-1`, ...(service === "next" ? { realtimeUrl: "ws://localhost:4100" } : {}) },
    };
    if (reply.throw) throw reply.throw;
    return { status: reply.status, json: async () => { if (reply.body === undefined) throw new Error("not json"); return reply.body; } };
  };
  return { fetchImpl, calls };
}
async function run(overrides = {}, connectionOptions = {}, extra = {}) {
  const conn = connection(connectionOptions);
  const { fetchImpl, calls } = fetcher(overrides, conn.trace);
  const result = await m.verifyE2EStack({
    baseUrl: "http://localhost:3100/ignored/path", databaseUrl: "postgresql://guard@127.0.0.1/bridge_test",
    nonce, fetchImpl, connect: conn.connect, ...extra,
  });
  return { result, trace: conn.trace, calls };
}
const classes = (r) => r.result.failures.map((f) => `${f.service}:${f.class}`).join("|");
const released = (r) => r.trace.at(-2) === "rollback" && r.trace.at(-1) === "close";
function expectFailure(r, want) {
  if (r.result.ok) fail("expected failure, got ok");
  if (classes(r) !== want) fail(`expected ${want}, got ${classes(r)}`);
  if (!released(r)) fail("transaction was not rolled back and closed");
  const report = m.formatReport(r.result).join("\n");
  for (const f of r.result.failures) {
    if (!report.includes(`${f.service}: ${f.class}`) || !report.includes(m.REMEDIATION[f.class])) fail(`report lacks class-specific remediation for ${f.class}`);
  }
}

const scenarios = {
  async vector() {
    for (const c of vector.cases) {
      const objid = m.deriveObjid(c.nonce);
      if (objid !== c.objid || m.composeKey(objid) !== c.key || m.fingerprint(c.nonce, c.database) !== c.fingerprint) fail(`vector mismatch for ${c.nonce}`);
    }
    for (const b of vector.composeKeyBoundaries) if (m.composeKey(b.objid) !== b.key) fail("key boundary mismatch");
    if (m.E2E_STACK_LOCK_CLASS !== vector.contract.lockClass) fail("lock class drift");
    if (m.NONCE_PATTERN.source !== vector.contract.noncePattern) fail("nonce pattern drift");
    const d = vector.disjointFrom;
    if ([d.sessionLifecycleLockClass, d.classReplacementLockClass, ...d.hashtextOneKeyHighWords].includes(m.E2E_STACK_LOCK_CLASS)) fail("lock class collides");
  },
  async success() {
    const r = await run();
    if (!r.result.ok) fail(`expected ok, got ${classes(r)}`);
    if (r.calls.length !== 15) fail(`expected 15 samples, got ${r.calls.length}`);
    if (!r.calls.every((c) => c.init.redirect === "manual" && c.init.headers.Connection === "close")) fail("samples must not follow redirects or reuse connections");
    if (r.trace[0] !== "begin" || !r.trace[1].startsWith("lock:") || r.trace[1] !== `lock:${vector.cases[0].key}`) fail("lock must be taken inside the transaction with the vector key");
    if (!released(r)) fail("not released on success");
    if (r.result.realtimeOrigin !== "http://localhost:4100") fail("realtime origin must come from the Next.js report");
    if (!r.calls.some((c) => c.url.startsWith("http://localhost:4100/e2e-stack?nonce="))) fail("hocuspocus must be fetched on the derived origin");
    if (r.calls.some((c) => c.url.includes("/ignored/path"))) fail("only the base origin may be used");
    if (m.instancesLine(r.result.instances) !== "E2E_STACK_INSTANCES next=next-1 go=go-1 hocuspocus=hocuspocus-1") fail("instance line shape");
    const report = m.formatReport(r.result).join("\n");
    if (!report.includes("E2E stack target: http://localhost:3100") || !report.includes("E2E realtime origin: http://localhost:4100")) fail("target and derived origin must be printed");
  },
  async notAttested404() { expectFailure(await run({ go: () => ({ status: 404 }) }), "go:not attested"); },
  async hocuspocusDefault200() { expectFailure(await run({ hocuspocus: () => ({ status: 200 }) }), "hocuspocus:not attested"); },
  async malformedFingerprint() { expectFailure(await run({ go: () => ({ status: 200, body: { fingerprint: "XYZ", instance: "g" } }) }), "go:not attested"); },
  async unsafeInstance() { expectFailure(await run({ go: () => ({ status: 200, body: { fingerprint: good, instance: "g 1;rm" } }) }), "go:not attested"); },
  async mismatch() { expectFailure(await run({ go: () => ({ status: 200, body: { fingerprint: "0".repeat(64), instance: "g" } }) }), "go:mismatch"); },
  async multipleInstances() { expectFailure(await run({ go: (n) => ({ status: 200, body: { fingerprint: good, instance: n > 2 ? "g-2" : "g-1" } }) }), "go:multiple instances"); },
  async noRealtimeOrigin() { expectFailure(await run({ next: () => ({ status: 200, body: { fingerprint: good, instance: "n" } }) }), "hocuspocus:no realtime origin"); },
  async nonWebsocketRealtime() { expectFailure(await run({ next: () => ({ status: 200, body: { fingerprint: good, instance: "n", realtimeUrl: "http://localhost:4100" } }) }), "hocuspocus:no realtime origin"); },
  async nextDownLeavesHocuspocusUnchecked() { expectFailure(await run({ next: () => ({ status: 404 }) }), "next:not attested|hocuspocus:unchecked"); },
  async redirect() { expectFailure(await run({ go: () => ({ status: 302 }) }), "go:redirect"); },
  async timeout() { expectFailure(await run({ go: () => ({ throw: Object.assign(new Error("t"), { name: "TimeoutError" }) }) }), "go:timeout"); },
  async unreachable() { expectFailure(await run({ go: () => ({ throw: new TypeError("fetch failed") }) }), "go:unreachable"); },
  async lockLost() { expectFailure(await run({ go: () => ({ status: 404 }) }, { loseLock: true }), "gate:lock lost"); },
  async seedInstanceMismatch() { expectFailure(await run({}, {}, { expectedInstances: { go: "go-1", next: "next-OTHER", hocuspocus: "hocuspocus-1" } }), "next:multiple instances"); },
  async seedInstanceMatch() { const r = await run({}, {}, { expectedInstances: { go: "go-1", next: "next-1", hocuspocus: "hocuspocus-1" } }); if (!r.result.ok) fail(classes(r)); },
  async nonTestUrlNeverConnects() {
    let connected = false;
    try { await m.verifyE2EStack({ baseUrl: "http://x", databaseUrl: "postgresql://u@h/bridge", connect: async () => { connected = true; throw new Error("x"); } }); } catch { /* expected */ }
    if (connected) fail("connected to a non-_test database");
  },
  async liveNameMustMatchParsedName() {
    const conn = connection({ database: "other_test" });
    try { await m.verifyE2EStack({ baseUrl: "http://x", databaseUrl: "postgresql://u@h/bridge_test", nonce, fetchImpl: async () => fail("must not sample"), connect: conn.connect }); fail("accepted a different live database"); } catch { /* expected */ }
    if (!released({ trace: conn.trace })) fail("not released after a live-name refusal");
  },
  async importIsInert() { if (process.exitCode !== undefined) fail("importing the module ran the CLI"); },
};
if (!scenarios[scenario]) fail(`unknown scenario ${scenario}`);
await scenarios[scenario]();
NODE

# `command` bypasses the node() stubs earlier cases leave defined: expect runs
# its cases in this shell, not a subshell.
e2e_stack_selftest() { command node "$E2E_STACK_SELFTEST" "$REPO_ROOT/scripts/check-e2e-stack.mjs" "$REPO_ROOT/scripts/tests/e2e-stack-vector.json" "$1"; }
expect 0 "verifier key derivation, fingerprint, lock class, and nonce pattern match the shared vector" e2e_stack_selftest vector
expect 0 "verifier success: lock inside the transaction, five samples per origin, no redirects, target and derived origin printed, rollback then close" e2e_stack_selftest success
expect 0 "a 404 is reported per service as not attested with its remediation" e2e_stack_selftest notAttested404
expect 0 "Hocuspocus's default unhandled 200 is reported as not attested" e2e_stack_selftest hocuspocusDefault200
expect 0 "a malformed fingerprint is not attested" e2e_stack_selftest malformedFingerprint
expect 0 "an instance id unsafe for a shell environment is not attested" e2e_stack_selftest unsafeInstance
expect 0 "a fingerprint mismatch blocks" e2e_stack_selftest mismatch
expect 0 "a changing instance id on one origin blocks as multiple instances" e2e_stack_selftest multipleInstances
expect 0 "a missing realtime URL blocks as no realtime origin with its own remediation" e2e_stack_selftest noRealtimeOrigin
expect 0 "a non-websocket realtime URL blocks as no realtime origin" e2e_stack_selftest nonWebsocketRealtime
expect 0 "Hocuspocus is reported unchecked, not misconfigured, when Next.js did not attest" e2e_stack_selftest nextDownLeavesHocuspocusUnchecked
expect 0 "a redirect blocks and is never followed" e2e_stack_selftest redirect
expect 0 "a timeout blocks" e2e_stack_selftest timeout
expect 0 "an unreachable service blocks" e2e_stack_selftest unreachable
expect 0 "a dead verifier transaction is reported as lock lost, not as a foreign stack" e2e_stack_selftest lockLost
expect 0 "seed-setup instance ids differing from the gate's block" e2e_stack_selftest seedInstanceMismatch
expect 0 "seed-setup instance ids equal to the gate's pass" e2e_stack_selftest seedInstanceMatch
expect 0 "the verifier never connects to a database whose parsed name is not _test" e2e_stack_selftest nonTestUrlNeverConnects
expect 0 "the verifier refuses when the live database differs from the parsed name, and still releases" e2e_stack_selftest liveNameMustMatchParsedName
expect 0 "importing the verifier opens no connection and does not run the CLI" e2e_stack_selftest importIsInert

if rg -Fq 'const { default: postgres } = await import("postgres");' "$REPO_ROOT/scripts/check-e2e-stack.mjs" \
  && ! rg -q '^import .*"postgres"' "$REPO_ROOT/scripts/check-e2e-stack.mjs" \
  && rg -Fq 'max: 1,' "$REPO_ROOT/scripts/check-e2e-stack.mjs" \
  && rg -Fq 'await sql.reserve()' "$REPO_ROOT/scripts/check-e2e-stack.mjs" \
  && rg -Fq 'pg_advisory_xact_lock(${key}::bigint)' "$REPO_ROOT/scripts/check-e2e-stack.mjs" \
  && ! rg -q 'pg_advisory_lock\(|pg_advisory_unlock' "$REPO_ROOT/scripts/check-e2e-stack.mjs"; then
  ok "verifier loads its driver lazily and holds a one-key transaction-scoped lock on one reserved connection"
else
  bad "verifier loads its driver lazily and holds a one-key transaction-scoped lock on one reserved connection"
fi

# ── 15. validator and governance hardening cannot silently regress ──────────
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

if rg -Fq 'hostname = decodeURIComponent(parsed.hostname);' "$VALIDATOR" \
  && rg -Fq 'hostname.includes(",")' "$VALIDATOR" \
  && rg -Fq 'rawPathname.includes("%") && !encodedPathname' "$VALIDATOR" \
  && rg -Fq 'parsed.pathname = `/${databaseName}`;' "$VALIDATOR" \
  && rg -Fq 'if (value.includes("#")) {' "$VALIDATOR" \
  && rg -Fq 'await validateLiveDatabase(parsed.toString());' "$VALIDATOR"; then
  ok "validator rejects fragments and encoded routing while canonicalizing its live URL"
else
  bad "validator rejects fragments and encoded routing while canonicalizing its live URL"
fi

if rg -Fq 'import net from "node:net";' "$VALIDATOR" \
  && rg -Fq 'socket: socketController.createSocket,' "$VALIDATOR" \
  && rg -Fq 'if (attempted)' "$VALIDATOR" \
  && rg -Fq 'throw new Error("test database probe permits one connection attempt")' "$VALIDATOR" \
  && rg -Fq 'let rawSocket;' "$VALIDATOR" \
  && rg -Fq 'rawSocket = net.createConnection({' "$VALIDATOR" \
  && rg -Fq 'host: options.host[0],' "$VALIDATOR" \
  && rg -Fq 'port: options.port[0],' "$VALIDATOR" \
  && rg -Fq 'rawSocket.host = options.host[0];' "$VALIDATOR" \
  && rg -Fq 'rawSocket.port = options.port[0];' "$VALIDATOR" \
  && rg -Fq 'rawSocket.once("close", () => {' "$VALIDATOR" \
  && rg -Fq 'reject(new Error("socket closed before connect"));' "$VALIDATOR" \
  && rg -Fq 'destroy() {' "$VALIDATOR" \
  && rg -Fq 'rawSocket?.destroy();' "$VALIDATOR" \
  && [[ "$(rg -Fc 'socketController.destroy();' "$VALIDATOR")" == "2" ]]; then
  ok "validator destroys its one-shot raw socket and preserves SSL host metadata"
else
  bad "validator destroys its one-shot raw socket and preserves SSL host metadata"
fi

governance_block="$(sed -n '/\*\*Governance docs\*\*/,/The hook and the gate script/p' "$REPO_ROOT/AGENTS.md")"
if [[ "$governance_block" == *'scripts/check-test-database-url.mjs'* \
  && "$governance_block" == *'scripts/tests/test-guards.sh'* \
  && "$governance_block" == *'scripts/ci-local.sh'* \
  && "$governance_block" == *'scripts/check-e2e-stack.mjs'* ]]; then
  ok "AGENTS governance safeguard names every test gate artifact"
else
  bad "AGENTS governance safeguard names every test gate artifact"
fi

# ── 15. permanent review-gate governance cannot drift ───────────────────────
# These deliberately inspect semantic clauses rather than copied paragraphs.
# All four canonical documents must make the design gate enforceable in the same
# way, while preserving the separate risk-tiered roster for plan/code reviews.
GOVERNANCE_DOCS=(
  "$REPO_ROOT/AGENTS.md"
  "$REPO_ROOT/docs/reviewers.md"
  "$REPO_ROOT/docs/development-workflow.md"
  "$REPO_ROOT/docs/coding-agent.md"
)

has_permanent_design_gate_contract() {
  local document="$1"

  rg -Pqi '(?s)design.{0,240}gate' "$document" \
    && rg -Pqi '(?s)exactly two.{0,120}required reviewers' "$document" \
    && rg -Pqi '(?s)design.{0,600}Sol.{0,120}gpt-5\.6-sol.{0,120}(?:reasoning effort )?high' "$document" \
    && rg -Pqi '(?s)design.{0,600}Claude Code.{0,120}claude-fable-5' "$document" \
    && rg -Pqi '(?is)design.{0,600}exact (?:substantive )?commit' "$document" \
    && rg -Pqi '(?is)design.{0,600}read-only' "$document"
}

has_uncapped_consensus_contract() {
  local document="$1"

  rg -Pqi '(?is)design.{0,240}gate' "$document" \
    && rg -Pqi '(?is)plan-review gate' "$document" \
    && rg -Pqi '(?is)code-review gate' "$document" \
    && rg -Pqi '(?i)(?:uncapped|no numeric (?:review )?round cap)' "$document" \
    && rg -Pqi '(?is)consensus.{0,180}no open blocker|no open blocker.{0,180}consensus' "$document" \
    && ! rg -Pqi '(?i)max_review_rounds|review[- ]round cap|rounds? at the cap' "$document"
}

has_unavailable_reviewer_pause_contract() {
  local document="$1"

  rg -Pqi '(?is)(?:unavailable.{0,100}required reviewer|required reviewer.{0,100}unavailable).{0,240}pause' "$document" \
    && rg -Pqi '(?is)pause.{0,180}(?:substitut|replace|waiv)|(?:substitut|replace|waiv).{0,180}pause' "$document"
}

for governance_document in "${GOVERNANCE_DOCS[@]}"; do
  governance_name="${governance_document#"$REPO_ROOT/"}"
  if has_permanent_design_gate_contract "$governance_document"; then
    ok "$governance_name fixes the exact Sol + Fable read-only design roster to one commit"
  else
    bad "$governance_name fixes the exact Sol + Fable read-only design roster to one commit"
  fi

  if has_uncapped_consensus_contract "$governance_document"; then
    ok "$governance_name makes design, plan, and code gates uncapped consensus loops"
  else
    bad "$governance_name makes design, plan, and code gates uncapped consensus loops"
  fi

  if has_unavailable_reviewer_pause_contract "$governance_document"; then
    ok "$governance_name pauses unavailable required reviewers instead of substituting or waiving"
  else
    bad "$governance_name pauses unavailable required reviewers instead of substituting or waiving"
  fi
done

for model_policy_document in "$REPO_ROOT/AGENTS.md" "$REPO_ROOT/docs/coding-agent.md"; do
  model_policy_name="${model_policy_document#"$REPO_ROOT/"}"
  if rg -Pqi '(?is)user.{0,80}(?:/model )?pin.{0,120}overrides?' "$model_policy_document"; then
    ok "$model_policy_name gives an explicit user model pin priority over implementation defaults"
  else
    bad "$model_policy_name gives an explicit user model pin priority over implementation defaults"
  fi
done

phase_seven="$(sed -n '/^### Phase 7 /,/^### Phase 8 /p' "$REPO_ROOT/docs/plans/094-session-whiteboard.md")"
if [[ "$phase_seven" =~ [Aa]ll[[:space:]]new[[:space:]]or[[:space:]]changed[[:space:]]tests[[:space:]]are[[:space:]]delegated[[:space:]]to[[:space:]]Terra ]]; then
  ok "Plan 094 applies the user test-model pin to Terra"
else
  bad "Plan 094 applies the user test-model pin to Terra"
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

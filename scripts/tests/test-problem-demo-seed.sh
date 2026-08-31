#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
seed_file="$repo_root/scripts/seed_problem_demo.sql"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

require_text() {
  local text="$1"
  grep -Fq -- "$text" "$seed_file" || fail "seed lacks documented prerequisite: $text"
}

require_text 'INSERT INTO users'
require_text 'INSERT INTO auth_providers'
require_text 'INSERT INTO organizations'
require_text 'INSERT INTO org_memberships'
require_text 'eve@demo.edu'
require_text 'alice@demo.edu'
require_text 'bob@demo.edu'
require_text 'frank@demo.edu'
require_text 'diana@demo.edu'
require_text 'admin@e2e.test'
require_text "current_database() ~ '_test$'"
require_text '$2'
require_text 'INSERT INTO chapters'
require_text 'INSERT INTO chapter_documents'

if grep -Fq -- 'INSERT INTO teaching_units' "$seed_file" || grep -Fq -- 'INSERT INTO unit_documents' "$seed_file"; then
  fail 'seed still targets pre-0026 teaching_units/unit_documents relations'
fi

prerequisite_block=$(sed -n '/Demo organization + login identities/,/-- ---------- Course ----------/p' "$seed_file")
printf '%s\n' "$prerequisite_block" | grep -Fq 'ON CONFLICT (id) DO NOTHING' || fail 'prerequisite rows are not fixed-ID no-ops on re-run'
if printf '%s\n' "$prerequisite_block" | grep -Fq 'DO UPDATE'; then
  fail 'prerequisite rows rewrite state instead of remaining no-ops on re-run'
fi

printf 'problem demo seed contract: pass\n'

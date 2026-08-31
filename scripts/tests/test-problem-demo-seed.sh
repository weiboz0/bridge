#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
canonical_seed="$repo_root/scripts/seed_problem_demo.sql"
seed_file="${SEED_FILE:-$canonical_seed}"
database_url="${CHECK_TEST_DATABASE_URL:?CHECK_TEST_DATABASE_URL must name the guarded test database}"
admin_user_id='00000000-0000-0000-0000-0000000e0006'
admin_provider_id='00000000-0000-0000-0000-0000000f0006'
frank_membership_id='00000000-0000-0000-0000-0000000b0005'
temp_dir=''

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

psql_checked() {
  psql -X -q -v ON_ERROR_STOP=1 "$database_url" "$@"
}

query_scalar() {
  psql -X -At -q -v ON_ERROR_STOP=1 "$database_url" -c "$1"
}

run_seed() {
  psql_checked -f "$1" >/dev/null
}

expect_seed_failure() {
  if psql_checked -f "$1" >/dev/null 2>&1; then
    fail "near-miss seed unexpectedly succeeded: $(basename "$1")"
  fi
}

reset_admin_fixture() {
  psql_checked <<SQL
BEGIN;
DELETE FROM auth_providers WHERE id = '$admin_provider_id'::uuid;
DELETE FROM users WHERE id = '$admin_user_id'::uuid;
COMMIT;
SQL
}

reset_frank_membership() {
  psql_checked <<SQL
BEGIN;
DELETE FROM org_memberships WHERE id = '$frank_membership_id'::uuid;
COMMIT;
SQL
}

restore_canonical_fixture() {
  [[ -z "$database_url" ]] || run_seed "$canonical_seed" || true
}

cleanup() {
  local status=$?
  if [[ -n "$temp_dir" ]]; then rm -rf "$temp_dir"; fi
  restore_canonical_fixture
  exit "$status"
}
trap cleanup EXIT

assert_scalar_true() {
  local label="$1"
  local sql="$2"
  if [[ "$(query_scalar "$sql")" != 'true' ]]; then
    printf 'VERIFY FAILED: %s\n' "$label" >&2
    return 1
  fi
}

verify_fixture() {
  assert_scalar_true 'fixed users, active status, admin bit, or email providers are wrong' "
    WITH expected(email, id, is_admin) AS (
      VALUES
        ('eve@demo.edu','d0d3b031-a483-4214-97fb-48c9584f4dcb'::uuid,false),
        ('alice@demo.edu','242fea26-1527-4a10-b208-af4cad1e1102'::uuid,false),
        ('bob@demo.edu','179aee9f-cce3-46f1-ac5f-f5cfbeb0531b'::uuid,false),
        ('frank@demo.edu','00000000-0000-0000-0000-0000000e0004'::uuid,false),
        ('diana@demo.edu','00000000-0000-0000-0000-0000000e0005'::uuid,false),
        ('admin@e2e.test','00000000-0000-0000-0000-0000000e0006'::uuid,true)
    )
    SELECT (COUNT(*) = 6 AND bool_and(
      u.id = e.id AND u.is_platform_admin = e.is_admin AND u.status = 'active' AND
      (SELECT count(*) FROM auth_providers p WHERE p.user_id = u.id AND p.provider = 'email' AND p.provider_user_id = e.email) = 1
    ))::text
    FROM expected e JOIN users u ON u.email = e.email;" || return 1

  assert_scalar_true 'fixed active organization memberships are wrong' "
    WITH expected(id, user_id, role) AS (
      VALUES
        ('00000000-0000-0000-0000-0000000b0001'::uuid,'d0d3b031-a483-4214-97fb-48c9584f4dcb'::uuid,'teacher'::org_member_role),
        ('00000000-0000-0000-0000-0000000b0002'::uuid,'d0d3b031-a483-4214-97fb-48c9584f4dcb'::uuid,'org_admin'::org_member_role),
        ('00000000-0000-0000-0000-0000000b0003'::uuid,'242fea26-1527-4a10-b208-af4cad1e1102'::uuid,'student'::org_member_role),
        ('00000000-0000-0000-0000-0000000b0004'::uuid,'179aee9f-cce3-46f1-ac5f-f5cfbeb0531b'::uuid,'student'::org_member_role),
        ('00000000-0000-0000-0000-0000000b0005'::uuid,'00000000-0000-0000-0000-0000000e0004'::uuid,'org_admin'::org_member_role),
        ('00000000-0000-0000-0000-0000000b0006'::uuid,'00000000-0000-0000-0000-0000000e0005'::uuid,'parent'::org_member_role)
    )
    SELECT (COUNT(*) = 6 AND bool_and(
      m.user_id = e.user_id AND m.role = e.role AND m.status = 'active' AND m.org_id = 'd386983b-6da4-4cb8-8057-f2aa70d27c07'::uuid
    ))::text
    FROM expected e JOIN org_memberships m ON m.id = e.id;" || return 1

  assert_scalar_true 'seed did not create the current chapter/document rows' "
    SELECT (
      (SELECT count(*) FROM chapters WHERE id IN ('00000000-0000-0000-0000-0000000a1001'::uuid, '00000000-0000-0000-0000-0000000a1002'::uuid)) = 2 AND
      (SELECT count(*) FROM chapter_documents WHERE chapter_id IN ('00000000-0000-0000-0000-0000000a1001'::uuid, '00000000-0000-0000-0000-0000000a1002'::uuid)) = 2
    )::text;" || return 1

  assert_scalar_true 'seed did not create the documented course, problem, or class rows' "
    SELECT (
      (SELECT count(*) FROM organizations WHERE id = 'd386983b-6da4-4cb8-8057-f2aa70d27c07'::uuid) = 1 AND
      (SELECT count(*) FROM courses WHERE id = '00000000-0000-0000-0000-0000000aa001'::uuid) = 1 AND
      (SELECT count(*) FROM topics WHERE id IN ('00000000-0000-0000-0000-000000010001'::uuid, '00000000-0000-0000-0000-000000010002'::uuid)) = 2 AND
      (SELECT count(*) FROM problems WHERE id IN ('00000000-0000-0000-0000-000000020001'::uuid, '00000000-0000-0000-0000-000000020002'::uuid, '00000000-0000-0000-0000-000000020003'::uuid, '00000000-0000-0000-0000-000000020004'::uuid)) = 4 AND
      (SELECT count(*) FROM topic_problems WHERE topic_id IN ('00000000-0000-0000-0000-000000010001'::uuid, '00000000-0000-0000-0000-000000010002'::uuid)) = 4 AND
      (SELECT count(*) FROM problem_solutions WHERE id IN ('00000000-0000-0000-0000-00000055d001'::uuid, '00000000-0000-0000-0000-00000055d002'::uuid, '00000000-0000-0000-0000-00000055d003'::uuid, '00000000-0000-0000-0000-00000055d004'::uuid)) = 4 AND
      (SELECT count(*) FROM test_cases WHERE id IN ('00000000-0000-0000-0000-000000301001'::uuid, '00000000-0000-0000-0000-000000301002'::uuid, '00000000-0000-0000-0000-000000302001'::uuid, '00000000-0000-0000-0000-000000302002'::uuid, '00000000-0000-0000-0000-000000302003'::uuid, '00000000-0000-0000-0000-000000303001'::uuid, '00000000-0000-0000-0000-000000303002'::uuid, '00000000-0000-0000-0000-000000303003'::uuid, '00000000-0000-0000-0000-000000304001'::uuid, '00000000-0000-0000-0000-000000304002'::uuid, '00000000-0000-0000-0000-000000304003'::uuid, '00000000-0000-0000-0000-000000304004'::uuid)) = 12 AND
      (SELECT count(*) FROM classes WHERE id = '00000000-0000-0000-0000-000000040001'::uuid) = 1 AND
      (SELECT count(*) FROM class_memberships WHERE id IN ('00000000-0000-0000-0000-000000050001'::uuid, '00000000-0000-0000-0000-000000050002'::uuid, '00000000-0000-0000-0000-000000050003'::uuid)) = 3 AND
      (SELECT count(*) FROM class_settings WHERE id = '00000000-0000-0000-0000-000000060001'::uuid) = 1
    )::text;" || return 1

  psql -X -At -q -v ON_ERROR_STOP=1 "$database_url" -c "
    SELECT password_hash FROM users
    WHERE id IN (
      'd0d3b031-a483-4214-97fb-48c9584f4dcb'::uuid,
      '242fea26-1527-4a10-b208-af4cad1e1102'::uuid,
      '179aee9f-cce3-46f1-ac5f-f5cfbeb0531b'::uuid,
      '00000000-0000-0000-0000-0000000e0004'::uuid,
      '00000000-0000-0000-0000-0000000e0005'::uuid,
      '$admin_user_id'::uuid
    ) ORDER BY id;" |
    node --input-type=module -e "
      import fs from 'node:fs';
      import bcrypt from 'bcryptjs';
      const hashes = fs.readFileSync(0, 'utf8').trim().split('\\n').filter(Boolean);
      if (hashes.length !== 6 || !hashes.every((hash) => bcrypt.compareSync('bridge123', hash))) process.exit(1);
    " || { printf 'VERIFY FAILED: seed password hashes do not authenticate bridge123\n' >&2; return 1; }
}

fixture_fingerprint() {
  query_scalar "
    WITH rows AS (
      SELECT 'user:' || id || ':' || email || ':' || password_hash || ':' || is_platform_admin || ':' || status AS value
      FROM users WHERE id IN (
        'd0d3b031-a483-4214-97fb-48c9584f4dcb'::uuid, '242fea26-1527-4a10-b208-af4cad1e1102'::uuid,
        '179aee9f-cce3-46f1-ac5f-f5cfbeb0531b'::uuid, '00000000-0000-0000-0000-0000000e0004'::uuid,
        '00000000-0000-0000-0000-0000000e0005'::uuid, '$admin_user_id'::uuid
      )
      UNION ALL SELECT 'provider:' || id || ':' || user_id || ':' || provider || ':' || provider_user_id FROM auth_providers WHERE id IN (
        '00000000-0000-0000-0000-0000000f0001'::uuid, '00000000-0000-0000-0000-0000000f0002'::uuid,
        '00000000-0000-0000-0000-0000000f0003'::uuid, '00000000-0000-0000-0000-0000000f0004'::uuid,
        '00000000-0000-0000-0000-0000000f0005'::uuid, '$admin_provider_id'::uuid
      )
      UNION ALL SELECT 'membership:' || id || ':' || user_id || ':' || role || ':' || status FROM org_memberships WHERE id IN (
        '00000000-0000-0000-0000-0000000b0001'::uuid, '00000000-0000-0000-0000-0000000b0002'::uuid,
        '00000000-0000-0000-0000-0000000b0003'::uuid, '00000000-0000-0000-0000-0000000b0004'::uuid,
        '$frank_membership_id'::uuid, '00000000-0000-0000-0000-0000000b0006'::uuid
      )
      UNION ALL SELECT 'chapter:' || id || ':' || topic_id || ':' || title FROM chapters WHERE id IN (
        '00000000-0000-0000-0000-0000000a1001'::uuid, '00000000-0000-0000-0000-0000000a1002'::uuid
      )
      UNION ALL SELECT 'document:' || chapter_id || ':' || md5(blocks::text) FROM chapter_documents WHERE chapter_id IN (
        '00000000-0000-0000-0000-0000000a1001'::uuid, '00000000-0000-0000-0000-0000000a1002'::uuid
      )
      UNION ALL SELECT 'organization:' || to_jsonb(o)::text FROM organizations o WHERE id = 'd386983b-6da4-4cb8-8057-f2aa70d27c07'::uuid
      UNION ALL SELECT 'course:' || to_jsonb(c)::text FROM courses c WHERE id = '00000000-0000-0000-0000-0000000aa001'::uuid
      UNION ALL SELECT 'topic:' || to_jsonb(t)::text FROM topics t WHERE id IN ('00000000-0000-0000-0000-000000010001'::uuid, '00000000-0000-0000-0000-000000010002'::uuid)
      UNION ALL SELECT 'problem:' || to_jsonb(p)::text FROM problems p WHERE id IN ('00000000-0000-0000-0000-000000020001'::uuid, '00000000-0000-0000-0000-000000020002'::uuid, '00000000-0000-0000-0000-000000020003'::uuid, '00000000-0000-0000-0000-000000020004'::uuid)
      UNION ALL SELECT 'topic_problem:' || to_jsonb(tp)::text FROM topic_problems tp WHERE topic_id IN ('00000000-0000-0000-0000-000000010001'::uuid, '00000000-0000-0000-0000-000000010002'::uuid)
      UNION ALL SELECT 'solution:' || to_jsonb(ps)::text FROM problem_solutions ps WHERE id IN ('00000000-0000-0000-0000-00000055d001'::uuid, '00000000-0000-0000-0000-00000055d002'::uuid, '00000000-0000-0000-0000-00000055d003'::uuid, '00000000-0000-0000-0000-00000055d004'::uuid)
      UNION ALL SELECT 'test_case:' || to_jsonb(tc)::text FROM test_cases tc WHERE id IN ('00000000-0000-0000-0000-000000301001'::uuid, '00000000-0000-0000-0000-000000301002'::uuid, '00000000-0000-0000-0000-000000302001'::uuid, '00000000-0000-0000-0000-000000302002'::uuid, '00000000-0000-0000-0000-000000302003'::uuid, '00000000-0000-0000-0000-000000303001'::uuid, '00000000-0000-0000-0000-000000303002'::uuid, '00000000-0000-0000-0000-000000303003'::uuid, '00000000-0000-0000-0000-000000304001'::uuid, '00000000-0000-0000-0000-000000304002'::uuid, '00000000-0000-0000-0000-000000304003'::uuid, '00000000-0000-0000-0000-000000304004'::uuid)
      UNION ALL SELECT 'class:' || to_jsonb(c)::text FROM classes c WHERE id = '00000000-0000-0000-0000-000000040001'::uuid
      UNION ALL SELECT 'class_membership:' || to_jsonb(cm)::text FROM class_memberships cm WHERE id IN ('00000000-0000-0000-0000-000000050001'::uuid, '00000000-0000-0000-0000-000000050002'::uuid, '00000000-0000-0000-0000-000000050003'::uuid)
      UNION ALL SELECT 'class_setting:' || to_jsonb(cs)::text FROM class_settings cs WHERE id = '00000000-0000-0000-0000-000000060001'::uuid
    ) SELECT md5(string_agg(value, E'\\n' ORDER BY value)) FROM rows;"
}

temp_dir=$(mktemp -d /dev/shm/bridge-problem-demo-seed.XXXXXX)
node "$repo_root/scripts/check-test-database-url.mjs"

# Baseline: run the actual seed, verify all behavior, then prove a second run
# leaves every fixed fixture row and chapter document byte-for-byte unchanged.
run_seed "$seed_file"
verify_fixture || fail 'canonical seed did not establish its fixture contract'
baseline=$(fixture_fingerprint)
run_seed "$seed_file"
[[ "$(fixture_fingerprint)" == "$baseline" ]] || fail 'second seed run changed fixed fixture state'

# A mutated non-test guard runs only against bridge_test. The mutation changes
# the actual guard to a non-test literal, so it must omit both admin rows. If
# the production condition became unconditional, the mutation would not match
# and this assertion would fail without ever opening a non-test connection.
reset_admin_fixture
non_test_guard="$temp_dir/non-test-guard.sql"
awk 'index($0, "WHERE current_database() ~") { print "WHERE '\''non_test_target'\'' ~ '\''_test$'\''"; next } { print }' "$seed_file" > "$non_test_guard"
run_seed "$non_test_guard"
admin_count=$(query_scalar "SELECT count(*)::text FROM users WHERE id = '$admin_user_id'::uuid")
admin_provider_count=$(query_scalar "SELECT count(*)::text FROM auth_providers WHERE id = '$admin_provider_id'::uuid")
[[ "$admin_count" == '0' ]] || fail 'admin guard created a known-password admin for a simulated non-test database'
[[ "$admin_provider_count" == '0' ]] || fail 'admin guard created an auth provider for a simulated non-test database'
run_seed "$canonical_seed"
verify_fixture || fail 'canonical seed did not restore simulated non-test admin state'

# Each mutation must be rejected by the same behavioral verifier rather than
# by source fragments. Cleanup targets only the fixed row under test and the
# canonical seed restores it before the next mutation.
reset_admin_fixture
missing_provider="$temp_dir/missing-provider.sql"
awk 'index($0, "WHERE current_database() ~") { seen++; if (seen == 2) { print "WHERE false"; next } } { print }' "$seed_file" > "$missing_provider"
run_seed "$missing_provider"
if verify_fixture 2>/dev/null; then fail 'omitted admin auth provider was accepted'; fi
run_seed "$canonical_seed"
verify_fixture || fail 'canonical seed did not restore omitted provider state'

reset_frank_membership
wrong_membership="$temp_dir/wrong-membership.sql"
sed "s/'00000000-0000-0000-0000-0000000b0005', 'd386983b-6da4-4cb8-8057-f2aa70d27c07', '00000000-0000-0000-0000-0000000e0004', 'org_admin', 'active'/'00000000-0000-0000-0000-0000000b0005', 'd386983b-6da4-4cb8-8057-f2aa70d27c07', '00000000-0000-0000-0000-0000000e0004', 'teacher', 'suspended'/" "$seed_file" > "$wrong_membership"
run_seed "$wrong_membership"
if verify_fixture 2>/dev/null; then fail 'inactive or wrong membership was accepted'; fi
reset_frank_membership
run_seed "$canonical_seed"
verify_fixture || fail 'canonical seed did not restore membership state'

reset_admin_fixture
invalid_bcrypt="$temp_dir/invalid-bcrypt.sql"
awk 'index($0, "E2E Admin") { gsub(/\$2b\$10\$[A-Za-z0-9.\/]+/, "not-a-bcrypt-hash") } { print }' "$seed_file" > "$invalid_bcrypt"
run_seed "$invalid_bcrypt"
if verify_fixture 2>/dev/null; then fail 'invalid bcrypt hash was accepted'; fi
reset_admin_fixture
run_seed "$canonical_seed"
verify_fixture || fail 'canonical seed did not restore bcrypt state'

invalid_chapter_columns="$temp_dir/invalid-chapter-columns.sql"
sed '0,/INSERT INTO chapters (/s//INSERT INTO chapters (missing_column,/' "$seed_file" > "$invalid_chapter_columns"
expect_seed_failure "$invalid_chapter_columns"

missing_conflict="$temp_dir/missing-conflict.sql"
sed '0,/ON CONFLICT (id) DO NOTHING;/s//;/' "$seed_file" > "$missing_conflict"
expect_seed_failure "$missing_conflict"

# A late SQL failure must roll back the newly inserted admin. This exercises
# BEGIN; concretely. The following canonical run exercises COMMIT; concretely.
reset_admin_fixture
transaction_failure="$temp_dir/transaction-failure.sql"
sed '0,/^COMMIT;$/s//SELECT 1 \/ 0;\nCOMMIT;/' "$seed_file" > "$transaction_failure"
expect_seed_failure "$transaction_failure"
admin_count=$(query_scalar "SELECT count(*)::text FROM users WHERE id = '$admin_user_id'::uuid")
[[ "$admin_count" == '0' ]] || fail 'late seed failure committed the admin row; BEGIN is missing or ineffective'
run_seed "$canonical_seed"
verify_fixture || fail 'canonical seed did not commit and restore admin state'

[[ "$(fixture_fingerprint)" == "$baseline" ]] || fail 'test cleanup did not restore the exact fixed fixture state'
printf 'problem demo seed integration contract: pass\n'

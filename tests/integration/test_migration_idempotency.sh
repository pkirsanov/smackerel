#!/usr/bin/env bash
#
# BUG-031-001 / SCN-031-BUG001-B1 — Integration suite is healthy across
# consecutive runs (migration idempotency).
#
# Reproduces the crash-loop chain that was observed in the field:
#
#   1. The test postgres data volume survives teardown (Defect A: post-command
#      `--volumes` is silently dropped by the pre-fix argv parser, so
#      `down --volumes` from the integration cleanup trap actually leaves the
#      volume on disk).
#   2. On the next `up` the database still carries every DDL object from the
#      consolidated initial migration (annotations.chk_rating_range, etc).
#   3. The `schema_migrations` ledger is divergent from disk (e.g. partial
#      reset during a prior failed run, or a consolidated/renamed migration
#      where `version=001` was never recorded against the new file). The
#      migration runner therefore re-applies `001_initial_schema.sql`.
#   4. On pre-fix HEAD the bare `ALTER TABLE annotations ADD CONSTRAINT
#      chk_rating_range` fails with SQLSTATE 42710 (duplicate_object),
#      smackerel-core exits non-zero, Docker restart-loops it, and
#      /api/health never converges.
#
# We simulate step 3 deterministically by truncating `schema_migrations`
# while the postgres volume is retained (DDL objects intact). After the fix
# the constraint is wrapped in a `DO $$ ... EXCEPTION WHEN duplicate_object
# THEN NULL; END $$;` block and the second `up` reaches health.
#
# Adversarial input: pre-existing chk_rating_range constraint plus an empty
# schema_migrations ledger — exactly the condition that triggers SQLSTATE
# 42710 on pre-fix HEAD.
#
# This script MUST FAIL on pre-fix HEAD and PASS post-fix.
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_DIR/scripts/lib/runtime.sh"

TEST_ENV="test"
TEST_VOLUME="smackerel-test-postgres-data"

cleanup() {
  timeout 60 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true
  docker volume rm -f "$TEST_VOLUME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Hard-clean baseline so the bug is provably reached via the mid-test
# ledger reset, not via any leftover state from a previous session.
timeout 60 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true
docker volume rm -f "$TEST_VOLUME" >/dev/null 2>&1 || true

ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
CORE_URL="$(smackerel_env_value "$ENV_FILE" "CORE_EXTERNAL_URL")"
AUTH_TOKEN="$(smackerel_env_value "$ENV_FILE" "SMACKEREL_AUTH_TOKEN")"
PG_USER="$(smackerel_env_value "$ENV_FILE" "POSTGRES_USER")"
PG_DB="$(smackerel_env_value "$ENV_FILE" "POSTGRES_DB")"

assert_health() {
  local label="$1"
  local elapsed=0
  local response=""
  while [[ $elapsed -lt 120 ]]; do
    if response="$(curl --max-time 5 -fsS -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL/api/health" 2>/dev/null)"; then
      if python3 -c 'import json,sys; p=json.loads(sys.argv[1]); assert p["services"]["postgres"]["status"]=="up"; assert p["services"]["nats"]["status"]=="up"; assert p["services"]["ml_sidecar"]["status"]=="up"' "$response" 2>/dev/null; then
        echo "[$label] healthy: $response"
        return 0
      fi
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  echo "FAIL: [$label] /api/health did not converge to all services up within 120s" >&2
  echo "Last response: $response" >&2
  docker ps --filter name=smackerel-test-smackerel-core --format 'table {{.Names}}\t{{.Status}}' >&2 || true
  docker logs --tail 80 smackerel-test-smackerel-core-1 2>&1 | sed 's/^/  core| /' >&2 || true
  return 1
}

# Run 1 — clean volume, fresh schema. This populates schema_migrations and
# creates every DDL object from 001_initial_schema.sql (including
# annotations.chk_rating_range).
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" up
assert_health "run 1"

# Reset only the row for the consolidated initial migration. This faithfully
# reproduces the field scenario (e.g. a partial reset, or a consolidated
# `001_initial_schema.sql` whose row was never recorded against the new
# file name/digest) without forcing every later migration to re-apply.
# Deterministically forces the migration runner to re-execute
# `001_initial_schema.sql` against a database where chk_rating_range
# already exists — the exact precondition that trips Defect B on pre-fix
# HEAD.
docker exec smackerel-test-postgres-1 \
  psql -U "$PG_USER" -d "$PG_DB" -v ON_ERROR_STOP=1 \
  -c "DELETE FROM schema_migrations WHERE version = '001_initial_schema.sql';"

# Tear down WITHOUT --volumes so the postgres data volume survives.
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down

# Sanity check: the named postgres volume MUST still be present, otherwise
# the second-run assertion below would not actually exercise the bug.
if ! docker volume ls --format '{{.Name}}' | grep -qx "$TEST_VOLUME"; then
    echo "FAIL: precondition not met — $TEST_VOLUME was removed by 'down' (volume should survive without --volumes)" >&2
    exit 1
fi

# Run 2 — same volume, empty schema_migrations, existing DDL objects.
# The migration runner re-applies 001_initial_schema.sql.
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" up
assert_health "run 2"

echo "PASS: core reached /api/health on consecutive runs over a retained postgres volume with re-applied initial migration"

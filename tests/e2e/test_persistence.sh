#!/usr/bin/env bash
# E2E test: Verify data persists across docker compose down/up cycle
# Scenario: SCN-002-004
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_DIR/scripts/lib/runtime.sh"

TEST_ENV="test"

cleanup() {
    echo "Cleaning up test stack..."
    timeout 60 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "=== SCN-002-004: Data persistence across restarts ==="

# Clean start
cleanup

# Start services
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" up

# Wait for healthy
echo "Waiting for services..."
sleep 20

ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
POSTGRES_USER="$(smackerel_env_value "$ENV_FILE" "POSTGRES_USER")"
POSTGRES_DB="$(smackerel_env_value "$ENV_FILE" "POSTGRES_DB")"
COMPOSE_PROJECT="$(smackerel_compose_project "$TEST_ENV")"
COMPOSE_ARGS=(docker compose --project-name "$COMPOSE_PROJECT" --env-file "$ENV_FILE" -f "$REPO_DIR/docker-compose.yml")

sleep 20

# Insert test data via psql
echo "Inserting test artifact..."
timeout 60 "${COMPOSE_ARGS[@]}" exec --interactive=false -T postgres \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, created_at, updated_at)
VALUES ('test-persist-001', 'note', 'Persistence Test', 'hash-persist-test', 'test', NOW(), NOW())
ON CONFLICT (id) DO NOTHING;
"

# Verify insert
COUNT_BEFORE=$("${COMPOSE_ARGS[@]}" exec --interactive=false -T postgres \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At -c "SELECT COUNT(*) FROM artifacts WHERE id='test-persist-001';")

if [ "$COUNT_BEFORE" != "1" ]; then
    echo "FAIL: Insert verification failed (count=$COUNT_BEFORE)"
    exit 1
fi
echo "Insert verified (count=$COUNT_BEFORE)"

# Stop all services (but keep volumes)
echo "Stopping services (preserving volumes)..."
timeout 90 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down

# Restart services
echo "Restarting services..."
timeout 180 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" up

# Wait for postgres after restart
sleep 20

# Verify data persisted
COUNT_AFTER=$("${COMPOSE_ARGS[@]}" exec --interactive=false -T postgres \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At -c "SELECT COUNT(*) FROM artifacts WHERE id='test-persist-001';")

if [ "$COUNT_AFTER" != "1" ]; then
    echo "FAIL: Data did not persist across restart (count=$COUNT_AFTER)"
    exit 1
fi

echo "PASS: SCN-002-004 (data persisted, count=$COUNT_AFTER)"

#!/usr/bin/env bash
# E2E test: Verify data persists across docker compose down/up cycle
# Scenario: SCN-002-004
set -euo pipefail

COMPOSE_PROJECT="smackerel-test-persist"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

cleanup() {
    echo "Cleaning up test stack..."
    docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" down -v --remove-orphans 2>/dev/null || true
}
trap cleanup EXIT

echo "=== SCN-002-004: Data persistence across restarts ==="

# Clean start
cleanup

# Start services
docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" --env-file "$REPO_DIR/.env" up -d

# Wait for healthy
echo "Waiting for services..."
sleep 20

# Insert test data via psql
echo "Inserting test artifact..."
docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" exec -T postgres \
    psql -U smackerel -d smackerel -c "
INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, created_at, updated_at)
VALUES ('test-persist-001', 'note', 'Persistence Test', 'hash-persist-test', 'test', NOW(), NOW())
ON CONFLICT (id) DO NOTHING;
"

# Verify insert
COUNT_BEFORE=$(docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" exec -T postgres \
    psql -U smackerel -d smackerel -t -c "SELECT COUNT(*) FROM artifacts WHERE id='test-persist-001';" | tr -d '[:space:]')

if [ "$COUNT_BEFORE" != "1" ]; then
    echo "FAIL: Insert verification failed (count=$COUNT_BEFORE)"
    exit 1
fi
echo "Insert verified (count=$COUNT_BEFORE)"

# Stop all services (but keep volumes)
echo "Stopping services (preserving volumes)..."
docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" down

# Restart services
echo "Restarting services..."
docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" --env-file "$REPO_DIR/.env" up -d

# Wait for healthy
echo "Waiting for services after restart..."
sleep 20

# Verify data persisted
COUNT_AFTER=$(docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" exec -T postgres \
    psql -U smackerel -d smackerel -t -c "SELECT COUNT(*) FROM artifacts WHERE id='test-persist-001';" | tr -d '[:space:]')

if [ "$COUNT_AFTER" != "1" ]; then
    echo "FAIL: Data did not persist across restart (count=$COUNT_AFTER)"
    exit 1
fi

echo "PASS: SCN-002-004 (data persisted, count=$COUNT_AFTER)"

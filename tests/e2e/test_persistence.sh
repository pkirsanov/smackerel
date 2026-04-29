#!/usr/bin/env bash
# E2E test: Verify data persists across docker compose down/up cycle
# Scenario: SCN-002-004
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TEST_ENV="test"
source "$SCRIPT_DIR/lib/helpers.sh"
ARTIFACT_ID="test-persist-001"

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
e2e_setup
e2e_wait_healthy 120

# Insert test data via psql
echo "Inserting test artifact..."
INSERT_OUTPUT="$(e2e_psql "
INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, created_at, updated_at)
VALUES ('$ARTIFACT_ID', 'note', 'Persistence Test', 'hash-persist-test', 'test', NOW(), NOW())
ON CONFLICT (id) DO NOTHING;
")"
echo "Insert completed ($INSERT_OUTPUT)"

# Verify insert
COUNT_BEFORE="$(e2e_psql "SELECT COUNT(*) FROM artifacts WHERE id='$ARTIFACT_ID';")"

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
e2e_setup
e2e_wait_healthy 120

# Verify data persisted
COUNT_AFTER="$(e2e_psql "SELECT COUNT(*) FROM artifacts WHERE id='$ARTIFACT_ID';")"

if [ "$COUNT_AFTER" != "1" ]; then
    echo "FAIL: Data did not persist across restart (count=$COUNT_AFTER)"
    exit 1
fi

echo "PASS: SCN-002-004 (data persisted, count=$COUNT_AFTER)"

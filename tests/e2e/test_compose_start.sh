#!/usr/bin/env bash
# E2E test: Verify docker compose cold start brings all services up
# Scenario: SCN-002-001
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

echo "=== SCN-002-001: Docker compose cold start ==="

# Ensure clean state
cleanup

# Start services
echo "Starting services..."
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" up

ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
CORE_URL="$(smackerel_env_value "$ENV_FILE" "CORE_EXTERNAL_URL")"

# Wait for services to be healthy (max 60s)
echo "Waiting for services to be healthy..."
TIMEOUT=60
ELAPSED=0
while [ $ELAPSED -lt $TIMEOUT ]; do
    HEALTHY=$(smackerel_compose "$TEST_ENV" ps --format json 2>/dev/null | grep -c '"healthy"' || true)
    if [ "$HEALTHY" -ge 4 ] 2>/dev/null; then
        echo "All services healthy after ${ELAPSED}s"
        break
    fi
    sleep 2
    ELAPSED=$((ELAPSED + 2))
done

if [ $ELAPSED -ge $TIMEOUT ]; then
    echo "FAIL: Services did not become healthy within ${TIMEOUT}s"
    smackerel_compose "$TEST_ENV" ps
    smackerel_compose "$TEST_ENV" logs
    exit 1
fi

# Check health endpoint
echo "Checking /api/health..."
HEALTH_RESPONSE=$(curl -sf --max-time 5 "$CORE_URL/api/health" || true)
if [ -z "$HEALTH_RESPONSE" ]; then
    echo "FAIL: /api/health did not respond"
    exit 1
fi

echo "Health response: $HEALTH_RESPONSE"

# Verify status field exists
STATUS=$(echo "$HEALTH_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['status'])" 2>/dev/null || true)
if [ -z "$STATUS" ]; then
    echo "FAIL: status field missing from health response"
    exit 1
fi

echo "PASS: SCN-002-001 (status=$STATUS)"

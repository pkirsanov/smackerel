#!/usr/bin/env bash
# E2E test: Verify docker compose cold start brings all services up
# Scenario: SCN-002-001
set -euo pipefail

COMPOSE_PROJECT="smackerel-test-compose"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

cleanup() {
    echo "Cleaning up test stack..."
    docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" down -v --remove-orphans 2>/dev/null || true
}
trap cleanup EXIT

echo "=== SCN-002-001: Docker compose cold start ==="

# Ensure clean state
cleanup

# Start services
echo "Starting services..."
docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" --env-file "$REPO_DIR/.env" up -d

# Wait for services to be healthy (max 60s)
echo "Waiting for services to be healthy..."
TIMEOUT=60
ELAPSED=0
while [ $ELAPSED -lt $TIMEOUT ]; do
    HEALTHY=$(docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" ps --format json 2>/dev/null | grep -c '"healthy"' || true)
    TOTAL=$(docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" ps --format json 2>/dev/null | wc -l || true)
    if [ "$HEALTHY" -ge 4 ] 2>/dev/null; then
        echo "All services healthy after ${ELAPSED}s"
        break
    fi
    sleep 2
    ELAPSED=$((ELAPSED + 2))
done

if [ $ELAPSED -ge $TIMEOUT ]; then
    echo "FAIL: Services did not become healthy within ${TIMEOUT}s"
    docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" ps
    docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" logs
    exit 1
fi

# Check health endpoint
echo "Checking /api/health..."
HEALTH_RESPONSE=$(curl -sf --max-time 5 http://localhost:8080/api/health || true)
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

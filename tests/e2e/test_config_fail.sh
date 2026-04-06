#!/usr/bin/env bash
# E2E test: Missing required config fails startup with explicit error
# Scenario: SCN-002-044
set -euo pipefail

COMPOSE_PROJECT="smackerel-test-config"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

cleanup() {
    echo "Cleaning up test stack..."
    docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" down -v --remove-orphans 2>/dev/null || true
}
trap cleanup EXIT

echo "=== SCN-002-044: Missing required config fails startup ==="

# Clean state
cleanup

# Create a deliberately incomplete .env (missing LLM_PROVIDER, LLM_MODEL, LLM_API_KEY)
TEMP_ENV=$(mktemp)
cat > "$TEMP_ENV" <<'EOF'
DATABASE_URL=postgres://smackerel:smackerel@postgres:5432/smackerel?sslmode=disable
NATS_URL=nats://nats:4222
SMACKEREL_AUTH_TOKEN=test-token
EOF

# Start only infrastructure so core can attempt to start
docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" up -d postgres nats
sleep 10

# Build core image
docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" build smackerel-core 2>/dev/null

# Run smackerel-core with incomplete config — expect failure
echo "Starting smackerel-core with incomplete config..."
set +e
docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" run \
    --rm --no-deps \
    --env-file "$TEMP_ENV" \
    smackerel-core 2>&1 | tee /tmp/config-fail-output.txt
EXIT_CODE=$?
set -e

rm -f "$TEMP_ENV"

# Verify the process exited with non-zero code
if [ "$EXIT_CODE" -eq 0 ]; then
    echo "FAIL: smackerel-core exited 0 with missing config — should have failed"
    exit 1
fi
echo "Process exited with code $EXIT_CODE (expected non-zero)"

# Verify error message names the missing variables
OUTPUT=$(cat /tmp/config-fail-output.txt)
FOUND_MISSING=0

for VAR in LLM_PROVIDER LLM_MODEL LLM_API_KEY; do
    if echo "$OUTPUT" | grep -q "$VAR"; then
        echo "  Error message names missing variable: $VAR"
        FOUND_MISSING=$((FOUND_MISSING + 1))
    fi
done

if [ "$FOUND_MISSING" -lt 3 ]; then
    echo "FAIL: Error message did not name all missing variables (found $FOUND_MISSING/3)"
    echo "Output was: $OUTPUT"
    exit 1
fi

echo "PASS: SCN-002-044 (exit=$EXIT_CODE, named $FOUND_MISSING missing variables)"

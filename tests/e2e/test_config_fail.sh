#!/usr/bin/env bash
# E2E test: Missing required config fails startup with explicit error
# Scenario: SCN-002-044
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_DIR/scripts/lib/runtime.sh"

TEST_ENV="test"
SOURCE_ENV=""
COMPOSE_PROJECT=""
CORE_IMAGE=""

cleanup() {
    echo "Cleaning up test stack..."
    timeout 60 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "=== SCN-002-044: Missing required config fails startup ==="

# Clean state
cleanup

"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" config generate >/dev/null
SOURCE_ENV="$(smackerel_require_env_file "$TEST_ENV")"
COMPOSE_PROJECT="$(smackerel_compose_project "$TEST_ENV")"
CORE_IMAGE="${COMPOSE_PROJECT}-smackerel-core:latest"

# Create a deliberately incomplete env file (missing LLM_PROVIDER, LLM_MODEL, LLM_API_KEY)
TEMP_ENV=$(mktemp)
cp "$SOURCE_ENV" "$TEMP_ENV"
sed -i '/^LLM_PROVIDER=/d' "$TEMP_ENV"
sed -i '/^LLM_MODEL=/d' "$TEMP_ENV"
sed -i '/^LLM_API_KEY=/d' "$TEMP_ENV"

# Start only infrastructure so core can attempt to start
docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" --env-file "$SOURCE_ENV" up -d postgres nats
sleep 10

# Build core image
docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" --env-file "$SOURCE_ENV" build smackerel-core

# Run smackerel-core with incomplete config — expect failure
echo "Starting smackerel-core with incomplete config..."
set +e
OUTPUT=$(timeout 60 docker run --rm --env-file "$TEMP_ENV" "$CORE_IMAGE" 2>&1)
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

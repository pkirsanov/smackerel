#!/usr/bin/env bash
# Stress test: Search completes in <3s with 1000+ artifacts
# DoD item: Scope 05 search performance validation
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_DIR/scripts/lib/runtime.sh"

TEST_ENV="test"
ARTIFACT_COUNT=1100
SEARCH_ITERATIONS=10
MAX_SEARCH_TIME_MS=3000

cleanup() {
  if [ "${STACK_MANAGED:-0}" = "0" ]; then
    timeout 60 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true
    # Force-remove explicitly-named test volumes
    local env_file
    env_file="$(smackerel_env_file "$TEST_ENV" 2>/dev/null || true)"
    if [ -n "$env_file" ] && [ -f "$env_file" ]; then
      local pg_vol nats_vol ollama_vol
      pg_vol="$(smackerel_env_value "$env_file" "POSTGRES_VOLUME_NAME")"
      nats_vol="$(smackerel_env_value "$env_file" "NATS_VOLUME_NAME")"
      ollama_vol="$(smackerel_env_value "$env_file" "OLLAMA_VOLUME_NAME")"
      docker volume rm "$pg_vol" "$nats_vol" "$ollama_vol" 2>/dev/null || true
    fi
  fi
}
trap cleanup EXIT

echo "=== Search Stress Test ==="
echo "  Target: $ARTIFACT_COUNT artifacts, search < ${MAX_SEARCH_TIME_MS}ms"
echo ""

# --- Stack lifecycle ---
if [ "${STACK_MANAGED:-0}" = "0" ]; then
  cleanup
  "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" up
fi

ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
CORE_URL="$(smackerel_env_value "$ENV_FILE" "CORE_EXTERNAL_URL")"
AUTH_TOKEN="$(smackerel_env_value "$ENV_FILE" "SMACKEREL_AUTH_TOKEN")"
POSTGRES_USER="$(smackerel_env_value "$ENV_FILE" "POSTGRES_USER")"
POSTGRES_DB="$(smackerel_env_value "$ENV_FILE" "POSTGRES_DB")"

# Wait for services to become healthy
elapsed=0
while [[ $elapsed -lt 90 ]]; do
  if curl --max-time 5 -fsS "$CORE_URL/api/health" >/dev/null 2>&1; then
    echo "Services healthy after ${elapsed}s"
    break
  fi
  sleep 2
  elapsed=$((elapsed + 2))
done

if [[ $elapsed -ge 90 ]]; then
  echo "FAIL: Services did not become healthy within 90s" >&2
  exit 1
fi

# --- Seed 1100 synthetic artifacts ---
echo ""
echo "Seeding $ARTIFACT_COUNT synthetic artifacts..."

# Seed in batches to avoid exceeding OS argument length limit
BATCH_SIZE=100
batch_start=1
while [[ $batch_start -le $ARTIFACT_COUNT ]]; do
  batch_end=$((batch_start + BATCH_SIZE - 1))
  if [[ $batch_end -gt $ARTIFACT_COUNT ]]; then
    batch_end=$ARTIFACT_COUNT
  fi

  SQL_BATCH="BEGIN;"
  for i in $(seq $batch_start $batch_end); do
    case $((i % 5)) in
      0) art_type="article"; topic="technology" ;;
      1) art_type="bookmark"; topic="finance" ;;
      2) art_type="note"; topic="leadership" ;;
      3) art_type="article"; topic="science" ;;
      4) art_type="bookmark"; topic="design" ;;
    esac

    SQL_BATCH+="
INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, summary, created_at, updated_at)
VALUES (
  'stress-search-$(printf '%04d' "$i")',
  '$art_type',
  'Stress Test Article $i about $topic and innovation strategies',
  'stress-hash-$i',
  'stress-test',
  'Summary $i covering $topic trends, best practices, and emerging patterns in the field',
  NOW() - INTERVAL '$(( (i % 365) + 1 )) days',
  NOW()
) ON CONFLICT (id) DO NOTHING;"
  done
  SQL_BATCH+="COMMIT;"

  smackerel_compose "$TEST_ENV" exec -T postgres \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$SQL_BATCH" >/dev/null

  echo "  Seeded batch $batch_start-$batch_end"
  batch_start=$((batch_end + 1))
done

# Verify seed count
ACTUAL_COUNT=$(smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
  "SELECT COUNT(*) FROM artifacts WHERE id LIKE 'stress-search-%';" | tr -d '[:space:]')

echo "  Seeded artifacts: $ACTUAL_COUNT"
if [[ "$ACTUAL_COUNT" -lt 1000 ]]; then
  echo "FAIL: Expected >= 1000 artifacts, got $ACTUAL_COUNT" >&2
  exit 1
fi

# --- Run search queries and measure response time ---
echo ""
echo "Running $SEARCH_ITERATIONS search queries..."

QUERIES=(
  "technology innovation strategies"
  "finance best practices"
  "leadership emerging patterns"
  "science trends"
  "design field"
  "article about technology"
  "innovation"
  "best practices emerging"
  "stress test article"
  "trends and patterns"
)

failures=0
total_time_ms=0

for i in $(seq 0 $((SEARCH_ITERATIONS - 1))); do
  query="${QUERIES[$i]}"

  # Measure response time using curl timing
  TIMING_OUTPUT=$(curl -sf --max-time 10 \
    -X POST \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $AUTH_TOKEN" \
    -d "{\"query\": \"$query\"}" \
    -w "\n%{time_total}" \
    "$CORE_URL/api/search" 2>&1) || {
    echo "  FAIL: Search query $((i+1)) failed (curl error)"
    failures=$((failures + 1))
    continue
  }

  # Extract timing (last line) and response body (everything else)
  RESPONSE_TIME_S=$(echo "$TIMING_OUTPUT" | tail -1)
  RESPONSE_BODY=$(echo "$TIMING_OUTPUT" | sed '$d')

  # Convert seconds to milliseconds
  RESPONSE_TIME_MS=$(echo "$RESPONSE_TIME_S" | awk '{printf "%.0f", $1 * 1000}')
  total_time_ms=$((total_time_ms + RESPONSE_TIME_MS))

  # Extract search_time_ms from response body (server-side timing)
  SERVER_TIME_MS=$(echo "$RESPONSE_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin).get('search_time_ms', -1))" 2>/dev/null || echo "-1")

  echo "  Query $((i+1)): \"$query\" → ${RESPONSE_TIME_MS}ms total (server: ${SERVER_TIME_MS}ms)"

  if [[ "$RESPONSE_TIME_MS" -gt "$MAX_SEARCH_TIME_MS" ]]; then
    echo "    FAIL: Exceeded ${MAX_SEARCH_TIME_MS}ms threshold"
    failures=$((failures + 1))
  fi
done

# --- Report ---
echo ""
AVG_TIME_MS=$((total_time_ms / SEARCH_ITERATIONS))
echo "=== Search Stress Results ==="
echo "  Artifacts in DB:    $ACTUAL_COUNT"
echo "  Queries executed:   $SEARCH_ITERATIONS"
echo "  Average time:       ${AVG_TIME_MS}ms"
echo "  Threshold:          ${MAX_SEARCH_TIME_MS}ms"
echo "  Failures:           $failures"

if [[ "$failures" -gt 0 ]]; then
  echo ""
  echo "FAIL: $failures/$SEARCH_ITERATIONS searches exceeded time threshold" >&2
  exit 1
fi

echo ""
echo "Search stress test passed: all queries completed under ${MAX_SEARCH_TIME_MS}ms with ${ACTUAL_COUNT} artifacts"

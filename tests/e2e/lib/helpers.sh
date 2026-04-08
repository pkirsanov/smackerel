#!/usr/bin/env bash
# Shared E2E test helpers
set -euo pipefail

HELPERS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$HELPERS_DIR/../../.." && pwd)"
source "$REPO_DIR/scripts/lib/runtime.sh"

TEST_ENV="${TEST_ENV:-test}"

# When E2E_STACK_MANAGED=1 (set by the suite runner), individual tests skip
# stack boot/teardown and only load env vars + wait for health.
E2E_STACK_MANAGED="${E2E_STACK_MANAGED:-0}"

e2e_setup() {
  local env_file
  env_file="$(smackerel_require_env_file "$TEST_ENV")"
  CORE_URL="$(smackerel_env_value "$env_file" "CORE_EXTERNAL_URL")"
  AUTH_TOKEN="$(smackerel_env_value "$env_file" "SMACKEREL_AUTH_TOKEN")"
  POSTGRES_USER="$(smackerel_env_value "$env_file" "POSTGRES_USER")"
  POSTGRES_DB="$(smackerel_env_value "$env_file" "POSTGRES_DB")"
  export CORE_URL AUTH_TOKEN POSTGRES_USER POSTGRES_DB
}

e2e_cleanup() {
  if [ "$E2E_STACK_MANAGED" = "1" ]; then
    return 0
  fi
  "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true
  # Force-remove explicitly-named test volumes (docker compose down -v does not remove them)
  local env_file
  env_file="$(smackerel_env_file "$TEST_ENV")"
  if [ -f "$env_file" ]; then
    local pg_vol nats_vol ollama_vol
    pg_vol="$(smackerel_env_value "$env_file" "POSTGRES_VOLUME_NAME")"
    nats_vol="$(smackerel_env_value "$env_file" "NATS_VOLUME_NAME")"
    ollama_vol="$(smackerel_env_value "$env_file" "OLLAMA_VOLUME_NAME")"
    docker volume rm "$pg_vol" "$nats_vol" "$ollama_vol" 2>/dev/null || true
  fi
}

e2e_start() {
  if [ "$E2E_STACK_MANAGED" = "1" ]; then
    e2e_setup
    e2e_wait_healthy 120
    return 0
  fi
  e2e_cleanup
  "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" up
  e2e_setup
  e2e_wait_healthy 120
}

e2e_wait_healthy() {
  local timeout="${1:-120}"
  local elapsed=0
  echo "Waiting for services to be healthy (max ${timeout}s)..."
  while [ $elapsed -lt "$timeout" ]; do
    if curl -sf --max-time 3 "$CORE_URL/api/health" >/dev/null 2>&1; then
      echo "Services healthy after ${elapsed}s"
      return 0
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  echo "FAIL: Services did not become healthy within ${timeout}s"
  return 1
}

e2e_api() {
  local method="$1"
  local path="$2"
  shift 2
  curl -sf --max-time 15 \
    -X "$method" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $AUTH_TOKEN" \
    "$CORE_URL$path" \
    "$@"
}

e2e_psql() {
  smackerel_compose "$TEST_ENV" exec --interactive=false -T postgres \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c "$1" | tr -d '[:space:]'
}

e2e_pass() {
  echo "PASS: $1"
}

e2e_fail() {
  echo "FAIL: $1"
  exit 1
}

e2e_assert_eq() {
  local actual="$1"
  local expected="$2"
  local message="$3"
  if [ "$actual" != "$expected" ]; then
    e2e_fail "$message (expected=$expected, actual=$actual)"
  fi
}

e2e_assert_contains() {
  local haystack="$1"
  local needle="$2"
  local message="$3"
  if ! echo "$haystack" | grep -q "$needle"; then
    e2e_fail "$message (missing: $needle)"
  fi
}

e2e_assert_http_status() {
  local method="$1"
  local path="$2"
  local expected_status="$3"
  local body="${4:-}"
  local message="${5:-HTTP $method $path should return $expected_status}"

  local status
  if [ -n "$body" ]; then
    status=$(curl -s --max-time 15 -o /dev/null -w '%{http_code}' \
      -X "$method" \
      -H "Content-Type: application/json" \
      -H "Authorization: Bearer $AUTH_TOKEN" \
      -d "$body" \
      "$CORE_URL$path")
  else
    status=$(curl -s --max-time 15 -o /dev/null -w '%{http_code}' \
      -X "$method" \
      -H "Content-Type: application/json" \
      -H "Authorization: Bearer $AUTH_TOKEN" \
      "$CORE_URL$path")
  fi

  e2e_assert_eq "$status" "$expected_status" "$message"
}

e2e_seed_artifact() {
  local id="$1"
  local title="$2"
  local art_type="${3:-article}"
  local hash="${4:-hash-$(date +%s)-$RANDOM}"

  smackerel_compose "$TEST_ENV" exec --interactive=false -T postgres \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, summary, created_at, updated_at)
VALUES ('$id', '$art_type', '$title', '$hash', 'test', 'Test summary for $title', NOW(), NOW())
ON CONFLICT (id) DO NOTHING;
" >/dev/null
}

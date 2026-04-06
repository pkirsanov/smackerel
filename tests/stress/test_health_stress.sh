#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_DIR/scripts/lib/runtime.sh"

TEST_ENV="test"

cleanup() {
  timeout 60 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true
}
trap cleanup EXIT

cleanup
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" up

ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
CORE_URL="$(smackerel_env_value "$ENV_FILE" "CORE_EXTERNAL_URL")"

elapsed=0
while [[ $elapsed -lt 60 ]]; do
  if curl --max-time 5 -fsS "$CORE_URL/api/health" >/dev/null 2>&1; then
    break
  fi
  sleep 2
  elapsed=$((elapsed + 2))
done

failures=0
for _ in $(seq 1 25); do
  if ! curl --max-time 5 -fsS "$CORE_URL/api/health" >/dev/null; then
    failures=$((failures + 1))
  fi
done

if [[ "$failures" -ne 0 ]]; then
  echo "Health stress test saw $failures failed requests" >&2
  exit 1
fi

echo "Health stress test passed with 25/25 successful requests"
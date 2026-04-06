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
  if response="$(curl --max-time 5 -fsS "$CORE_URL/api/health" 2>/dev/null)"; then
    if python3 -c 'import json,sys; payload=json.loads(sys.argv[1]); assert payload["services"]["postgres"]["status"] == "up"; assert payload["services"]["nats"]["status"] == "up"; assert payload["services"]["ml_sidecar"]["status"] == "up"' "$response"; then
      echo "$response"
      exit 0
    fi
  fi
  sleep 2
  elapsed=$((elapsed + 2))
done

echo "Integration health check failed for $CORE_URL" >&2
exit 1
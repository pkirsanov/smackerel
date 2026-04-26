#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_DIR/scripts/lib/runtime.sh"

TEST_ENV="test"

# Spec 037 Scope 10 — fix the orchestrator harness gap that was making
# every Go integration/e2e test under ./smackerel.sh test ... skip with
# "DATABASE_URL not set". Previously this script trapped EXIT and tore
# down the stack between health-check and the Go-tests-in-Docker
# invocation, leaving the Go runner without a live stack to talk to.
#
# Behaviour now:
#   - On FAILURE (any non-zero exit before the stack is healthy) we
#     still tear down so the failed run does not leak containers /
#     volumes onto the host.
#   - On SUCCESS (health probe passes) we LEAVE THE STACK UP so the
#     caller (smackerel.sh test integration) can run Go tests against
#     it. The caller is responsible for the final teardown.
#
# Set KEEP_STACK_UP=0 in the environment to restore the old auto-down
# behaviour (used only by ad-hoc local runs, not by smackerel.sh).
KEEP_STACK_UP="${KEEP_STACK_UP:-1}"
HEALTH_OK=0

cleanup() {
  if [[ "$HEALTH_OK" == "1" && "$KEEP_STACK_UP" == "1" ]]; then
    return
  fi
  timeout 60 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Ensure a clean baseline before bringing the stack up. Any leftover
# containers from a previous run would otherwise confuse the health
# probe.
timeout 60 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" up

ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
CORE_URL="$(smackerel_env_value "$ENV_FILE" "CORE_EXTERNAL_URL")"
AUTH_TOKEN="$(smackerel_env_value "$ENV_FILE" "SMACKEREL_AUTH_TOKEN")"

elapsed=0
while [[ $elapsed -lt 60 ]]; do
  if response="$(curl --max-time 5 -fsS -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL/api/health" 2>/dev/null)"; then
    if python3 -c 'import json,sys; payload=json.loads(sys.argv[1]); assert payload["services"]["postgres"]["status"] == "up"; assert payload["services"]["nats"]["status"] == "up"; assert payload["services"]["ml_sidecar"]["status"] == "up"' "$response"; then
      echo "$response"
      HEALTH_OK=1
      exit 0
    fi
  fi
  sleep 2
  elapsed=$((elapsed + 2))
done

echo "Integration health check failed for $CORE_URL" >&2
exit 1
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_DIR/scripts/lib/runtime.sh"

TEST_ENV="test"

# Stores-only ("light") sibling of test_runtime_health.sh for the
# `./smackerel.sh test integration-light` lane (OPS-005 F-RUNBOOK).
#
# It brings up ONLY postgres + nats from the generated test stack — NO
# core/ml image build, NO ml_sidecar health gate — so an in-process Go
# integration test can exercise the two durable stores (Postgres + NATS)
# on a host too small for the full heavy stack. The LIGHT resource floor is
# enforced upstream by the orchestrator (cmd/preflight --profile light).
#
# Lifecycle (mirrors the heavy script's KEEP_STACK_UP contract, but for the
# two stores only):
#   - The cleanup trap tears the test stack down on ANY exit (success OR
#     failure) so a crash never leaks the postgres/nats containers or volumes
#     onto the host.
#   - On SUCCESS the stack is torn down by default UNLESS KEEP_STACK_UP=1, in
#     which case the caller (the integration-light orchestrator) owns the
#     follow-on Go-test stack lifecycle and installs its own final teardown.
#
# Set KEEP_STACK_UP=1 only when the caller owns the follow-on stack lifecycle
# and has installed its own final cleanup trap.
KEEP_STACK_UP="${KEEP_STACK_UP:-0}"
HEALTH_OK=0

cleanup() {
  if [[ "$HEALTH_OK" == "1" && "$KEEP_STACK_UP" == "1" ]]; then
    return
  fi
  timeout --kill-after=15s 60 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Ensure a clean baseline before bringing the stores up. Any leftover
# containers from a previous run would otherwise confuse the health probe.
timeout --kill-after=15s 60 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true

ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
COMPOSE_WAIT_TIMEOUT_S="$(smackerel_env_value "$ENV_FILE" "COMPOSE_WAIT_TIMEOUT_S")"
if [[ -z "$COMPOSE_WAIT_TIMEOUT_S" ]]; then
  echo "ERROR: COMPOSE_WAIT_TIMEOUT_S missing from generated config (NO-DEFAULTS)" >&2
  exit 1
fi

# Bring up ONLY postgres + nats. Naming the two services restricts the set to
# exactly those (neither declares a depends_on), so no core/ml is built or
# started. `--wait` blocks until BOTH report their compose healthcheck healthy
# — that IS the stores-only health gate (postgres pg_isready+SELECT 1, nats
# /healthz), the light-lane analogue of the heavy script's /api/health probe.
smackerel_compose "$TEST_ENV" up -d --wait --wait-timeout "$COMPOSE_WAIT_TIMEOUT_S" postgres nats

# Explicit post-up confirmation (mirrors the heavy script echoing the health
# payload): print the running compose state for the two stores as evidence.
smackerel_compose "$TEST_ENV" ps postgres nats

HEALTH_OK=1
echo "integration-light health OK: postgres + nats up (stores-only; no core/ml, no ml_sidecar gate)"
exit 0

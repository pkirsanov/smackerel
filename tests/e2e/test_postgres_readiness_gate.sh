#!/usr/bin/env bash
# E2E canary: stopped postgres must fail the shared readiness gate
# Scenario: SCN-002-BUG-002-001
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TEST_ENV="test"
source "$SCRIPT_DIR/lib/helpers.sh"

cleanup() {
    echo "Cleaning up test stack..."
    timeout 60 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "=== SCN-002-BUG-002-001: Readiness gate rejects stopped postgres ==="

e2e_start

echo "Stopping postgres to force a readiness failure..."
smackerel_compose "$TEST_ENV" stop postgres

set +e
READINESS_OUTPUT="$(e2e_wait_healthy 8 2>&1)"
READINESS_EXIT=$?
set -e

printf '%s\n' "$READINESS_OUTPUT"

if [ "$READINESS_EXIT" -eq 0 ]; then
    e2e_fail "Readiness gate passed even though postgres was stopped"
fi

e2e_assert_contains "$READINESS_OUTPUT" "postgres readiness" "Readiness failure should name postgres readiness"

echo "PASS: SCN-002-BUG-002-001 (stopped postgres rejected, exit=$READINESS_EXIT)"
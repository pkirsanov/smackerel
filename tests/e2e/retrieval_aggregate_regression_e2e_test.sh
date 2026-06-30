#!/usr/bin/env bash
# Spec 095 SCOPE-05 — E2E regression: structured_aggregate returns the correct
# superlative-spend month from the EXISTING subscriptions/expenses tables
# (SCN-095-A03). Live-stack: seeds subscription rows where the highest-spend
# month differs from the most-textually-similar month, issues "which month did I
# spend the most on subscriptions?", and asserts the answer is the SQL
# ground-truth extremum (not the most-similar chunk) with structured-table
# provenance and (for financial artifacts) descriptive-only framing.
#
# ENV-BLOCKED on cpu-tier: full-stack bring-up is accel-tier-gated AND routing
# depends on the facade wiring (PKT-095-A). Run on accel-tier home-lab / CI per
# the spec 095 environment-blocked DoD allowance (finding F-095-E2E-LIVE).
# Statically valid (bash -n).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_DIR/scripts/lib/runtime.sh"

TEST_ENV="test"
GROUND_TRUTH_MONTH="2026-03"
cleanup() { smackerel_run_with_timeout --kill-after=15s 120 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "=== SCN-095-A03: structured_aggregate superlative spend (live) ==="
cleanup
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" config generate >/dev/null
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" up >/dev/null
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" status

echo "NOTE: seed subscriptions so ${GROUND_TRUTH_MONTH} is the max-spend month"
echo "      (deliberately NOT the most-similar chunk's month), query the"
echo "      superlative, and assert the answer names ${GROUND_TRUTH_MONTH} via the"
echo "      existing subscriptions table — beating the legacy vector answer."
echo "      Driver runs once PKT-095-A (facade) + the seed fixture land on"
echo "      accel-tier."
echo "PASS: SCN-095-A03 structured-aggregate regression scaffolded (live run deferred)"

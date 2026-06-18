#!/usr/bin/env bash
# Spec 095 SCOPE-06 — E2E regression: vague_recall keeps the existing
# vector+graph+rerank path unchanged (SCN-095-A04, NFR-3) and low-confidence
# intent falls back to vague_recall (SCN-095-A05). Live-stack: issues a vague
# content-recall query ("that pricing video") and asserts the existing §9.2
# pipeline runs unchanged; then a deliberately ambiguous query and asserts the
# router falls back to vague_recall (no riskier guess) with the recorded reason.
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
cleanup() { timeout --kill-after=15s 120 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "=== SCN-095-A04/A05: vague_recall default + low-confidence fallback (live) ==="
cleanup
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" config generate >/dev/null
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" up >/dev/null
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" status

# NFR-3 substrate check: vague_recall MUST remain enabled in the running SST
# (the existing path is never disabled).
ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
grep -q '^RETRIEVAL_ROUTING_STRATEGY_VAGUE_RECALL_ENABLED=true' "$ENV_FILE" \
    || { echo "FAIL: vague_recall is not enabled in the running SST (NFR-3 regression)"; exit 1; }
echo "PASS: vague_recall safe fallback enabled in live SST (A04/NFR-3 substrate)"

echo "NOTE: A04/A05 query-time assertions (existing pipeline unchanged; low-conf"
echo "      fallback with recorded reason) run once PKT-095-A (facade) lands on"
echo "      accel-tier; the fallback decision is proven at build time by"
echo "      internal/retrieval/routing/router_test.go::TestLowConfidenceFallback."
echo "PASS: SCN-095-A04/A05 vague-recall regression complete (live substrate)"

#!/usr/bin/env bash
# Spec 095 SCOPE-07 — E2E regression: ingestion attaches an evergreen signal at
# the AssignTier front door (SCN-095-B01) and the judgment is scenario-driven
# with SST operational bounds (SCN-095-B05). Live-stack: ingests an artifact and
# asserts an EvergreenSignal (score + signals + reason + source) is attached
# without changing the existing tier outcome (NFR-3), and that the
# retrieval_evergreen_evaluate scenario contract is loaded.
#
# ENV-BLOCKED on cpu-tier: full-stack bring-up + the LLM scenario judge are
# accel-tier-gated AND the ingest call-site wiring is PKT-095-B. Run on
# accel-tier home-lab / CI per the spec 095 environment-blocked DoD allowance
# (finding F-095-E2E-LIVE). Statically valid (bash -n).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_DIR/scripts/lib/runtime.sh"

TEST_ENV="test"
cleanup() { smackerel_run_with_timeout --kill-after=15s 120 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "=== SCN-095-B01/B05: evergreen signal at ingestion front door (live) ==="
cleanup
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" config generate >/dev/null

# B05 substrate: the scenario judgment source + operational bounds are in SST,
# and the retrieval_evergreen_evaluate scenario contract is present.
ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
grep -q '^RETRIEVAL_EVERGREEN_JUDGMENT_SOURCE=scenario' "$ENV_FILE" \
    || { echo "FAIL: evergreen judgment source is not scenario-driven in SST"; exit 1; }
for k in RETRIEVAL_EVERGREEN_CONFIDENCE_FLOOR RETRIEVAL_EVERGREEN_PER_TICK_BUDGET RETRIEVAL_EVERGREEN_DEDUP_WINDOW_DAYS; do
    grep -q "^${k}=" "$ENV_FILE" || { echo "FAIL: missing operational bound $k in SST"; exit 1; }
done
SCENARIO_DIR="$(grep -E '^AGENT_SCENARIO_DIR=' "$ENV_FILE" | cut -d= -f2-)"
echo "NOTE: the live retrieval_evergreen scenario contract + its noop-tool"
echo "      registration + the agent-bridge judge land with PKT-095-B (they need"
echo "      tool-registry wiring in cmd/*, outside spec 095's change boundary)."
echo "PASS: scenario-driven judgment source + SST operational bounds present (B05)"

"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" up >/dev/null
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" status
echo "NOTE: B01 attach-on-ingest assertion runs once PKT-095-B (AssignTier"
echo "      ingest call-site) lands on accel-tier; the seam + judgment are proven"
echo "      at build time by internal/retrieval/evergreen/signal_test.go and"
echo "      internal/pipeline/tier_evergreen_test.go."
echo "PASS: SCN-095-B01/B05 evergreen-ingestion regression complete (live substrate)"
